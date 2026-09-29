# M2 Windows live 授权记录与版本清单（阶段 2 前置门）

- **日期**：2026-09-29
- **授权人**：用户（本会话问答选择）
- **授权范围**：**全部授权（安装 + 启动 + 探测 + 停止）**；安装位置选择 **G: 盘**
- **证据链路**：授权发生于本会话的 ask-user-question 交互；本文件把该授权固化为仓库内可核验记录。**在该文件落盘之前执行的 SteamCMD 安装按 M2 §2 记为越门偏差**（见 `M2-windows-pre-live.md` 注记），补救 = 本记录落盘后于阶段 6 在门内复跑 validate。

## 1. 授权动作清单

| 动作 | 绝对路径 / 目标 | 端口与协议 | 时间窗口 | 停止与恢复办法 |
|---|---|---|---|---|
| SteamCMD 下载/安装/validate PZ（app 380870，匿名登录） | exe `G:\steamcmd\steamcmd.exe`；工作目录 `G:\steamcmd`；安装目录 `G:\gameserver-work\data\servers\pz_01\server_files` | 出站 HTTPS（Steam CDN），无入站监听 | 2026-09-29（一次性；可复跑） | 任务完成即退出；中断则记录 SteamCMD 输出，必要时删除半成品目录后重跑 |
| 启动 PZ 服务端（descriptor 向量） | `<install>\jre64\bin\java.exe` | 游戏端口 `16261/16262`（UDP）本机绑定；本机服务 `127.0.0.1:18769` | 2026-09-29 | 先经控制台下发退出命令，超时后 `taskkill /T /F` 杀进程树；停止后检查残留进程与端口 |
| 就绪/端口探测（日志 marker，60s 窗口） | 本机日志读取（`*** SERVER STARTED ***`） | 仅本机回环 | 同上 | 探测超时不停止进程；失败即记录 |
| 配置写入（INI 与 SandboxVars，含 secret 不得回显） | `G:\gameserver-work\data\servers\pz_01\Zomboid\Server\*` | 无 | 同上 | 写入前备份，失败回滚；`.bak` 多代保留 |
| 清理/停止后核对 | `tasklist`、`netstat` | 16261/16262/18769 | 同上 | 无残留即完成 |

**边界（未授权）**：不改防火墙、不改系统服务、不动 `data/` 与用户现有游戏实例、不做 M1-B 生产接管、不声明平台支持。

## 2. 端口口径统一

| 用途 | 端口 | 出现位置 |
|---|---|---|
| 本机服务（Web/API/WS） | `127.0.0.1:18769` | 本文件、`TARGET-MANIFEST-windows.md`、目标机启动命令 |
| PZ 游戏端口（UDP） | `16261`（SERVER_PORT）、`16262`（DIRECT_PORT） | 同上 |

## 3. 版本清单（`steamcmd +app_info_print 380870`，2026-09-29）

命令：`steamcmd.exe +login anonymous +app_info_print 380870 +quit`
完整输出 SHA-256：`aa0361c82414c8e3859d3a9806449a965772c464cf1fc38c946299c69d8564c8`（298 行）；逐字摘录：`docs/acceptance/evidence/pz-appinfo-380870.txt`

| branch | buildid | 说明 | 模板 `versions[]` 建议 |
|---|---|---|---|
| `public` | `25485538` | 默认分支 | `label: 稳定版（public）`，`default: true` |
| `unstable` | `25485538` | Unstable | `label: 测试版（unstable）` |
| `legacy41` | `24928750` | Build 41.78.21 | `label: 旧版 41.78.21` |
| `42.19` | `24929695` | Build 42.19.2 | `label: 42.19.2` |

说明：`branch == default_branch`（public）时**不传** `-beta`；其余分支传 `-beta <branch>`。分支名白名单 `^[A-Za-z0-9._-]{1,64}$`（`42.19`、`legacy41` 均满足）。

## 4. 验收

- [x] 授权记录落盘（本文件）；范围、路径、端口、窗口、停止与恢复办法齐备
- [x] `app_info_print` 输出归档（含命令、时间、SHA-256）
- [x] 端口在证据与 `WINDOWS-SETUP.md` 中一致（18769 / 16261 / 16262）
- [ ] 用户对本记录确认（阶段 6 前）
