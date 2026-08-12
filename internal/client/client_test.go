package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/httpx"
	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/testx"
	"github.com/lcylpzls/updatex/internal/core"
)

// fakeSource 测试用自定义发布源。
type fakeSource struct {
	m *core.Manifest
}

func (f *fakeSource) Latest(context.Context) (*core.Manifest, error) {
	return f.m, nil
}

// testLogger 构造写入丢弃目标的日志器。
func testLogger() logx.Logger {
	logger, err := logx.NewBuilder().EnableWriter(io.Discard, logx.InfoLevel).Build()
	if err != nil {
		panic(err)
	}
	return logger
}

// newManifestServer 构造清单与资产测试服务器。
func newManifestServer(t *testing.T, version string, asset []byte, shaOverride string) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(asset)
	sha := hex.EncodeToString(sum[:])
	if shaOverride != "" {
		sha = shaOverride
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			m := fmt.Sprintf(`{"version":%q,"published_at":"2026-08-12T00:00:00Z",`+
				`"notes":"发布说明","platforms":{%q:{"url":%q,"sha256":%q,"size":%d}}}`,
				version, runtime.GOOS+"_"+runtime.GOARCH, srv.URL+"/app.bin", sha, len(asset))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(m))
		case "/app.bin":
			_, _ = w.Write(asset)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// baseConfig 构造一次更新测试的基础配置。
func baseConfig(srv *httptest.Server, target, current string, action AfterUpdateAction) Config {
	return Config{
		ManifestURL:    srv.URL + "/manifest.json",
		CurrentVersion: current,
		ExecutablePath: target,
		AfterUpdate:    action,
		AllowHTTP:      true,
	}
}

// TestNewClientValidation 覆盖配置校验分支。
func TestNewClientValidation(t *testing.T) {
	cases := []Config{
		{CurrentVersion: "", ManifestURL: "https://x/update.json", AfterUpdate: AfterUpdateContinue},
		{CurrentVersion: "1.0.0", AfterUpdate: AfterUpdateContinue},
		{CurrentVersion: "1.0.0", ManifestURL: "https://x/update.json",
			Source: &fakeSource{m: &core.Manifest{}}, AfterUpdate: AfterUpdateContinue},
		{CurrentVersion: "1.0.0", ManifestURL: "https://x/update.json",
			AfterUpdate: AfterUpdateAction(99)},
		{CurrentVersion: "1.0.0", ManifestURL: "https://x/update.json",
			AfterUpdate: AfterUpdateRestart},
		{CurrentVersion: "1.0.0", ManifestURL: "http://x/update.json",
			AfterUpdate: AfterUpdateContinue},
	}
	for i, cfg := range cases {
		if _, err := NewClient(cfg); !errx.Is(err, core.CodeInvalidConfig) {
			t.Fatalf("用例 %d 应报配置错误：%v", i, err)
		}
	}
}

// TestNewClientProtocols 覆盖协议与 TLS 选项分支。
func TestNewClientProtocols(t *testing.T) {
	for _, p := range []httpx.Protocol{
		httpx.ProtocolAuto, httpx.ProtocolHTTP1, httpx.ProtocolHTTP2, httpx.ProtocolHTTP3,
	} {
		c, err := NewClient(Config{
			ManifestURL:    "https://x/update.json",
			CurrentVersion: "1.0.0",
			AfterUpdate:    AfterUpdateContinue,
			Protocol:       p,
		})
		testx.RequireNoError(t, err)
		testx.RequireTrue(t, c != nil)
	}
	c, err := NewClient(Config{
		ManifestURL:    "https://x/update.json",
		CurrentVersion: "1.0.0",
		AfterUpdate:    AfterUpdateContinue,
		InsecureTLS:    true,
	})
	testx.RequireNoError(t, err)
	testx.RequireTrue(t, c != nil)

	injected, err := httpx.New()
	testx.RequireNoError(t, err)
	c, err = NewClient(Config{
		ManifestURL:    "https://x/update.json",
		CurrentVersion: "1.0.0",
		AfterUpdate:    AfterUpdateContinue,
		HTTPClient:     injected,
	})
	testx.RequireNoError(t, err)
	testx.RequireTrue(t, c != nil)
}

// TestNewClientCustomSource 覆盖自定义发布源分支。
func TestNewClientCustomSource(t *testing.T) {
	c, err := NewClient(Config{
		Source:         &fakeSource{m: &core.Manifest{Version: "1.1.0", Platforms: map[string]core.Asset{}}},
		CurrentVersion: "1.0.0",
		AfterUpdate:    AfterUpdateContinue,
	})
	testx.RequireNoError(t, err)
	testx.RequireTrue(t, c != nil)
}

