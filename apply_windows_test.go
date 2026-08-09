//go:build windows

package updatex

import (
	"context"
	"errors"
	"testing"
)

// TestApplyWindowsPlaceholder 覆盖 Windows 占位（v0.2 实现替换）。
func TestApplyWindowsPlaceholder(t *testing.T) {
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, err := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrReplaceFailed) {
		t.Fatalf("Windows 占位应返回替换失败，实际：%v", err)
	}
}

// TestApplyWindowsFlow 注入替换函数覆盖完整流程（与 Unix 对齐）。
func TestApplyWindowsFlow(t *testing.T) {
	orig := replaceExec
	replaceExec = func(_, _ string) (bool, error) { return true, nil }
	defer func() { replaceExec = orig }()
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, err := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	info, err := u.Apply(context.Background())
	if err != nil || !info.RestartRequired {
		t.Fatalf("注入替换应返回重启需求：info=%+v err=%v", info, err)
	}
	var restarted bool
	if _, err := u.ApplyAndRestart(context.Background(), func() error {
		restarted = true
		return nil
	}); err != nil || !restarted {
		t.Fatalf("ApplyAndRestart 应调用重启：restarted=%v err=%v", restarted, err)
	}
}
