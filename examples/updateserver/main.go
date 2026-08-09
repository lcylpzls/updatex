// updateserver 示例：基于 webx 的 HTTP/3 升级服务端。
// 提供 /update.json（发布清单）与 /download（升级资产）。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/lcylpzls/logx"
	"github.com/lcylpzls/updatex/examples/internal/updateserver"
)

func main() {
	var (
		listen  = flag.String("listen", "127.0.0.1:9443", "HTTP/3 监听地址")
		cert    = flag.String("cert", "", "TLS 证书文件（PEM）")
		key     = flag.String("key", "", "TLS 私钥文件（PEM）")
		version = flag.String("version", "1.1.0", "发布版本（语义化版本）")
		notes   = flag.String("notes", "HTTP/3 示例更新", "变更说明")
		asset   = flag.String("asset", "", "升级资产文件路径")
	)
	flag.Parse()
	if *cert == "" || *key == "" || *asset == "" {
		fmt.Fprintln(os.Stderr, "用法：updateserver -cert 证书 -key 私钥 -asset 资产文件 [-listen 地址] [-version 版本] [-notes 说明]")
		os.Exit(2)
	}
	data, err := os.ReadFile(*asset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取资产失败：%v\n", err)
		os.Exit(1)
	}
	logger, err := logx.NewBuilder().EnableWriter(os.Stdout, logx.InfoLevel).Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败：%v\n", err)
		os.Exit(1)
	}
	s, err := updateserver.NewServer(updateserver.Config{
		Version: *version,
		Notes:   *notes,
		Asset:   data,
	}, *cert, *key, *listen, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造服务失败：%v\n", err)
		os.Exit(1)
	}
	logger.Info("updateserver：HTTP/3 升级服务启动", logx.Fields(logx.String("地址", *listen)))
	if err := s.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "服务退出异常：%v\n", err)
		os.Exit(1)
	}
}
