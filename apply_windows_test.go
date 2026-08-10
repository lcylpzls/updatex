//go:build windows

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

// TestApplyWindowsSuccess 覆盖真实 Windows 延迟替换与 Bootstrap 闭环。
func TestApplyWindowsSuccess(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app.exe")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, err := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: target, AllowHTTP: true})
	testx.RequireNoError(t, err)

	info, err := u.Apply(context.Background())
	if err != nil || !info.RestartRequired || info.Version != "1.1.0" {
		t.Fatalf("延迟替换应返回重启需求：info=%+v err=%v", info, err)
	}
	if data, readErr := os.ReadFile(target + ".new"); readErr != nil || string(data) != "new" {
		t.Fatalf("新版本应暂存为 .new：%q err=%v", data, readErr)
	}
	if data, readErr := os.ReadFile(target + ".pending"); readErr != nil || string(data) != "1.1.0" {
		t.Fatalf("标记应记录目标版本：%q err=%v", data, readErr)
	}
	// 新进程启动时执行 Bootstrap：替换完成并清理标记。
	if err := Bootstrap(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if data, readErr := os.ReadFile(target); readErr != nil || string(data) != "new" {
		t.Fatalf("Bootstrap 后目标应为新版本：%q err=%v", data, readErr)
	}
	for _, name := range []string{target + ".new", target + ".pending", target + ".old"} {
		if _, statErr := os.Stat(name); !os.IsNotExist(statErr) {
			t.Fatalf("Bootstrap 后不应残留 %s：%v", name, statErr)
		}
	}
}

// TestApplyWindowsPendingWriteFailure 覆盖标记写入失败时清理 .new。
func TestApplyWindowsPendingWriteFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app.exe")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, err := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: target, AllowHTTP: true})
	testx.RequireNoError(t, err)

	origWrite := writeFile
	writeFile = func(string, []byte, os.FileMode) error { return errors.New("写入失败") }
	defer func() { writeFile = origWrite }()
	if _, err := u.Apply(context.Background()); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("标记写入失败应报错，实际：%v", err)
	}
	if _, statErr := os.Stat(target + ".new"); !os.IsNotExist(statErr) {
		t.Fatalf("标记失败应清理 .new：%v", statErr)
	}
	if _, statErr := os.Stat(target + ".pending"); !os.IsNotExist(statErr) {
		t.Fatalf("标记失败不应残留 .pending：%v", statErr)
	}
}

// TestReplaceExecutableInvalidArgs 覆盖空参数校验。
func TestReplaceExecutableInvalidArgs(t *testing.T) {
	if _, err := replaceExecutable("", "x", "1.1.0"); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("空目标路径应报配置错误，实际：%v", err)
	}
}

// TestReplaceExecutableStageFailure 覆盖暂存新版本失败。
func TestReplaceExecutableStageFailure(t *testing.T) {
	origRename := renameFile
	renameFile = func(string, string) error { return errors.New("暂存失败") }
	defer func() { renameFile = origRename }()
	if _, err := replaceExecutable("app.exe", "new", "1.1.0"); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("暂存失败应报替换失败，实际：%v", err)
	}
}

// TestWritePendingFailures 覆盖标记写入各阶段失败。
func TestWritePendingFailures(t *testing.T) {
	origCreate := createTempFile
	createTempFile = func(string, string) (*os.File, error) { return nil, errors.New("创建失败") }
	if err := writePending("app.exe", "1.1.0"); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("临时文件创建失败应报替换失败，实际：%v", err)
	}
	createTempFile = origCreate

	origClose := closeFile
	closeFile = func(*os.File) error { return errors.New("关闭失败") }
	if err := writePending("app.exe", "1.1.0"); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("关闭失败应报替换失败，实际：%v", err)
	}
	closeFile = origClose

	origRename := renameFile
	renameFile = func(string, string) error { return errors.New("落位失败") }
	if err := writePending("app.exe", "1.1.0"); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("落位失败应报替换失败，实际：%v", err)
	}
	renameFile = origRename
}

