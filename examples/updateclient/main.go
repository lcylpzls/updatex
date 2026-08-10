// updateclient 示例：基于 httpx HTTP/3 客户端与 updatex 的升级客户端。
// 演示：拉取清单 → 版本检查 → 下载校验 → 替换目标文件。
package main

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"os"
	"time"

	"github.com/lcylpzls/clix"
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
	app, err := clix.New("updateclient", "0.7.0",
		clix.WithDescription("HTTP/3 升级客户端示例"),
		clix.WithIO(os.Stdout, os.Stderr),
		clix.WithGlobalFlags(
			clix.StringFlag("manifest", "发布清单 URL").Required(),
			clix.StringFlag("current", "当前版本").Default("1.0.0"),
			clix.StringFlag("target", "目标可执行文件路径").Required(),
			clix.BoolFlag("http3", "使用 HTTP/3 传输").Default(true),
			clix.BoolFlag("allow-http", "允许明文 HTTP（仅测试）").Default(false),
			clix.BoolFlag("insecure", "跳过 TLS 证书校验（自签证书示例）").Default(true),
			clix.StringFlag("verify-key", "Ed25519 公钥（十六进制，可选）"),
		),
		clix.WithRootAction(runClient),
	)
	if err != nil {
		panic(err)
	}
	os.Exit(app.Execute(context.Background(), os.Args[1:]))
}

// runClient 执行完整升级流程（clix 根 Action）。
func runClient(ctx context.Context, c *clix.Context) error {
	var pub []byte
	if v := c.GlobalString("verify-key"); v != "" {
		keyBytes, err := hex.DecodeString(v)
		if err != nil {
			return err
		}
		pub = keyBytes
	}
	logger, err := logx.NewBuilder().EnableWriter(os.Stdout, logx.InfoLevel).Build()
	if err != nil {
		return err
	}
	opts := Options{
		ManifestURL:     c.GlobalString("manifest"),
		CurrentVersion:  c.GlobalString("current"),
		Target:          c.GlobalString("target"),
		AllowHTTP:       c.GlobalBool("allow-http"),
		UseHTTP3:        c.GlobalBool("http3"),
		InsecureTLS:     c.GlobalBool("insecure"),
		VerifyPublicKey: pub,
	}
	return run(ctx, opts, logger)
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
