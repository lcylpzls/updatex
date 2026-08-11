package updatex

import (
	"context"

	"github.com/lcylpzls/updatex/internal/core"
)

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

type (
	VersionSource = core.VersionSource
	Metrics       = core.Metrics
	Config        = core.Config
	UpdateInfo    = core.UpdateInfo
	Updater       = core.Updater
	TraceAttr     = core.TraceAttr
	TraceHook     = core.TraceHook
	Manifest      = core.Manifest
	Asset         = core.Asset
	Server        = core.Server
	ServerOption  = core.ServerOption
	Middleware    = core.Middleware
)

func New(cfg Config) (*Updater, error) { return core.New(cfg) }
func Bootstrap(ctx context.Context, executablePath string) error {
	return core.Bootstrap(ctx, executablePath)
}
func ParseManifest(data []byte) (*Manifest, error) { return core.ParseManifest(data) }
func NewServer(assetsDir string, opts ...ServerOption) (*Server, error) {
	return core.NewServer(assetsDir, opts...)
}
func WithAdminToken(token string) ServerOption  { return core.WithAdminToken(token) }
func WithManifestPath(path string) ServerOption { return core.WithManifestPath(path) }
func WithManifestURL(path string) ServerOption  { return core.WithManifestURL(path) }
func WithAssetsURL(path string) ServerOption    { return core.WithAssetsURL(path) }
