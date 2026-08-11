package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	testx "github.com/lcylpzls/testx"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lcylpzls/errx"
)

// TestNewErrors 覆盖构造校验。
func TestNewErrors(t *testing.T) {
	if _, err := New(Config{}); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("空源应报错，实际：%v", err)
	}
	if _, err := New(Config{Source: &stubSource{}, CurrentVersion: "bad"}); !errx.Is(err, CodeInvalidVersion) {
		t.Fatalf("坏版本应报错，实际：%v", err)
	}
	if _, err := New(Config{Source: &stubSource{}, CurrentVersion: "1.0.0", MaxDownloadBytes: -1}); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("负上限应报错，实际：%v", err)
	}
	if _, err := New(Config{Source: &stubSource{}, CurrentVersion: "1.0.0", VerifyPublicKey: []byte("short")}); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("公钥长度非法应报错，实际：%v", err)
	}
	// 默认可执行文件路径解析失败。
	orig := executablePathFn
	executablePathFn = func() (string, error) { return "", errors.New("解析失败") }
	defer func() { executablePathFn = orig }()
	if _, err := New(Config{Source: &stubSource{}, CurrentVersion: "1.0.0"}); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("路径解析失败应报错，实际：%v", err)
	}
	// 默认路径解析成功（当前测试进程有效）。
	executablePathFn = os.Executable
	u, err := New(Config{Source: &stubSource{}, CurrentVersion: "1.0.0"})
	if err != nil || u.executablePath == "" {
		t.Fatalf("默认路径应可用：%v", err)
	}
}

// TestNewVVersion 覆盖带 v 前缀的当前版本初始化（issue #1）。
func TestNewVVersion(t *testing.T) {
	u, err := New(Config{Source: &stubSource{}, CurrentVersion: "v0.3.0", ExecutablePath: "x"})
	testx.RequireNoError(t, err)

	if got := u.current; got.major != 0 || got.minor != 3 || got.patch != 0 {
		t.Fatalf("解析不符：%+v", got)
	}
}

// TestApplySourceFailure 覆盖 Apply 的源失败分支。
func TestApplySourceFailure(t *testing.T) {
	u, err := New(Config{Source: &stubSource{err: ErrFetchFailed},
		CurrentVersion: "1.0.0", ExecutablePath: "x"})
	testx.RequireNoError(t, err)

	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrFetchFailed) {
		t.Fatalf("源失败应透传，实际：%v", err)
	}
}

// TestCheck 覆盖检查分支。
func TestCheck(t *testing.T) {
	ctx := context.Background()
	src := &stubSource{manifest: newStubManifest("1.1.0", "https://x/download", strings.Repeat("ab", 32))}
	u, err := New(Config{Source: src, CurrentVersion: "1.0.0", ExecutablePath: "x"})
	testx.RequireNoError(t, err)

	info, err := u.Check(ctx)
	if err != nil || !info.HasUpdate || info.Version != "1.1.0" {
		t.Fatalf("应有更新：%+v err=%v", info, err)
	}
	u2, _ := New(Config{Source: src, CurrentVersion: "1.1.0", ExecutablePath: "x"})
	info2, err := u2.Check(ctx)
	if err != nil || info2.HasUpdate {
		t.Fatalf("应无更新：%+v err=%v", info2, err)
	}
	srcFail := &stubSource{err: ErrFetchFailed}
	u3, _ := New(Config{Source: srcFail, CurrentVersion: "1.0.0", ExecutablePath: "x"})
	if _, err := u3.Check(ctx); !errors.Is(err, ErrFetchFailed) {
		t.Fatalf("源失败应透传，实际：%v", err)
	}
	// 平台缺失。
	srcOther := &stubSource{manifest: &Manifest{Version: "1.1.0",
		Platforms: map[string]Asset{"other_arch": {URL: "https://x", SHA256: strings.Repeat("ab", 32)}}}}
	u4, _ := New(Config{Source: srcOther, CurrentVersion: "1.0.0", ExecutablePath: "x"})
	if _, err := u4.Check(ctx); !errors.Is(err, ErrPlatformUnsupported) {
		t.Fatalf("平台缺失应报错，实际：%v", err)
	}
	// 清单版本非法。
	srcBad := &stubSource{manifest: &Manifest{Version: "bad",
		Platforms: map[string]Asset{runtime.GOOS + "_" + runtime.GOARCH: {URL: "https://x", SHA256: strings.Repeat("ab", 32)}}}}
	u5, _ := New(Config{Source: srcBad, CurrentVersion: "1.0.0", ExecutablePath: "x"})
	if _, err := u5.Check(ctx); !errx.Is(err, CodeInvalidVersion) {
		t.Fatalf("坏版本应报错，实际：%v", err)
	}
}

