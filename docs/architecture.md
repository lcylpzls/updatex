# updatex 架构详解

> 本文是项目最需要跟进的部分：组件、状态机、平台时序与安全模型。

## 1. 模块划分

```
updatex/
├── updatex.go          # 公开门面：NewServer / NewClient + 类型与错误码
├── internal/
│   ├── core/           # 共享引擎（零 webx）：Manifest/Asset/ParseManifest、
│   │                   #   semver、SHA256/Ed25519 校验、替换、Updater、错误码
│   ├── server/         # 服务端：Server/NewServer/Reload/HandleAdmin/RegisterWebx
│   ├── client/         # 客户端：Client/NewClient/Run/AfterUpdateAction
│   └── source/         # HTTP 清单源（唯一通道，基于 httpx）
├── examples/
│   ├── updateserver/   # webx HTTP/3 升级服务端示例
│   └── updateclient/   # 一键升级客户端示例
└── docs/
```

依赖方向：`updatex` → `internal/server` / `internal/client` →
`internal/core`；`internal/client` → `internal/source` → `internal/core`。
`internal/core` 不依赖 webx；`internal/server` 是唯一依赖 webx 的包。
全库无循环依赖。

## 2. 组件关系

```
业务程序
  │ NewServer / RegisterWebx（服务端）        NewClient / Run（客户端）
  ▼                                            ▼
internal/server（webx 适配）             internal/client（一键闭环）
  │ 清单路由 / 资产静态 / 管理路由              │ Bootstrap → Check → Apply → 动作
  ▼                                            ▼
internal/core（共享引擎）                 internal/source（HTTP 清单源）
  │ Manifest / semver / verify / replace        │ httpx 客户端（支持 HTTP/1/2/3）
  └──────────────┬──────────────────────────────┘
                 ▼
           errx / logx / cryptox / validx / tracex-contract
```

## 3. 服务端设计

`NewServer(assetsDir, opts...)` 只做配置与清单加载，不持有任何 HTTP 服务器。
`RegisterWebx(ws)` 在 `webx.Start()` 之前把能力注册进调用方的 webx 实例：

1. 清单路由：`GET <manifestURL>`，处理器每次读取当前清单快照，
   `Reload()` 后即时生效，无需重新注册；
2. 资产静态服务：`ServeStaticDirWithOptions` 挂到 `<assetsURL>`，
   由 webx 路由器提供 GET/HEAD 与路径穿越防护；
3. 管理分组：`/updates/admin`，`tokenGuard`（`X-Api-Token` 常量时间比较）
   作为分组中间件，内置 `/status`，`HandleAdmin` 注册的自定义路由
   全部走同一鉴权。

`HandleAdmin(method, pattern, h)` 在注册前校验方法、路径与令牌配置，
失败返回 `CodeInvalidConfig`，避免把错误拖到请求期。

## 4. 客户端状态机（Run）

```
开始
  │ Bootstrap(ctx)   ← Windows：完成上次未完成的延迟替换（Unix 无操作）
  ▼
Check（拉取清单 → semver 比较 → 可选验签）
  │ 无更新
  ▼
返回 Result{Updated:false}
  │ 有更新
  ▼
Apply（重新拉取清单 → 验签 → 解析资产地址 → 下载 → SHA256 流式校验 → 替换）
  │ 失败 → 返回错误（业务继续启动，不阻塞）
  ▼
按 AfterUpdate：
  ├─ Continue → 返回 Result{Updated:true}
  ├─ Exit     → 记录日志 → os.Exit(0)
  └─ Restart  → 异步启动 RestartCommand（cmd /C 或 sh -c）
                 启动失败返回错误不退出；成功 → os.Exit(0)
```

## 5. 平台时序

### 5.1 正常检查（Unix）

