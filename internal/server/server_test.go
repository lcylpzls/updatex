package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/testx"
	"github.com/lcylpzls/updatex/internal/core"
	"github.com/lcylpzls/webx"
)

// testServer 是一次测试中的 webx 服务实例。
type testServer struct {
	base string
	cli  *http.Client
	ws   *webx.Server
	done chan error
}

// get 发起 GET 请求（可带管理令牌）。
func (ts *testServer) get(t *testing.T, path, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.base+path, nil)
	testx.RequireNoError(t, err)
	if token != "" {
		req.Header.Set("X-Api-Token", token)
	}
	resp, err := ts.cli.Do(req)
	testx.RequireNoError(t, err)
	return resp
}

// newServerDir 构造带清单与资产的临时目录。
func newServerDir(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"version":"` + version + `",` +
		`"published_at":"2026-08-12T00:00:00Z",` +
		`"notes":"release",` +
		`"platforms":{"linux_amd64":{"url":"/updates/assets/app.bin",` +
		`"sha256":"` + strings.Repeat("a", 64) + `","size":4}}}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.bin"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// startTestWebx 创建并启动带自签证书的 webx 服务，注册 updatex 服务端。
func startTestWebx(t *testing.T, s *Server) *testServer {
	t.Helper()
	certFile, keyFile := writeTestCert(t)
	ws := webx.NewServer(webx.Config{
		TLSCertFile:     certFile,
		TLSKeyFile:      keyFile,
		ShutdownTimeout: 3 * time.Second,
	}, testLogger())
	ws.UseHttp1or2Listen("127.0.0.1:0", true)
	testx.RequireNoError(t, s.RegisterWebx(ws))
	done := make(chan error, 1)
	go func() { done <- ws.Start() }()
	addr := ""
	for i := 0; i < 500; i++ {
		if a := ws.ListenerAddr(); a != "" {
			addr = a
			break
		}
		select {
		case err := <-done:
			t.Fatalf("服务启动失败：%v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if addr == "" {
		t.Fatal("服务启动超时")
	}
	ts := &testServer{
		base: "https://" + addr,
		cli: &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
			Timeout:   5 * time.Second,
		},
		ws:   ws,
		done: done,
	}
	t.Cleanup(func() {
		_ = ws.Stop(context.Background())
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	})
	return ts
}

// testLogger 构造写入丢弃目标的日志器。
func testLogger() logx.Logger {
	logger, err := logx.NewBuilder().EnableWriter(io.Discard, logx.InfoLevel).Build()
	if err != nil {
		panic(err)
	}
	return logger
}

// TestNewServerValidation 覆盖服务端构造校验。
func TestNewServerValidation(t *testing.T) {
	if _, err := NewServer(""); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("空目录应报错：%v", err)
	}
	if _, err := NewServer(filepath.Join(t.TempDir(), "missing")); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("不存在目录应报错：%v", err)
	}
	f := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(f, []byte("x"), 0o600)
	if _, err := NewServer(f); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("文件路径应报错：%v", err)
	}
	empty := t.TempDir()
	if _, err := NewServer(empty); !errx.Is(err, core.CodeManifestInvalid) {
		t.Fatalf("缺清单应报错：%v", err)
	}
	bad := t.TempDir()
	_ = os.WriteFile(filepath.Join(bad, "manifest.json"), []byte("bad"), 0o600)
	if _, err := NewServer(bad); !errx.Is(err, core.CodeManifestInvalid) {
		t.Fatalf("非法清单应报错：%v", err)
	}
	// nil 选项应被忽略。
	if _, err := NewServer(newServerDir(t, "1.0.0"), nil); err != nil {
		t.Fatalf("nil 选项应被忽略：%v", err)
	}
}

