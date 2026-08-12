// Package client 提供面向应用程序的一键自动更新客户端。
// 用户在 main 最前创建实例并调用 Run，内部完成检查、下载、校验、替换与更新后动作。
package client

import (
	"context"
	"crypto/tls"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/lcylpzls/errx"
	"github.com/lcylpzls/httpx"
	_ "github.com/lcylpzls/httpx/http3" // 注册 HTTP/3 传输
	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/updatex/internal/core"
	"github.com/lcylpzls/updatex/internal/source"
)

// 默认 HTTP 客户端超时。
const defaultTimeout = 30 * time.Second

// AfterUpdateAction 更新成功后的动作。
type AfterUpdateAction int

const (
	// AfterUpdateContinue 继续运行，返回结果由用户决定后续。
	AfterUpdateContinue AfterUpdateAction = iota
	// AfterUpdateExit 更新成功后退出进程。
	AfterUpdateExit
	// AfterUpdateRestart 更新成功后异步执行用户命令并退出进程。
	AfterUpdateRestart
)

// Result 是 Run 的返回结果。
type Result struct {
	// Updated 是否已应用更新。
	Updated bool
	// Version 目标版本。
	Version string
	// RestartRequired Windows 延迟替换时为 true。
	RestartRequired bool
	// Notes 变更说明。
	Notes string
}

// Config 客户端配置。
type Config struct {
	// ManifestURL 发布清单地址（与 Source 二选一）。
	ManifestURL string
	// Source 自定义发布源（与 ManifestURL 二选一）。
	Source core.VersionSource
	// CurrentVersion 当前版本（必填）。
	CurrentVersion string
	// ExecutablePath 目标可执行文件路径（默认 os.Executable）。
	ExecutablePath string
	// AfterUpdate 更新成功后的动作（必填）。
	AfterUpdate AfterUpdateAction
	// RestartCommand 重启命令（AfterUpdate=Restart 时必填，如 systemctl restart xxx）。
	RestartCommand string
	// VerifyPublicKey Ed25519 公钥（可选，配置后强制校验清单签名）。
	VerifyPublicKey []byte
	// AllowHTTP 允许明文 HTTP（仅测试/内网）。
	AllowHTTP bool
	// Logger 结构化日志器（可选）。
	Logger logx.Logger
	// Protocol 传输协议（默认自动 HTTP/1.1 + HTTP/2）。
	Protocol httpx.Protocol
	// InsecureTLS 跳过 TLS 证书校验（仅测试自签证书）。
	InsecureTLS bool
	// HTTPClient 自定义 httpx 客户端（可选）。
	HTTPClient *httpx.Client
	// MaxDownloadBytes 下载大小上限（默认 512 MiB）。
	MaxDownloadBytes int64
	// Metrics 指标回调（可选）。
	Metrics core.Metrics
	// TraceHook 链路追踪钩子（可选）。
	TraceHook core.TraceHook
	// AssetURLResolver 资产下载地址解析器（可选）。
	// 入参为清单 URL 与资产，返回实际下载地址；nil 时沿用 asset.URL。
	AssetURLResolver func(manifestURL string, asset core.Asset) string
}

// Client 自动更新客户端。
type Client struct {
	updater *core.Updater
	cfg     Config
}

// 可替换的进程退出、命令启动、Bootstrap 与客户端构造（测试注入用）。
var (
	exitFn        = os.Exit
	execStart     = func(c *exec.Cmd) error { return c.Start() }
	bootstrap     = core.Bootstrap
	newHTTPClient = func(opts ...httpx.Option) (*httpx.Client, error) {
		return httpx.New(opts...)
	}
)

// NewClient 构造自动更新客户端。
func NewClient(cfg Config) (*Client, error) {
	if cfg.CurrentVersion == "" {
		return nil, errx.NewCode(core.CodeInvalidConfig, "当前版本不能为空")
	}
	if cfg.ManifestURL == "" && cfg.Source == nil {
		return nil, errx.NewCode(core.CodeInvalidConfig, "发布源不能为空：请设置 ManifestURL 或 Source")
	}
	if cfg.ManifestURL != "" && cfg.Source != nil {
		return nil, errx.NewCode(core.CodeInvalidConfig, "ManifestURL 与 Source 只能设置一个")
	}
	switch cfg.AfterUpdate {
	case AfterUpdateContinue, AfterUpdateExit, AfterUpdateRestart:
	default:
		return nil, errx.NewCode(core.CodeInvalidConfig, "更新后动作非法")
	}
	if cfg.AfterUpdate == AfterUpdateRestart && strings.TrimSpace(cfg.RestartCommand) == "" {
		return nil, errx.NewCode(core.CodeInvalidConfig, "重启动作必须提供 RestartCommand")
	}
	manifestURL := normalizeManifestURL(cfg.ManifestURL)

	client := cfg.HTTPClient
	if client == nil {
		clientOpts := []httpx.Option{httpx.WithTimeout(defaultTimeout)}
		if cfg.Protocol != httpx.ProtocolAuto {
			clientOpts = append(clientOpts, httpx.WithProtocol(cfg.Protocol))
		}
		if cfg.InsecureTLS {
			clientOpts = append(clientOpts, httpx.WithTLSClientConfig(&tls.Config{InsecureSkipVerify: true}))
		}
		var err error
		client, err = newHTTPClient(clientOpts...)
		if err != nil {
			return nil, err
		}
	}

	var src core.VersionSource
	if cfg.Source != nil {
		src = cfg.Source
	} else {
		opts := []source.HTTPSourceOption{source.WithHTTPClient(client)}
		switch cfg.Protocol {
		case httpx.ProtocolHTTP2:
			opts = append(opts, source.WithHTTP2(true))
		case httpx.ProtocolHTTP3:
			opts = append(opts, source.WithHTTP3(true))
		}
		httpSrc, err := source.NewHTTPSource(manifestURL, cfg.AllowHTTP, opts...)
		if err != nil {
			return nil, err
		}
		src = httpSrc
	}

	var resolve func(asset core.Asset) string
	if cfg.AssetURLResolver != nil {
		resolve = func(asset core.Asset) string {
			return cfg.AssetURLResolver(manifestURL, asset)
		}
	}
	u, err := core.New(core.Config{
		Source:           src,
		CurrentVersion:   cfg.CurrentVersion,
		ExecutablePath:   cfg.ExecutablePath,
		MaxDownloadBytes: cfg.MaxDownloadBytes,
		AllowHTTP:        cfg.AllowHTTP,
		Logger:           cfg.Logger,
		Metrics:          cfg.Metrics,
		HTTPClient:       client,
		VerifyPublicKey:  cfg.VerifyPublicKey,
		TraceHook:        cfg.TraceHook,
		AssetURLResolver: resolve,
	})
	if err != nil {
		return nil, err
	}
	return &Client{updater: u, cfg: cfg}, nil
}

