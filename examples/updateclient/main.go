// updateclient 示例：基于 httpx HTTP/3 客户端与 updatex 的升级客户端。
// 演示：拉取清单 → 版本检查 → 下载校验 → 替换目标文件。
package main

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/lcylpzls/httpx"
	_ "github.com/lcylpzls/httpx/http3" // 注册 HTTP/3 传输
	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/updatex"
	"github.com/lcylpzls/updatex/source"
)

// Options 升级客户端选项。
type Options struct {
	ManifestURL     string
	CurrentVersion  string
	Target          string
	AllowHTTP       bool
	UseHTTP3        bool
	InsecureTLS     bool
	VerifyPublicKey []byte
}

func main() {
	var (
		manifest  = flag.String("manifest", "", "发布清单 URL")
		current   = flag.String("current", "1.0.0", "当前版本")
		target    = flag.String("target", "", "目标可执行文件路径")
		http3     = flag.Bool("http3", true, "使用 HTTP/3 传输")
		allowHTTP = flag.Bool("allow-http", false, "允许明文 HTTP（仅测试）")
		insecure  = flag.Bool("insecure", true, "跳过 TLS 证书校验（自签证书示例）")
		verifyKey = flag.String("verify-key", "", "Ed25519 公钥（十六进制，可选）")
	)
	flag.Parse()
	if *manifest == "" || *target == "" {
		fmt.Fprintln(os.Stderr, "用法：updateclient -manifest URL -target 目标文件 [-current 版本] [-http3] [-insecure] [-verify-key 公钥]")
		os.Exit(2)
	}
	var pub []byte
	if *verifyKey != "" {
		keyBytes, err := hex.DecodeString(*verifyKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "公钥不是合法十六进制：%v\n", err)
			os.Exit(1)
		}
		pub = keyBytes
	}
	logger, err := logx.NewBuilder().EnableWriter(os.Stdout, logx.InfoLevel).Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败：%v\n", err)
		os.Exit(1)
	}
	opts := Options{
		ManifestURL:     *manifest,
		CurrentVersion:  *current,
		Target:          *target,
		AllowHTTP:       *allowHTTP,
		UseHTTP3:        *http3,
		InsecureTLS:     *insecure,
		VerifyPublicKey: pub,
	}
	if err := run(context.Background(), opts, logger); err != nil {
		fmt.Fprintf(os.Stderr, "升级失败：%v\n", err)
		os.Exit(1)
	}
}

// run 执行完整升级流程（测试与命令共用）。
func run(ctx context.Context, opts Options, logger logx.Logger) error {
	clientOpts := []httpx.Option{httpx.WithTimeout(30 * time.Second)}
	if opts.UseHTTP3 {
		clientOpts = append(clientOpts, httpx.WithProtocol(httpx.ProtocolHTTP3))
	}
	if opts.InsecureTLS {
		clientOpts = append(clientOpts, httpx.WithTLSClientConfig(&tls.Config{InsecureSkipVerify: true}))
	}
	client, err := httpx.New(clientOpts...)
	if err != nil {
		return err
	}
	src, err := source.NewHTTPSource(opts.ManifestURL, opts.AllowHTTP, source.WithHTTPClient(client))
	if err != nil {
		return err
	}
	u, err := updatex.New(updatex.Config{
		Source:          src,
		CurrentVersion:  opts.CurrentVersion,
		ExecutablePath:  opts.Target,
		AllowHTTP:       opts.AllowHTTP,
		HTTPClient:      client,
		VerifyPublicKey: opts.VerifyPublicKey,
		Logger:          logger,
	})
	if err != nil {
		return err
	}
	info, err := u.Check(ctx)
	if err != nil {
		return err
	}
	logger.Info("updateclient：版本检查完成",
		logx.Fields(logx.Bool("有更新", info.HasUpdate), logx.String("目标版本", info.Version)))
	if !info.HasUpdate {
		logger.Info("updateclient：当前已是最新版本", logx.Fields())
		return nil
	}
	applied, err := u.Apply(ctx)
	if err != nil {
		return err
	}
	if applied.RestartRequired {
		// Windows 延迟替换：模拟新进程启动时完成替换。
		if err := updatex.Bootstrap(ctx, opts.Target); err != nil {
			return err
		}
	}
	logger.Info("updateclient：升级完成",
		logx.Fields(logx.String("目标版本", applied.Version)))
	return nil
}
