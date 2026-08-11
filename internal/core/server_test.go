package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/testx"
)

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

// TestNewServerValidation 覆盖服务端构造校验。
func TestNewServerValidation(t *testing.T) {
	if _, err := NewServer(""); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("空目录应报错：%v", err)
	}
	if _, err := NewServer(filepath.Join(t.TempDir(), "missing")); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("不存在目录应报错：%v", err)
	}
	f := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(f, []byte("x"), 0o600)
	if _, err := NewServer(f); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("文件路径应报错：%v", err)
	}
	empty := t.TempDir()
	if _, err := NewServer(empty); !errx.Is(err, CodeManifestInvalid) {
		t.Fatalf("缺清单应报错：%v", err)
	}
	bad := t.TempDir()
	_ = os.WriteFile(filepath.Join(bad, "manifest.json"), []byte("bad"), 0o600)
	if _, err := NewServer(bad); !errx.Is(err, CodeManifestInvalid) {
		t.Fatalf("非法清单应报错：%v", err)
	}
}

// TestServerManifestAndAssets 覆盖清单与资产路由。
func TestServerManifestAndAssets(t *testing.T) {
	dir := newServerDir(t, "1.2.3")
	s, err := NewServer(dir)
	testx.RequireNoError(t, err)
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/updates/manifest.json", nil))
	testx.RequireEqual(t, rec.Code, http.StatusOK)
	testx.RequireTrue(t, strings.Contains(rec.Body.String(), `"version":"1.2.3"`))

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/updates/assets/app.bin", nil))
	testx.RequireEqual(t, rec.Code, http.StatusOK)
	testx.RequireEqual(t, rec.Body.String(), "data")

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/updates/assets/missing.bin", nil))
	testx.RequireEqual(t, rec.Code, http.StatusNotFound)
}

// TestServerCustomOptions 覆盖自定义清单路径与路由前缀。
func TestServerCustomOptions(t *testing.T) {
	dir := newServerDir(t, "3.0.0")
	os.Rename(filepath.Join(dir, "manifest.json"), filepath.Join(dir, "custom.json"))
	s, err := NewServer(dir,
		WithManifestPath(filepath.Join(dir, "custom.json")),
		WithManifestURL("/custom/manifest.json"),
		WithAssetsURL("/custom/assets/"),
		WithAdminToken("tok"),
	)
	testx.RequireNoError(t, err)
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/custom/manifest.json", nil))
	testx.RequireEqual(t, rec.Code, http.StatusOK)
	testx.RequireTrue(t, strings.Contains(rec.Body.String(), `"version":"3.0.0"`))

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/custom/assets/app.bin", nil))
	testx.RequireEqual(t, rec.Code, http.StatusOK)

	// 默认管理前缀仍生效。
	req := httptest.NewRequest(http.MethodGet, "/updates/admin/status", nil)
	req.Header.Set("X-Api-Token", "tok")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	testx.RequireEqual(t, rec.Code, http.StatusOK)
}

// TestAssetsHandlerTraversal 覆盖路径穿越防护。
func TestAssetsHandlerTraversal(t *testing.T) {
	dir := newServerDir(t, "1.0.0")
	_ = os.WriteFile(filepath.Join(filepath.Dir(dir), "outside.txt"), []byte("x"), 0o600)
	h := assetsHandler(dir)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/../outside.txt", nil))
	testx.RequireEqual(t, rec.Code, http.StatusNotFound)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.bin", nil))
	testx.RequireEqual(t, rec.Code, http.StatusOK)
}

// TestServerAdmin 覆盖管理路由鉴权与自定义路由。
func TestServerAdmin(t *testing.T) {
	dir := newServerDir(t, "1.0.0")
	s, err := NewServer(dir, WithAdminToken("secret"))
	testx.RequireNoError(t, err)
	_ = s.HandleAdmin("/info", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/updates/admin/status", nil))
	testx.RequireEqual(t, rec.Code, http.StatusUnauthorized)

	req := httptest.NewRequest(http.MethodGet, "/updates/admin/status", nil)
	req.Header.Set("X-Api-Token", "secret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	testx.RequireEqual(t, rec.Code, http.StatusOK)
	testx.RequireTrue(t, strings.Contains(rec.Body.String(), `"ok":true`))

	req = httptest.NewRequest(http.MethodGet, "/updates/admin/info", nil)
	req.Header.Set("X-Api-Token", "secret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	testx.RequireEqual(t, rec.Code, http.StatusNoContent)
}

// TestServerAdminNoToken 覆盖未配置令牌时管理路由不挂载。
func TestServerAdminNoToken(t *testing.T) {
	dir := newServerDir(t, "1.0.0")
	s, err := NewServer(dir)
	testx.RequireNoError(t, err)
	if err := s.HandleAdmin("/info", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("未配置令牌应报错：%v", err)
	}
	if err := s.HandleAdmin("", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("空 pattern 应报错：%v", err)
	}
	if err := s.HandleAdmin("/x", nil); !errx.Is(err, CodeInvalidConfig) {
		t.Fatalf("nil 处理器应报错：%v", err)
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/updates/admin/status", nil))
	testx.RequireEqual(t, rec.Code, http.StatusNotFound)
}

// TestServerMiddleware 覆盖热插拔全局中间件。
func TestServerMiddleware(t *testing.T) {
	dir := newServerDir(t, "1.0.0")
	s, err := NewServer(dir)
	testx.RequireNoError(t, err)
	s.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test", "1")
			next.ServeHTTP(w, r)
		})
	})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/updates/manifest.json", nil))
	testx.RequireEqual(t, rec.Header().Get("X-Test"), "1")

	s.Use(nil)
}

// TestServerReload 覆盖清单热更新。
func TestServerReload(t *testing.T) {
	dir := newServerDir(t, "1.0.0")
	s, err := NewServer(dir)
	testx.RequireNoError(t, err)

	newManifest := strings.ReplaceAll(
		string(mustRead(t, filepath.Join(dir, "manifest.json"))), "1.0.0", "2.0.0")
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(newManifest), 0o600)
	testx.RequireNoError(t, s.Reload())

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/updates/manifest.json", nil))
	testx.RequireTrue(t, strings.Contains(rec.Body.String(), `"version":"2.0.0"`))

	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("bad"), 0o600)
	if err := s.Reload(); !errx.Is(err, CodeManifestInvalid) {
		t.Fatalf("非法清单应报错：%v", err)
	}
}

// TestServerRegisterTo 覆盖注册到外部 mux。
func TestServerRegisterTo(t *testing.T) {
	dir := newServerDir(t, "1.0.0")
	s, err := NewServer(dir)
	testx.RequireNoError(t, err)
	mux := http.NewServeMux()
	s.RegisterTo(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/updates/manifest.json", nil))
	testx.RequireEqual(t, rec.Code, http.StatusOK)
}

// mustRead 读取文件（测试辅助）。
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	testx.RequireNoError(t, err)
	return b
}
