# M3.6 Windows 隔离根可靠性长跑（隔离数据根基线验收）

> 证据口径：**隔离数据根基线验收**（非生产路径；ADR §11.3 r5 / ROADMAP rev3）。
> 目标机：DESKTOP-9M8FOG7（Windows 10 Pro 10.0.19045.6466，x64）；工作根 `G:\gameserver-work`。
> 提交：`2e1effa`（M3.5 完成点）；向量：launcher-descriptor；口令两 token；`GAMESERVER_PZ_HOME=C:\Users\admin\Zomboid`。
> 平台声明：仅 Windows 10 x64；Linux/macOS 未验证（本证据不改变该状态）。

## 1. A 段：连续启停 + 崩溃注入（2026-09-30，服务端口 18776）

原始日志：`G:\gameserver-work\logs\m36a.txt`（脚本 `/c/Users/admin/AppData/Local/Temp/gs-m36a.sh`）。

### 1.1 连续 start/stop 10 轮（第 6 轮含 `taskkill /F` 强杀）

| 轮次 | start | stop | running | 残留（进程 / 端口） |
|---|---|---|---|---|
| 1 | 200 | 200 | false | 0 / 0 |
| 2 | 200 | 200 | false | 0 / 0 |
| 3 | 200 | 200 | false | 0 / 0 |
| 4 | 200 | 200 | false | 0 / 0 |
| 5 | 200 | 200 | false | 0 / 0 |
| 6（强杀） | 200 | 200 | false | 0 / 0（`ROUND6_KILL` 记录 PID 后 `taskkill /F`，随后 stop 幂等成功） |
| 7–10 | 200 | 200 | false | 0 / 0（每轮 el≈45s） |

**判定：PASS（10/10 轮无残留进程与端口）。**

> **Harness 缺陷记录（如实说明，不掩盖）**：日志中的 `ROUNDn_RESIDUE_FAIL`/`ROUNDS_RESIDUE_FAIL=10` 是**脚本自身的判定缺陷**，不是真实残留——`residue()` 里 `grep -c ... || echo 0` 在命令替换中产生了额外一行 `0`，使 `grep -E 'procs=0.*ports=0'` 跨行失配。真实数值见上表（每轮 `procs=0`、`ports=0`，逐行可核）。该缺陷已定位，不影响结论；后续脚本应改为 `|| true`。

### 1.2 崩溃注入（4 类）

| 注入 | 动作 | 结果 | 判定 |
|---|---|---|---|
| **二进程争锁** | `gs-lockhold` 独立进程持锁 40s，同时调用 start | `start=409 {"detail":"instance owned by another process"}`；释放后 `start=200` | **PASS**（失败关闭 + 释放后恢复） |
| **端口占用** | PowerShell 绑定 UDP 16261 后调用 start | `start=200`，随后 `procs=1 ports=1` | **待复核（记录为差异）**：既有契约规定 start 以"子进程创建成功"即 200；该注入未能证明游戏在端口冲突下失败关闭。需在下一轮以 readiness/游戏日志复核（见 §3 未解除项） |
| **缺制品** | 移走 `jre64/bin/java.exe` | `start=500`，`procs=0` | **PASS**（失败关闭，未起进程；随后恢复制品） |
| **外部改写配置** | 将 `servertest.ini` 截断为 3 行后查询/启动 | `is_installed=true`；`start=200` | **PASS（按既有语义）**：受管键按 `ApplyGameConfig` 契约在启动前重写；证据为 start 前配置同步路径未报错。原始 INI 已从 `.m36bak` 还原 |

### 1.3 观测窗口启动校验

`OBS_START=200`、`OBS_READY="readiness":"ready"`（120s 窗口内就绪）、`OBS_RESIDUE=procs=1 ports=2` → 运行态正常（1 进程 / 2 端口）。

## 2. B 段：观测窗口（进行中）

脚本 `/c/Users/admin/AppData/Local/Temp/gs-m36b.sh`（`SAMPLES=300`，每 60s 一次 ≈ 5 小时），日志 `G:\gameserver-work\logs\m36b.txt`。
采样字段：`running`、`readiness`、`memory_mb`、`cpu_percent`、`handles`（java 进程句柄总数）、`procs`、`started_marker`（环形缓冲中 `SERVER STARTED` 可检索次数——关键事件可检索性判定）。

首批样本（示例）：

```
SAMPLE 0 2026-09-30T00:44:36Z running:true readiness:checking memory_mb:0 cpu_percent:0 handles=1919 procs=1 started_marker=0
```

**判定规则（预先定义）**：
- 进程存活且 `readiness=ready` 持续；
- 句柄/内存无单调增长趋势（对比首末样本）；
- `started_marker ≥ 1` 且运行期不丢失（环形缓冲仍可检索关键事件）。

**已收集样本（2026-09-30T00:44:36Z → 01:35:16Z，50 个样本 / 约 50 分钟）**：

| 指标 | 结果 |
|---|---|
| `running` | true × 50/50（全程存活） |
| `readiness` | `ready` × 49，`checking` × 1（首个样本，正常启动过程） |
| `procs` | 1 × 50/50（无额外进程/崩溃重启） |
| `started_marker` | 0 → 1，其后恒为 1（环形缓冲中关键事件仍可检索，未发生静默丢失） |
| `handles` | min 1919（启动中）→ max 5447；首 5 样本均值 4737 → 末 5 样本均值 5340（**+603**） |

**判定**：有限窗口内**未见崩溃、进程丢失、就绪回退或关键事件丢失**（该部分为 PASS 证据）；但 **句柄在 50 分钟内呈缓慢上升趋势（+603）**，该窗口不足以区分「JVM 预热/GC 滞后」与「缓慢泄漏」——按规则记 `BLOCKED(时间窗口)`，**不记为 PASS**，需完整观测窗口（≥4h，理想 8h）复核。

**时间窗口说明**：完整 8h 观测超出单次会话窗口；按 m3 矩阵规则，该行记 `BLOCKED(时间窗口)` 并附已收集样本与趋势，**不得记为 PASS**。

**已知观测限制（如实记录）**：`/api/status` 在本目标机上 `memory_mb`/`cpu_percent` 恒为 0（进程状态提供者未在 Windows 填充该两项）；句柄数据来自 `Get-Process java | Handles`（PowerShell 独立口径），内存趋势因此**未能由 API 口径核验**。该限制属观测工具面，不影响 M3 其它行。

## 3. 未解除项（不伪称完成）

1. **端口占用注入**的失败关闭语义未证明（见 §1.2），需以游戏日志/就绪状态复核。
2. **8h 观测**：完整窗口待更长时间运行；当前为有限窗口样本。
3. 隧道中断场景未触发（条件未发生）；现场保留清单已定义（进程/端口/锁/日志/证据文件 + 清理责任=运维）。
4. 中止规则：连续 2 次非预期副作用即中止并保留现场（本轮未触发）。
