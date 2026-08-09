# updatex 版本路线

> 目标：v0.1.0 起每版完成即全自动 CI + Release，全部通过后进入下一版；
> 定版级质量贯穿全程（100% 覆盖、race、fuzz、三平台 CI、govulncheck）。

## v0.1.0 — 核心闭环

- HTTP 清单源 + 清单解析；轻量 semver 比较与降级防护；
- `Check`/`Apply`/`Bootstrap` API；
- SHA256 流式校验；Unix 原子替换；
- errx 错误码全集、logx 结构化日志。

## v0.2.0 — Windows 与回滚

- Windows 启动时替换（.new + .pending + Bootstrap）；
- 替换前备份与失败回滚；
- 平台资产缺失错误细化；三平台 CI 覆盖。

## v0.3.0 — GitHub 源与签名

- GitHub Releases 源（Release API + 资产匹配）；
- Ed25519 清单签名校验（可选）；
- 大小上限与 HTTPS 强制。

## v0.4.0 — 双实例示例与可观测

- `examples/updateserver`：webx HTTP/3 服务端（自签证书、
  `/update.json` + `/download`）；
- `examples/updateclient`：httpx HTTP/3 客户端 + updatex
  （临时目标文件 1.0.0 → 1.1.0 升级验证）；
- Metrics 注入；日志事件补齐；
- fuzz：清单解析、semver 解析；
- 边界矩阵：版本比较、校验失败、下载中断、TOCTOU 重查。

## v0.5.0 — 发布前终审

- README / ERRORS.md / LICENSE / 示例（自更新演示）定稿；
- 依赖整理、govulncheck、静态检查全量；
- 收口于 v0.5.0（roadmap 完成）。

## 质量门禁（每版）

```powershell
go test -count=1 ./...
go test -count=1 -coverprofile=coverage.out ./...   # 核心包 100%
go test -race -count=1 ./...
go vet ./... && staticcheck ./...
go test -run '^$' -fuzz '^FuzzManifest$' -fuzztime=10s .
govulncheck ./...
```

CI：ubuntu/windows/macos 三平台 + fuzz job + govulncheck job +
Release（tag 触发）。
