# updatex 设计定版

> 版本：v0.0.0（规划定稿） · 状态：文档已定版，代码未开始

## 1. 定位

updatex 是 **Go 程序的自动升级组件**：程序集成后具备
「检查 → 下载 → 校验 → 替换 → 重启」的完整闭环。
它只解决程序侧逻辑，不包含构建、托管与分发基础设施。

## 2. 范围边界（明确不做）

| 不做 | 原因与替代 |
| --- | --- |
| 构建与产物托管 | 由 CI（GitHub Actions）与发布流程负责 |
| 增量/差分更新 | 自用场景产物较小，全量下载足够；接口预留 |
| 多版本并行 / 蓝绿 | 单二进制自更新模型 |
| 安装器 / 服务注册 | 系统服务由业务处理 |
| 更新服务器实现 | updatex 是客户端；服务端只需输出约定清单 |
| 平台签名（代码签名） | 依赖 OS 签名体系；updatex 提供自校验 Ed25519 |

## 3. 核心概念

| 术语 | 含义 |
| --- | --- |
| VersionSource | 发布源接口：拉取当前平台可用的最新发布信息 |
| Manifest | 发布清单：版本、变更说明、各平台资产（URL/校验和/大小/签名） |
| UpdateInfo | 检查结果：是否有更新、目标版本、资产描述 |
| Verifier | 校验器：SHA256 必需，Ed25519 可选 |
| Replacer | 替换器：平台特定的原子替换实现 |
| Bootstrap | Windows 启动时替换入口（处理延迟替换标记） |

## 4. 发布清单格式（HTTP 源约定）

```json
{
  "version": "1.2.0",
  "published_at": "2026-08-09T00:00:00Z",
  "notes": "变更说明",
  "platforms": {
    "linux_amd64": {
      "url": "https://cdn.example.com/app-v1.2.0-linux-amd64",
      "sha256": "64 位十六进制",
      "size": 12345678
    },
    "windows_amd64": {
      "url": "https://cdn.example.com/app-v1.2.0-windows-amd64.exe",
      "sha256": "64 位十六进制",
      "size": 12345678
    }
  },
  "signature": "base64(ed25519(清单 JSON 原文))"
}
```

约束：

- `version` 为语义化版本（`主.次.补丁`，可选 `-预发布` 与 `+构建`）；
- `platforms` 键为 `GOOS_GOARCH`（如 `linux_amd64`、`windows_arm64`）；
- `sha256` 必填；`signature` 可选（配置公钥时必填）；
- 清单 JSON 的**规范化字节**参与签名（不重新序列化，直接签原文）。

## 5. 数据流

```
Check(ctx, cfg)
  → source.Latest(ctx)        拉取清单
  → semver.Compare(新, 当前)   版本比较与降级防护
  → 返回 UpdateInfo{HasUpdate}

Apply(ctx, cfg, info)
  → source.Latest(ctx)        重新拉取清单（自包含，无 Check 前置依赖）
  → semver.Compare(新, 当前)   版本比较与降级防护
  → asset := manifest.Asset（当前平台）
  → verifier.SHA256(下载流)    流式校验
  → verifier.Ed25519(清单)     可选签名校验
  → replacer.Replace(临时文件, 目标)
       ├─ unix:    rename 原子替换
       └─ windows: 写 .new + .pending 标记 → 需重启

Bootstrap(ctx, cfg)  // 启动时调用
  → 检查 .pending：存在则替换旧 .new → 清理标记
```

## 6. 版本比较（自研轻量 semver）

- 格式：`主.次.补丁`（数字，可为多段）＋ 可选 `-预发布` ＋ 可选 `+构建`；
- 比较规则（对齐 semver 2.0 核心）：
  - 主/次/补丁数字按数值比较；
  - 无预发布 > 有预发布；预发布按点分段比较（数字段数值、字母段字典序）；
  - 构建元数据不参与优先级；
- 非法版本返回 `ErrInvalidVersion`。

## 7. 降级防护

- `Check` 发现新版本 ≤ 当前版本：返回 `UpdateInfo{HasUpdate: false}`，
  不报错（正常“已是最新”）；
- 显式指定目标版本且低于当前：返回 `ErrDowngrade`；
- `Apply` 内部重新拉取清单并校验，不依赖 `Check` 的缓存结果，
  从根上消除「检查后清单被替换」的 TOCTOU 窗口。

## 8. 错误码（errx）

