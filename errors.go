package updatex

import "github.com/lcylpzls/errx"

// 错误码统一以 updatex_ 为前缀。
const (
	CodeInvalidConfig       errx.Code = "updatex_invalid_config"
	CodeInvalidVersion      errx.Code = "updatex_invalid_version"
	CodeManifestInvalid     errx.Code = "updatex_manifest_invalid"
	CodeFetchFailed         errx.Code = "updatex_fetch_failed"
	CodeDownloadFailed      errx.Code = "updatex_download_failed"
	CodeChecksumMismatch    errx.Code = "updatex_checksum_mismatch"
	CodeSignatureInvalid    errx.Code = "updatex_signature_invalid"
	CodeDowngrade           errx.Code = "updatex_downgrade"
	CodePlatformUnsupported errx.Code = "updatex_platform_unsupported"
	CodeReplaceFailed       errx.Code = "updatex_replace_failed"
	CodeRollbackFailed      errx.Code = "updatex_rollback_failed"
)

// 预定义错误值，可用 errx.Is / errors.Is 判断。
var (
	ErrInvalidConfig       = errx.New(errx.KindInvalid, CodeInvalidConfig, "配置非法")
	ErrInvalidVersion      = errx.New(errx.KindInvalid, CodeInvalidVersion, "版本号非法")
	ErrManifestInvalid     = errx.New(errx.KindInvalid, CodeManifestInvalid, "发布清单非法")
	ErrFetchFailed         = errx.New(errx.KindUnavailable, CodeFetchFailed, "拉取发布清单失败")
	ErrDownloadFailed      = errx.New(errx.KindUnavailable, CodeDownloadFailed, "下载更新资产失败")
	ErrChecksumMismatch    = errx.New(errx.KindDataLoss, CodeChecksumMismatch, "SHA256 校验失败")
	ErrSignatureInvalid    = errx.New(errx.KindForbidden, CodeSignatureInvalid, "签名无效")
	ErrDowngrade           = errx.New(errx.KindConflict, CodeDowngrade, "拒绝版本回退")
	ErrPlatformUnsupported = errx.New(errx.KindNotFound, CodePlatformUnsupported, "当前平台无可用资产")
	ErrReplaceFailed       = errx.New(errx.KindUnavailable, CodeReplaceFailed, "替换可执行文件失败")
	ErrRollbackFailed      = errx.New(errx.KindUnavailable, CodeRollbackFailed, "回滚失败")
)
