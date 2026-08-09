# updatex

自研程序自动升级库：集成后程序具备版本检查、下载校验、原子替换
与重启能力，与 errx / logx / confx / httpx 生态打通。

> 当前状态：**v0.1.0（核心闭环）已发布**。Windows 延迟替换、GitHub
> Releases 源、Ed25519 签名与 HTTP/3 双实例示例按路线图持续推进。

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

## 快速上手

```go
import (
	"context"
	"log"

	"github.com/lcylpzls/updatex"
	"github.com/lcylpzls/updatex/source"
)

func main() {
	src, err := source.NewHTTPSource("https://cdn.example.com/update.json", false)
	if err != nil {
		log.Fatal(err)
	}
u, err := updatex.New(updatex.Config{
		Source:         src,
		CurrentVersion: "1.0.0",
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	info, err := u.Check(ctx) // 预览：是否有更新
	if err != nil {
		log.Fatal(err)
	}
	if info.HasUpdate {
		_, err = u.ApplyAndRestart(ctx, func() error {
			return nil // 业务重启逻辑
		})
		if err != nil {
			log.Fatal(err)
		}
	}
}
```

## 已发布版本

- v0.1.0：核心闭环（检查 / 下载校验 / Unix 原子替换 / HTTP 清单源）。

详细版本规划见 [docs/roadmap.md](docs/roadmap.md)，更新记录见
[CHANGELOG.md](CHANGELOG.md)。

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