// TestNewClientBuildErrors 覆盖客户端构造失败分支。
func TestNewClientBuildErrors(t *testing.T) {
	orig := newHTTPClient
	newHTTPClient = func(...httpx.Option) (*httpx.Client, error) {
		return nil, errors.New("客户端构造失败")
	}
	_, err := NewClient(Config{
		ManifestURL:    "https://x/update.json",
		CurrentVersion: "1.0.0",
		AfterUpdate:    AfterUpdateContinue,
	})
	newHTTPClient = orig
	testx.RequireTrue(t, err != nil)

	if _, err := NewClient(Config{
		ManifestURL:     "https://x/update.json",
		CurrentVersion:  "1.0.0",
		AfterUpdate:     AfterUpdateContinue,
		MaxDownloadBytes: -1,
	}); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("负下载上限应报错：%v", err)
	}
	if _, err := NewClient(Config{
		ManifestURL:     "https://x/update.json",
		CurrentVersion:  "1.0.0",
		AfterUpdate:     AfterUpdateContinue,
		VerifyPublicKey: []byte{1, 2, 3},
	}); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("非法公钥应报错：%v", err)
	}
}

// TestRunNoUpdate 覆盖无更新分支。
func TestRunNoUpdate(t *testing.T) {
	srv := newManifestServer(t, "1.0.0", []byte("新版本二进制内容"), "")
	target := filepath.Join(t.TempDir(), "app")
	_ = os.WriteFile(target, []byte("旧版本二进制内容"), 0o755)
	c, err := NewClient(baseConfig(srv, target, "1.0.0", AfterUpdateContinue))
	testx.RequireNoError(t, err)
	res, err := c.Run(context.Background())
	testx.RequireNoError(t, err)
	testx.RequireEqual(t, res.Updated, false)
	testx.RequireEqual(t, res.Version, "1.0.0")
	data, _ := os.ReadFile(target)
	testx.RequireEqual(t, string(data), "旧版本二进制内容")
}

// TestRunUpdateContinue 覆盖完整更新闭环（继续运行）。
func TestRunUpdateContinue(t *testing.T) {
	asset := []byte("新版本二进制内容")
	srv := newManifestServer(t, "1.1.0", asset, "")
	target := filepath.Join(t.TempDir(), "app")
	_ = os.WriteFile(target, []byte("旧版本二进制内容"), 0o755)
	cfg := baseConfig(srv, target, "1.0.0", AfterUpdateContinue)
	cfg.Logger = testLogger()
	cfg.Metrics = core.Metrics{
		CheckTotal:     func(int) {},
		CheckFailures:  func(error) {},
		UpdateSuccess:  func(string) {},
		UpdateFailures: func(error) {},
	}
	cfg.TraceHook = core.TraceHook(nil)
	c, err := NewClient(cfg)
	testx.RequireNoError(t, err)
	res, err := c.Run(context.Background())
	testx.RequireNoError(t, err)
	testx.RequireEqual(t, res.Updated, true)
	testx.RequireEqual(t, res.Version, "1.1.0")
	testx.RequireEqual(t, res.Notes, "发布说明")
	if res.RestartRequired {
		// Windows 延迟替换：模拟新进程启动时完成替换。
		testx.RequireNoError(t, core.Bootstrap(context.Background(), target))
	}
	data, _ := os.ReadFile(target)
	testx.RequireEqual(t, string(data), string(asset))
}

// TestRunUpdateExit 覆盖退出动作。
func TestRunUpdateExit(t *testing.T) {
	asset := []byte("新版本二进制内容")
	srv := newManifestServer(t, "1.1.0", asset, "")
	target := filepath.Join(t.TempDir(), "app")
	_ = os.WriteFile(target, []byte("旧版本二进制内容"), 0o755)
	var codes []int
	orig := exitFn
	exitFn = func(code int) { codes = append(codes, code) }
	defer func() { exitFn = orig }()
	c, err := NewClient(baseConfig(srv, target, "1.0.0", AfterUpdateExit))
	testx.RequireNoError(t, err)
	res, err := c.Run(context.Background())
	testx.RequireNoError(t, err)
	testx.RequireEqual(t, res.Updated, true)
	testx.RequireEqual(t, codes, []int{0})
}