// TestRegisterWebxManifestAndAssets 覆盖清单与资产路由。
func TestRegisterWebxManifestAndAssets(t *testing.T) {
	s, err := NewServer(newServerDir(t, "1.2.3"))
	testx.RequireNoError(t, err)
	ts := startTestWebx(t, s)

	resp := ts.get(t, "/updates/manifest.json", "")
	testx.RequireEqual(t, resp.StatusCode, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	testx.RequireTrue(t, strings.Contains(string(body), `"version":"1.2.3"`))

	resp = ts.get(t, "/updates/assets/app.bin", "")
	testx.RequireEqual(t, resp.StatusCode, http.StatusOK)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	testx.RequireEqual(t, string(body), "data")

	resp = ts.get(t, "/updates/assets/missing.bin", "")
	testx.RequireEqual(t, resp.StatusCode, http.StatusNotFound)
	resp.Body.Close()
}

// TestRegisterWebxTraversal 覆盖静态资产路径穿越防护。
func TestRegisterWebxTraversal(t *testing.T) {
	dir := newServerDir(t, "1.0.0")
	outside := filepath.Join(filepath.Dir(dir), "outside.txt")
	_ = os.WriteFile(outside, []byte("x"), 0o600)
	s, err := NewServer(dir)
	testx.RequireNoError(t, err)
	ts := startTestWebx(t, s)

	req, _ := http.NewRequest(http.MethodGet, ts.base+"/updates/assets/../outside.txt", nil)
	resp, err := ts.cli.Do(req)
	testx.RequireNoError(t, err)
	testx.RequireEqual(t, resp.StatusCode, http.StatusNotFound)
	resp.Body.Close()
}

// TestRegisterWebxCustomOptions 覆盖自定义清单路径、路由前缀与管理令牌。
func TestRegisterWebxCustomOptions(t *testing.T) {
	dir := newServerDir(t, "3.0.0")
	os.Rename(filepath.Join(dir, "manifest.json"), filepath.Join(dir, "custom.json"))
	s, err := NewServer(dir,
		WithManifestPath(filepath.Join(dir, "custom.json")),
		WithManifestURL("/custom/manifest.json"),
		WithAssetsURL("/custom/assets/"),
		WithAdminToken("tok"),
	)
	testx.RequireNoError(t, err)
	ts := startTestWebx(t, s)

	resp := ts.get(t, "/custom/manifest.json", "")
	testx.RequireEqual(t, resp.StatusCode, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	testx.RequireTrue(t, strings.Contains(string(body), `"version":"3.0.0"`))

	resp = ts.get(t, "/custom/assets/app.bin", "")
	testx.RequireEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	resp = ts.get(t, "/updates/admin/status", "")
	testx.RequireEqual(t, resp.StatusCode, http.StatusUnauthorized)
	resp.Body.Close()

	resp = ts.get(t, "/updates/admin/status", "tok")
	testx.RequireEqual(t, resp.StatusCode, http.StatusOK)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	testx.RequireTrue(t, strings.Contains(string(body), `"ok":true`))
}

// TestRegisterWebxAdminRoutes 覆盖全部方法的管理路由与令牌鉴权。
func TestRegisterWebxAdminRoutes(t *testing.T) {
	s, err := NewServer(newServerDir(t, "1.0.0"), WithAdminToken("secret"))
	testx.RequireNoError(t, err)
	methods := []string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodPatch, http.MethodHead, http.MethodOptions,
	}
	for _, m := range methods {
		m := m
		testx.RequireNoError(t, s.HandleAdmin(m, "/all", func(c *webx.Context) {
			c.Writer().Header().Set("X-Admin", c.Request().Method)
			c.Writer().WriteHeader(http.StatusNoContent)
		}))
	}
	ts := startTestWebx(t, s)

	resp := ts.get(t, "/updates/admin/status", "")
	testx.RequireEqual(t, resp.StatusCode, http.StatusUnauthorized)
	resp.Body.Close()
	resp = ts.get(t, "/updates/admin/status", "wrong")
	testx.RequireEqual(t, resp.StatusCode, http.StatusUnauthorized)
	resp.Body.Close()

	for _, m := range methods {
		req, _ := http.NewRequest(m, ts.base+"/updates/admin/all", nil)
		req.Header.Set("X-Api-Token", "secret")
		resp, err := ts.cli.Do(req)
		testx.RequireNoError(t, err)
		testx.RequireEqual(t, resp.StatusCode, http.StatusNoContent)
		testx.RequireEqual(t, resp.Header.Get("X-Admin"), m)
		resp.Body.Close()
	}
}

// TestRegisterWebxNoAdminToken 覆盖未配置令牌时管理路由不挂载。
func TestRegisterWebxNoAdminToken(t *testing.T) {
	s, err := NewServer(newServerDir(t, "1.0.0"))
	testx.RequireNoError(t, err)
	if err := s.HandleAdmin(http.MethodGet, "/info", func(*webx.Context) {}); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("未配置令牌应报错：%v", err)
	}
	ts := startTestWebx(t, s)
	resp := ts.get(t, "/updates/admin/status", "")
	testx.RequireEqual(t, resp.StatusCode, http.StatusNotFound)
	resp.Body.Close()
}

