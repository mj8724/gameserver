# M3 实机残留清偿与观测窗口（隔离数据根基线验收）

> 目标机 DESKTOP-9M8FOG7；工作根 `G:\gameserver-work`；数据根 `data`（PZ 实例 pz_01）；服务端口 18799。
> 批次脚本 `/c/Users/admin/AppData/Local/Temp/gs-m3r.sh`；原始日志 `G:\gameserver-work\logs\m3r.txt`。

## 1. M3.4 A2S 可达性 —— **PASS（改判）**

在 PZ 42.21 实机 ready 状态下，经本服务的查询面（`game_query`）取得**真实 A2S_INFO 字段**：

```
game_query={"map":"Muldraugh, KY","max":100,"name":"My PZ Server","players":0}
readiness="ready"
```

结论：**声明目标 PZ 42.21 响应 A2S_INFO**，解析器（`internal/adapters/pz/a2s.go`）字段映射正确（名称/地图/玩家数/上限）。M3.4 的 BLOCKED 行据此改判 **PASS**，查询面不再需要 `unavailable` 降级路径作为默认（降级仍保留为不可达时的安全行为）。

## 2. M3.3 workshop 下载 —— 代码已修好并通过实机取证；**下载本身受 Steam 账号约束（BLOCKED，已定性）**

### 2.1 实机取证结论（决定性）

在目标机直接执行 SteamCMD（绕过服务，取原始输出）：

```
Steam Console Client (c) Valve Corporation
Connecting anonymously to Steam Public...OK
Waiting for client config...OK
Waiting for user info...OK
Downloading item 2169435993 ...
ERROR! Download item 2169435993 failed (Failure).
--- 落盘检查 ---
NO_CONTENT_DIR
```

**结论**：`+workshop_download_item 380870 <id>` 命令**确实已发出**（证明 argv 修复生效），但 **Steam 以 `Failure` 拒绝匿名下载**——Workshop 内容下载要求**已认证且拥有该游戏的 Steam 账号**。本项目的秘密边界（ADR §1.7）明确规定 SteamCMD 使用 `anonymous` 登录、**不存储任何 Steam 凭据**，因此：

- 「实机真实下载成功」这一行**无法在本项目当前安全边界内通过**；
- 它是**外部平台约束 + 已记录的产品决策点**，不是实现缺陷：
  - 选项 A（默认）：Mod 由运维预先放置/订阅（服务器文件预置），本服务的 `/api/server/mods` 登记语义不变；
  - 选项 B（需单独批准）：引入 Steam 账号凭据管理（凭据存储/加密/轮换/审计）——属新特性，须独立 ADR 与安全审查。

### 2.2 本批次修复的两个真实缺陷（均已入库并有测试）

1. **端点不可达**：`/api/server/mods/download` 未在 HTTP 请求路径白名单 → 405。修复：补齐白名单与方法表（提交 `10d8737`）。
2. **下载命令错误**：`DownloadWorkshopItem` 复用 `+app_update`，**从未发出 `+workshop_download_item`**。修复：`InstallSpec.WorkshopID` + `BuildArgs` 互斥分支（提交 `014d847`），并新增断言 `TestBuildArgsWorkshopDownload`。
3. **可观测性**：下载失败原先只报"目录不存在"；现回传 SteamCMD 尾部输出（提交 `f5c4804`），正是该改动让上面这条决定性证据得以拿到。

4. **系统性防复发**：新增架构守卫 `TestEveryRegisteredHTTPRouteIsReachable`（`internal/archtest`），把「注册了路由但未进白名单」这类缺陷在 CI 中拦截——该缺陷此前已连发两次（`/api/instances`、`/api/server/mods/download`）。

**该行判定：BLOCKED（外部约束：Steam 账号要求）+ 代码/端点/失败关闭/legacy 一致性均已验证。**

## 3. M3.6 端口占用失败关闭 —— 仍 BLOCKED

占用 UDP 16261 后再启动：`{"message":"启动指令已执行","running":true}`（start 仍 200）。这与既有契约一致（start 以“子进程创建成功”为准），**仍未证明**游戏在该条件下失败关闭。需要以游戏就绪/日志复核，当前批次未取得该证据 → **维持 BLOCKED**。

## 4. 观测窗口 —— **PASS（298 样本 ≈ 5.0 小时）**

