//go:build !windows

package updatex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestApplySuccess 覆盖 Unix 真实原子替换。
func TestApplySuccess(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv, sha := assetServer(t, "new-binary")
	defer srv.Close()
	u, err := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: target, AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	info, err := u.Apply(context.Background())
	if err != nil || info.Version != "1.1.0" || info.RestartRequired {
		t.Fatalf("更新失败：%+v err=%v", info, err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "new-binary" {
		t.Fatalf("替换内容不符：%q err=%v", data, err)
	}
	st, _ := os.Stat(target)
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("权限应为 0755：%v", st.Mode())
	}
}

// TestApplyReplaceFailure 覆盖 Unix 替换失败（目标目录不存在）。
func TestApplyReplaceFailure(t *testing.T) {
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0",
		ExecutablePath: filepath.Join(t.TempDir(), "missing", "app"),
		AllowHTTP:      true})
	if _, err := u.Apply(context.Background()); !errors.Is(err, ErrReplaceFailed) {
		t.Fatalf("替换失败应报错，实际：%v", err)
	}
}