// TestApplyWindowsFlow 注入替换函数覆盖完整流程（与 Unix 对齐）。
func TestApplyWindowsFlow(t *testing.T) {
	orig := replaceExec
	replaceExec = func(_, _, _ string) (bool, error) { return true, nil }
	defer func() { replaceExec = orig }()
	srv, sha := assetServer(t, "new")
	defer srv.Close()
	u, err := New(Config{Source: &stubSource{
		manifest: newStubManifest("1.1.0", srv.URL+"/download", sha)},
		CurrentVersion: "1.0.0", ExecutablePath: "x", AllowHTTP: true})
	testx.RequireNoError(t, err)

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

// TestBootstrapNoPending 覆盖无标记场景。
func TestBootstrapNoPending(t *testing.T) {
	if err := Bootstrap(context.Background(), filepath.Join(t.TempDir(), "app.exe")); err != nil {
		t.Fatalf("无标记应直接返回：%v", err)
	}
}

// TestBootstrapMissingNew 覆盖标记存在但新版本缺失。
func TestBootstrapMissingNew(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app.exe")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(target+".pending", []byte("1.1.0"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(context.Background(), target); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("新版本缺失应报错，实际：%v", err)
	}
	if _, statErr := os.Stat(target + ".pending"); statErr != nil {
		t.Fatalf("失败应保留标记以便重试：%v", statErr)
	}
}

// TestBootstrapEmptyPending 覆盖空标记。
func TestBootstrapEmptyPending(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app.exe")
	if err := writeFile(target+".pending", []byte("  "), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(context.Background(), target); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("空标记应报错，实际：%v", err)
	}
}

// TestBootstrapReadFailure 覆盖标记读取失败。
func TestBootstrapReadFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app.exe")
	if err := os.Mkdir(target+".pending", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(context.Background(), target); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("标记读取失败应报错，实际：%v", err)
	}
}

// TestBootstrapRollback 覆盖替换失败回滚成功与回滚失败。
func TestBootstrapRollback(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app.exe")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+".new", []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(target+".pending", []byte("1.1.0"), 0o600); err != nil {
		t.Fatal(err)
	}
	origRename := renameFile
	calls := 0
	renameFile = func(old, new string) error {
		calls++
		if calls == 2 {
			return errors.New("落位失败")
		}
		return origRename(old, new)
	}
	err := bootstrap(target)
	renameFile = origRename
	if !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("替换失败应报错，实际：%v", err)
	}
	if data, readErr := os.ReadFile(target); readErr != nil || string(data) != "old" {
		t.Fatalf("回滚后应恢复旧版本：%q err=%v", data, readErr)
	}
	if _, statErr := os.Stat(target + ".pending"); statErr != nil {
		t.Fatalf("失败应保留标记以便重试：%v", statErr)
	}

	// 回滚也失败：应返回 CodeRollbackFailed。
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+".new", []byte("new"), 0o644); err != nil {
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
	err = bootstrap(target)
	renameFile = origRename
	if !errx.Is(err, CodeRollbackFailed) {
		t.Fatalf("回滚失败应报错，实际：%v", err)
	}
}

// TestBootstrapBackupFailure 覆盖启动时备份失败。
func TestBootstrapBackupFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app.exe")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+".new", []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(target+".pending", []byte("1.1.0"), 0o600); err != nil {
		t.Fatal(err)
	}
	origRename := renameFile
	renameFile = func(string, string) error { return errors.New("备份失败") }
	defer func() { renameFile = origRename }()
	if err := Bootstrap(context.Background(), target); !errx.Is(err, CodeReplaceFailed) {
		t.Fatalf("备份失败应报错，实际：%v", err)
	}
	if _, statErr := os.Stat(target + ".pending"); statErr != nil {
		t.Fatalf("失败应保留标记以便重试：%v", statErr)
	}
}
