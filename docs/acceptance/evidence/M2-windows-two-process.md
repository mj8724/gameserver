# M2-SINGLEWRITER 双独立进程与恢复链路 — Windows 实机证据

- **日期**：2026-09-29
- **主机**：DESKTOP-9M8FOG7（Windows 10 Pro 10.0.19045.6466，AMD64，16 vCPU，32 GB RAM）
- **受测提交**：`b39ed1ae`（`feat/go-modular-skeleton`，Go 1.27.1 windows/amd64）
- **工作根**：`G:\gameserver-work`（隔离数据根 `G:\gameserver-work\twoprocess\data`，仅监听 `127.0.0.1:18769`）
- **工具**：`cmd/gameserver`（真实服务）+ `cmd/gs-lockhold`（验收辅助：独立 OS 进程持锁）

## 1. 步骤与结果

| 步骤 | 动作 | 观察结果 | 判定 |
|---|---|---|---|
| 1 | 进程 B（`gs-lockhold.exe`，PID 5464）持有实例锁 | `LOCKED 5464 G:\gameserver-work\twoprocess\data\servers` | — |
| 1 | 进程 A（真实服务）发出变更请求 | **409** `{"detail":"instance owned by another process"}`（逐字与 legacy 契约一致） | PASS |
| 1 | 只读状态查询 | **200** | PASS |
| 1 | 锁定期间的写入 | `state/` 目录文件数 = 0（无任何写入） | PASS |
| 2 | 强制终止持锁进程（`taskkill /F` + `kill -9`） | 变更请求 **409** `{"detail":"recovery required"}`，所有权记录保留（失败关闭，不自动清理） | PASS |
| 3 | 显式 `gameserver recover` | `recover: action=cleared_stale alive=false`，exit 0 | PASS |
| 3 | 恢复后再次变更 | **200**，`state/instance.json` 出现 `"MAX_PLAYERS": 24` | PASS |

## 2. 本次实测发现并修复的缺陷

**Windows 存活探测误报**：`os.FindProcess` 只能证明「进程对象可被打开」——Windows 在仍有句柄（例如父 shell 的作业句柄）引用已终止进程时会继续允许打开，因此强杀后 `recover` 曾拒绝执行并报 `recorded process <pid> is still alive`（实测复现）。
修复：`internal/adapters/oslock/probe_windows.go` 改用 `OpenProcess(PROCESS_QUERY_INFORMATION)` + **`GetExitCodeProcess`** 判定 `STILL_ACTIVE`，可区分「已终止但被引用」与「真正运行」。修复后步骤 3 立即成功。

## 3. 边界声明

- 这是 **Windows** 上两个独立 OS 进程的实机证据（用户已把 Windows 定为第一支持目标）。
- 矩阵中该行的 **Linux/Ubuntu LTS 变体仍未验证**（无具名主机），不得据此声称 Linux 支持。
- 本证据不涉及真实 PZ/SteamCMD：安装、启动、端口与就绪仍属 `M2-PZ-LIVE`（BLOCKED，需逐项授权）。
