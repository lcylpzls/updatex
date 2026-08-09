package updatex

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/httpx"
	"github.com/lcylpzls/logx"
)

const (
	defaultMaxDownload = 512 << 20 // 512 MiB
	defaultHTTPTimeout = 30 * time.Second
)

// replaceExec 可替换的替换函数（测试注入用）。
var replaceExec = replaceExecutable

// executablePathFn 可替换的可执行文件路径解析（测试注入用）。
var executablePathFn = os.Executable

// createTempFile 可替换的临时文件创建（测试注入用）。
var createTempFile = os.CreateTemp

// newHTTPClient 可替换的默认 HTTP 客户端构造（测试注入用）。
var newHTTPClient = func() (*httpx.Client, error) {
	return httpx.New(httpx.WithTimeout(defaultHTTPTimeout))
}

// VersionSource 发布源接口：拉取当前平台可用的最新发布清单。
type VersionSource interface {
	Latest(ctx context.Context) (*Manifest, error)
}

// Metrics 外部注入的更新指标回调（全部可选，nil 跳过）。
type Metrics struct {
	CheckTotal     func(delta int)
	CheckFailures  func(err error)
	UpdateSuccess  func(version string)
	UpdateFailures func(err error)
}

// Config 自动升级配置。
type Config struct {
	// Source 发布源（必填）。
	Source VersionSource
	// CurrentVersion 当前版本（必填，语义化版本）。
	CurrentVersion string
	// ExecutablePath 目标二进制路径（默认 os.Executable）。
	ExecutablePath string
	// MaxDownloadBytes 下载大小上限（默认 512 MiB）。
	MaxDownloadBytes int64
	// AllowHTTP 允许明文 HTTP（仅测试/内网；默认 false）。
	AllowHTTP bool
	// Logger 结构化日志器（可选）。
	Logger logx.Logger
	// Metrics 指标回调（可选）。
	Metrics Metrics
	// HTTPClient 资产下载客户端（可选，默认 httpx 30s 超时）。
	HTTPClient *httpx.Client
	// VerifyPublicKey Ed25519 公钥（可选；配置后强制校验清单签名）。
	VerifyPublicKey []byte
	// TraceHook 链路追踪钩子（可选）。
	TraceHook TraceHook
}

// UpdateInfo 检查结果。
type UpdateInfo struct {
	// HasUpdate 是否有可用更新。
	HasUpdate bool
	// Version 目标版本。
	Version string
	// Notes 变更说明。
	Notes string
	// Asset 当前平台资产。
	Asset Asset
	// RestartRequired Windows 延迟替换时为 true。
	RestartRequired bool
}

// Updater 自动升级器（Check/Apply 并发安全，每次调用独立执行）。
type Updater struct {
	cfg            Config
	current        semver
	executablePath string
	httpClient     *httpx.Client
}

// New 构造升级器。
func New(cfg Config) (*Updater, error) {
	if cfg.Source == nil {
		return nil, errInvalidConfig("发布源不能为空")
	}
	current, err := parseVersion(cfg.CurrentVersion)
	if err != nil {
		return nil, err
	}
	if cfg.MaxDownloadBytes == 0 {
		cfg.MaxDownloadBytes = defaultMaxDownload
	}
	if cfg.MaxDownloadBytes < 0 {
		return nil, errInvalidConfig("下载大小上限不能为负")
	}
	if len(cfg.VerifyPublicKey) > 0 && len(cfg.VerifyPublicKey) != ed25519.PublicKeySize {
		return nil, errInvalidConfig("Ed25519 公钥长度非法")
	}
	execPath := cfg.ExecutablePath
	if execPath == "" {
		execPath, err = executablePathFn()
		if err != nil {
			return nil, errx.Wrap(err, errx.KindUnavailable, CodeInvalidConfig, "无法确定可执行文件路径")
		}
	}
	client := cfg.HTTPClient
	if client == nil {
		client, err = newHTTPClient()
		if err != nil {
			return nil, err
		}
	}
	return &Updater{
		cfg:            cfg,
		current:        current,
		executablePath: execPath,
		httpClient:     client,
	}, nil
}

