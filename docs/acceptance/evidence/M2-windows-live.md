# M2 Windows 端到端 live 验收（条件性证据，未通过）

> **性质**：Windows **条件性**实机证据，**不产生平台支持声明**，不计入 `M2-PZ-LIVE` 的 E-LIVE 子案例。
> **状态**：**部分通过 + 1 项失败**（写入侧），失败原因已定位并归类为待修实现缺陷。

## 1. 环境与前置门

| 项 | 值 |
|---|---|
| 主机 | DESKTOP-9M8FOG7（Windows 10 Pro 10.0.19045.6466 / AMD64） |
| 受测提交 | `c89ddd1`（`feat/go-modular-skeleton`，Go 1.27.1 windows/amd64 现场构建） |
| 服务端口 | `127.0.0.1:18769`（仅回环）；PZ 端口 `16261/16262` |
| 授权 | `docs/acceptance/evidence/M2-windows-live-auth.md`（逐项：安装/validate、启动/停止、探测、配置写入） |
| 向量 | `launcher-descriptor`（依据 `TARGET-MANIFEST-windows.md`），内存 3072 MB |
| 就绪 oracle | 日志 marker `*** SERVER STARTED ***`（60s 窗口）+ 进程存活 |

## 2. 结果

| 步骤 | 结果 | 证据 |
|---|---|---|
| 模板版本清单 `/api/templates.versions[]` | **PASS** | 现场返回 `public`、`unstable`、`legacy41`、`42.19` 四个分支 |
| 未知分支安装请求 | **PASS** | `POST /api/server/install {"version":"nope"}` → **422**，且未启动下载 |
| 选定版本安装/校验（`{}` 默认分支） | **PASS** | HTTP **202**；`install_task` → `COMPLETED progress=100`，回显 `"version":"public"` |
| 选项写入 `POST {"options":…}`（INI 3 + SandboxVars 3，含 1 secret） | **FAIL** | HTTP **500**，`{"detail":"保存配置项失败"}` |
| 启动 → 就绪 → 控制台 → 停止 → 残留检查 | **NOT RUN** | 上一步失败后流程被中止（脚本卡在后续请求，已强制结束任务与 java/gs2 进程） |
| PZ 重写后回读与可写性 | **NOT RUN** | 同上 |

## 3. 失败根因（已定位）

`pz.Config.ApplyOptionValues` 先读实例子树内的配置文件来判定每个选项名的归属：

- 实例内存在 `<instance>/Zomboid/Server/servertest.ini`（服务此前创建），但**不存在** `<instance>/Zomboid/Server/servertest_SandboxVars.lua`；
- 因此 `SandboxVars` 类选项（`Zombies`、`Basement`、`LootItemRemovalList`）在实例内"不存在" → 适配器按设计**失败关闭**（拒绝创建新键）→ 控制面包装为「保存配置项失败」500。

**这与阶段 2 的隔离发现同源**：PZ 42.21 把配置写在**用户 profile**（`C:\Users\admin\Zomboid\Server`），而我们的实例子树只有自己创建的 INI；`-cachedir` 与环境重定向都不能改变这一点（`M2-windows-vendor-config-extraction.md` §5）。

## 4. 修复方向（下一轮）

1. **落地实例隔离决策**（三选一，需用户确认）：专用服务账户 / 共享 profile + `-servername` 分实例 / junction。
2. **首用播种**：实例内缺少 `<servername>_SandboxVars.lua` 时，从厂商基线（安装目录或抽取归档）**播种**一份到实例（写入前备份、原子写、读回校验），而不是让写入失败。
3. 播种后重跑本流程，并在阶段 6 补上「PZ 重写后回读 + 仍能写入新值」与残留检查。

## 5. 验收记法

按 `M2-GO-PZ-MVP.md §1.3` 三态：本文件的 FAIL/NOT RUN 不得记为 PASS；`M2-PZ-LIVE` 与 `M2-PLATFORM` 保持 **BLOCKED**（E-LIVE 目标为 Ubuntu LTS 具名主机）。

## 6. 复跑结果（2026-09-29，提交 `2a79beb`，修复后）