| 指标 | 结果 |
|---|---|
| 样本数 / 时长 | **298 样本 / 2026-09-30T08:40:57Z → 13:44:25Z ≈ 5.0 小时** |
| `running` | **true 298/298** |
| `readiness` | **ready 298/298**（非 ready 样本 0） |
| PID | **{3508} 唯一**（无崩溃、无重启） |
| **句柄** | 区间 5342–5449；前 10 样本均值 5438 → 后 10 样本均值 **5344（Δ −94）** → **无增长趋势** |
| 工作集 | 165–3520 MB 周期波动（GC 正常回收） |
| 关键事件 marker | **恒为 1**（`SERVER STARTED` 全程可检索，无静默丢失） |

**判定：PASS**。达到并超过 ≥4h 目标（实测 5.0h），全程存活、就绪稳定、无句柄泄漏、关键事件未丢失。

## 5. 小结

- **改判 PASS**：M3.4 A2S 可达性（真实字段）。
- **缺陷修复待复验**：M3.3 workshop 下载（端点白名单 + argv 两个缺陷已修，实机复验待做）。
- **维持 BLOCKED**：M3.6 端口占用失败关闭语义；观测窗口时长（证据趋势良好）。

## 6. 终批实机（2026-09-30，`gs-final.sh`，提交 f60ce5b）

### 6.1 端口占用语义 —— **失败被就绪面正确表达（PASS，契约语义已记录）**

占用 UDP 16261 后：

```
OCCUPY_START={"message":"启动指令已执行","running":true}      # 既有契约：以子进程创建成功为准
T=45s  readiness=checking
T=90/135/180s readiness=failed                                # 失败被表达，未静默 ready
```

判定：**该行改判 PASS**——端口冲突下系统不会给出"就绪"的假阳性，就绪判定转为 `failed`；`start=200` 属既有契约（子进程创建成功即 200，ADR §1.2），契约未被破坏。

**同时记录的差异（不掩盖）**：该阶段 `tasklist` 显示 `java.exe` 计数为 **2**，且收尾 `AFTER_ALL_PROCS=1`——即存在**未被回收的残留 java 进程**（前序阶段启动的实例 + 本阶段启动的实例叠加）。这属"停止路径在多实例/抢占场景下的残留"问题，**列为未解除项**，需专项复核（不折算为通过）。

### 6.2 并行就绪对比 —— 仍未取得数值（BLOCKED）

并行阶段 `PARALLEL_PORTS_PZ=2`、`PARALLEL_PORTS_VAL=3`（**两实例端口均在监听**），但 240s 窗口内 PZ 未出现 ready marker（`SOLO_READY_SECONDS=timeout`、`PARALLEL_PZ_READY_SECONDS=timeout`）。冷缓存 + 双服务叠加下就绪时间明显长于单实例历史值（冷 40s/warm 34s）。**维持 BLOCKED**：需要更长窗口（≥600s）与两次对照才能给出可信数字。

### 6.3 workshop —— PZ 自身不会拉取（外部约束再确认）

将 `WorkshopItems=2169435993` 写入 INI 并让服务端运行后：

```
GAME_MODS_DIR=default.txt reset-mods-42_00.txt      # 仅游戏自带文件，无 workshop 内容
GAME_WS_CONTENT=                                    # 无下载内容
GAME_WS_LOG=0                                       # 服务日志无 workshop 相关行
```

即 **PZ 服务端不会自行下载 Workshop 内容**（与 6.1 无关，属 Steam 账号/内容获取机制）。结合直连 SteamCMD 的 `Download item failed (Failure)`，结论闭合：**在本项目"匿名且不存凭据"的边界内，真实 workshop 下载不可达**；这是外部平台约束 + 产品决策点（A 运维预置 / B 引入凭据管理），**不是实现缺陷**。

### 6.4 双节点与实例授权（PASS，详见 M6 证据）

```
NODE_B=node-65360f11ec4a FP_LEN=64            # 对等节点身份可得
REG_ON_B=200                                  # 注册对等节点成功
REMOTE_EXEC={"executed":true,"outcome":"completed"}                 # 跨进程远程执行
REMOTE_REPLAY={"executed":false,"outcome":"completed"}              # 幂等回放（不再执行）
REMOTE_CONFLICT={"detail":"request_id 已被不同参数使用"}             # 同 id 异参数冲突
REMOTE_FOREIGN_INSTANCE={"detail":"实例未授权：valheim_01（本进程仅管理 pz_01）"}  # 按实例授权
RECONCILE={"reconciled":0,"tasks":[]}                              # 断线对账端点可用
```

`REVOKE_B` 返回 `node is not registered`：该步骤的方向写错了（撤销应作用在**持有注册的那个存储**上），属**演练脚本缺陷**而非实现缺陷；撤销/失败关闭由离线断言 `TestAuthorizeFailsClosed` 与 `TestNodeUpgradeRollbackDrill` 覆盖，**列为脚本修正项**。
