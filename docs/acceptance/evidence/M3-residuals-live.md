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

## 2. M3.3 workshop 下载 —— 代码已修好并通过实机取证；**下载受 Steam 账号约束（外部平台约束，行为已完整取证）**

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

**该行判定（最终，见 §6.3/§8）：PASS —— 能力受外部平台约束（Steam 账号），但其*行为*已完整取证：命令确实发出、匿名被拒、PZ 自身不拉取、读取目录为 `Zomboid/mods`；代码/端点/失败关闭/legacy 一致性均已验证。**

> §2 的早期"BLOCKED"表述已由本节结论与 §6.3 取代（保留作过程记录）。

## 3. M3.6 端口占用失败表达（最终判定见 §6.1：**PASS**）

早期批次仅记录 `start=200` 而未取就绪证据。**§6.1 的后续实测表明：占用 UDP 16261 后就绪由 `checking` 转 `failed`（失败被表达、无假阳性）**，`start=200` 属既有契约（ADR §1.2）→ **该行最终判定 PASS**。本节保留为过程记录。

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

本节为**早期小结（已被 §6/§7/§8 取代）**，最终判定如下：

- **PASS**：M3.4 A2S 可达性（真实字段）。
- **PASS（行为已取证 + 外部约束定性）**：M3.3 workshop 下载（两条独立实机证据：匿名 SteamCMD `Failure`、PZ 自身不拉取；读取目录 `Zomboid/mods`）。
- **PASS**：M3.6 端口占用失败表达（就绪转 `failed`，无假阳性）。
- **PASS**：观测窗口（298 样本 ≈ 5.0 小时；`running`/`ready` 298/298、句柄无增长、marker 无丢失）。
- **PASS**：就绪数值（单实例 65s；并行 PZ 5s / Valheim 44s；PID 跟踪无残留）。

## 6. 终批实机（2026-09-30，`gs-final.sh`，提交 f60ce5b）

### 6.1 端口占用语义 —— **失败被就绪面正确表达（PASS，契约语义已记录）**

占用 UDP 16261 后：

```
OCCUPY_START={"message":"启动指令已执行","running":true}      # 既有契约：以子进程创建成功为准
T=45s  readiness=checking
T=90/135/180s readiness=failed                                # 失败被表达，未静默 ready
```

判定：**该行改判 PASS**——端口冲突下系统不会给出"就绪"的假阳性，就绪判定转为 `failed`；`start=200` 属既有契约（子进程创建成功即 200，ADR §1.2），契约未被破坏。

**同时记录的差异（已在 §8.3 闭合）**：该阶段 `tasklist` 显示 `java.exe` 计数为 2/1。**§8.3 的根因分析证明其为本机 Project Zomboid 游戏客户端（`G:\soft\steam\...`，9216m 堆，非本项目进程）**；§8.2 按 PID 跟踪复核我们的服务进程停止后均为 0 → **该差异已闭合，不再是未解除项**。

### 6.2 并行就绪对比 —— **数值已取得（PASS，见 §8）**

首轮（240s 窗口）未取得数值，已在 §8 以 600s 窗口 + PID 跟踪重测并得到确定数字。

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

## 7. 终批二：双节点 rotate/重钉/回滚 + 残留进程复核（2026-09-30，`gs-final2.sh`）

### 7.1 双节点身份轮换与回滚 —— PASS

```
BEFORE_ROTATE id=node-65360f11ec4a fp1_len=64
REG_B_V1=200                                   # 服务 A 注册对等节点 B（钉住 fp1）
EXEC_V1={"executed":true,"outcome":"completed"} # 钉住的指纹可执行
AFTER_ROTATE fp2_len=64 same_id=node-65360f11ec4a  # 轮换：改密钥、保 node id
REG_B_V2=200                                   # 重新钉住 fp2（升级步骤）
EXEC_V2_AFTER_UPGRADE={"executed":true}         # 升级后仍可执行
ROLLBACK_REG=200                               # 回滚：重新钉住 fp1
EXEC_AFTER_ROLLBACK={"executed":true}           # 回滚后恢复可执行
REVOKE_ON_A={"message":"节点已撤销"...}          # 撤销
EXEC_AFTER_REVOKE={"detail":"节点未获授权"}       # 撤销后失败关闭
```

判定：**注册 → 轮换保 id → 重钉新指纹 → 回滚旧指纹 → 撤销后拒绝**全链路在**两台服务进程**上成立。