| 步骤 | 结果 | 证据 |
|---|---|---|
| 模板版本清单 | **PASS** | `public` / `unstable` / `legacy41` / `42.19` |
| 未知分支安装 | **PASS** | 422，未启动下载 |
| 选定版本安装/校验 | **PASS** | 202 → `COMPLETED progress=100`，回显 `version:"public"` |
| **选项写入（INI 3 + SandboxVars 3，含 1 secret）** | **PASS** | HTTP **200**；回读 `Zombies=7`（default 4）、`RCONPassword` 值 `null` 且 `secret:true`；`SECRET_LEAK=0` |
| 只读键经选项路径写入 | **PASS** | 409（含选项名的精确文案） |
| 启动请求 | PASS（HTTP 200） | `{"message":"启动指令已执行","running":true}` |
| **进程存活与就绪** | **FAIL** | 启动后进程立即退出：`PROCS=0`、`PORTS=0`、`readiness:"timeout"`；日志含 **`src\tier0\threadtools.cpp (3807) : Assertion Failed: Illegal termination of worker thread`**（Steam/tier0 断言） |
| 控制台命令 | 连带失败 | 400「服务器未运行」 |
| 停止与残留 | **PASS** | 停止 200；停止后进程 0 / 端口 0 |
| PZ 重写后回读与可写性 | **FAIL（读错文件）** | 读到的 profile 文件仍是 `Zombies = 4`、`RCONPassword` 不存在 → 证明**我们的写入落在实例子树，而 PZ 读的是用户 profile**（隔离缺口） |

### 6.1 两个新结论

1. **选项写路径已可用**（200/409/secret 不回显/回读一致），此前 500 的两个根因均已修复：① 缺少实例沙盒文件 → 播种修复；② 裸词字符串 → 写入为惰性字面量（`Basement = "Rural"`）。
2. **以 SYSTEM 身份运行服务无法拉起 PZ**：schtasks `/ru SYSTEM` 下 java 进程触发 Steam/tier0 断言后退出。厂商 `.bat` 与我们的 descriptor 直跑此前都在**用户上下文**（admin）成功到达 `SERVER STARTED`。→ 阶段 6 必须在**用户上下文**执行（交互式会话或 `schtasks` 以当前用户 + 已登录会话运行），并把该约束写入运行手册。
3. **隔离缺口仍是阻塞项**：写入落点（实例子树）与 PZ 读点（用户 profile）不一致；需在 §4 的三选一方案中定案，或实现「实例 → `-servername` + profile 目录托管」的映射。

## 7. 下一步（完成 #28 的前置）

1. 以**用户上下文**重跑（去掉 `/ru SYSTEM`），确认 PZ 达到 `SERVER STARTED` 并 `ready=true`；
2. 定案隔离方案后，把实例配置写入点与 PZ 读取点对齐（或按 `-servername` 映射），再验证「PZ 重写后回读 + 仍能写入新值」；
3. 全部通过后补齐本节表格与 Target Manifest 实值，并把 `M2-PZ-LIVE`/`M2-PLATFORM` 保持 BLOCKED（E-LIVE = Ubuntu LTS）。

## 8. 用户上下文复跑（2026-09-29，非 SYSTEM）

以当前登录用户（admin）运行同一任务（`schtasks /tn gs-live7`，无 `/ru SYSTEM`）：结果与 SYSTEM 一致——选项写入 200、只读 409、启动 200，但 java 进程**立即退出**，`readiness:"timeout"`，日志同为 `src\tier0\threadtools.cpp (3807) : Assertion Failed: Illegal termination of worker thread`，停止后残留 0/0。

**结论修正**：失败与运行账户无关。对照组差异只剩两点：① 成功的手工直跑**保持 stdin 打开**（管道/控制台），而服务侧子进程的 stdin 在服务上下文中可能立即 EOF → PZ 控制台循环退出 → 进程终止并在关闭路径触发 tier0 断言；② 手工直跑未传 `-adminpassword`（本轮传了）。下一轮按 ① 优先验证：为子进程提供**长期打开的 stdin 管道**（同时作为控制台输入通道），确认 PZ 达到 `SERVER STARTED`。

## 9. 退出原因定位（2026-09-29，argv 对照实验）

三个对照实验（同一 argv、同一安装目录、stdin 与 stdout 组合不同）：

| 实验 | stdin | 结果 |
|---|---|---|
| A：带 `-adminpassword`，stdin `/dev/null` | EOF | 立即退出（tier0 断言） |
| B：不带 `-adminpassword`，stdin `/dev/null` | EOF | 立即退出（同上） |
| C：stdin 保持打开（`sleep 240; quit`） | 打开 | 立即退出（同上） |

