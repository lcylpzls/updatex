package updatex_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcylpzls/updatex"
)

// TestPublicAPI 黑盒冒烟测试：覆盖根包全部转发函数、类型别名与常量。
func TestPublicAPI(t *testing.T) {
	_, err := updatex.ParseManifest([]byte(`{"version":"1.2.3","platforms":{}}`))
	if err != nil {
		t.Logf("ParseManifest 返回错误（可接受）：%v", err)
	}

	_, err = updatex.New(updatex.Config{})
	if err != nil {
		t.Logf("New 返回错误（可接受）：%v", err)
	}

	_ = updatex.Bootstrap(context.Background(), "nonexistent-executable")

	dir := t.TempDir()
	manifest := `{"version":"1.2.3","published_at":"2026-08-12T00:00:00Z",` +
		`"platforms":{"linux_amd64":{"url":"/app.bin","sha256":"` +
		strings.Repeat("a", 64) + `","size":4}}}`
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600)
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
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/updates/manifest.json", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version":"1.2.3"`) {
		t.Fatalf("清单路由异常：%d %s", rec.Code, rec.Body.String())
	}
	srv.Use(nil)
	_ = srv.Reload()
	srv.RegisterTo(http.NewServeMux())

	_ = updatex.CodeInvalidConfig
	_ = updatex.CodeInvalidVersion
	_ = updatex.CodeManifestInvalid
	_ = updatex.CodeFetchFailed
	_ = updatex.CodeDownloadFailed
	_ = updatex.CodeChecksumMismatch
	_ = updatex.CodeSignatureInvalid
	_ = updatex.CodeDowngrade
	_ = updatex.CodePlatformUnsupported
	_ = updatex.CodeReplaceFailed
	_ = updatex.CodeRollbackFailed

	var _ updatex.VersionSource
	var _ updatex.Metrics
	var _ updatex.Config
	var _ updatex.UpdateInfo
	var _ updatex.Updater
	var _ updatex.TraceAttr
	var _ updatex.TraceHook
	var _ updatex.Manifest
	var _ updatex.Asset
	var _ updatex.Server
	var _ updatex.ServerOption
	var _ updatex.Middleware
}
