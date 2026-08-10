// Package updateserver 提供基于 webx 的 HTTP/3 升级服务端库代码，
// 供命令与端到端测试共用。
package updateserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/updatex"
	"github.com/lcylpzls/webx/v2"
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
}

// NewServer 构造未启动的 webx HTTP/3 升级服务。
func NewServer(cfg Config, certFile, keyFile, listen string, logger logx.Logger) (*webx.Server, error) {
	if cfg.Version == "" || len(cfg.Asset) == 0 {
		return nil, errors.New("版本与资产不能为空")
	}
	sum := sha256.Sum256(cfg.Asset)
	shaHex := hex.EncodeToString(sum[:])
	key := runtime.GOOS + "_" + runtime.GOARCH
	s := webx.NewServer(webx.Config{
		TLSCertFile:     certFile,
		TLSKeyFile:      keyFile,
		ShutdownTimeout: 5 * time.Second,
	}, logger)
	s.UseHttp3Listen(listen)
	s.RegisterRoutes([]webx.Route{
		{
			Method: http.MethodGet,
			Path:   "/update.json",
			Handler: func(c *webx.Context) {
				if cfg.OnRequest != nil {
					cfg.OnRequest(c.Request().Proto)
				}
				m := &updatex.Manifest{
					Version:     cfg.Version,
					Notes:       cfg.Notes,
					PublishedAt: time.Now().UTC(),
					Platforms: map[string]updatex.Asset{
						key: {
							URL:    "https://" + c.Request().Host + "/download",
							SHA256: shaHex,
							Size:   int64(len(cfg.Asset)),
						},
					},
				}
				_ = c.JSON(http.StatusOK, m)
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/download",
			Handler: func(c *webx.Context) {
				if cfg.OnRequest != nil {
					cfg.OnRequest(c.Request().Proto)
				}
				c.SetHeaderCanonical("Content-Type", "application/octet-stream")
				_, _ = c.Writer().Write(cfg.Asset)
			},
		},
	})
	return s, nil
}

// StartAndWait 启动服务并等待监听就绪，返回服务实例与监听地址。
func StartAndWait(ctx context.Context, cfg Config, certFile, keyFile, listen string, logger logx.Logger) (*webx.Server, string, error) {
	s, err := NewServer(cfg, certFile, keyFile, listen, logger)
	if err != nil {
		return nil, "", err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start() }()
	for i := 0; i < 500; i++ {
		if addr := s.ListenerAddr(); addr != "" {
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
