package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lcylpzls/httpx"
	"github.com/lcylpzls/updatex"
)

var goodManifest = `{"version":"1.1.0","platforms":{"linux_amd64":` +
	`{"url":"https://x/a","sha256":"` + strings.Repeat("ab", 32) + `","size":1}}}`

// stubHTTPClient 可配置的客户端桩。
type stubHTTPClient struct {
	resp *http.Response
	err  error
	fn   func(context.Context, string, ...httpx.RequestOption) (*http.Response, error)
}

func (c *stubHTTPClient) Get(ctx context.Context, url string, opts ...httpx.RequestOption) (*http.Response, error) {
	if c.fn != nil {
		return c.fn(ctx, url, opts...)
	}
	return c.resp, c.err
}

// failReader 读取即失败的读取器。
type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errors.New("读取故障") }

// TestNewHTTPSource 覆盖构造校验。
func TestNewHTTPSource(t *testing.T) {
	if _, err := NewHTTPSource("", true); !errors.Is(err, updatex.ErrInvalidConfig) {
		t.Fatalf("空 URL 应报错，实际：%v", err)
	}
	if _, err := NewHTTPSource("http://x/update.json", false); !errors.Is(err, updatex.ErrInvalidConfig) {
		t.Fatalf("明文 HTTP 应报错，实际：%v", err)
	}
	if _, err := NewHTTPSource("https://x/update.json", false); err != nil {
		t.Fatal(err)
	}
	if _, err := NewHTTPSource("http://x/update.json", true, WithHTTPClient(nil)); !errors.Is(err, updatex.ErrInvalidConfig) {
		t.Fatalf("空客户端应报错，实际：%v", err)
	}
	s, err := NewHTTPSource("http://x/update.json", true, WithHTTP3(true))
	if err != nil || s.protocol != protocolHTTP3 {
		t.Fatalf("HTTP/3 选项应生效：%v", err)
	}
	s2, err := NewHTTPSource("http://x/update.json", true, WithHTTP2(true))
	if err != nil || s2.protocol != protocolHTTP2 {
		t.Fatalf("HTTP/2 选项应生效：%v", err)
	}
	// 注入自定义客户端。
	custom := &stubHTTPClient{}
	s3, err := NewHTTPSource("http://x/update.json", true, WithHTTPClient(custom))
	if err != nil || s3.client != custom {
		t.Fatalf("自定义客户端应生效：%v", err)
	}
	// 默认客户端构造失败。
	orig := newDefaultClient
	newDefaultClient = func(protocol) (httpClient, error) { return nil, errors.New("构造失败") }
	defer func() { newDefaultClient = orig }()
	if _, err := NewHTTPSource("https://x/update.json", false); err == nil {
		t.Fatal("客户端构造失败应报错")
	}
}

// TestLatest 覆盖清单拉取分支。
func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/update.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(goodManifest))
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
		case "/bad":
			_, _ = w.Write([]byte("not-json"))
		case "/huge":
			_, _ = w.Write(make([]byte, 1<<20+10))
		case "/truncated":
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("short"))
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	s, err := NewHTTPSource(srv.URL+"/update.json", true)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Latest(ctx)
	if err != nil || m.Version != "1.1.0" {
		t.Fatalf("拉取失败：%+v err=%v", m, err)
	}
	sErr, _ := NewHTTPSource(srv.URL+"/error", true)
	if _, err := sErr.Latest(ctx); err == nil {
		t.Fatal("非 200 应报错")
	}
	sBad, _ := NewHTTPSource(srv.URL+"/bad", true)
	if _, err := sBad.Latest(ctx); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
	sHuge, _ := NewHTTPSource(srv.URL+"/huge", true)
	if _, err := sHuge.Latest(ctx); !errors.Is(err, updatex.ErrFetchFailed) {
		t.Fatalf("超限应报拉取失败，实际：%v", err)
	}
	sTrunc, _ := NewHTTPSource(srv.URL+"/truncated", true)
	if _, err := sTrunc.Latest(ctx); !errors.Is(err, updatex.ErrFetchFailed) {
		t.Fatalf("读取错误应报拉取失败，实际：%v", err)
	}
	// 请求错误。
	wantErr := errors.New("请求失败")
	badClient := &stubHTTPClient{err: wantErr}
	sConn, _ := NewHTTPSource("https://x/update.json", false, WithHTTPClient(badClient))
	if _, err := sConn.Latest(ctx); !errors.Is(err, wantErr) {
		t.Fatalf("请求错误应透传，实际：%v", err)
	}
	// 响应体读取错误。
	if _, err := readLimited(failReader{}, 10); !errors.Is(err, updatex.ErrFetchFailed) {
		t.Fatalf("读取错误应报拉取失败，实际：%v", err)
	}
}

// TestReadLimited 覆盖响应体大小上限分支。
func TestReadLimited(t *testing.T) {
	if _, err := readLimited(strings.NewReader(strings.Repeat("x", 11)), 10); !errors.Is(err, updatex.ErrFetchFailed) {
		t.Fatalf("超限应报拉取失败，实际：%v", err)
	}
	data, err := readLimited(strings.NewReader("abc"), 10)
	if err != nil || string(data) != "abc" {
		t.Fatalf("正常读取应成功：%q err=%v", data, err)
	}
}