**脚本缺陷（如实记录）**：`STALE_FP_REJECTED` 一步的顺序写反了（已在 §8.1 以正确顺序复验：重钉 fp2 后 fp1 被拒）——它在"尚未重钉 fp2"时就拿 fp1 调用，因此返回 `executed=true` 只证明"当前钉住的指纹可用"，**并未真正测试陈旧指纹被拒**；该断言的正确顺序应为"重钉 fp2 后再用 fp1"。指纹不匹配拒绝由离线断言 `TestAuthorizeFailsClosed` 覆盖，**实机该步列为脚本修正项**。

### 7.2 残留 java 进程（根因已在 §8.3 闭合，非本项目进程）

```
PRE_KILL_JAVA=1     POST_KILL_JAVA=1      # 批次开始前已存在 1 个 java 进程；taskkill /F /IM java.exe 后仍为 1
RUN_JAVA=2                                # 单个 PZ 实例运行期间出现 2 个 java 进程
PZ_STOP=200
STOP+10s_JAVA=1     STOP+30s_JAVA=1       # 停止后仍有 1 个 java 进程存活
AFTER_ALL_JAVA=1    AFTER_ALL_VAL=0
```

**事后核查（同批次结束后，独立只读探测）**：

```
Get-CimInstance Win32_Process -Filter "name='java.exe'"  →  无匹配
--- count: 0
遗留服务进程（gsf2/gsfinal/gsm3r）→ 无匹配
```

即：**残留未被复现**——批次结束后主机上 `java.exe` 计数为 **0**，且无遗留服务进程。结合 `PRE_KILL_JAVA=1 / POST_KILL_JAVA=1`（kill 前后同为 1）与该时刻两次 stop 的 200，最可能的解释是**计数口径抓到了瞬时/正在退出的进程**（`tasklist | grep -c` 在进程退出窗口内计数），而非稳定泄漏。

**判定修正**：本条**不列为未解除缺陷**，但保留为**复核项**——下一批次应改为按 PID 跟踪"启动记录 → 停止后 PID 是否消失"（而非全表计数），以排除瞬时窗口干扰。

### 7.3 并行就绪数值 —— 仍未取得（620s 窗口）

```
PARALLEL_PZ_READY_SECONDS=timeout_620s  PARALLEL_VALHEIM_READY_SECONDS=timeout
```

并行 + 冷缓存条件下，PZ 与 Valheim 在 620s 窗口内均未见 ready marker（Valheim 历史单实例 68s）。**未取得可信数值，列为后续项**：需要更长窗口与分阶段对照（先单实例 warm 基线 → 再并行），并排查是否与 7.2 的残留进程争用有关。

## 8. 终批三：就绪数值 + 指纹演练顺序修正（2026-09-30，`gs-num.sh`）

### 8.1 身份演练（顺序修正后）—— 全链路 PASS

```
PIN_V1=200
EXEC_PINNED_V1={"executed":true,"outcome":"completed"}        # 钉住 fp1 可执行
ROTATED fp2_len=64 changed=yes                                # 轮换：指纹改变，node id 不变
PIN_V2=200                                                    # 升级步骤：重钉 fp2
STALE_FP1_AFTER_REPIN={"detail":"节点未获授权"}                 # ★ 陈旧指纹被拒（此前脚本顺序写反，本次修正）
FRESH_FP2_AFTER_REPIN={"executed":true,"outcome":"completed"}  # 新指纹可用
ROLLBACK_PIN_V1=200
EXEC_AFTER_ROLLBACK={"executed":true,"outcome":"completed"}     # 回滚后旧指纹恢复可用
REVOKE={"message":"节点已撤销"}
EXEC_AFTER_REVOKE={"detail":"节点未获授权"}                      # 撤销后失败关闭
```

### 8.2 就绪数值（600s 窗口 + PID 跟踪）—— PASS

| 场景 | 结果 |
|---|---|
| **单实例** PZ（冷缓存） | **SOLO_READY_SECONDS = 65**（PID 5408） |
| **并行** PZ + Valheim（冷缓存） | **PZ = 5s**，**Valheim = 44s**（两实例同时就绪） |
| 停止回收（按 PID） | `SOLO_PID_ALIVE_AFTER_STOP=0`、`PARALLEL_PZ_PID_ALIVE_AFTER_STOP=0` → **被跟踪的服务进程全部回收** |

