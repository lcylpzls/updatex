package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/testx"
	"github.com/lcylpzls/updatex"
	"github.com/lcylpzls/updatex/examples/internal/testutil"
	"github.com/lcylpzls/updatex/examples/internal/updateserver"
)

// TestRunHTTP3Upgrade 验证 HTTP/3 端到端升级（1.0.0 → 1.1.0）。
func TestRunHTTP3Upgrade(t *testing.T) {
	certFile, keyFile := testutil.WriteTestCert(t)
	asset := []byte("新版本二进制内容")
	var protos []string
	var mu sync.Mutex
	record := func(p string) {
		mu.Lock()
		protos = append(protos, p)
		mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s, addr, err := updateserver.StartAndWait(ctx, updateserver.Config{
		Version:   "1.1.0",
		Notes:     "HTTP/3 示例更新",
		Asset:     asset,
		OnRequest: record,
	}, certFile, keyFile, "127.0.0.1:0", testLogger())
	testx.RequireNoError(t, err)

	defer s.Stop(context.Background())

	target := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(target, []byte("旧版本二进制内容"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		ManifestURL:    "https://" + addr, // 根地址，自动补 /updates/manifest.json
		CurrentVersion: "1.0.0",
		Target:         target,
		UseHTTP3:       true,
		InsecureTLS:    true,
		AfterUpdate:    updatex.AfterUpdateContinue,
	}
	res, err := run(ctx, opts, testLogger())
	if err != nil || !res.Updated || res.Version != "1.1.0" {
		t.Fatalf("升级失败：res=%+v err=%v", res, err)
	}
	if res.RestartRequired {
		// Windows 延迟替换：模拟新进程再次执行 Run，由内部 Bootstrap 完成替换。
		res2, err := run(ctx, opts, testLogger())
		if err != nil {
			t.Fatalf("模拟重启失败：%v", err)
		}
		_ = res2
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != string(asset) {
		t.Fatalf("升级后目标内容不符：%q err=%v", data, err)
	}
	mu.Lock()
	hitHTTP3 := false
	for _, p := range protos {
		if p == "HTTP/3.0" {
			hitHTTP3 = true
			break
		}
	}
	mu.Unlock()
	testx.RequireTrue(t, hitHTTP3)

	// 已是最新版本：不替换。
	opts.CurrentVersion = "1.1.0"
	res2, err := run(ctx, opts, testLogger())
	if err != nil || res2.Updated {
		t.Fatalf("检查最新版本失败：res=%+v err=%v", res2, err)
	}
	data, err = os.ReadFile(target)
	if err != nil || string(data) != string(asset) {
		t.Fatalf("最新版本不应替换目标：%q err=%v", data, err)
	}
}

// testLogger 构造写入丢弃目标的日志器。
func testLogger() logx.Logger {
	logger, err := logx.NewBuilder().EnableWriter(io.Discard, logx.InfoLevel).Build()
	if err != nil {
		panic(err)
	}
	return logger
}
