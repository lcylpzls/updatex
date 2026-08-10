//go:build !windows

package updatex

import (
	"context"
	"errors"
	testx "github.com/lcylpzls/testx"
	"os"
	"path/filepath"
	"testing"

	"github.com/lcylpzls/errx"
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
	testx.RequireNoError(t, err)

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
	if _, statErr := os.Stat(target + ".bak"); !os.IsNotExist(statErr) {
		t.Fatalf("成功后不应残留备份：%v", statErr)
	}
}

// TestApplyReplaceFailure 覆盖 Unix 备份当前版本失败。
func TestApplyReplaceFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	origRename := renameFile
	renameFile = func(string, string) error { return errors.New("备份失败") }
	defer func() { renameFile = origRename }()
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: target, AllowHTTP: true})
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("替换失败应报错，实际：%v", err)
	}
	if data, readErr := os.ReadFile(target); readErr != nil || string(data) != "old" {
		t.Fatalf("备份失败不应影响当前版本：%q err=%v", data, readErr)
	}
}

// TestApplyRollback 覆盖替换失败后回滚成功与回滚失败。
func TestApplyRollback(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv, sha := assetServer(t, "new-binary")
	defer srv.Close()
	u, err := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: target, AllowHTTP: true})
	testx.RequireNoError(t, err)

	// 第一次 rename（备份）成功，第二次 rename（新版本落位）失败，
	// 第三次 rename（恢复备份）成功：应返回替换失败并恢复旧版本。
	origRename := renameFile
	calls := 0
	renameFile = func(old, new string) error {
		calls++
		if calls == 2 {
			return errors.New("落位失败")
		}
		return origRename(old, new)
	}
	_, err = u.Apply(context.Background())
	renameFile = origRename
	if !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("替换失败应报错，实际：%v", err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil || string(data) != "old" {
		t.Fatalf("回滚后应恢复旧版本：%q err=%v", data, readErr)
	}
	if _, statErr := os.Stat(target + ".bak"); !os.IsNotExist(statErr) {
		t.Fatalf("回滚后不应残留备份：%v", statErr)
	}

	// 回滚也失败：应返回 CodeRollbackFailed。
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	calls = 0
	renameFile = func(old, new string) error {
		calls++
		if calls == 2 || calls == 3 {
			return errors.New("失败")
		}
		return origRename(old, new)
	}
	_, err = u.Apply(context.Background())
	renameFile = origRename
	if !errx.Is(err, CodeRollbackFailed) {
		t.Fatalf("回滚失败应报错，实际：%v", err)
	}
}

// TestApplyChmodFailure 覆盖设置执行权限失败。
func TestApplyChmodFailure(t *testing.T) {
	origChmod := chmodFile
	chmodFile = func(string, os.FileMode) error { return errors.New("权限失败") }
	defer func() { chmodFile = origChmod }()
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, _ := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: filepath.Join(t.TempDir(), "app"),
		AllowHTTP: true})
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("权限失败应报替换失败，实际：%v", err)
	}
}
