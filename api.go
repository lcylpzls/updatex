package updatex

import (
	"github.com/lcylpzls/httpx"
	"github.com/lcylpzls/updatex/internal/client"
	"github.com/lcylpzls/updatex/internal/core"
	"github.com/lcylpzls/updatex/internal/server"
)

// 错误码统一以 updatex_ 为前缀。
const (
	CodeInvalidConfig       = core.CodeInvalidConfig
	CodeInvalidVersion      = core.CodeInvalidVersion
	CodeManifestInvalid     = core.CodeManifestInvalid
	CodeFetchFailed         = core.CodeFetchFailed
	CodeDownloadFailed      = core.CodeDownloadFailed
	CodeChecksumMismatch    = core.CodeChecksumMismatch
	CodeSignatureInvalid    = core.CodeSignatureInvalid
	CodeDowngrade           = core.CodeDowngrade
	CodePlatformUnsupported = core.CodePlatformUnsupported
	CodeReplaceFailed       = core.CodeReplaceFailed
	CodeRollbackFailed      = core.CodeRollbackFailed
)

// 预定义错误值，可用 errx.Is / errors.Is 判断。
var (
	ErrInvalidConfig       = core.ErrInvalidConfig
	ErrInvalidVersion      = core.ErrInvalidVersion
	ErrManifestInvalid     = core.ErrManifestInvalid
	ErrFetchFailed         = core.ErrFetchFailed
	ErrDownloadFailed      = core.ErrDownloadFailed
	ErrChecksumMismatch    = core.ErrChecksumMismatch
	ErrSignatureInvalid    = core.ErrSignatureInvalid
	ErrDowngrade           = core.ErrDowngrade
	ErrPlatformUnsupported = core.ErrPlatformUnsupported
	ErrReplaceFailed       = core.ErrReplaceFailed
	ErrRollbackFailed      = core.ErrRollbackFailed
)

// 公开类型：服务端、客户端与共享契约。
type (
	VersionSource     = core.VersionSource
	Metrics           = core.Metrics
	TraceAttr         = core.TraceAttr
	TraceHook         = core.TraceHook
	Manifest          = core.Manifest
	Asset             = core.Asset
	Server            = server.Server
	ServerOption      = server.ServerOption
	Client            = client.Client
	ClientConfig      = client.Config
	Result            = client.Result
	AfterUpdateAction = client.AfterUpdateAction
	Protocol          = httpx.Protocol
)

// 传输协议常量（默认自动 HTTP/1.1 + HTTP/2）。
const (
	ProtocolAuto  = httpx.ProtocolAuto
	ProtocolHTTP1 = httpx.ProtocolHTTP1
	ProtocolHTTP2 = httpx.ProtocolHTTP2
	ProtocolHTTP3 = httpx.ProtocolHTTP3
)

// 更新后动作常量。
const (
	AfterUpdateContinue = client.AfterUpdateContinue
	AfterUpdateExit     = client.AfterUpdateExit
	AfterUpdateRestart  = client.AfterUpdateRestart
)

// ParseManifest 解析发布清单。
func ParseManifest(data []byte) (*Manifest, error) {
	return core.ParseManifest(data)
}

// SameOriginResolver 返回同源资产地址：manifestURL 的 origin + asset.URL 的 path。
// manifestURL 或 asset.URL 无法解析时原样返回 asset.URL。
func SameOriginResolver(manifestURL string, asset Asset) string {
	return client.SameOriginResolver(manifestURL, asset)
}

// NewServer 创建自更新服务端实例；注册到 webx 前调用 RegisterWebx。
func NewServer(assetsDir string, opts ...ServerOption) (*Server, error) {
	return server.NewServer(assetsDir, opts...)
}

// WithAdminToken 设置管理路由鉴权令牌；为空表示不挂载管理路由。
func WithAdminToken(token string) ServerOption {
	return server.WithAdminToken(token)
}

// WithManifestPath 设置清单文件路径，默认 assetsDir/manifest.json。
func WithManifestPath(path string) ServerOption {
	return server.WithManifestPath(path)
}

// WithManifestURL 设置清单对外路由，默认 /updates/manifest.json。
func WithManifestURL(path string) ServerOption {
	return server.WithManifestURL(path)
}

// WithAssetsURL 设置资产静态服务前缀，默认 /updates/assets/。
func WithAssetsURL(path string) ServerOption {
	return server.WithAssetsURL(path)
}

// NewClient 创建自动更新客户端实例；main 最前调用 Client.Run 完成闭环。
func NewClient(cfg ClientConfig) (*Client, error) {
	return client.NewClient(cfg)
}
