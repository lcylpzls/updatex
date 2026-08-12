// updateclient 示例：基于 updatex 客户端工厂的一键自动升级。
// main 最前创建客户端并调用 Run，完成检查 → 下载校验 → 替换 → 更新后动作。
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"os"

	"github.com/lcylpzls/clix"
	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/updatex"
)

// Options 升级客户端选项（命令与测试共用）。
type Options struct {
	ManifestURL     string
	CurrentVersion  string
	Target          string
	AllowHTTP       bool
	UseHTTP3        bool
	InsecureTLS     bool
	VerifyPublicKey []byte
	AfterUpdate     updatex.AfterUpdateAction
	RestartCommand  string
}

func main() {
	app, err := clix.New("updateclient", "0.8.0",
		clix.WithDescription("HTTP/3 一键升级客户端示例"),
		clix.WithIO(os.Stdout, os.Stderr),
		clix.WithGlobalFlags(
			clix.StringFlag("manifest", "发布清单 URL").Required(),
			clix.StringFlag("current", "当前版本").Default("1.0.0"),
			clix.StringFlag("target", "目标可执行文件路径").Required(),
			clix.BoolFlag("http3", "使用 HTTP/3 传输").Default(true),
			clix.BoolFlag("allow-http", "允许明文 HTTP（仅测试）").Default(false),
			clix.BoolFlag("insecure", "跳过 TLS 证书校验（自签证书示例）").Default(true),
			clix.StringFlag("verify-key", "Ed25519 公钥（十六进制，可选）").Default(""),
			clix.StringFlag("after-update", "更新后动作：continue/exit/restart").Default("continue"),
			clix.StringFlag("restart-command", "重启动作的命令行（如 systemctl restart xxx）").Default(""),
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
	action, err := parseAfterUpdate(c.GlobalString("after-update"))
	if err != nil {
		return err
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
		AfterUpdate:     action,
		RestartCommand:  c.GlobalString("restart-command"),
	}
	res, err := run(ctx, opts, logger)
	if err != nil {
		return err
	}
	if res.Updated {
		logger.Info("updateclient：升级完成", logx.Fields(logx.String("目标版本", res.Version)))
	} else {
		logger.Info("updateclient：当前已是最新版本", logx.Fields())
	}
	return nil
}

// parseAfterUpdate 解析更新后动作参数。
func parseAfterUpdate(s string) (updatex.AfterUpdateAction, error) {
	switch s {
	case "continue":
		return updatex.AfterUpdateContinue, nil
	case "exit":
		return updatex.AfterUpdateExit, nil
	case "restart":
		return updatex.AfterUpdateRestart, nil
	default:
		return 0, errors.New("非法更新后动作：" + s)
	}
}

// run 执行完整升级流程（测试与命令共用）。
func run(ctx context.Context, opts Options, logger logx.Logger) (*updatex.Result, error) {
	cfg := updatex.ClientConfig{
		ManifestURL:     opts.ManifestURL,
		CurrentVersion:  opts.CurrentVersion,
		ExecutablePath:  opts.Target,
		AllowHTTP:       opts.AllowHTTP,
		Logger:          logger,
		AfterUpdate:     opts.AfterUpdate,
		RestartCommand:  opts.RestartCommand,
		VerifyPublicKey: opts.VerifyPublicKey,
		InsecureTLS:     opts.InsecureTLS,
	}
	if opts.UseHTTP3 {
		cfg.Protocol = updatex.ProtocolHTTP3
	}
	c, err := updatex.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return c.Run(ctx)
}
