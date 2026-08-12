// updateserver 示例：基于 webx + updatex 的 HTTP/3 升级服务端。
// 提供 /updates/manifest.json 清单与 /updates/assets/ 升级资产。
package main

import (
	"context"
	"os"

	"github.com/lcylpzls/clix"
	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/updatex/examples/internal/updateserver"
)

func main() {
	app, err := clix.New("updateserver", "0.8.0",
		clix.WithDescription("HTTP/3 升级服务端示例"),
		clix.WithIO(os.Stdout, os.Stderr),
		clix.WithGlobalFlags(
			clix.StringFlag("listen", "HTTP/3 监听地址").Default("127.0.0.1:9443"),
			clix.StringFlag("cert", "TLS 证书文件（PEM）").Required(),
			clix.StringFlag("key", "TLS 私钥文件（PEM）").Required(),
			clix.StringFlag("version", "发布版本（语义化版本）").Default("1.1.0"),
			clix.StringFlag("notes", "变更说明").Default("HTTP/3 示例更新"),
			clix.StringFlag("asset", "升级资产文件路径").Required(),
			clix.StringFlag("admin-token", "管理路由令牌（可选）").Default(""),
		),
		clix.WithRootAction(runServer),
	)
	if err != nil {
		panic(err)
	}
	os.Exit(app.Execute(context.Background(), os.Args[1:]))
}

// runServer 启动 HTTP/3 升级服务（clix 根 Action）。
func runServer(_ context.Context, c *clix.Context) error {
	data, err := os.ReadFile(c.GlobalString("asset"))
	if err != nil {
		return err
	}
	logger, err := logx.NewBuilder().EnableWriter(os.Stdout, logx.InfoLevel).Build()
	if err != nil {
		return err
	}
	s, err := updateserver.NewServer(updateserver.Config{
		Version:    c.GlobalString("version"),
		Notes:      c.GlobalString("notes"),
		Asset:      data,
		AdminToken: c.GlobalString("admin-token"),
	}, c.GlobalString("cert"), c.GlobalString("key"), c.GlobalString("listen"), logger)
	if err != nil {
		return err
	}
	if err := s.SetBaseURL("https://" + c.GlobalString("listen")); err != nil {
		return err
	}
	logger.Info("updateserver：HTTP/3 升级服务启动", logx.Fields(logx.String("地址", c.GlobalString("listen"))))
	return s.Start()
}
