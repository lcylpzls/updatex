# updatex

自研程序自动更新基座：服务端基于 webx 提供清单与资产，客户端一键完成
版本检查、下载校验、原子替换与更新后动作，与 errx / logx / httpx / webx
生态打通。

> 当前状态：**v1.4.1**。

## 定位

updatex **不是分发平台**，不解决「怎么构建产物、怎么托管」；它解决
程序侧每个自更新需求都要重复的部分：

- 服务端：基于 webx 注册清单路由、资产静态服务与管理路由（`NewServer` +
  `RegisterWebx`）；
- 客户端：main 最前一行 `NewClient` + `Run`，内部完成检查、下载、
  校验、替换与更新后动作（继续 / 退出 / 重启）；
- 完整性：SHA256 校验和（必需）+ Ed25519 清单签名（可选）；
- 替换：Unix 原子替换，Windows 启动时替换（延迟替换）；
- 安全：默认仅 HTTPS，单一自建更新通道（不支持 GitHub 等外部渠道）；
- 可观测：logx 结构化日志、外部注入 Metrics 与 TraceHook；
- 错误语义：统一 errx 错误码。

## 快速上手

### 服务端（挂载到 webx）

```go
// 资产目录需包含 manifest.json 与各平台二进制。
s, err := updatex.NewServer("./assets", updatex.WithAdminToken("令牌"))
if err != nil {
	log.Fatal(err)
}

ws := webx.NewServer(webx.Config{
	TLSCertFile: "cert.pem",
	TLSKeyFile:  "key.pem",
}, logger)
ws.UseHttp1or2Listen(":8443", true)
ws.UseHttp3Listen(":8443")
if err := s.RegisterWebx(ws); err != nil { // 必须在 Start 前调用
	log.Fatal(err)
}
log.Fatal(ws.Start())
```

默认路由：

- `GET /updates/manifest.json`：发布清单；
- `/updates/assets/`：升级资产静态服务；
- `/updates/admin/status`：管理状态（配置 `WithAdminToken` 后启用，
  需携带 `X-Api-Token`）。

### 客户端（main 最前）

```go
c, err := updatex.NewClient(updatex.ClientConfig{
	ManifestURL:     "https://updates.example.com", // 根地址，自动补 /updates/manifest.json
	CurrentVersion: "1.0.0",
	AfterUpdate:    updatex.AfterUpdateExit, // 或 Continue / Restart
	RestartCommand: "systemctl restart myapp", // AfterUpdate=Restart 时必填
	Logger:         logger,
	// 资产地址按“拉清单的这台服务器”解析，清单里可写占位 URL。
	AssetURLResolver: updatex.SameOriginResolver,
})
if err != nil {
	log.Fatal(err)
}
if _, err := c.Run(context.Background()); err != nil {
	log.Printf("自更新失败：%v", err) // 失败不阻塞业务启动
}
```

`Run` 内部流程：Windows 启动时替换（Bootstrap）→ 拉取清单 → 版本检查 →
有更新则**验签 → 解析资产地址 → 下载校验替换** → 按 `AfterUpdate` 执行动作。
`ManifestURL` 填服务端根地址时自动补 `/updates/manifest.json`；
`AssetURLResolver` 在验签之后生效，`nil` 时沿用清单里的 `asset.URL`。

## 目录

```
updatex/
├── updatex.go          # 公开门面：NewServer / NewClient + 类型与错误码
├── internal/
│   ├── core/           # 共享引擎（清单/版本/校验/替换/Updater）
│   ├── server/         # 服务端实现（webx 适配）
│   ├── client/         # 客户端实现（httpx 集成 + 一键闭环）
│   └── source/         # HTTP 清单源（唯一通道）
├── docs/
│   ├── README.md       # 文档索引
│   ├── architecture.md # 架构详解
│   └── api.md          # API 定版
└── examples/
    ├── updateserver/   # webx HTTP/3 升级服务端示例
    └── updateclient/   # 一键升级客户端示例
```

文档索引见 [docs/README.md](docs/README.md)，更新记录见
[CHANGELOG.md](CHANGELOG.md)。

## License

MIT © [lcylpzls](https://github.com/lcylpzls)
