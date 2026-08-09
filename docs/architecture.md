# updatex 架构详解

> 本文是项目最需要跟进的部分：组件、状态机、平台时序与安全模型。

## 1. 模块划分

```
updatex/
├── updatex.go          # 根包：Check/Apply/Bootstrap/Config/错误
├── version.go          # 轻量 semver 解析与比较
├── manifest.go         # 清单结构、解析与平台选择
├── verify.go           # SHA256 流式校验
├── replace_unix.go     # Unix 原子替换（build tag）
├── replace_windows.go  # Windows 启动时替换（build tag）
├── source/
│   ├── source.go       # VersionSource 接口
│   ├── http_impl.go    # HTTP 清单源（httpx 客户端）
  │   └── github.go       # GitHub Releases 源（v0.3.0 已落地）
└── updatex_test.go     # 根包测试（注入 httptest 源）
```

## 2. 组件关系

```
业务程序
  │ Check / Apply / Bootstrap / ApplyAndRestart
  ▼
updatex 根包（编排）
  ├── source.VersionSource   拉取清单（HTTP / GitHub）
  ├── manifest               解析与平台选择
  ├── version                semver 比较
  ├── verify                 校验和 / 签名
  └── replacer               平台替换（unix / windows）
```

根包只做编排，各子组件单一职责、可独立测试。

## 3. 状态机（Check → Apply）

```
Idle
  │ Check(ctx)
  ▼
Fetched ── 拉取失败 ──► Failed(fetch)
  │ 版本比较
  ▼
Compared ── 无更新 ──► UpToDate（HasUpdate=false）
  │ 有更新
  ▼
Downloaded ── 下载/校验失败 ──► Failed(download|checksum|signature)
  │ 替换
  ▼
Replaced ── unix ──► Done（可立即重启）
  │ windows
  ▼
RestartRequired ── 业务重启 ──► 新进程 Bootstrap ──► Done
```

> `Apply` 是自包含流程（重新拉取清单），`Check` 仅作为预览；
> 状态机以 `Apply` 为完整路径。

每个阶段失败都带 `stage` 上下文（fetch/download/verify/replace），
日志与 Metrics 可定位。

## 4. 关键时序

### 4.1 正常检查（Unix）

```
Check
 1. source.Latest(ctx)                 GET manifest.json（HTTPS）
 2. manifest.Parse(bytes)              版本、平台资产、签名
 3. semver.Compare(latest, current)    降级防护
 4. 返回 UpdateInfo{HasUpdate:true, Version, Asset}

Apply
 1. 重新拉取清单（TOCTOU 防护）并比对版本
 2. 打开 Asset.URL，流式读入临时文件，同时 SHA256
 3. 校验和匹配 → chmod 0755
 4. rename(当前, .bak) → rename(临时, 当前)
 5. 删除 .bak；失败时恢复 .bak
```

### 4.2 Windows 启动时替换

```
Apply（旧进程）
 1-3 同上（下载校验到 目标.new）
 4. 写 目标.pending 标记（内容 = 目标版本）
 5. 返回 RestartRequired

Bootstrap（新进程，main 最前调用）
 1. 读 .pending：不存在 → 直接返回
 2. 校验 .new 存在与大小
 3. rename(目标, .old) → rename(.new, 目标)
 4. 删除 .pending 与 .old
 5. 失败：保留 .pending（下次启动重试），返回错误
```

`.pending` 写入必须是原子的（临时文件 + rename），避免半写标记。

## 5. 安全模型

### 5.1 传输与完整性

- 清单与资产均通过 HTTPS 拉取（HTTP 仅测试可开）；
- SHA256 在下载流中并行计算，校验失败即删除临时文件；
- `MaxDownloadBytes` 限制（默认 512 MiB），流式截断。

### 5.2 真实性（可选签名）

- Ed25519 公钥经配置注入；
- 签名对象为清单 JSON **规范化字节**（`Signature` 置空后序列化，
  字段顺序与 map 排序由 `encoding/json` 保证）；
- 配置公钥但清单无签名 → `ErrSignatureInvalid`；
- 未配置公钥 → 跳过签名校验（文档明确降级风险）。

### 5.3 TOCTOU 防护

`Check` 与 `Apply` 分离时，检查后清单可能被替换：

- `Apply` 会重新拉取清单并比对目标版本；
- 版本不一致返回 `ErrManifestInvalid`（要求重新 Check）。

### 5.4 临时文件

- `os.CreateTemp` 生成，0600 权限；
- 校验通过后才 chmod 0755；
- 所有失败路径 defer 清理。

## 6. 平台差异明细

| 能力 | Unix | Windows |
| --- | --- | --- |
| 原子替换 | rename 同目录 | 不支持运行中替换 |
| 替换时机 | 立即 | 下次启动（Bootstrap） |
| 备份 | .bak 同目录 | .old 同目录 |
| 标记文件 | 无 | .pending |
| 权限 | chmod 0755 | 保持默认 |

## 7. 配置结构

```go
type Config struct {
	Source          VersionSource      // 发布源（必填）
	CurrentVersion  string             // 当前版本（必填，semver）
	ExecutablePath  string             // 目标二进制路径（默认 os.Executable）
	VerifyPublicKey []byte             // Ed25519 公钥（可选）
	MaxDownloadBytes int64             // 默认 512 MiB
	AllowHTTP       bool               // 仅测试
	Logger          logx.Logger        // 可选
	Metrics         Metrics            // 可选
	HTTPClient      *http.Client       // 可选（默认 30s 超时）
}
```

## 8. 集成建议

- webx 服务：启动时 `Bootstrap` → 后台 goroutine `Check` →
  管理员 API 触发 `ApplyAndRestart`；
- jobx：用 `Every` 定时检查；
- confx：`Config` 结构体可直接作为 confx 目标；
- 自更新示例（v0.4.0）：
  - `examples/updateserver`：webx HTTP/3 服务端（自签证书、
    `/update.json` + `/download`）；
  - `examples/updateclient`：httpx HTTP/3 客户端 + updatex，
    将临时目标二进制从 1.0.0 升级到 1.1.0。

### 8.1 HTTP/3 传输细节

- 客户端 `HTTPSource` 默认使用 httpx（`WithHTTP3` 切换 HTTP/3）；
- HTTP/3 需要 TLS 与 UDP；示例自签证书由服务端启动时生成；
- 清单与资产走同一传输通道，体现端到端 HTTP/3 升级。