```
Check
 1. HTTPSource.Latest(ctx)          GET manifest.json（HTTPS）
 2. ParseManifest(bytes)            版本、平台资产、可选签名
 3. semver.Compare(latest, current) 降级防护
 4. 返回 HasUpdate 与目标版本

Apply
 1. 重新拉取清单（TOCTOU 防护）并比对版本
 2. 打开 Asset.URL，流式读入临时文件，同时 SHA256
 3. 校验和匹配 → chmod 0755
 4. rename(当前, .bak) → rename(临时, 当前)
 5. 删除 .bak；失败时恢复 .bak
```

### 5.2 Windows 启动时替换

```
Apply（旧进程）
 1-3 同上（下载校验到 目标.new）
 4. 写 目标.pending 标记（内容 = 目标版本）
 5. 返回 RestartRequired=true

新进程 main 最前执行 Run
 1. Bootstrap：读 .pending → 校验 .new → rename(目标, .old)
     → rename(.new, 目标) → 删除 .pending 与 .old
 2. 失败保留 .pending（下次启动重试）
 3. 继续正常 Check/Apply
```

`.pending` 写入必须是原子的（临时文件 + rename），避免半写标记。

## 6. 安全模型

### 6.1 传输与完整性

- 默认仅 HTTPS；`AllowHTTP` 仅用于测试/内网；
- SHA256 在下载流中并行计算，校验失败即删除临时文件；
- `MaxDownloadBytes` 限制（默认 512 MiB），流式截断；
- 更新通道固定为自建服务端（`internal/source` 只有 HTTP 清单源），
  不接入 GitHub 等外部渠道。

### 6.2 真实性（可选签名）

- Ed25519 公钥经 `VerifyPublicKey` 注入；
- 签名对象为清单 JSON 规范化字节（`Signature` 置空后序列化）；
- 配置公钥但清单无签名 → `ErrSignatureInvalid`；
- 未配置公钥 → 跳过签名校验（文档明确降级风险）。

### 6.5 资产地址解析

- `AssetURLResolver` 在**验签之后、下载之前**调用，签名载荷中的
  `asset.URL` 不被改写，多环境无需生成多份清单；
- 默认 `nil` 时沿用 `asset.URL` 原值，行为与旧版一致；
- 内置 `SameOriginResolver`：取清单 origin + 资产 path，
  即“我拉清单的这台服务器”，适合私网/公网共用一份清单；
- 解析后的地址仍受 HTTPS 强制校验约束（`AllowHTTP` 除外）。

### 6.3 TOCTOU 防护

`Apply` 会重新拉取清单并比对目标版本，不依赖 `Check` 的缓存结果；
版本不一致返回错误，要求重新检查。

### 6.4 管理路由

- `X-Api-Token` 使用常量时间比较；
- 未配置 `WithAdminToken` 时管理分组完全不挂载；
- 自定义管理路由必须通过 `HandleAdmin` 注册，强制走同一鉴权。

## 7. 平台差异明细

| 能力 | Unix | Windows |
| --- | --- | --- |
| 原子替换 | rename 同目录 | 不支持运行中替换 |
| 替换时机 | 立即 | 下次启动（Bootstrap） |
| 备份 | .bak 同目录 | .old 同目录 |
| 标记文件 | 无 | .pending |
| 权限 | chmod 0755 | 保持默认 |
| 重启命令 | /bin/sh -c | cmd.exe /C |

## 8. 集成建议

- webx 服务：`NewServer` + `RegisterWebx(ws)`，`Start` 前完成注册；
- 业务 main：最前调用 `NewClient` + `Run`，失败仅记录日志，不阻塞启动；
- `AfterUpdate` 由业务按部署形态选择：systemd/supervisor 用
  `Restart`（如 `systemctl restart xxx`），Windows 服务用
  `Restart`（服务重启指令）或 `Exit`（由服务管理器拉起）；
- confx：`ClientConfig` 结构体可直接作为 confx 目标。
- 迁移说明：若调用方曾在 source 阶段改写资产 URL，应删除该改写，
  改为配置 `updatex.SameOriginResolver`（否则启用签名后校验必然失败）。
