// Package source 提供发布源实现（HTTP 清单源，唯一通道为自建更新服务端）。
package source

import (
	"context"

	"github.com/lcylpzls/updatex/internal/core"
)

// HTTPSource HTTP 清单源：从固定 URL 拉取发布清单。
type HTTPSource struct {
	url      string
	client   httpClient
	protocol protocol
}

// HTTPSourceOption HTTP 源配置项。
type HTTPSourceOption func(*HTTPSource) error

// WithHTTP3 切换 HTTP/3 传输。
func WithHTTP3(enable bool) HTTPSourceOption {
	return func(s *HTTPSource) error {
		if enable {
			s.protocol = protocolHTTP3
		}
		return nil
	}
}

// WithHTTP2 切换 HTTP/2 传输。
func WithHTTP2(enable bool) HTTPSourceOption {
	return func(s *HTTPSource) error {
		if enable {
			s.protocol = protocolHTTP2
		}
		return nil
	}
}

// WithHTTPClient 注入自定义 httpx 客户端。
func WithHTTPClient(client httpClient) HTTPSourceOption {
	return func(s *HTTPSource) error {
		if client == nil {
			return core.ErrInvalidConfig
		}
		s.client = client
		return nil
	}
}

// Latest 拉取并解析最新发布清单。
func (s *HTTPSource) Latest(ctx context.Context) (*core.Manifest, error) {
	resp, err := s.client.Get(ctx, s.url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := readLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, err
	}
	return core.ParseManifest(data)
}
