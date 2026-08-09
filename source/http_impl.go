package source

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lcylpzls/httpx"
	_ "github.com/lcylpzls/httpx/http3" // 注册 HTTP/3 传输
	"github.com/lcylpzls/updatex"
)

// protocol 传输协议选择。
type protocol int

const (
	protocolAuto protocol = iota
	protocolHTTP2
	protocolHTTP3
)

// httpClient 最小客户端接口（httpx 实现）。
type httpClient interface {
	Get(ctx context.Context, url string, opts ...httpx.RequestOption) (*http.Response, error)
}

// newDefaultClient 可替换的默认客户端构造（测试注入用）。
var newDefaultClient = func(p protocol) (httpClient, error) {
	opts := []httpx.Option{httpx.WithTimeout(30 * time.Second)}
	switch p {
	case protocolHTTP2:
		opts = append(opts, httpx.WithProtocol(httpx.ProtocolHTTP2))
	case protocolHTTP3:
		opts = append(opts, httpx.WithProtocol(httpx.ProtocolHTTP3))
	}
	return httpx.New(opts...)
}

// NewHTTPSource 构造 HTTP 清单源。
func NewHTTPSource(url string, allowHTTP bool, opts ...HTTPSourceOption) (*HTTPSource, error) {
	if url == "" {
		return nil, updatex.ErrInvalidConfig
	}
	if !allowHTTP && !strings.HasPrefix(url, "https://") {
		return nil, updatex.ErrInvalidConfig
	}
	s := &HTTPSource{url: url, allowHTTP: allowHTTP, protocol: protocolAuto}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	if s.client == nil {
		client, err := newDefaultClient(s.protocol)
		if err != nil {
			return nil, err
		}
		s.client = client
	}
	return s, nil
}

// readLimited 读取受限大小的响应体。
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, updatex.ErrFetchFailed
	}
	if int64(len(data)) > limit {
		return nil, updatex.ErrFetchFailed
	}
	return data, nil
}