| 错误码 | 含义 | Kind | 建议 HTTP |
| --- | --- | --- | --- |
| `updatex_invalid_config` | 配置非法 | invalid_argument | 400 |
| `updatex_invalid_version` | 版本号非法 | invalid_argument | 400 |
| `updatex_manifest_invalid` | 清单解析/字段非法 | invalid_argument | 400 |
| `updatex_fetch_failed` | 拉取清单失败 | unavailable | 503 |
| `updatex_download_failed` | 下载资产失败 | unavailable | 503 |
| `updatex_checksum_mismatch` | SHA256 校验失败 | data_loss | 500 |
| `updatex_signature_invalid` | Ed25519 签名无效 | forbidden | 403 |
| `updatex_downgrade` | 拒绝版本回退 | conflict | 409 |
| `updatex_platform_unsupported` | 当前平台无资产 | not_found | 404 |
| `updatex_replace_failed` | 替换失败 | unavailable | 503 |
| `updatex_rollback_failed` | 回滚失败 | unavailable | 503 |

## 9. 可观测性

### 9.1 日志（logx）

| 事件 | 字段 |
| --- | --- |
| 检查完成 | `updatex_current`、`updatex_latest`、`updatex_has_update` |
| 下载开始 | `updatex_version`、`updatex_size` |
| 校验通过 | `updatex_version`、`updatex_sha256` |
| 替换完成 | `updatex_version`、`updatex_backup` |
| 失败告警 | `error`、`updatex_stage`（fetch/download/verify/replace） |

### 9.2 Metrics（外部注入）

```go
type Metrics struct {
	CheckTotal     func(delta int)
	CheckFailures  func(err error)
	UpdateSuccess  func(version string)
	UpdateFailures func(err error)
}
```

## 10. 安全模型

- 传输：仅 HTTPS（HTTP 源默认拒绝明文，可显式关闭仅测试）；
- 完整性：SHA256 流式校验（下载与校验并行，不落未校验数据）；
- 真实性：Ed25519 签名（清单签名，公钥注入；可选但推荐）；
- 大小：`MaxDownloadBytes`（默认 512 MiB）防恶意大文件；
- 临时文件：`os.CreateTemp` + 0600 权限，替换前再次校验；
- 降级：默认禁止回退；
- TOCTOU：Apply 前重新拉取清单比对，避免检查与下载之间清单被替换。

## 11. 平台替换策略

### 11.1 Unix（linux/macos）

1. 下载到临时文件并校验；
2. `chmod 0755`；
3. 备份当前二进制到 `目标 + ".bak"`（rename）；
4. `rename(临时, 目标)` 原子替换；
5. 失败时回滚（恢复 .bak）。

运行中的旧进程继续使用旧 inode，新进程启动即新版本。

### 11.2 Windows

运行中的 exe 被占用，无法 rename。采用**启动时替换**：

1. 下载到 `目标 + ".new"` 并校验；
2. 写入 `目标 + ".pending"` 标记；
3. 返回 `RestartRequired`（业务负责重启进程）；
4. 新进程启动时调用 `Bootstrap`：看到 .pending → 替换 .new → 清理标记。

## 12. 依赖

```go
require (
	github.com/lcylpzls/errx v1.2.0
	github.com/lcylpzls/logx v1.0.0
	github.com/lcylpzls/httpx v1.0.0 // HTTP 客户端（支持 HTTP/3）
	github.com/lcylpzls/webx v1.2.2 // 仅示例服务端
)
```

HTTP 清单源默认使用 httpx 客户端（可切换 HTTP/1/2/3）；
服务端示例使用 webx（HTTP/3 监听）。

## 13. 质量门禁

- 核心包 100% 语句覆盖（含 source/verify/replace 各子包）；
- `-race` 全绿、连续多轮无偶发竞态；
- fuzz：清单解析、semver 解析；
- 三平台 CI（替换路径按平台执行）+ govulncheck + Release；
- 边界矩阵：版本比较边界、校验失败、下载中断、平台缺失、回滚。

## 14. 双实例示例（HTTP/3 升级链路）

`examples/updateserver` 与 `examples/updateclient` 组成最小升级闭环：

- 服务端：webx + 自签 TLS 证书 + HTTP/3 监听，
  提供 `/update.json`（清单）与 `/download`（二进制资产）；
- 客户端：httpx（HTTP/3）拉取清单与资产 → updatex 校验替换
  临时目标文件，验证版本从 1.0.0 升级到 1.1.0；
- 两个实例均使用基座生态（webx/httpx/logx/errx），
  体现“HTTP/3 承载自动升级”的完整链路。