// TestCheckSignature 覆盖清单签名校验分支。
func TestCheckSignature(t *testing.T) {
	ctx := context.Background()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	testx.RequireNoError(t, err)

	base := newStubManifest("1.1.0", "https://x/download", strings.Repeat("ab", 32))
	signManifest(t, base, priv)
	u, err := New(Config{Source: &stubSource{manifest: base},
		CurrentVersion: "1.0.0", ExecutablePath: "x", VerifyPublicKey: pub})
	testx.RequireNoError(t, err)

	if _, err := u.Check(ctx); err != nil {
		t.Fatalf("合法签名应通过检查：%v", err)
	}
	// 篡改清单。
	tampered := *base
	tampered.Notes = "被篡改"
	u2, _ := New(Config{Source: &stubSource{manifest: &tampered},
		CurrentVersion: "1.0.0", ExecutablePath: "x", VerifyPublicKey: pub})
	if _, err := u2.Check(ctx); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("篡改清单应校验失败，实际：%v", err)
	}
	// 配置公钥但清单无签名。
	noSig := newStubManifest("1.1.0", "https://x/download", strings.Repeat("ab", 32))
	u3, _ := New(Config{Source: &stubSource{manifest: noSig},
		CurrentVersion: "1.0.0", ExecutablePath: "x", VerifyPublicKey: pub})
	if _, err := u3.Check(ctx); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("缺签名应校验失败，实际：%v", err)
	}
}

// TestMetricsAndLog 覆盖指标与日志回调。
func TestMetricsAndLog(t *testing.T) {
	var checkTotal, checkFail, updateSuccess, updateFail atomic.Int32
	m := Metrics{
		CheckTotal:     func(int) { checkTotal.Add(1) },
		CheckFailures:  func(error) { checkFail.Add(1) },
		UpdateSuccess:  func(string) { updateSuccess.Add(1) },
		UpdateFailures: func(error) { updateFail.Add(1) },
	}
	u, err := New(Config{Source: &stubSource{manifest: newStubManifest("1.1.0", "https://x", strings.Repeat("ab", 32))},
		CurrentVersion: "1.0.0", ExecutablePath: "x", Metrics: m, Logger: testLogger()})
	testx.RequireNoError(t, err)

	_, _ = u.Check(context.Background())
	if checkTotal.Load() != 1 {
		t.Fatalf("检查计数应为 1：%d", checkTotal.Load())
	}
	u2, _ := New(Config{Source: &stubSource{err: ErrFetchFailed},
		CurrentVersion: "1.0.0", ExecutablePath: "x", Metrics: m, Logger: testLogger()})
	_, _ = u2.Check(context.Background())
	if checkFail.Load() != 1 {
		t.Fatalf("检查失败计数应为 1：%d", checkFail.Load())
	}
}

// TestBootstrap 覆盖启动替换入口（平台行为）。
func TestBootstrap(t *testing.T) {
	if err := Bootstrap(context.Background(), "x"); err != nil {
		t.Fatalf("Bootstrap 不应报错：%v", err)
	}
}