// normalizeManifestURL 规范化清单地址：无路径或仅 "/" 时自动补服务端默认路径。
func normalizeManifestURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = core.DefaultManifestURL
	}
	return u.String()
}

// SameOriginResolver 返回同源资产地址：manifestURL 的 origin + asset.URL 的 path。
// manifestURL 或 asset.URL 无法解析时原样返回 asset.URL。
func SameOriginResolver(manifestURL string, asset core.Asset) string {
	m, err := url.Parse(manifestURL)
	if err != nil || m.Scheme == "" || m.Host == "" {
		return asset.URL
	}
	a, err := url.Parse(asset.URL)
	if err != nil || a.Path == "" {
		return asset.URL
	}
	m.Path = a.Path
	return m.String()
}

// Run 执行完整更新闭环：Bootstrap → 检查 → 下载校验替换 → 更新后动作。
// 应在 main 最前调用一次；更新成功且配置为退出/重启时不会正常返回。
func (c *Client) Run(ctx context.Context) (*Result, error) {
	execPath := c.updater.ExecutablePath()
	c.logInfo("updatex：开始自更新检查", logx.String("updatex_current", c.cfg.CurrentVersion))
	if err := bootstrap(ctx, execPath); err != nil {
		c.logError("updatex：启动时替换失败", err)
		return nil, err
	}
	info, err := c.updater.Check(ctx)
	if err != nil {
		c.logError("updatex：版本检查失败", err)
		return nil, err
	}
	if !info.HasUpdate {
		c.logInfo("updatex：当前已是最新版本", logx.String("updatex_version", info.Version))
		return &Result{Version: info.Version}, nil
	}
	applied, err := c.updater.Apply(ctx)
	if err != nil {
		c.logError("updatex：更新失败", err)
		return nil, err
	}
	c.logInfo("updatex：更新已应用", logx.String("updatex_version", applied.Version))
	return c.afterUpdate(applied)
}

// afterUpdate 按配置执行更新后动作。
func (c *Client) afterUpdate(info *core.UpdateInfo) (*Result, error) {
	result := &Result{
		Updated:         true,
		Version:         info.Version,
		RestartRequired: info.RestartRequired,
		Notes:           info.Notes,
	}
	switch c.cfg.AfterUpdate {
	case AfterUpdateContinue:
		return result, nil
	case AfterUpdateExit:
		c.logInfo("updatex：按配置退出进程", logx.String("updatex_version", info.Version))
		exitFn(0)
		return result, nil
	case AfterUpdateRestart:
		if err := runRestartCommand(c.cfg.RestartCommand); err != nil {
			c.logError("updatex：重启命令启动失败", err)
			return nil, err
		}
		c.logInfo("updatex：重启命令已启动，进程退出",
			logx.String("updatex_command", c.cfg.RestartCommand))
		exitFn(0)
		return result, nil
	default:
		return nil, errx.NewCode(core.CodeInvalidConfig, "更新后动作非法")
	}
}

// runRestartCommand 异步启动用户提供的重启命令（不等待结束）。
func runRestartCommand(command string) error {
	cmd := restartCmdFor(runtime.GOOS, command)
	if err := execStart(cmd); err != nil {
		return errx.Wrap(err, errx.KindUnavailable, core.CodeReplaceFailed, "重启命令启动失败")
	}
	return nil
}

// restartCmdFor 按操作系统构造重启命令（windows 用 cmd /C，其余用 sh -c）。
func restartCmdFor(goos, command string) *exec.Cmd {
	if goos == "windows" {
		return exec.Command("cmd.exe", "/C", command)
	}
	return exec.Command("/bin/sh", "-c", command)
}

// logInfo 输出结构化信息日志（未注入日志器时跳过）。
func (c *Client) logInfo(msg string, fields ...logx.Field) {
	if c.cfg.Logger == nil {
		return
	}
	c.cfg.Logger.WithContext(context.Background()).Info(msg, logx.Fields(fields...))
}

// logError 输出结构化错误日志（未注入日志器时跳过）。
func (c *Client) logError(msg string, err error) {
	if c.cfg.Logger == nil {
		return
	}
	c.cfg.Logger.WithContext(context.Background()).Error(msg, logx.Fields(logx.Any("error", err)))
}