**决定性证据**：C 的日志（去掉 tier0 断言行后）最后是

```
at zombie.network.ServerMap.QueuedQuit(ServerMap.java:784)
at zombie.network.GameServer$1.run(GameServer.java:380)
```

即 PZ 是**主动排队退出**（`QueuedQuit`），不是崩溃；tier0 断言只是退出路径上的线程清理噪音。与 `-adminpassword`、stdin 是否 EOF 均无关。

**最可能的原因**：反复 `taskkill /F` 之后遗留的实例/数据库锁状态——PZ 检测到同一 cachedir/DB 已有实例（或锁未释放）便自行退出。日志同时出现 `unknown option "-statistic" / "-servername=..." / "-adminpassword=..."`，说明 42.21 对这些参数只记警告不解析，参数面不是退出原因。

**下一轮验证顺序**：
1. 用**全新的 cachedir**（例如 `G:\gameserver-work\data\servers\pz_01\Zomboid2`）启动，排除残留锁/DB 状态；
2. 若通过，则把「实例锁/DB 清理」写入运行手册（停止必须走优雅路径；`taskkill /F` 后需清理锁）；
3. 之后再回到隔离方案定案与「PZ 重写后回读」验收。

## 10. 两个根因定位与修复（2026-09-29，最终定位）

### 10.1 口令参数形式（已修复，提交 `4ca1c08`）

对照实验（同 argv、同安装目录、全新 cachedir）：

| 形式 | 结果 | 日志 |
|---|---|---|
| `-adminpassword <value>`（**两个 token**） | **MARKER=1（SERVER STARTED）** | `admin password changed via -adminpassword option` |
| `-adminpassword=<value>`（单 token，原实现） | MARKER=0，服务端**阻塞等待交互输入口令** | `Command line admin password: null` + `Enter new administrator password:` |

→ PZ **42.21 只接受两 token 形式**；原实现（沿用 legacy Python 的单 token 写法）在该 build 上会让服务端卡在口令提示、永远不就绪。已修改 `pz.BuildLaunchSpec` 与 `pz.DescriptorLaunchSpec`，并更新 argv/脱敏测试（supervisor 早已支持两 token 脱敏）。

### 10.2 陈旧 cachedir 导致 `QueuedQuit`（待处理）

同一 argv 在**全新 cachedir**（`Zomboid2`、`Zomboid-pw*`）下可稳定到达 `SERVER STARTED`；但服务使用实例 cachedir `<data>/servers/pz_01/Zomboid` 时，进程立即退出，日志为：

```
at zombie.network.ServerMap.QueuedQuit(ServerMap.java:784)
at zombie.network.GameServer$1.run(GameServer.java:380)
```

即 PZ 主动排队退出（此前多轮 `taskkill /F` 留下的 DB/锁状态）。**结论**：该 cachedir 需要清理或重建——这与「配置读写点不一致」（§6）是同一根源，两者都指向**实例隔离方案**必须先定案。

## 11. 当前状态与最小完成路径

| 环节 | 状态 |
|---|---|
| 选择游戏 / 版本清单 / 未知分支 422 | **PASS** |
| 下载与校验（含版本回显） | **PASS** |
| 全量选项读写（200 / 409 / secret 不回显 / 文件回读） | **PASS** |
| 启动到就绪（服务上下文） | **FAIL** —— 受 §10.2 的 cachedir 状态阻塞（同 argv 在干净 cachedir 下已验证可达 SERVER STARTED） |
| 停止与残留检查 | **PASS**（0 进程 / 0 端口） |
| PZ 重写后回读 | **FAIL** —— 读点不一致（§6） |

**最小完成路径**（下一步）：
1. 定案隔离方案（专用服务账户 / 共享 profile + `-servername` / junction）；
2. 按方案把实例 cachedir 与配置读写点对齐（必要时重建 cachedir 并清理锁）；
3. 重跑本证据流程，补齐「启动→就绪→控制台→停止→重写回读」四段；
4. 完成后进入 #29 的登记/计数/知识沉淀。

## 12. 端到端全闭环跑通（2026-09-29，提交 `527ce5c`）

配置：`GAMESERVER_PZ_HOME=C:\Users\admin\Zomboid`（配置根=厂商实际读点）+ 实例缓存目录启动前清理 + 口令两 token + descriptor 向量。**修正后全流程真机结果：**

