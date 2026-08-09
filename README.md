# updatex

自研程序自动升级库：集成后程序具备版本检查、下载校验、原子替换
与重启能力，与 errx / logx / confx / httpx 生态打通。

> 当前状态：**设计定版阶段（v0.0.0）**。文档已定稿，代码尚未开始。

## 定位

updatex **不是分发平台**，不解决「怎么构建产物、怎么托管」；它解决
程序侧每个自更新需求都要重复的部分：

- 版本检查：拉取发布清单、语义化版本比较、降级防护；
- 发布源：HTTP 清单源、GitHub Releases 源（接口抽象）；
- 下载校验：SHA256 校验和（必需）+ Ed25519 签名（可选）；
- 原子替换：Unix 原子 rename，Windows 启动时替换（延迟替换）；
- 回滚：替换前备份，失败可恢复；
- 可观测：logx 结构化日志、外部注入 Metrics；
- 错误语义：统一 errx 错误码。

## 快速上手（规划草案）

```go
src := source.NewHTTPSource("https://cdn.example.com/update.json", nil, false)
u, err := updatex.New(updatex.Config{
	Source:         src,
	CurrentVersion: "1.0.0",
})

info, err := u.Check(ctx)          // 预览：是否有更新
if info.HasUpdate {
	_, err = u.ApplyAndRestart(ctx, func() error {
		return nil // 业务重启逻辑
	})
}
```

## 目录

```
updatex/
├── docs/
│   ├── README.md          # 文档索引
│   ├── design.md          # 设计定版（定位/范围/数据流/错误码）
│   ├── architecture.md    # 架构详解（组件/状态机/平台时序/安全模型）
│   ├── api.md             # API 定版（完整签名与语义）
│   ├── research.md        # 领域调研与设计取舍
│   └── roadmap.md         # 版本路线
├── examples/
│   ├── updateserver/      # webx HTTP/3 升级服务端
│   └── updateclient/      # httpx HTTP/3 升级客户端
└── README.md
```

## License

MIT © [lcylpzls](https://github.com/lcylpzls)
