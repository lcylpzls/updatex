package updatex_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcylpzls/updatex"
)

// TestPublicAPI 黑盒冒烟测试：覆盖根包全部工厂函数、类型别名与常量。
func TestPublicAPI(t *testing.T) {
	manifestBody := `{"version":"1.2.3","published_at":"2026-08-12T00:00:00Z",` +
		`"notes":"发布说明",` +
		`"platforms":{"linux_amd64":{"url":"/updates/assets/app.bin",` +
		`"sha256":"` + strings.Repeat("a", 64) + `","size":4}}}`
	m, err := updatex.ParseManifest([]byte(manifestBody))
	if err != nil || m.Version != "1.2.3" {
		t.Fatalf("ParseManifest 失败：%v", err)
	}
	if _, err := updatex.ParseManifest([]byte("bad")); err == nil {
		t.Fatal("非法清单应报错")
	}
	if got := updatex.SameOriginResolver(
		"https://node.example.com:19091/updates/manifest.json",
		updatex.Asset{URL: "https://update.invalid/updates/assets/app.bin"},
	); got != "https://node.example.com:19091/updates/assets/app.bin" {
		t.Fatalf("SameOriginResolver 不符：%q", got)
	}

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifestBody), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "app.bin"), []byte("data"), 0o600)
	srv, err := updatex.NewServer(dir,
		updatex.WithAdminToken("secret"),
		updatex.WithManifestPath(filepath.Join(dir, "manifest.json")),
		updatex.WithManifestURL("/updates/manifest.json"),
		updatex.WithAssetsURL("/updates/assets/"),
	)
	if err != nil || srv == nil {
		t.Fatalf("NewServer 失败：%v", err)
	}
	if _, err := updatex.NewServer(""); err == nil {
		t.Fatal("空目录应报错")
	}

	c, err := updatex.NewClient(updatex.ClientConfig{
		ManifestURL:    "https://example.com/updates/manifest.json",
		CurrentVersion: "1.0.0",
		AfterUpdate:    updatex.AfterUpdateContinue,
	})
	if err != nil || c == nil {
		t.Fatalf("NewClient 失败：%v", err)
	}
	if _, err := updatex.NewClient(updatex.ClientConfig{}); err == nil {
		t.Fatal("空配置应报错")
	}

	// 类型与常量引用。
	var (
		_ updatex.VersionSource
		_ updatex.Metrics
		_ updatex.TraceAttr
		_ updatex.TraceHook
		_ updatex.Manifest
		_ updatex.Asset
		_ *updatex.Server
		_ updatex.ServerOption
		_ *updatex.Client
		_ updatex.ClientConfig
		_ updatex.Result
		_ updatex.AfterUpdateAction
		_ updatex.Protocol
	)
	_ = []updatex.Protocol{updatex.ProtocolAuto, updatex.ProtocolHTTP1,
		updatex.ProtocolHTTP2, updatex.ProtocolHTTP3}
	_ = []updatex.AfterUpdateAction{updatex.AfterUpdateContinue,
		updatex.AfterUpdateExit, updatex.AfterUpdateRestart}
	_ = updatex.CodeInvalidConfig
	_ = updatex.ErrInvalidConfig
}