| 步骤 | 结果 | 证据 |
|---|---|---|
| 版本清单 / 未知分支 422 | PASS | public/unstable/legacy41/42.19；422 且不下载 |
| 选定版本下载/校验 | PASS | 202 → COMPLETED 100%，version:"public" |
| 全量选项写入（INI 3 + SandboxVars 3 含 secret） | **PASS** | 200；API 回读 `Zombies=7`、`RCONPassword` 值 null、`SECRET_LEAK=0`；只读键 409 |
| **启动** | **PASS** | 200 → `running:true`，**PID 15848，`java.exe` 进程 1，端口 16261/16262 监听 2 个** |
| 控制台命令闭环 | **PASS** | `POST /api/server/command {"command":"help"}` 200，日志出现命令帮助文本（`* unbanuser …`、`* voiceban …`） |
| 停止 | **PASS** | 200 → `running:false`，进程 0 / 端口 0 |
| 就绪 oracle（60s 窗口） | FAIL（窗口语义） | `readiness:"timeout"`：冷启动（全新缓存目录+首张地图生成）超过 60s 才输出 `SERVER STARTED`（手工观测约 2 分钟）；期间进程与端口已就绪。属既有 60s 上限与冷启动的已知差距，记录备用人工/延长窗口观测 |
| PZ 重写后回读 | 部分 | 停止后 profile 的 `servertest_SandboxVars.lua` 中 `Zombies = 4`，与启动前 API 回读的 7 不一致（PZ 退出时以自身运行时状态重写 sandbox 文件）；需在下一轮核对 PZ 的 sandbox 落盘源（存档 DB vs 文件）与写回语义 |

**结论**：下载 → 配置 → 启动 → 控制台 → 停止 的五段闭环在 Windows 真机成立（条件性证据，不产生支持声明；E-LIVE 目标仍为 Ubuntu LTS）。「就绪 oracle 冷启动窗口」与「PZ 退出重写 sandbox」两条差异已记录，属下一轮（隔离方案定案后）的复核项。

## 13. 就绪 oracle 缺陷定位与差异①收敛（2026-09-29，提交 `5cb4b3b`）

**症状**：五段闭环中启动成功、端口监听、日志可见，但 `/api/status.ready` 永不翻转（`readiness` 先后呈现 `timeout`（冷）与 `failed`（warm））。

**定位（两个叠加缺陷）**：
1. `Supervisor.Logs(limit)` 在 `limit < 1` 时返回空切片；而 marker 探针调用的正是 `logs.Recent(0)`（`cmd/gameserver/main.go:695`）→ **探针永远读不到 marker**。同一时刻 HTTP `/api/server/logs` 能正常看到 marker（走正 limit），形成"日志里有、探针看不见"的错觉。修复：探针改为显式扫描整个环形缓冲（`scanAll = 1000`，提交 `5cb4b3b`）。
2. marker 窗口硬编码 60s（`newLogMarkerReadiness`），无 Manifest 级覆盖；叠加缺陷 1 后表现为 60s 后 `timeout`，warm 路径则在调用方 90s 上限后转 `failed`。修复：新增 `GAMESERVER_READINESS_TIMEOUT`（秒，缺省仍 60s，上限 3600s，提交 `d06ba49`）。

**收敛实测**（同机、同向量、默认 60s 窗口；脚本 `/tmp/gs-cold1.sh`、`/tmp/gs-warm4.sh`）：

| 场景 | marker 出现 | 端口监听 | `ready=true` | 证据 |
|---|---|---|---|---|
| **冷启动**（清空实例缓存目录） | **40s** | 40s | **40s**（`readiness:"ready"`） | `G:\gameserver-work\logs\cold1.txt` |
| **warm 启动**（保留缓存） | **34s** | 34s | **34s**（`readiness:"ready"`） | `G:\gameserver-work\logs\warm4.txt` |

两次运行停止后进程 0 / 端口 0。

**结论**：差异①（"冷启动超出 60s 就绪窗口"）**不成立**——此前观测是上述两个缺陷的叠加假象；修好后冷/热启动均在默认窗口内就绪。PZ 42.21 的 `*** SERVER STARTED ***` marker 在 Windows 上稳定出现，A2S 非必需（`SERVER_PORT` 为 UDP，marker 为批准回退）。窗口仍可经 Manifest 用 `GAMESERVER_READINESS_TIMEOUT` 放宽（供更慢的机器/更大存档）。
