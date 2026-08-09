package updatex

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/httpx"
)

// stubReplace 注入替换函数并返回恢复函数。
func stubReplace(t *testing.T, fn func(string, string, string) (bool, error)) {
	t.Helper()
	orig := replaceExec
	replaceExec = fn
	t.Cleanup(func() { replaceExec = orig })
}

// TestApplyDowngrade 覆盖拒绝回退。
func TestApplyDowngrade(t *testing.T) {
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("0.9.0", "https://x", strings.Repeat("ab", 32))},
		CurrentVersion: "1.0.0", ExecutablePath: "x"})
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrDowngrade) {
		t.Fatalf("应拒绝回退，实际：%v", err)
	}
}

// TestApplyBadVersion 覆盖清单版本非法。
func TestApplyBadVersion(t *testing.T) {
	u, _ := New(Config{Source: &stubSource{
		manifest: &Manifest{Version: "bad",
			Platforms: map[string]Asset{"x": {URL: "https://x", SHA256: strings.Repeat("ab", 32)}}}},
		CurrentVersion: "1.0.0", ExecutablePath: "x"})
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeInvalidVersion) {
		t.Fatalf("坏版本应报错，实际：%v", err)
	}
}

// TestApplyPlatformMissing 覆盖平台资产缺失。
func TestApplyPlatformMissing(t *testing.T) {
	u, _ := New(Config{Source: &stubSource{
		manifest: &Manifest{Version: "1.1.0",
			Platforms: map[string]Asset{"other": {URL: "https://x", SHA256: strings.Repeat("ab", 32)}}}},
		CurrentVersion: "1.0.0", ExecutablePath: "x"})
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrPlatformUnsupported) {
		t.Fatalf("平台缺失应报错，实际：%v", err)
	}
}

// TestApplyHTTPSEnforced 覆盖明文资产拒绝。
func TestApplyHTTPSEnforced(t *testing.T) {
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", "http://x/download", strings.Repeat("ab", 32))},
		CurrentVersion: "1.0.0", ExecutablePath: "x"})
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeDownloadFailed) {
		t.Fatalf("明文资产应被拒绝，实际：%v", err)
	}
}

// TestApplyDownloadConnFailed 覆盖连接失败。
func TestApplyDownloadConnFailed(t *testing.T) {
	srv := httptest.NewServer(nil)
	url := srv.URL + "/download"
	srv.Close()
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", url, strings.Repeat("ab", 32))},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true})
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeDownloadFailed) {
		t.Fatalf("连接失败应报错，实际：%v", err)
	}
}

// TestApplyChecksumMismatch 覆盖校验失败（通用）。
func TestApplyChecksumMismatch(t *testing.T) {
	srv, _ := assetServer(t, "tampered")
	defer srv.Close()
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", strings.Repeat("00", 32))},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true})
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("校验失败应报错，实际：%v", err)
	}
}

// TestApplyDownloadNon200 覆盖资产端点非 200。
func TestApplyDownloadNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/missing", strings.Repeat("ab", 32))},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true})
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeDownloadFailed) {
		t.Fatalf("非 200 应报下载失败，实际：%v", err)
	}
}

// TestApplySignature 覆盖 Apply 签名校验分支。
func TestApplySignature(t *testing.T) {
	stubReplace(t, func(_, _, _ string) (bool, error) { return false, nil })
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	m := newStubManifest("1.1.0", srv.URL+"/download", sha)
	signManifest(t, m, priv)
	u, err := New(Config{Source: &stubSource{manifest: m},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true, VerifyPublicKey: pub})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Apply(context.Background()); err != nil {
		t.Fatalf("合法签名应通过应用：%v", err)
	}
	tampered := *m
	tampered.Notes = "被篡改"
	u2, _ := New(Config{Source: &stubSource{manifest: &tampered},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true, VerifyPublicKey: pub})
	if _, err := u2.Apply(context.Background()); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("篡改清单应校验失败，实际：%v", err)
	}
}

// TestApplyTempCreateFailure 覆盖临时文件创建失败。
func TestApplyTempCreateFailure(t *testing.T) {
	orig := createTempFile
	createTempFile = func(string, string) (*os.File, error) {
		return nil, errors.New("创建失败")
	}
	defer func() { createTempFile = orig }()
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true})
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeDownloadFailed) {
		t.Fatalf("临时文件创建失败应报错，实际：%v", err)
	}
}

// TestApplyMetricsAndLog 覆盖成功/失败指标与日志。
func TestApplyMetricsAndLog(t *testing.T) {
	stubReplace(t, func(_, _, _ string) (bool, error) { return false, nil })
	var success, failure atomic.Int32
	m := Metrics{
		UpdateSuccess:  func(string) { success.Add(1) },
		UpdateFailures: func(error) { failure.Add(1) },
	}
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true, Metrics: m, Logger: testLogger()})
	if _, err := u.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if success.Load() != 1 {
		t.Fatalf("成功计数应为 1：%d", success.Load())
	}
	u2, _ := New(Config{Source: &stubSource{err: ErrFetchFailed},
		CurrentVersion: "1.0.0", ExecutablePath: "x", Metrics: m})
	if _, err := u2.Apply(context.Background()); !errors.Is(err, ErrFetchFailed) {
		t.Fatalf("应报错：%v", err)
	}
	if failure.Load() != 1 {
		t.Fatalf("失败计数应为 1：%d", failure.Load())
	}
}

// TestApplyAndRestartErrors 覆盖 Apply 失败与重启失败。
func TestApplyAndRestartErrors(t *testing.T) {
	stubReplace(t, func(_, _, _ string) (bool, error) { return true, nil })
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("0.9.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true})
	if _, err := u.ApplyAndRestart(context.Background(), func() error { return nil }); !errors.Is(err, ErrDowngrade) {
		t.Fatalf("Apply 失败应透传，实际：%v", err)
	}
	u2, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true})
	if _, err := u2.ApplyAndRestart(context.Background(), func() error {
		return errors.New("重启失败")
	}); err == nil {
		t.Fatal("重启失败应透传")
	}
}

// TestNewHTTPClientFailure 覆盖默认客户端构造失败。
func TestNewHTTPClientFailure(t *testing.T) {
	orig := newHTTPClient
	newHTTPClient = func() (*httpx.Client, error) { return nil, errors.New("构造失败") }
	defer func() { newHTTPClient = orig }()
	if _, err := New(Config{Source: &stubSource{}, CurrentVersion: "1.0.0", ExecutablePath: "x"}); err == nil {
		t.Fatal("客户端构造失败应报错")
	}
}
