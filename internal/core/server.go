package core

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lcylpzls/errx"
)

// 默认服务端路由与清单文件名。
const (
	defaultManifestFile = "manifest.json"
	defaultManifestURL  = "/updates/manifest.json"
	defaultAssetsURL    = "/updates/assets/"
	defaultAdminPrefix  = "/updates/admin"
)

// Middleware 是服务端可热插拔的标准中间件。
type Middleware func(http.Handler) http.Handler

// serverConfig 是服务端配置。
type serverConfig struct {
	assetsDir    string
	manifestPath string
	manifestURL  string
	assetsURL    string
	adminToken   string
}

// ServerOption 修改服务端配置。
type ServerOption func(*serverConfig)

// WithAdminToken 设置管理路由鉴权令牌；为空表示不挂载管理路由。
func WithAdminToken(token string) ServerOption {
	return func(c *serverConfig) { c.adminToken = token }
}

// WithManifestPath 设置清单文件路径，默认 assetsDir/manifest.json。
func WithManifestPath(path string) ServerOption {
	return func(c *serverConfig) {
		if path != "" {
			c.manifestPath = path
		}
	}
}

// WithManifestURL 设置清单对外路由，默认 /updates/manifest.json。
func WithManifestURL(path string) ServerOption {
	return func(c *serverConfig) {
		if path != "" {
			c.manifestURL = path
		}
	}
}

// WithAssetsURL 设置资产静态服务前缀，默认 /updates/assets/。
func WithAssetsURL(path string) ServerOption {
	return func(c *serverConfig) {
		if path != "" {
			c.assetsURL = path
		}
	}
}

// adminRoute 是一条管理路由。
type adminRoute struct {
	pattern string
	handler http.Handler
}

// Server 是自更新服务端封装：清单、资产静态服务与管理路由。
type Server struct {
	mu          sync.RWMutex
	cfg         serverConfig
	manifest    *Manifest
	mws         []Middleware
	adminRoutes []adminRoute
}

// NewServer 创建自更新服务端。
// assetsDir 必须存在，且默认包含 manifest.json 清单文件。
func NewServer(assetsDir string, opts ...ServerOption) (*Server, error) {
	if assetsDir == "" {
		return nil, errx.NewCode(CodeInvalidConfig, "资产目录不能为空")
	}
	info, err := os.Stat(assetsDir)
	if err != nil {
		return nil, errx.WrapCode(err, CodeInvalidConfig, "资产目录不可用："+assetsDir)
	}
	if !info.IsDir() {
		return nil, errx.NewCode(CodeInvalidConfig, "资产路径不是目录："+assetsDir)
	}
	cfg := serverConfig{
		assetsDir:    assetsDir,
		manifestPath: filepath.Join(assetsDir, defaultManifestFile),
		manifestURL:  defaultManifestURL,
		assetsURL:    defaultAssetsURL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	s := &Server{cfg: cfg}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// Reload 重新读取并解析清单文件。
func (s *Server) Reload() error {
	data, err := os.ReadFile(s.cfg.manifestPath)
	if err != nil {
		return errx.WrapCode(err, CodeManifestInvalid, "读取清单文件失败："+s.cfg.manifestPath)
	}
	m, err := ParseManifest(data)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.manifest = m
	s.mu.Unlock()
	return nil
}

// Use 追加全局中间件（可热插拔，Handler 每次按当前链重建）。
func (s *Server) Use(mw ...Middleware) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range mw {
		if m != nil {
			s.mws = append(s.mws, m)
		}
	}
}

// HandleAdmin 注册管理路由（需先配置 WithAdminToken）。
func (s *Server) HandleAdmin(pattern string, h http.Handler) error {
	if pattern == "" || !strings.HasPrefix(pattern, "/") {
		return errx.NewCode(CodeInvalidConfig, "管理路由必须以 / 开头")
	}
	if h == nil {
		return errx.NewCode(CodeInvalidConfig, "管理路由处理器不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.adminToken == "" {
		return errx.NewCode(CodeInvalidConfig, "未配置管理令牌，无法注册管理路由")
	}
	s.adminRoutes = append(s.adminRoutes, adminRoute{pattern: pattern, handler: h})
	return nil
}

// RegisterTo 将全部路由注册到外部 mux。
func (s *Server) RegisterTo(mux *http.ServeMux) {
	h := s.Handler()
	// 标准库 mux 无“默认”模式，这里把服务端处理器挂到根路径。
	mux.Handle("/", h)
}

// Handler 返回完整服务端处理器：中间件 + 清单/资产/管理路由。
func (s *Server) Handler() http.Handler {
	s.mu.RLock()
	cfg := s.cfg
	manifest := s.manifest
	mws := append([]Middleware(nil), s.mws...)
	adminRoutes := append([]adminRoute(nil), s.adminRoutes...)
	s.mu.RUnlock()

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.manifestURL, func(w http.ResponseWriter, _ *http.Request) {
		// Manifest 字段均可序列化，不会失败。
		data, _ := json.Marshal(manifest)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(data)
	})
	mux.Handle(cfg.assetsURL, http.StripPrefix(cfg.assetsURL, assetsHandler(cfg.assetsDir)))

	if cfg.adminToken != "" {
		adminMux := http.NewServeMux()
		adminMux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"ok":true}`))
		})
		for _, ar := range adminRoutes {
			adminMux.Handle(ar.pattern, ar.handler)
		}
		mux.Handle(defaultAdminPrefix+"/",
			tokenGuard(cfg.adminToken, http.StripPrefix(defaultAdminPrefix, adminMux)))
	}

	var h http.Handler = mux
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// assetsHandler 提供资产静态服务，并做路径穿越防护。
func assetsHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	base := filepath.Clean(dir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		full := filepath.Clean(filepath.Join(base, filepath.FromSlash(name)))
		if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

// tokenGuard 校验 X-Api-Token（常量时间比较）。
func tokenGuard(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Api-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"未授权"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
