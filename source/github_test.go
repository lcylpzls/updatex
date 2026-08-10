package source

import (
	"context"
	"errors"
	"fmt"
	testx "github.com/lcylpzls/testx"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/httpx"
	"github.com/lcylpzls/updatex"
)

// githubFixture 构造 GitHub API 模拟服务；响应体在请求时按服务地址生成。
func githubFixture(t *testing.T, token, shaName string, bodyFn func(serverURL string) string) *httptest.Server {
	t.Helper()
	assetName := "app_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
	shaHex := strings.Repeat("ab", 32)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(bodyFn(srv.URL)))
		case "/downloads/" + assetName:
			_, _ = w.Write([]byte("binary"))
		case "/downloads/" + shaName:
			_, _ = w.Write([]byte(shaHex + "  " + assetName + "\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return srv
}

// releaseBody 构造标准 Release 响应体。
func releaseBody(serverURL, shaName string) string {
	assetName := "app_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
	return fmt.Sprintf(`{"tag_name":"v1.1.0","published_at":"2026-08-09T00:00:00Z",`+
		`"body":"更新说明","assets":[`+
		`{"name":%q,"browser_download_url":%q,"size":3,"state":"uploaded"},`+
		`{"name":%q,"browser_download_url":%q,"size":64,"state":"uploaded"}]}`,
		assetName, serverURL+"/downloads/"+assetName, shaName, serverURL+"/downloads/"+shaName)
}

// TestNewGitHubSource 覆盖构造校验。
func TestNewGitHubSource(t *testing.T) {
	if _, err := NewGitHubSource(""); !errors.Is(err, updatex.ErrInvalidConfig) {
		t.Fatalf("空仓库应报错，实际：%v", err)
	}
	if _, err := NewGitHubSource("onlyowner"); !errors.Is(err, updatex.ErrInvalidConfig) {
		t.Fatalf("缺仓库名应报错，实际：%v", err)
	}
	if _, err := NewGitHubSource("o/r", WithGitHubToken(" ")); !errors.Is(err, updatex.ErrInvalidConfig) {
		t.Fatalf("空令牌应报错，实际：%v", err)
	}
	if _, err := NewGitHubSource("o/r", WithGitHubClient(nil)); !errors.Is(err, updatex.ErrInvalidConfig) {
		t.Fatalf("空客户端应报错，实际：%v", err)
	}
	if _, err := NewGitHubSource("o/r", withAPIBase("")); !errors.Is(err, updatex.ErrInvalidConfig) {
		t.Fatalf("空 API 基地址应报错，实际：%v", err)
	}
	s, err := NewGitHubSource("o/r", WithGitHubToken("tok"))
	if err != nil || s.owner != "o" || s.repo != "r" || s.token != "tok" {
		t.Fatalf("构造不符：%+v err=%v", s, err)
	}
}

// TestGitHubLatest 覆盖 GitHub 源成功与失败分支。
func TestGitHubLatest(t *testing.T) {
	ctx := context.Background()
	assetName := "app_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
	shaName := strings.TrimSuffix(assetName, ".zip") + ".sha256"
	shaHex := strings.Repeat("ab", 32)

	// 成功 + 令牌认证 + 去扩展名校验和命名。
	srv := githubFixture(t, "tok", shaName, func(u string) string { return releaseBody(u, shaName) })
	defer srv.Close()
	s, err := NewGitHubSource("o/r", WithGitHubToken("tok"), withAPIBase(srv.URL))
	testx.RequireNoError(t, err)

	m, err := s.Latest(ctx)
	if err != nil || m.Version != "1.1.0" || m.Notes != "更新说明" || m.PublishedAt.IsZero() {
		t.Fatalf("拉取失败：%+v err=%v", m, err)
	}
	asset := m.Platforms[runtime.GOOS+"_"+runtime.GOARCH]
	if asset.URL != srv.URL+"/downloads/"+assetName || asset.SHA256 != shaHex || asset.Size != 3 {
		t.Fatalf("资产映射不符：%+v", asset)
	}

	// 无令牌访问受保护仓库应失败。
	sNoToken, _ := NewGitHubSource("o/r", withAPIBase(srv.URL))
	if _, err := sNoToken.Latest(ctx); !errors.Is(err, updatex.ErrFetchFailed) {
		t.Fatalf("非 200 应报拉取失败，实际：%v", err)
	}

	// 空标签。
	srvEmpty := githubFixture(t, "", shaName, func(string) string { return `{"assets":[]}` })
	defer srvEmpty.Close()
	sEmpty, _ := NewGitHubSource("o/r", withAPIBase(srvEmpty.URL))
	if _, err := sEmpty.Latest(ctx); !errors.Is(err, updatex.ErrManifestInvalid) {
		t.Fatalf("空标签应报清单错误，实际：%v", err)
	}

	// 坏 JSON。
	srvBad := githubFixture(t, "", shaName, func(string) string { return "not-json" })
	defer srvBad.Close()
	sBad, _ := NewGitHubSource("o/r", withAPIBase(srvBad.URL))
	if _, err := sBad.Latest(ctx); !errx.Is(err, updatex.CodeManifestInvalid) {
		t.Fatalf("坏 JSON 应报清单错误，实际：%v", err)
	}

	// 无当前平台资产。
	srvNo := githubFixture(t, "", shaName, func(string) string {
		return `{"tag_name":"v1.1.0","assets":[{"name":"app_solaris_sparc.zip","state":"uploaded"}]}`
	})
	defer srvNo.Close()
	sNo, _ := NewGitHubSource("o/r", withAPIBase(srvNo.URL))
	if _, err := sNo.Latest(ctx); !errors.Is(err, updatex.ErrPlatformUnsupported) {
		t.Fatalf("缺平台资产应报错，实际：%v", err)
	}

	// 资产未上传。
	srvDraft := githubFixture(t, "", shaName, func(string) string {
		return fmt.Sprintf(`{"tag_name":"v1.1.0","assets":[{"name":%q,"state":"draft"}]}`,
			"app_"+runtime.GOOS+"_"+runtime.GOARCH+".zip")
	})
	defer srvDraft.Close()
	sDraft, _ := NewGitHubSource("o/r", withAPIBase(srvDraft.URL))
	if _, err := sDraft.Latest(ctx); !errors.Is(err, updatex.ErrPlatformUnsupported) {
		t.Fatalf("草稿资产应忽略，实际：%v", err)
	}

	// 发布时间非法：忽略错误，时间为零。
	srvTime := githubFixture(t, "", shaName, func(u string) string {
		return strings.Replace(releaseBody(u, shaName),
			`"published_at":"2026-08-09T00:00:00Z"`, `"published_at":"bad-time"`, 1)
	})
	defer srvTime.Close()
	sTime, _ := NewGitHubSource("o/r", withAPIBase(srvTime.URL))
	m2, err := sTime.Latest(ctx)
	if err != nil || !m2.PublishedAt.IsZero() {
		t.Fatalf("坏时间应置零：%+v err=%v", m2, err)
	}
}

// TestGitHubSHA256 覆盖校验和缺失与错误响应。
func TestGitHubSHA256(t *testing.T) {
	ctx := context.Background()
	assetName := "app_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
	shaName := assetName + ".sha256"

	// 校验和资产缺失。
	srv := githubFixture(t, "", shaName, func(u string) string {
		return fmt.Sprintf(`{"tag_name":"v1.1.0","assets":[`+
			`{"name":%q,"browser_download_url":%q,"size":3,"state":"uploaded"}]}`,
			assetName, u+"/downloads/"+assetName)
	})
	defer srv.Close()
	s, err := NewGitHubSource("o/r", withAPIBase(srv.URL))
	testx.RequireNoError(t, err)

	if _, err := s.Latest(ctx); !errors.Is(err, updatex.ErrManifestInvalid) {
		t.Fatalf("缺校验和应报清单错误，实际：%v", err)
	}

	// 校验和端点非 200。
	srvErr := githubFixture(t, "", shaName, func(u string) string {
		return fmt.Sprintf(`{"tag_name":"v1.1.0","assets":[`+
			`{"name":%q,"browser_download_url":%q,"size":3,"state":"uploaded"},`+
			`{"name":%q,"browser_download_url":%q,"size":64,"state":"uploaded"}]}`,
			assetName, u+"/downloads/"+assetName, shaName, u+"/missing-sha")
	})
	defer srvErr.Close()
	sErr, _ := NewGitHubSource("o/r", withAPIBase(srvErr.URL))
	if _, err := sErr.Latest(ctx); !errors.Is(err, updatex.ErrFetchFailed) {
		t.Fatalf("校验和端点非 200 应报拉取失败，实际：%v", err)
	}
}

// TestParseSHA256File 覆盖校验和解析分支。
func TestParseSHA256File(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	if got, err := parseSHA256File([]byte(strings.ToUpper(sha) + "  app.bin\n")); err != nil || got != sha {
		t.Fatalf("解析不符：%q err=%v", got, err)
	}
	if _, err := parseSHA256File([]byte("short")); !errors.Is(err, updatex.ErrManifestInvalid) {
		t.Fatalf("长度不足应报错，实际：%v", err)
	}
	if _, err := parseSHA256File([]byte(strings.Repeat("zz", 32))); !errors.Is(err, updatex.ErrManifestInvalid) {
		t.Fatalf("非法字符应报错，实际：%v", err)
	}
}

// TestGitHubClientError 覆盖请求错误透传。
func TestGitHubClientError(t *testing.T) {
	wantErr := errors.New("请求失败")
	s, err := NewGitHubSource("o/r", WithGitHubClient(&stubHTTPClient{err: wantErr}))
	testx.RequireNoError(t, err)

	if _, err := s.Latest(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("请求错误应透传，实际：%v", err)
	}
}

// TestGitHubDefaultClientFailure 覆盖默认客户端构造失败。
func TestGitHubDefaultClientFailure(t *testing.T) {
	orig := newDefaultClient
	newDefaultClient = func(protocol) (httpClient, error) { return nil, errors.New("构造失败") }
	defer func() { newDefaultClient = orig }()
	if _, err := NewGitHubSource("o/r"); err == nil {
		t.Fatal("客户端构造失败应报错")
	}
}

// TestGitHubReadErrors 覆盖清单读取与资产请求错误。
func TestGitHubReadErrors(t *testing.T) {
	ctx := context.Background()
	shaName := "app_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip.sha256"
	srv := githubFixture(t, "", shaName, func(u string) string { return releaseBody(u, shaName) })
	defer srv.Close()

	// API 响应体读取错误。
	apiErr := &stubHTTPClient{fn: func(context.Context, string, ...httpx.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK,
			Body: io.NopCloser(failReader{})}, nil
	}}
	s1, _ := NewGitHubSource("o/r", WithGitHubClient(apiErr))
	if _, err := s1.Latest(ctx); !errors.Is(err, updatex.ErrFetchFailed) {
		t.Fatalf("清单读取错误应报拉取失败，实际：%v", err)
	}

	// 校验和资产请求错误。
	shaErr := errors.New("资产请求失败")
	stub := &stubHTTPClient{fn: func(_ context.Context, url string, _ ...httpx.RequestOption) (*http.Response, error) {
		if strings.Contains(url, "releases/latest") {
			return http.Get(srv.URL + "/repos/o/r/releases/latest")
		}
		return nil, shaErr
	}}
	s2, _ := NewGitHubSource("o/r", WithGitHubClient(stub))
	if _, err := s2.Latest(ctx); !errors.Is(err, shaErr) {
		t.Fatalf("资产请求错误应透传，实际：%v", err)
	}

	// 校验和响应体读取错误。
	stub3 := &stubHTTPClient{fn: func(_ context.Context, url string, _ ...httpx.RequestOption) (*http.Response, error) {
		if strings.Contains(url, "releases/latest") {
			return http.Get(srv.URL + "/repos/o/r/releases/latest")
		}
		return &http.Response{StatusCode: http.StatusOK,
			Body: io.NopCloser(failReader{})}, nil
	}}
	s3, _ := NewGitHubSource("o/r", WithGitHubClient(stub3))
	if _, err := s3.Latest(ctx); !errors.Is(err, updatex.ErrFetchFailed) {
		t.Fatalf("校验和读取错误应报拉取失败，实际：%v", err)
	}
}
