# 更新日志

## [v0.7.0] - 2026-08-10

### 变更

- 家族测试底座接入：根包与 source 子包测试改用语义等价的
  testx 断言（含 Require* 致命断言）；
- 测试依赖新增 `testx v1.2.0`，errx 同步升级 v1.4.0。

### 质量

- 根包语句覆盖率 100%；race / vet / staticcheck 全绿。

## [v0.6.0] - 2026-08-10

### 变更

- examples 子模块升级 webx → `github.com/lcylpzls/webx/v2 v2.0.1`
  （升级服务示例同步迁移模块路径）。

## [v0.5.0] - 2026-08-10

### 新增

- `TraceHook` 链路追踪钩子（零依赖接口 + `Config.TraceHook`）：
  Check / Apply 自动埋点（updatex.current_version 属性），
  由 tracex 等外部适配器接入。

### 质量

- 根包与 source 包覆盖率保持 100%；race / vet 全绿。

## [v0.4.1] - 2026-08-10

### 修复

- `parseVersion` 接受可选小写 `v` 前缀（如 `v1.2.3`），与 git tag
  惯例及主流 semver 库一致；修复带 `v` 前缀的当前版本/清单导致
  初始化失败的问题（issue #1）；
- 补充 v 前缀解析、比较、清单与 fuzz 种子测试。

### 质量

- 根包与 source 包覆盖率保持 100%；race / vet / fuzz 全绿。

## [v0.4.0] - 2026-08-10

### 新增

- 双实例 HTTP/3 示例（独立嵌套模块 `examples/`，不污染库依赖）：
  - `examples/updateserver`：基于 webx 的 HTTP/3 升级服务端，
    提供 `/update.json` 清单与 `/download` 资产；
  - `examples/updateclient`：基于 httpx HTTP/3 客户端 + updatex，
    端到端演示 1.0.0 → 1.1.0 升级（含 Windows 延迟替换闭环）；
- 端到端测试断言真实协商 `HTTP/3.0` 协议；
- 依赖升级：httpx v1.0.2（修复客户端超时取消 H2/H3 响应体）。
- CI/Release 增加 Linux 多发行版容器矩阵（debian / fedora /
  rockylinux / archlinux / alpine，纯 Go 构建链路 CGO_ENABLED=0）。

### 说明

- 示例测试已接入三平台 CI 与 Release 质量门禁；
- HTTP/3 升级要求 httpx ≥ v1.0.2。

## [v0.3.0] - 2026-08-09

### 新增

- GitHub Releases 源：`NewGitHubSource` + 令牌/客户端注入，
  资产命名约定 `<名称>_<GOOS>_<GOARCH>`，配套 `.sha256` 校验和
  文件自动拉取解析；
- Ed25519 清单签名：`Config.VerifyPublicKey` 注入公钥后强制
  校验，`Manifest.VerifySignature` 公开 API，签名载荷为
  `Signature` 置空后的规范化 JSON；
- `FuzzVerifySignature` 模糊目标并接入 CI。

## [v0.2.0] - 2026-08-09

### 新增

- Windows 启动时替换：`.new` 暂存 + `.pending` 原子标记 +
  `Bootstrap` 替换清理（失败保留标记，下次启动重试）；
- Unix 替换前备份 `.bak`，替换失败自动回滚；
- 下载临时文件改为与目标二进制同目录，消除跨文件系统
  rename 失败隐患；
- 平台文件操作注入接缝（rename/remove/stat/chmod/读写），
  保证各平台错误分支 100% 覆盖。

### 修复

- v0.1.0 中 Unix 测试断言使用 `errors.Is` 匹配 errx 包装错误
  不生效的问题（改用 `errx.Is` 错误码匹配）。

## [v0.1.0] - 2026-08-09

### 新增

- 核心闭环：`Check` / `Apply` / `ApplyAndRestart` / `Bootstrap`；
- 发布清单结构 `Manifest` 与 JSON 解析（平台资产选择）；
- 轻量语义化版本解析与比较（含预发布标识符，拒绝降级）；
- SHA256 流式校验（512 MiB 默认上限）与 HTTPS 强制；
- Unix 原子替换与 Windows 启动时替换占位；
- errx 错误码全集与 logx 结构化日志、外部指标注入；
- HTTP 清单源（httpx 客户端，支持 HTTP/2 / HTTP/3 传输）；
- 根包与 source 包语句覆盖率 100%，三平台 CI、模糊测试与
  govulncheck 质量门禁。

### 说明

- Windows 延迟替换将在 v0.2.0 落地，当前 `replace_windows.go`
  为安全占位实现。
