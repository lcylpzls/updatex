# updatex API 定版

> 版本：v1.4.0 · 已实现签名与代码一致。

## 1. 包结构

```go
updatex          // 公开门面：NewServer / NewClient + 类型与错误码
updatex/internal/core    // 共享引擎（不可导入）
updatex/internal/server  // 服务端实现（不可导入）
updatex/internal/client  // 客户端实现（不可导入）
updatex/internal/source  // HTTP 清单源（不可导入）
```

根包是唯一对外入口，用户只 import `github.com/lcylpzls/updatex`。

## 2. 服务端 API

```go
func NewServer(assetsDir string, opts ...ServerOption) (*Server, error)
func WithAdminToken(token string) ServerOption   // 为空不挂载管理路由
func WithManifestPath(path string) ServerOption  // 默认 assetsDir/manifest.json
func WithManifestURL(path string) ServerOption   // 默认 /updates/manifest.json
func WithAssetsURL(path string) ServerOption     // 默认 /updates/assets/

type Server struct{ /* 不可直接构造 */ }
func (s *Server) Reload() error                                  // 热更新清单
func (s *Server) HandleAdmin(method, pattern string, h webx.HandlerFunc) error
func (s *Server) RegisterWebx(ws *webx.Server) error             // 必须在 webx.Start 前调用
```

语义：

- `NewServer`：校验资产目录并解析清单，失败返回对应 errx 错误码；
- `Reload`：重新读取清单文件并热更新，无需重新注册路由；
- `HandleAdmin`：仅配置 `WithAdminToken` 后可调用，`method` 支持
  GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS，注册的路由统一挂到
  `/updates/admin` 分组并强制 `X-Api-Token` 鉴权（常量时间比较）；
- `RegisterWebx`：注册清单路由、资产静态服务与管理分组；
  可对多个 webx 实例重复调用。

默认路由：

| 路由 | 方法 | 说明 |
| --- | --- | --- |
| `/updates/manifest.json` | GET | 发布清单（Reload 后即时生效） |
| `/updates/assets/` | GET/HEAD | 资产静态服务（路径穿越防护） |
| `/updates/admin/status` | GET | 管理状态，需 `X-Api-Token` |

## 3. 客户端 API

```go
func NewClient(cfg ClientConfig) (*Client, error)

type Client struct{ /* 不可直接构造 */ }
func (c *Client) Run(ctx context.Context) (*Result, error)

type ClientConfig struct {
	ManifestURL     string               // 与 Source 二选一
	Source          VersionSource        // 与 ManifestURL 二选一
	CurrentVersion  string               // 必填
	ExecutablePath  string               // 默认 os.Executable
	AfterUpdate     AfterUpdateAction    // 必填
	RestartCommand  string               // AfterUpdate=Restart 时必填
	VerifyPublicKey []byte               // 可选，Ed25519 公钥
	AllowHTTP       bool                 // 默认 false，仅测试/内网
	Logger          logx.Logger          // 可选
	Protocol        Protocol             // 默认 ProtocolAuto
	InsecureTLS     bool                 // 默认 false，仅测试
	HTTPClient      *httpx.Client        // 可选
	MaxDownloadBytes int64               // 默认 512 MiB
	Metrics         Metrics              // 可选
	TraceHook       TraceHook            // 可选
}

type Result struct {
	Updated         bool
	Version         string
	RestartRequired bool   // Windows 延迟替换时为 true
	Notes           string
}

type AfterUpdateAction int
const (
	AfterUpdateContinue AfterUpdateAction = iota // 继续运行，返回结果
	AfterUpdateExit                              // 更新成功后 os.Exit(0)
	AfterUpdateRestart                           // 异步启动命令后 os.Exit(0)
)

type Protocol = httpx.Protocol
const (
	ProtocolAuto  = httpx.ProtocolAuto  // HTTP/1.1 + HTTP/2（ALPN）
	ProtocolHTTP1 = httpx.ProtocolHTTP1
	ProtocolHTTP2 = httpx.ProtocolHTTP2
	ProtocolHTTP3 = httpx.ProtocolHTTP3 // 传输注册由包内部完成
)
```

`Run` 语义：

1. 先执行 Windows 启动时替换（Bootstrap），完成上次未完成的替换；
2. 拉取清单并版本比较，无更新直接返回 `Result{Updated:false}`；
3. 有更新则下载、SHA256 校验（可选 Ed25519 验签）、替换；
4. 按 `AfterUpdate` 执行：Continue 返回结果；Exit 直接退出；
   Restart 异步启动用户命令（Windows `cmd /C`，Unix `sh -c`），
   启动失败返回错误、进程不退出，成功后退出。

## 4. 共享类型

```go
type VersionSource interface {
	Latest(ctx context.Context) (*Manifest, error)
}

type Manifest struct {
	Version     string
	PublishedAt time.Time
	Notes       string
	Platforms   map[string]Asset // 键为 GOOS_GOARCH
	Signature   string
}
func ParseManifest(data []byte) (*Manifest, error)
func (m *Manifest) AssetFor(goos, goarch string) (Asset, error)
func (m *Manifest) VerifySignature(publicKey []byte) error

type Asset struct {
	URL    string
	SHA256 string
	Size   int64
}
```

## 5. 错误值清单

```go
const (
	CodeInvalidConfig       = "updatex_invalid_config"
	CodeInvalidVersion      = "updatex_invalid_version"
	CodeManifestInvalid     = "updatex_manifest_invalid"
	CodeFetchFailed         = "updatex_fetch_failed"
	CodeDownloadFailed      = "updatex_download_failed"
	CodeChecksumMismatch    = "updatex_checksum_mismatch"
	CodeSignatureInvalid    = "updatex_signature_invalid"
	CodeDowngrade           = "updatex_downgrade"
	CodePlatformUnsupported = "updatex_platform_unsupported"
	CodeReplaceFailed       = "updatex_replace_failed"
	CodeRollbackFailed      = "updatex_rollback_failed"
)

var (
	ErrInvalidConfig       = ...
	ErrInvalidVersion      = ...
	ErrManifestInvalid     = ...
	ErrFetchFailed         = ...
	ErrDownloadFailed      = ...
	ErrChecksumMismatch    = ...
	ErrSignatureInvalid    = ...
	ErrDowngrade           = ...
	ErrPlatformUnsupported = ...
	ErrReplaceFailed       = ...
	ErrRollbackFailed      = ...
)
```

## 6. 完整示例

```go
// 服务端
s, _ := updatex.NewServer("./assets", updatex.WithAdminToken("令牌"))
s.HandleAdmin("POST", "/reload", func(c *webx.Context) {
	_ = s.Reload()
	c.JSON(http.StatusOK, map[string]bool{"ok": true})
})
_ = s.RegisterWebx(ws)

// 客户端（main 最前）
c, _ := updatex.NewClient(updatex.ClientConfig{
	ManifestURL:    "https://updates.example.com/updates/manifest.json",
	CurrentVersion: "1.0.0",
	AfterUpdate:    updatex.AfterUpdateContinue,
	Logger:         logger,
})
res, err := c.Run(ctx)
if err == nil && res.Updated {
	os.Exit(0) // 由业务决定重启时机
}
```
