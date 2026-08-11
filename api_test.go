package updatex_test

import (
	"context"
	"testing"

	"github.com/lcylpzls/updatex"
)

// TestPublicAPI 黑盒冒烟测试：覆盖根包全部转发函数、类型别名与常量。
func TestPublicAPI(t *testing.T) {
	_, err := updatex.ParseManifest([]byte(`{"version":"1.2.3","platforms":{}}`))
	if err != nil {
		t.Logf("ParseManifest 返回错误（可接受）：%v", err)
	}

	_, err = updatex.New(updatex.Config{})
	if err != nil {
		t.Logf("New 返回错误（可接受）：%v", err)
	}

	_ = updatex.Bootstrap(context.Background(), "nonexistent-executable")

	_ = updatex.CodeInvalidConfig
	_ = updatex.CodeInvalidVersion
	_ = updatex.CodeManifestInvalid
	_ = updatex.CodeFetchFailed
	_ = updatex.CodeDownloadFailed
	_ = updatex.CodeChecksumMismatch
	_ = updatex.CodeSignatureInvalid
	_ = updatex.CodeDowngrade
	_ = updatex.CodePlatformUnsupported
	_ = updatex.CodeReplaceFailed
	_ = updatex.CodeRollbackFailed

	var _ updatex.VersionSource
	var _ updatex.Metrics
	var _ updatex.Config
	var _ updatex.UpdateInfo
	var _ updatex.Updater
	var _ updatex.TraceAttr
	var _ updatex.TraceHook
	var _ updatex.Manifest
	var _ updatex.Asset
}
