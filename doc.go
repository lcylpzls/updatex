// Package updatex 提供面向应用程序的自动更新基座：服务端与客户端两个工厂门面。
//
// 服务端通过 NewServer 创建，并调用 Server.RegisterWebx 挂载到 webx 服务；
// 客户端通过 NewClient 创建，main 最前调用 Client.Run 完成
// “检查 → 下载 → 校验 → 替换 → 更新后动作”全闭环。
// 实现主体位于 internal/server 与 internal/client，共享引擎位于 internal/core。
package updatex