// TestHandleAdminValidation 覆盖管理路由参数校验。
func TestHandleAdminValidation(t *testing.T) {
	s, err := NewServer(newServerDir(t, "1.0.0"), WithAdminToken("secret"))
	testx.RequireNoError(t, err)
	ok := func(*webx.Context) {}
	if err := s.HandleAdmin("", "/x", ok); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("空方法应报错：%v", err)
	}
	if err := s.HandleAdmin("TRACE", "/x", ok); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("非法方法应报错：%v", err)
	}
	if err := s.HandleAdmin(http.MethodGet, "", ok); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("空路径应报错：%v", err)
	}
	if err := s.HandleAdmin(http.MethodGet, "x", ok); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("非 / 开头路径应报错：%v", err)
	}
	if err := s.HandleAdmin(http.MethodGet, "/x", nil); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("nil 处理器应报错：%v", err)
	}
}

// TestRegisterWebxErrors 覆盖注册参数校验。
func TestRegisterWebxErrors(t *testing.T) {
	s, err := NewServer(newServerDir(t, "1.0.0"))
	testx.RequireNoError(t, err)
	if err := s.RegisterWebx(nil); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("nil webx 应报错：%v", err)
	}
	badURL, err := NewServer(newServerDir(t, "1.0.0"), WithManifestURL("nope"))
	testx.RequireNoError(t, err)
	if err := badURL.RegisterWebx(webx.NewServer(webx.Config{}, testLogger())); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("非法清单路由应报错：%v", err)
	}
	badAssets, err := NewServer(newServerDir(t, "1.0.0"), WithAssetsURL("nope"))
	testx.RequireNoError(t, err)
	if err := badAssets.RegisterWebx(webx.NewServer(webx.Config{}, testLogger())); !errx.Is(err, core.CodeInvalidConfig) {
		t.Fatalf("非法资产路由应报错：%v", err)
	}
}

// TestReload 覆盖清单热更新。
func TestReload(t *testing.T) {
	dir := newServerDir(t, "1.0.0")
	s, err := NewServer(dir)
	testx.RequireNoError(t, err)
	ts := startTestWebx(t, s)

	newManifest := strings.ReplaceAll(
		string(mustRead(t, filepath.Join(dir, "manifest.json"))), "1.0.0", "2.0.0")
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(newManifest), 0o600)
	testx.RequireNoError(t, s.Reload())

	resp := ts.get(t, "/updates/manifest.json", "")
	testx.RequireEqual(t, resp.StatusCode, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	testx.RequireTrue(t, strings.Contains(string(body), `"version":"2.0.0"`))

	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("bad"), 0o600)
	if err := s.Reload(); !errx.Is(err, core.CodeManifestInvalid) {
		t.Fatalf("非法清单应报错：%v", err)
	}
}

// mustRead 读取文件（测试辅助）。
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	testx.RequireNoError(t, err)
	return b
}

// writeTestCert 生成自签名证书与私钥，返回文件路径。
func writeTestCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testx.RequireNoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "updatex-服务端测试"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	testx.RequireNoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	testx.RequireNoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	testx.RequireNoError(t, os.WriteFile(certFile, certPEM, 0o600))
	testx.RequireNoError(t, os.WriteFile(keyFile, keyPEM, 0o600))
	return certFile, keyFile
}