// Check 检查是否有可用更新（仅拉取与比较，不下载）。
func (u *Updater) Check(ctx context.Context) (info *UpdateInfo, err error) {
	ctx, end := u.startTrace(ctx, "updatex.check",
		TraceAttr{Key: "updatex.current_version", Value: u.cfg.CurrentVersion})
	defer func() { end(err) }()
	u.metricCheck()
	m, err := u.cfg.Source.Latest(ctx)
	if err != nil {
		u.metricCheckFailure(err)
		u.logError("check", err)
		return nil, err
	}
	latest, err := parseVersion(m.Version)
	if err != nil {
		u.metricCheckFailure(err)
		return nil, err
	}
	if err := u.verifyManifest(m); err != nil {
		u.metricCheckFailure(err)
		u.logError("check-signature", err)
		return nil, err
	}
	asset, err := m.AssetFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		u.metricCheckFailure(err)
		return nil, err
	}
	info = &UpdateInfo{Version: m.Version, Notes: m.Notes, Asset: asset}
	if compareVersion(latest, u.current) > 0 {
		info.HasUpdate = true
	}
	u.logCheck(m.Version, info.HasUpdate)
	return info, nil
}

// Apply 执行完整更新流程（重新拉取清单 → 比较 → 下载 → 校验 → 替换）。
func (u *Updater) Apply(ctx context.Context) (info *UpdateInfo, err error) {
	ctx, end := u.startTrace(ctx, "updatex.apply",
		TraceAttr{Key: "updatex.current_version", Value: u.cfg.CurrentVersion})
	defer func() { end(err) }()
	m, err := u.cfg.Source.Latest(ctx)
	if err != nil {
		u.metricUpdateFailure(err)
		u.logError("apply-fetch", err)
		return nil, err
	}
	latest, err := parseVersion(m.Version)
	if err != nil {
		u.metricUpdateFailure(err)
		return nil, err
	}
	if err := u.verifyManifest(m); err != nil {
		u.metricUpdateFailure(err)
		u.logError("apply-signature", err)
		return nil, err
	}
	if compareVersion(latest, u.current) <= 0 {
		err := ErrDowngrade
		u.metricUpdateFailure(err)
		u.logError("apply-compare", err)
		return nil, err
	}
	asset, err := m.AssetFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		u.metricUpdateFailure(err)
		return nil, err
	}
	if !u.cfg.AllowHTTP && !strings.HasPrefix(asset.URL, "https://") {
		err := errx.New(errx.KindUnavailable, CodeDownloadFailed, "资产地址必须使用 HTTPS")
		u.metricUpdateFailure(err)
		return nil, err
	}
	tmp, err := u.downloadAndVerify(ctx, asset)
	if err != nil {
		u.metricUpdateFailure(err)
		u.logError("apply-download", err)
		return nil, err
	}
	defer os.Remove(tmp)
	restart, err := replaceExec(u.executablePath, tmp, m.Version)
	if err != nil {
		u.metricUpdateFailure(err)
		u.logError("apply-replace", err)
		return nil, err
	}
	u.metricUpdateSuccess(m.Version)
	u.logApplied(m.Version)
	return &UpdateInfo{
		HasUpdate:       true,
		Version:         m.Version,
		Notes:           m.Notes,
		Asset:           asset,
		RestartRequired: restart,
	}, nil
}

// startTrace 开始更新操作链路（无钩子时 no-op）。
func (u *Updater) startTrace(ctx context.Context, name string, attrs ...TraceAttr) (context.Context, func(error)) {
	if u.cfg.TraceHook == nil {
		return ctx, func(error) {}
	}
	return u.cfg.TraceHook.Start(ctx, name, attrs...)
}

