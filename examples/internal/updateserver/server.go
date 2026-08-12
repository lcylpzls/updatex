// Package updateserver 提供基于 webx + updatex 的 HTTP/3 升级服务端示例库，
// 供命令与端到端测试共用。
package updateserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/updatex"
	"github.com/lcylpzls/webx"
)

// Config 升级服务端配置。
type Config struct {
	// Version 发布版本（语义化版本）。
	Version string
	// Notes 变更说明。
	Notes string
	// Asset 升级资产内容。
	Asset []byte
	// OnRequest 每个请求回调（协议验证用，可为 nil）。
	OnRequest func(proto string)
	// AdminToken 管理路由令牌（可选）。
	AdminToken string
}

// Server 包装 webx 与 updatex 服务端。
type Server struct {
	webx      *webx.Server
	updater   *updatex.Server
	assetsDir string
	manifest  string
}

// NewServer 构造更新服务端：生成资产目录与清单文件，并注册到 webx。
func NewServer(cfg Config, certFile, keyFile, listen string, logger logx.Logger) (*Server, error) {
	if cfg.Version == "" || len(cfg.Asset) == 0 {
		return nil, errors.New("版本与资产不能为空")
	}
	dir, err := os.MkdirTemp("", "updatex-server-*")
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Server, error) {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	sum := sha256.Sum256(cfg.Asset)
	key := runtime.GOOS + "_" + runtime.GOARCH
	manifest := updatex.Manifest{
		Version:     cfg.Version,
		PublishedAt: time.Now().UTC(),
		Notes:       cfg.Notes,
		Platforms: map[string]updatex.Asset{
			key: {
				URL:    "https://HOST_PLACEHOLDER/updates/assets/app.bin",
				SHA256: hex.EncodeToString(sum[:]),
				Size:   int64(len(cfg.Asset)),
			},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.bin"), cfg.Asset, 0o600); err != nil {
		return fail(err)
	}
	opts := []updatex.ServerOption{}
	if cfg.AdminToken != "" {
		opts = append(opts, updatex.WithAdminToken(cfg.AdminToken))
	}
	updater, err := updatex.NewServer(dir, opts...)
	if err != nil {
		return fail(err)
	}
	ws := webx.NewServer(webx.Config{
		TLSCertFile:     certFile,
		TLSKeyFile:      keyFile,
		ShutdownTimeout: 5 * time.Second,
	}, logger)
	ws.UseHttp3Listen(listen)
	if cfg.OnRequest != nil {
		ws.UseGlobalMiddleware(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				cfg.OnRequest(r.Proto)
				next.ServeHTTP(w, r)
			})
		})
	}
	if err := updater.RegisterWebx(ws); err != nil {
		return fail(err)
	}
	return &Server{
		webx:      ws,
		updater:   updater,
		assetsDir: dir,
		manifest:  filepath.Join(dir, "manifest.json"),
	}, nil
}

// SetBaseURL 将清单中的资产地址替换为实际服务基地址并热更新清单。
func (s *Server) SetBaseURL(base string) error {
	data, err := os.ReadFile(s.manifest)
	if err != nil {
		return err
	}
	text := strings.ReplaceAll(string(data), "https://HOST_PLACEHOLDER", base)
	if err := os.WriteFile(s.manifest, []byte(text), 0o600); err != nil {
		return err
	}
	return s.updater.Reload()
}

// Start 启动服务（阻塞）。
func (s *Server) Start() error {
	return s.webx.Start()
}

// Stop 优雅关闭服务并清理临时资产目录。
func (s *Server) Stop(ctx context.Context) error {
	err := s.webx.Stop(ctx)
	_ = os.RemoveAll(s.assetsDir)
	return err
}

// ListenerAddr 返回监听地址（port 0 动态端口时可用）。
func (s *Server) ListenerAddr() string {
	return s.webx.ListenerAddr()
}

// StartAndWait 启动服务并等待监听就绪，返回服务实例与监听地址。
func StartAndWait(ctx context.Context, cfg Config, certFile, keyFile, listen string, logger logx.Logger) (*Server, string, error) {
	s, err := NewServer(cfg, certFile, keyFile, listen, logger)
	if err != nil {
		return nil, "", err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start() }()
	for i := 0; i < 500; i++ {
		if addr := s.ListenerAddr(); addr != "" {
			if err := s.SetBaseURL("https://" + addr); err != nil {
				return nil, "", err
			}
			return s, addr, nil
		}
		select {
		case err := <-errCh:
			return nil, "", err
		case <-ctx.Done():
			return nil, "", ctx.Err()
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	return nil, "", errors.New("服务启动超时")
}