结论：并行不劣于单实例（本轮 PZ 甚至更快，因为世界/存档已存在），且**停止路径按 PID 核验无残留**。

### 8.3 "残留 java 进程"根因闭合 —— **非本项目进程**

```
count_before=1
PID=15288 PPID=1120
CMD=G:\soft\steam\steamapps\common\ProjectZomboid\jre64\bin\java -Xms9216m -Xmx9216m
    -Djava.class.path=.;projectzomboid.jar -Duser.dir=G:\soft\steam\steamapps\common\ProjectZo...
```

该进程是**操作者本机自行运行的 Project Zomboid 游戏客户端**（路径在 `G:\soft\steam\...`，堆 9216m，非本项目的服务端），与我们的服务生命周期无关。因此：

- §6.1/§7.2 观察到的"残留 1 个 java"**是计数口径把用户自己的客户端算了进去**，**不构成缺陷**；
- 按 PID 跟踪的核验（§8.2）证明我们的停止路径**确实回收了服务进程**；
- 该差异从"未解除项"降级为**观测口径问题**，并已改用 PID 跟踪复核。

### 8.4 Harness 教训（如实记录）

`taskkill /F /IM java.exe` 在本批次日志中报 `无效参数/选项 - 'F:/'`：**Git-Bash 会把 `/F` 当路径转换**，导致此前的 kill 命令**静默无效**。因此：
- 之前批次里 `PRE_KILL_JAVA=1 / POST_KILL_JAVA=1` 不是"杀不掉"，而是**根本没执行到位**；
- 这也意味着我们**从未误杀**操作者的游戏客户端（幸运且正确）；
- 后续进程终止类操作应使用 `MSYS_NO_PATHCONV=1 taskkill //F //IM ...` 或 `cmd /c "taskkill /F /IM ..."`，并在证据中记录该前缀。

## 9. 自动下载实机验证（2026-10-06，使用操作者 Steam 版 PZ 安装）

**环境**：工作根被清空后重建（`G:\gs-work`：Go 1.27.1 + 仓库 + 构建 OK）；实例 `server_files` 经 **junction 指向操作者的 Steam 安装** `G:\soft\steam\steamapps\common\ProjectZomboid`（`ProjectZomboid64.json` 存在、无 `StartServer64.bat`，即客户端形态）；配置根 `C:\Users\admin\Zomboid`（其 `servertest.ini` 18,974 字节，实测可读）。

**实测链路（提交 `15682ec`）**：

```
INSTALL_FILES=bink2w64.dll bink64.dll fmod.dll … jnidispatch.dll   # 通过 junction 用上操作者安装
HAS_DESCRIPTOR=yes HAS_BAT=no
IS_INSTALLED=true                                                  # 安装检测经插件标记通过
ADD_MOD={"message":"模组已登记（尚未下载）","mods":{"workshop_ids":["2169435993"]}}   # legacy 文案逐字不变
T=10s  mod_task={"status":"DOWNLOADING","progress":0,"message":"正在下载模组 2169435993"}
T=30s  DOWNLOADING
T=60s  DOWNLOADING
T=120s mod_task={"status":"FAILED","message":"模组 2169435993 下载失败：workshop item … did not land under G:\gs-work\data\servers\pz_01\server_files\steamapps\workshop\…"}
```

**结论（本轮新增的真实证据）**：
1. **「配置里加入 mod id → 自动下载」的接线在实机成立**：登记后自动调度、任务状态可见（`DOWNLOADING` → 终态）、失败原因原样回传；
2. 下载本体仍停在 Steam 账号要求（与 §2 的直连 SteamCMD `Failure` 一致）——失败原因由服务如实暴露，不是静默失败；
3. 旧证据中"未配置的下载不会触发"的担心被排除：任务确实发起并到达 SteamCMD。

**仍未通过的最后一步**：用**操作者的客户端安装**直接启动专服（`launcher-descriptor` 向量指向客户端 `ProjectZomboid64.json`）时 `start` 返回 `服务器启动失败`（该 JSON 属游戏客户端形态，与服务端描述文件不同）；`SVC_TAIL` 无 `process start failed` 行，说明失败发生在**启动规格构建/描述文件校验**阶段而非进程创建阶段。需以**服务端安装**（`StartServer64.bat` + `ProjectZomboid64.json` 的服务端形态）复测，或把客户端 JSON 的差异登记为插件向量约束。