// verifyManifest 配置公钥时校验清单签名；未配置公钥则跳过。
func (u *Updater) verifyManifest(m *Manifest) error {
	if len(u.cfg.VerifyPublicKey) == 0 {
		return nil
	}
	if m.Signature == "" {
		return ErrSignatureInvalid
	}
	return m.VerifySignature(u.cfg.VerifyPublicKey)
}

// ApplyAndRestart 执行更新并在需要时调用 restart（由业务决定退出方式）。
func (u *Updater) ApplyAndRestart(ctx context.Context, restart func() error) (*UpdateInfo, error) {
	info, err := u.Apply(ctx)
	if err != nil {
		return nil, err
	}
	if info.RestartRequired && restart != nil {
		if err := restart(); err != nil {
			return nil, errx.Wrap(err, errx.KindUnavailable, CodeReplaceFailed, "重启调用失败")
		}
	}
	return info, nil
}

// Bootstrap 处理 Windows 延迟替换标记；Unix 恒返回 nil。
func Bootstrap(_ context.Context, executablePath string) error {
	return bootstrap(executablePath)
}

// downloadAndVerify 下载资产到临时文件并流式校验 SHA256。
func (u *Updater) downloadAndVerify(ctx context.Context, asset Asset) (path string, err error) {
	resp, err := u.httpClient.Get(ctx, asset.URL)
	if err != nil {
		return "", errx.Wrap(err, errx.KindUnavailable, CodeDownloadFailed, "资产请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", errx.New(errx.KindUnavailable, CodeDownloadFailed,
			fmt.Sprintf("资产端点返回非 200：%d", resp.StatusCode))
	}
	tmp, err := createTempFile(filepath.Dir(u.executablePath), "updatex-*")
	if err != nil {
		return "", errx.Wrap(err, errx.KindUnavailable, CodeDownloadFailed, "临时文件创建失败")
	}
	defer func() {
		tmp.Close()
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	tee := io.TeeReader(resp.Body, tmp)
	if _, err := verifySHA256(tee, asset.SHA256, u.cfg.MaxDownloadBytes); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

// metricCheck 记录检查计数。
func (u *Updater) metricCheck() {
	if u.cfg.Metrics.CheckTotal != nil {
		u.cfg.Metrics.CheckTotal(1)
	}
}

// metricCheckFailure 记录检查失败。
func (u *Updater) metricCheckFailure(err error) {
	if u.cfg.Metrics.CheckFailures != nil {
		u.cfg.Metrics.CheckFailures(err)
	}
}

// metricUpdateSuccess 记录更新成功。
func (u *Updater) metricUpdateSuccess(version string) {
	if u.cfg.Metrics.UpdateSuccess != nil {
		u.cfg.Metrics.UpdateSuccess(version)
	}
}

// metricUpdateFailure 记录更新失败。
func (u *Updater) metricUpdateFailure(err error) {
	if u.cfg.Metrics.UpdateFailures != nil {
		u.cfg.Metrics.UpdateFailures(err)
	}
}

// logCheck 记录检查日志。
func (u *Updater) logCheck(latest string, has bool) {
	if u.cfg.Logger == nil {
		return
	}
	u.cfg.Logger.Info("updatex：版本检查完成", logx.Fields(
		logx.String("updatex_current", u.cfg.CurrentVersion),
		logx.String("updatex_latest", latest),
		logx.Bool("updatex_has_update", has),
	))
}

// logApplied 记录替换完成日志。
func (u *Updater) logApplied(version string) {
	if u.cfg.Logger == nil {
		return
	}
	u.cfg.Logger.Info("updatex：更新已应用", logx.Fields(
		logx.String("updatex_version", version),
	))
}

// logError 记录失败日志。
func (u *Updater) logError(stage string, err error) {
	if u.cfg.Logger == nil {
		return
	}
	u.cfg.Logger.Error("updatex：更新流程失败", logx.Fields(
		logx.String("updatex_stage", stage),
		logx.String("error", err.Error()),
	))
}

// errInvalidConfig 构造配置错误。
func errInvalidConfig(msg string) error {
	return errx.New(errx.KindInvalid, CodeInvalidConfig, msg)
}
