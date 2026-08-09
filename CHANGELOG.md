# 更新日志

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
