# updatex API 定版

> 版本：v1.1.1 · 已实现签名与代码一致。

## 1. 包结构

```go
updatex          // 根包：Check/Apply/Bootstrap/Config/UpdateInfo/错误
updatex/source   // VersionSource 接口 + HTTP/GitHub 实现
```

## 2. 核心类型

### 2.1 Config

```go
type Config struct {
	Source           VersionSource
	CurrentVersion   string
	ExecutablePath   string
	VerifyPublicKey  []byte
	MaxDownloadBytes int64
	AllowHTTP        bool
	Logger           logx.Logger
	Metrics          Metrics
	HTTPClient       *httpx.Client
}

```

### 2.2 UpdateInfo / Asset

```go
type UpdateInfo struct {
	HasUpdate  bool
	Version    string
	Notes      string
	Asset      Asset
	RestartRequired bool // Windows 延迟替换时为 true
}

type Asset struct {
	URL    string
	SHA256 string
	Size   int64
}
```

### 2.3 VersionSource

```go
type VersionSource interface {
	// Latest 返回当前平台可用的最新发布清单（原始字节 + 版本）。
	Latest(ctx context.Context) (*Manifest, error)
}
```

### 2.4 Manifest（导出，供自定义源实现）

```go
type Manifest struct {
	Version     string
	PublishedAt time.Time
	Notes       string
	Platforms   map[string]Asset
	Signature   string
}

func ParseManifest(data []byte) (*Manifest, error)
func (m *Manifest) AssetFor(goos, goarch string) (Asset, error)
func (m *Manifest) VerifySignature(publicKey []byte) error
```

## 3. 根包函数

```go
func New(cfg Config) (*Updater, error)
func (u *Updater) Check(ctx context.Context) (*UpdateInfo, error)
func (u *Updater) Apply(ctx context.Context) (*UpdateInfo, error)
func (u *Updater) ApplyAndRestart(ctx context.Context, restart func() error) (*UpdateInfo, error)
func Bootstrap(ctx context.Context, executablePath string) error
```

语义：

- `New`：配置校验失败返回 `ErrInvalidConfig`；
- `Check`：仅拉取与比较，不下载；
- `Apply`：自包含完整流程（重新拉取 → 比较 → 下载 → 校验 → 替换），
  不依赖 `Check` 前置；`RestartRequired=true` 时由业务重启；
- `ApplyAndRestart`：Apply 后若需重启则调用 `restart`（非阻塞，
  由业务决定延迟退出）；
- `Bootstrap`：处理 Windows `.pending` 标记；Unix 恒返回 nil。

## 4. source 子包

```go
// HTTP 清单源（v0.1.0）：默认基于 httpx（支持 HTTP/1/2/3）
func NewHTTPSource(url string, allowHTTP bool, opts ...HTTPSourceOption) (*HTTPSource, error)
func WithHTTP3(enable bool) HTTPSourceOption   // 切换 HTTP/3 传输
func WithHTTP2(enable bool) HTTPSourceOption   // 切换 HTTP/2 传输
func WithHTTPClient(client httpClient) HTTPSourceOption    // 注入自定义客户端（*httpx.Client 可直接传入）

// GitHub Releases 源
func NewGitHubSource(repo string, opts ...GitHubOption) (*GitHubSource, error)
func WithGitHubToken(token string) GitHubOption
func WithGitHubClient(client httpClient) GitHubOption
```

GitHub 资产命名约定：`<名称>_<GOOS>_<GOARCH>[.扩展名]`；
校验和文件约定：`<资产名>.sha256` 或 `<去扩展名>.sha256`
（内容为 64 位十六进制，可带文件名后缀）。

### 5.1 签名语义

- 签名载荷为 `Signature` 置空后的规范化 JSON（字段顺序与
  map 排序由 `encoding/json` 保证）；
- 配置公钥但清单无签名 → `ErrSignatureInvalid`；
- 未配置公钥 → 跳过签名校验（文档明确降级风险）。

## 5. 错误值清单

```go
var (
	ErrInvalidConfig      = errx.New(errx.KindInvalid, CodeInvalidConfig, "配置非法")
	ErrInvalidVersion     = errx.New(errx.KindInvalid, CodeInvalidVersion, "版本号非法")
	ErrManifestInvalid    = errx.New(errx.KindInvalid, CodeManifestInvalid, "发布清单非法")
	ErrFetchFailed        = errx.New(errx.KindUnavailable, CodeFetchFailed, "拉取发布清单失败")
	ErrDownloadFailed     = errx.New(errx.KindUnavailable, CodeDownloadFailed, "下载更新资产失败")
	ErrChecksumMismatch   = errx.New(errx.KindDataLoss, CodeChecksumMismatch, "SHA256 校验失败")
	ErrSignatureInvalid   = errx.New(errx.KindForbidden, CodeSignatureInvalid, "签名无效")
	ErrDowngrade          = errx.New(errx.KindConflict, CodeDowngrade, "拒绝版本回退")
	ErrPlatformUnsupported = errx.New(errx.KindNotFound, CodePlatformUnsupported, "当前平台无可用资产")
	ErrReplaceFailed      = errx.New(errx.KindUnavailable, CodeReplaceFailed, "替换可执行文件失败")
	ErrRollbackFailed     = errx.New(errx.KindUnavailable, CodeRollbackFailed, "回滚失败")
)
```

## 6. 完整示例

```go
src := source.NewHTTPSource("https://cdn.example.com/update.json", false)
u, err := updatex.New(updatex.Config{
	Source:         src,
	CurrentVersion: "1.0.0",
})

info, err := u.Check(ctx)
if info.HasUpdate {
	_, err = u.ApplyAndRestart(ctx, func() error {
		// 业务重启逻辑（如退出并交由守护进程拉起）。
		return nil
	})
}
```
