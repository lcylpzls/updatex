// Package server 提供基于 webx 的自更新服务端实现：清单、资产静态服务与管理路由。
package server

import (
	"crypto/subtle"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/updatex/internal/core"
	"github.com/lcylpzls/webx"
)

// 默认服务端路由与清单文件名。
const (
	defaultManifestFile = "manifest.json"
	defaultManifestURL  = "/updates/manifest.json"
	defaultAssetsURL    = "/updates/assets/"
	defaultAdminPrefix  = "/updates/admin"
)

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
	method  string
	pattern string
	handler webx.HandlerFunc
}

// Server 是自更新服务端：清单、资产静态服务与管理路由，通过 RegisterWebx 挂载到 webx。
type Server struct {
	mu          sync.RWMutex
	cfg         serverConfig
	manifest    *core.Manifest
	adminRoutes []adminRoute
}

// NewServer 创建自更新服务端。
// assetsDir 必须存在，且默认包含 manifest.json 清单文件。
func NewServer(assetsDir string, opts ...ServerOption) (*Server, error) {
	if assetsDir == "" {
		return nil, errx.NewCode(core.CodeInvalidConfig, "资产目录不能为空")
	}
	info, err := os.Stat(assetsDir)
	if err != nil {
		return nil, errx.WrapCode(err, core.CodeInvalidConfig, "资产目录不可用："+assetsDir)
	}
	if !info.IsDir() {
		return nil, errx.NewCode(core.CodeInvalidConfig, "资产路径不是目录："+assetsDir)
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

// Reload 重新读取并解析清单文件（热更新，无需重新注册路由）。
func (s *Server) Reload() error {
	data, err := os.ReadFile(s.cfg.manifestPath)
	if err != nil {
		return errx.WrapCode(err, core.CodeManifestInvalid, "读取清单文件失败："+s.cfg.manifestPath)
	}
	m, err := core.ParseManifest(data)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.manifest = m
	s.mu.Unlock()
	return nil
}

// HandleAdmin 注册管理路由（webx 原生处理器，需先配置 WithAdminToken）。
// method 支持 GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS。
func (s *Server) HandleAdmin(method, pattern string, h webx.HandlerFunc) error {
	if method == "" {
		return errx.NewCode(core.CodeInvalidConfig, "管理路由方法不能为空")
	}
	if !validMethod(method) {
		return errx.NewCodef(core.CodeInvalidConfig, "不支持的管理路由方法：%s", method)
	}
	if pattern == "" || !strings.HasPrefix(pattern, "/") {
		return errx.NewCode(core.CodeInvalidConfig, "管理路由必须以 / 开头")
	}
	if h == nil {
		return errx.NewCode(core.CodeInvalidConfig, "管理路由处理器不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.adminToken == "" {
		return errx.NewCode(core.CodeInvalidConfig, "未配置管理令牌，无法注册管理路由")
	}
	s.adminRoutes = append(s.adminRoutes, adminRoute{method: method, pattern: pattern, handler: h})
	return nil
}

// RegisterWebx 将清单、资产与管理路由注册到 webx 服务。
// 必须在 webx.Start() 之前调用；可在多个 webx 实例上重复调用。
func (s *Server) RegisterWebx(ws *webx.Server) error {
	if ws == nil {
		return errx.NewCode(core.CodeInvalidConfig, "webx 服务不能为空")
	}
	s.mu.RLock()
	cfg := s.cfg
	adminRoutes := append([]adminRoute(nil), s.adminRoutes...)
	s.mu.RUnlock()

	if !strings.HasPrefix(cfg.manifestURL, "/") {
		return errx.NewCode(core.CodeInvalidConfig, "清单路由必须以 / 开头")
	}
	if !strings.HasPrefix(cfg.assetsURL, "/") {
		return errx.NewCode(core.CodeInvalidConfig, "资产路由必须以 / 开头")
	}

	ws.RegisterRoute(webx.Route{
		Method:  http.MethodGet,
		Path:    cfg.manifestURL,
		Handler: s.manifestHandler,
	})
	ws.ServeStaticDirWithOptions(cfg.assetsURL, cfg.assetsDir, webx.StaticOptions{})

	if cfg.adminToken != "" {
		ws.RegisterRouteGroup(defaultAdminPrefix, func(rg *webx.RouteGroup) {
			rg.Use(tokenGuard(cfg.adminToken))
			rg.GET("/status", func(c *webx.Context) {
				_ = c.JSON(http.StatusOK, map[string]bool{"ok": true})
			})
			for _, ar := range adminRoutes {
				registerAdminRoute(rg, ar)
			}
		})
	}
	return nil
}

// manifestHandler 返回当前清单快照（Reload 后无需重新注册即生效）。
func (s *Server) manifestHandler(c *webx.Context) {
	s.mu.RLock()
	m := s.manifest
	s.mu.RUnlock()
	_ = c.JSON(http.StatusOK, m)
}

// registerAdminRoute 按方法注册管理路由。
func registerAdminRoute(rg *webx.RouteGroup, ar adminRoute) {
	switch ar.method {
	case http.MethodGet:
		rg.GET(ar.pattern, ar.handler)
	case http.MethodPost:
		rg.POST(ar.pattern, ar.handler)
	case http.MethodPut:
		rg.PUT(ar.pattern, ar.handler)
	case http.MethodDelete:
		rg.DELETE(ar.pattern, ar.handler)
	case http.MethodPatch:
		rg.PATCH(ar.pattern, ar.handler)
	case http.MethodHead:
		rg.HEAD(ar.pattern, ar.handler)
	case http.MethodOptions:
		rg.OPTIONS(ar.pattern, ar.handler)
	}
}

// validMethod 校验支持的管理路由方法。
func validMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodPatch, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// tokenGuard 校验 X-Api-Token（常量时间比较）。
func tokenGuard(token string) webx.HandlerFunc {
	return func(c *webx.Context) {
		got := c.Request().Header.Get("X-Api-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, "未授权", nil)
			return
		}
		c.Next()
	}
}
