# M2 计划与执行范围：用户决策记录

- **时间**：2026-09-28
- **方式**：`ask-user-question` 结构化工单
- **上游**：`docs/acceptance/M2-GO-PZ-MVP.md`、`docs/acceptance/M2-PLAN-REVIEW.md`、`docs/adr/ADR-001-decision.md`

## 用户答复

| # | 问题 | 用户选择 | 生效含义 |
|---|---|---|---|
| 1 | 独立 reviewer 无法派发（teammate 提供商额度耗尽）时如何审查 M2 计划 | **接受非独立复核 + 用户确认** | 允许以主代理复核替代独立审查，但必须在 `M2-PLAN-REVIEW.md` 明标"非独立"；独立审查恢复后应补做 |
| 2 | 是否确认 M2 详细验收计划的范围、入口门与 live 限制 | **确认范围与门槛（按当前计划）** | 14 ID / 95 子案例、每行 7 列、M1-A 与目标 commit CI 入口、Target Manifest 完整前不执行 live、逐项副作用授权，全部生效 |
| 3 | 剩余工作优先级 | 用户未选给选项，改为提供测试主机信息 | 见下 |

## 新信息：可用测试主机（Windows）

用户提供了一台通过 Cloudflare Tunnel 暴露的 Windows 游戏机：

| 用途 | 入口 |
|---|---|
| RDP（图形，本代理不可用） | `wingame.liubaitech.cn` / `winrdp.liubaitech.cn`（隧道内 `localhost:3389`） |
| SSH（命令行） | `winssh.liubaitech.cn`，用户 `admin`，ProxyCommand `cloudflared access ssh --hostname %h` |

**当前阻塞**：本机 `cloudflared access ssh` 返回 `websocket: bad handshake`，即 Cloudflare Access 客户端认证未完成（`~/.cloudflared` 不存在）；Pi 的 SSH 管理器亦处于锁定状态。需要用户二选一：

1. 在本机执行一次 `cloudflared access login winssh.liubaitech.cn` 完成浏览器认证；并在目标机配置密钥登录（或使用 Pi SSH 管理器保存凭据）。
2. 解锁 Pi SSH 管理器（发送 `#ssh`）并添加 `winssh.liubaitech.cn`（ProxyCommand + `admin` + 密钥），使 ssh 工具可直接使用。

## 由此生效的平台与范围约束

1. **支持声明仍只有 Linux Ubuntu LTS**（ADR §1.6 与 `ADR-001-decision.md`）。Windows 现可用于**条件性对抗/集成测试**（`M2-PROCESS` 的 Windows 行、安装/生命周期演练），但**不产生 Windows 支持声明**；除非用户另行决定把声明平台改为 Windows。
2. Windows 主机可用于 M1-A 的隔离副本演练（备份/恢复、promotion 中断、双进程单写者、回滚对账），这些演练**不改变 M1 的状态措辞**：M1 仍在无具名 Linux 目标时停在 M1-A。
3. **M1-B 仍未授权**：需要具名主机 + 维护窗口 + 用户单独授权；当前不执行任何生产接管。
4. 任何真实副作用（SteamCMD 下载、PZ 安装/启动、端口监听、防火墙/权限变更）仍需在该次操作的 Target Manifest 完整并经用户逐项授权后执行；`data/` 与既有用户数据只读。
5. 目标 commit 的 Go CI 必须重新取绿证（当前最新为 `3eea7a9` / run 36438544535）；旧 run 不能替代新提交。

## 状态

- M2 计划：**范围已确认**，但独立审查仍待补做（非独立复核已记录）。
- M2 执行（#14）：仍受 `#13`（M1-A）与 `#18`（本任务）门约束；未执行任何 M2 用例。
- M1-A（#13）：依赖 #19（Go 可运行基线）完成后进行。

## r4 决策补充（2026-09-29，本计划执行后归档）

| # | 决策 | 选项 | 选择 | 影响面 |
|---|---|---|---|---|
| D-R4-1 | 范围修订归档 | 确认归档 / 仅计数 / 都不确认 | **确认归档**（两项都接受） | M2 矩阵子案例计数调整（Windows 启动向量行重写 + 新增用例）、CI/演练脚本 gofmt 口径 `cmd internal` → `cmd internal tools`；按 §0 协议写入 ADR §11 范围修订记录 |
| D-R4-2 | 实例隔离方案 | 专用服务账户 / 共享 profile+`-servername` / junction / 暂缓 | **专用服务账户** | PZ 在 Windows 忽略 `-cachedir` 的配置语义的正式收敛：配置根与 profile 归服务账户所有；`GAMESERVER_PZ_HOME` 继续作为对齐机制；WINDOWS-SETUP.md 新增完整方案（#34） |
| D-R4-3 | 支持声明口径 | 提供 Ubuntu 主机 / 缩窄声明口径 / 保持 BLOCKED | **缩窄声明口径** | 声明对象 = 已实测的 Windows 10 x64（descriptor 向量 + 五段闭环证据，含两条已记录差异）；Linux/macOS 明确未验证；不再以 Ubuntu 主机为前提推进 M2 签核（ADR r4 + 矩阵行改写，#32/#33） |

未解除项（与 D-R4-3 不冲突，继续列入 M2-SIGNOFF）：Linux 实测仍未做（不得声称 Linux 支持）；独立审查 BLOCKED（teammate 额度）；M1-B 未授权。
