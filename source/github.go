package source

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/httpx"
	"github.com/lcylpzls/updatex"
)

// GitHubSource GitHub Releases 发布源：拉取最新 Release 并映射为清单。
// 资产命名约定：<名称>_<GOOS>_<GOARCH>[.扩展名]，
// 校验和文件约定：<资产名>.sha256 或 <去扩展名>.sha256（内容为 64 位十六进制）。
type GitHubSource struct {
	owner   string
	repo    string
	token   string
	client  httpClient
	apiBase string
}

// GitHubOption GitHub 源配置项。
type GitHubOption func(*GitHubSource) error

// WithGitHubToken 注入私有仓库访问令牌。
func WithGitHubToken(token string) GitHubOption {
	return func(s *GitHubSource) error {
		if strings.TrimSpace(token) == "" {
			return updatex.ErrInvalidConfig
		}
		s.token = token
		return nil
	}
}

// WithGitHubClient 注入自定义客户端（*httpx.Client 可直接传入）。
func WithGitHubClient(client httpClient) GitHubOption {
	return func(s *GitHubSource) error {
		if client == nil {
			return updatex.ErrInvalidConfig
		}
		s.client = client
		return nil
	}
}

// withAPIBase 覆盖 API 基地址（测试注入用）。
func withAPIBase(base string) GitHubOption {
	return func(s *GitHubSource) error {
		if strings.TrimSpace(base) == "" {
			return updatex.ErrInvalidConfig
		}
		s.apiBase = base
		return nil
	}
}

// NewGitHubSource 构造 GitHub Releases 源（仓库格式 owner/repo）。
func NewGitHubSource(repo string, opts ...GitHubOption) (*GitHubSource, error) {
	parts := strings.Split(strings.TrimSpace(repo), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, updatex.ErrInvalidConfig
	}
	s := &GitHubSource{owner: parts[0], repo: parts[1], apiBase: "https://api.github.com"}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	if s.client == nil {
		client, err := newDefaultClient(protocolAuto)
		if err != nil {
			return nil, err
		}
		s.client = client
	}
	return s, nil
}

// githubRelease GitHub Releases API 响应的最小子集。
type githubRelease struct {
	TagName     string        `json:"tag_name"`
	PublishedAt string        `json:"published_at"`
	Body        string        `json:"body"`
	Assets      []githubAsset `json:"assets"`
}

// githubAsset Release 资产。
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	State              string `json:"state"`
}

// Latest 拉取最新 Release 并构建当前平台清单。
func (s *GitHubSource) Latest(ctx context.Context) (*updatex.Manifest, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", s.apiBase, s.owner, s.repo)
	var opts []httpx.RequestOption
	if s.token != "" {
		opts = append(opts, httpx.WithBearer(s.token))
	}
	resp, err := s.client.Get(ctx, url, opts...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errx.Wrap(updatex.ErrFetchFailed, errx.KindUnavailable,
			updatex.CodeFetchFailed, fmt.Sprintf("GitHub API 返回非 200：%d", resp.StatusCode))
	}
	data, err := readLimited(resp.Body, 2<<20)
	if err != nil {
		return nil, err
	}
	var rel githubRelease
	if err := json.Unmarshal(data, &rel); err != nil {
		return nil, errx.Wrap(err, errx.KindInvalid, updatex.CodeManifestInvalid, "GitHub Release 解析失败")
	}
	if rel.TagName == "" {
		return nil, updatex.ErrManifestInvalid
	}
	asset, ok := matchGitHubAsset(rel.Assets)
	if !ok {
		return nil, updatex.ErrPlatformUnsupported
	}
	sha, err := s.fetchSHA256(ctx, rel.Assets, asset.Name)
	if err != nil {
		return nil, err
	}
	var publishedAt time.Time
	if rel.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, rel.PublishedAt); err == nil {
			publishedAt = t
		}
	}
	key := runtime.GOOS + "_" + runtime.GOARCH
	return &updatex.Manifest{
		Version:     strings.TrimPrefix(rel.TagName, "v"),
		PublishedAt: publishedAt,
		Notes:       rel.Body,
		Platforms: map[string]updatex.Asset{
			key: {URL: asset.BrowserDownloadURL, SHA256: sha, Size: asset.Size},
		},
	}, nil
}

// matchGitHubAsset 按当前平台匹配已上传资产。
func matchGitHubAsset(assets []githubAsset) (githubAsset, bool) {
	suffix := "_" + runtime.GOOS + "_" + runtime.GOARCH
	for _, a := range assets {
		if a.Name == "" || a.State != "uploaded" || strings.HasSuffix(a.Name, ".sha256") {
			continue
		}
		if strings.HasSuffix(a.Name, suffix) || strings.Contains(a.Name, suffix+".") {
			return a, true
		}
	}
	return githubAsset{}, false
}

// fetchSHA256 拉取并解析资产对应的校验和文件。
func (s *GitHubSource) fetchSHA256(ctx context.Context, assets []githubAsset, assetName string) (string, error) {
	candidates := []string{assetName + ".sha256"}
	if i := strings.LastIndex(assetName, "."); i > 0 {
		candidates = append(candidates, assetName[:i]+".sha256")
	}
	for _, name := range candidates {
		for _, a := range assets {
			if a.Name != name || a.State != "uploaded" {
				continue
			}
			data, err := s.fetchAssetBytes(ctx, a.BrowserDownloadURL, 4<<10)
			if err != nil {
				return "", err
			}
			return parseSHA256File(data)
		}
	}
	return "", updatex.ErrManifestInvalid
}

// fetchAssetBytes 拉取资产内容并限制大小。
func (s *GitHubSource) fetchAssetBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := s.client.Get(ctx, url)
	if err != nil {
		return nil, errx.Wrap(err, errx.KindUnavailable, updatex.CodeFetchFailed, "资产请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errx.Wrap(updatex.ErrFetchFailed, errx.KindUnavailable,
			updatex.CodeFetchFailed, fmt.Sprintf("资产端点返回非 200：%d", resp.StatusCode))
	}
	data, err := readLimited(resp.Body, limit)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// parseSHA256File 解析校验和文件：取首个空白分隔的 64 位十六进制。
func parseSHA256File(data []byte) (string, error) {
	s := strings.ToLower(strings.TrimSpace(string(data)))
	if i := strings.IndexAny(s, " \t\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) != 64 {
		return "", updatex.ErrManifestInvalid
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", updatex.ErrManifestInvalid
	}
	return s, nil
}