// TestRunUpdateRestart 覆盖重启动作（真实启动命令 + 注入退出）。
func TestRunUpdateRestart(t *testing.T) {
	asset := []byte("新版本二进制内容")
	srv := newManifestServer(t, "1.1.0", asset, "")
	target := filepath.Join(t.TempDir(), "app")
	_ = os.WriteFile(target, []byte("旧版本二进制内容"), 0o755)
	command := "true"
	if runtime.GOOS == "windows" {
		command = "echo ok"
	}
	var codes []int
	origExit := exitFn
	exitFn = func(code int) { codes = append(codes, code) }
	defer func() { exitFn = origExit }()
	cfg := baseConfig(srv, target, "1.0.0", AfterUpdateRestart)
	cfg.RestartCommand = command
	c, err := NewClient(cfg)
	testx.RequireNoError(t, err)
	res, err := c.Run(context.Background())
	testx.RequireNoError(t, err)
	testx.RequireEqual(t, res.Updated, true)
	testx.RequireEqual(t, codes, []int{0})
}

// TestRunRestartStartError 覆盖重启命令启动失败分支。
func TestRunRestartStartError(t *testing.T) {
	asset := []byte("新版本二进制内容")
	srv := newManifestServer(t, "1.1.0", asset, "")
	target := filepath.Join(t.TempDir(), "app")
	_ = os.WriteFile(target, []byte("旧版本二进制内容"), 0o755)
	origExec := execStart
	execStart = func(*exec.Cmd) error { return errors.New("启动失败") }
	defer func() { execStart = origExec }()
	var codes []int
	origExit := exitFn
	exitFn = func(code int) { codes = append(codes, code) }
	defer func() { exitFn = origExit }()
	cfg := baseConfig(srv, target, "1.0.0", AfterUpdateRestart)
	cfg.RestartCommand = "true"
	c, err := NewClient(cfg)
	testx.RequireNoError(t, err)
	if _, err := c.Run(context.Background()); err == nil {
		t.Fatal("重启命令启动失败应报错")
	}
	testx.RequireEqual(t, len(codes), 0)
}

// TestRestartCmdFor 覆盖双平台重启命令构造分支。
func TestRestartCmdFor(t *testing.T) {
	w := restartCmdFor("windows", "net stop x")
	testx.RequireEqual(t, filepath.Base(w.Path), "cmd.exe")
	testx.RequireEqual(t, len(w.Args), 3)
	testx.RequireEqual(t, w.Args[2], "net stop x")
	u := restartCmdFor("linux", "systemctl restart x")
	testx.RequireEqual(t, u.Path, "/bin/sh")
	testx.RequireEqual(t, len(u.Args), 3)
	testx.RequireEqual(t, u.Args[2], "systemctl restart x")
}

// TestRunBootstrapError 覆盖启动时替换失败分支。
func TestRunBootstrapError(t *testing.T) {
	srv := newManifestServer(t, "1.0.0", []byte("x"), "")
	target := filepath.Join(t.TempDir(), "app")
	_ = os.WriteFile(target, []byte("旧"), 0o755)
	orig := bootstrap
	bootstrap = func(context.Context, string) error { return errors.New("Bootstrap 失败") }
	defer func() { bootstrap = orig }()
	cfg := baseConfig(srv, target, "1.0.0", AfterUpdateContinue)
	cfg.Logger = testLogger()
	c, err := NewClient(cfg)
	testx.RequireNoError(t, err)
	if _, err := c.Run(context.Background()); err == nil {
		t.Fatal("Bootstrap 失败应报错")
	}
}

// TestRunCheckError 覆盖版本检查失败分支。
func TestRunCheckError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	target := filepath.Join(t.TempDir(), "app")
	_ = os.WriteFile(target, []byte("旧"), 0o755)
	c, err := NewClient(baseConfig(srv, target, "1.0.0", AfterUpdateContinue))
	testx.RequireNoError(t, err)
	if _, err := c.Run(context.Background()); err == nil {
		t.Fatal("清单拉取失败应报错")
	}
}

// TestRunApplyError 覆盖下载校验失败分支。
func TestRunApplyError(t *testing.T) {
	asset := []byte("新版本二进制内容")
	srv := newManifestServer(t, "1.1.0", asset, strings.Repeat("0", 64))
	target := filepath.Join(t.TempDir(), "app")
	_ = os.WriteFile(target, []byte("旧版本二进制内容"), 0o755)
	c, err := NewClient(baseConfig(srv, target, "1.0.0", AfterUpdateContinue))
	testx.RequireNoError(t, err)
	if _, err := c.Run(context.Background()); !errx.Is(err, core.CodeChecksumMismatch) {
		t.Fatalf("校验失败应报错，实际：%v", err)
	}
}

// TestAfterUpdateInvalid 覆盖非法动作兜底分支。
func TestAfterUpdateInvalid(t *testing.T) {
	c := &Client{cfg: Config{AfterUpdate: AfterUpdateAction(99)}}
	if _, err := c.afterUpdate(&core.UpdateInfo{Version: "1.0.0", Notes: "n"}); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("非法动作应报错：%v", err)
	}
}
