# 更新日志

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
