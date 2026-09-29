# M2 签核状态

> **r5 状态注记（2026-09-29）**：本文件顶部原「未签核」判定已被 **§6 r4 签核**取代——M2 在 r4 声明边界（Windows 10 x64 + launcher-descriptor 向量 + PZ 42.21）下已签核；残余项为 sandbox 重写差异（并入 M3 复核）与独立审查（豁免留痕，见 ADR §11.3）。M2 矩阵 E-LIVE（Linux）行按 r5 改判 `N/A—平台未声明`。

- **日期**：2026-09-29
- **判定（r5 注记取代）**：**M2 条件签核**——声明仅对 Windows 10 x64 + launcher-descriptor 向量 + PZ 42.21 成立（§6）；Linux/macOS 未验证，不得声称支持。
- **已完成**：离线矩阵全部可执行行（E-OFF，darwin 开发平台）45 PASS / 0 FAIL；`M2-BUILD` 门（本机门禁 + 目标提交 GitHub CI run 全绿）已满足。
- **未完成**：15 行 BLOCKED（需具名 Linux/Windows 目标主机、真实 PZ、浏览器或逐项授权），以及 M1-A 独立审查（供应商额度耗尽）。

## 1. 矩阵 ID 状态汇总

| 矩阵 ID | 子案例 | 离线执行 | 状态 | 阻塞原因（若有） |
|---|---|---|---|---|
| `M2-BUILD` | 2 | 2 | **PASS** | —（目标提交 CI 全绿，run/SHA 见证据文件） |
| `M2-API` | 3 | 4 | **PASS** | — |
| `M2-UI` | 2 | 0 | **BLOCKED** | 浏览器闭环需 E-BROWSER 人工操作；源码级分支断言已 PASS（`TestStaticUIContract`） |
| `M2-AUTH` | 3 | 3 | **PASS** | — |
| `M2-WS` | 6 | 5 | **部分** | 5 行 PASS（含 D3 wire 与源码级 UI 兼容）；浏览器 UI 稳态观察 BLOCKED |
| `M2-SINGLEWRITER` | 13 | 5 | **部分** | 对账/未持锁/非终态 journal/陈旧记录 PASS；**Windows 双独立进程实机 PASS**（`M2-windows-two-process.md`：409 精确文案、只读 200、无写入、强杀后 recovery required、recover 后恢复）；Linux 变体 BLOCKED |
| `M2-SECRET` | 4 | 3 | **部分** | 脱敏/边界 PASS；POSIX 秘密权限的 Linux 声明门 BLOCKED |
| `M2-INSTALL` | 7 | 3 | **部分** | 安装生命周期/取消/deadline/冲突/retry + SteamCMD 配置解析 + 端到端 install→start→stop PASS；真实 helper reap 的声明平台门 BLOCKED |
| `M2-RESTART` | 3 | 3 | **部分** | 重启持久化 + 陈旧 owner 失败关闭 + 停止后状态枚举 PASS；E-OS 声明行 BLOCKED |
| `M2-CONFIG` | 6 | 4 | **部分** | 原子写/兼容/校验/损坏 + D8 失败关闭（darwin 补充）PASS；Linux D8 声明门 BLOCKED |
| `M2-MIGRATE` | 30 | 3 | **部分** | 恢复表 30 个现场、中断注入、备份链路 PASS；目标文件系统 staging/同卷与 Linux 权限门 BLOCKED |
| `M2-PROCESS` | 6 | 5 | **部分** | 生命周期/日志/就绪/模板隔离 + Windows 树终止 argv（darwin 补充）PASS；Linux/Windows 启动向量与声明平台 helper BLOCKED |
| `M2-PZ-LIVE` | 7 | 0 | **BLOCKED** | 需 Target Manifest + 真实 SteamCMD/PZ + 逐项授权 |
| `M2-PLATFORM` | 3 | 0 | **BLOCKED** | 需具名 Ubuntu LTS 目标主机资格证据 |

证据文件：`docs/acceptance/evidence/M2-darwin-pre-live.md` 与同名 JSON；执行器 `docs/acceptance/rehearsals/m2-offline.sh`。

## 2. 签核所缺（逐条解除条件）

| 缺项 | 解除条件 |
|---|---|
| Windows 实机（首选目标） | **已接入并完成离线/双进程验收**（见 `M2-windows-pre-live.md`、`M2-windows-two-process.md`）；剩余为 `M2-PZ-LIVE`：需 Target Manifest + 真实副作用逐项授权（SteamCMD 安装 PZ、启动、端口） |
| Linux Ubuntu LTS 实机（次选） | 用户提供具名主机与访问方式；执行 M2-SINGLEWRITER 双进程、D8 权限门、启动向量与 reap 行 |
| Target Manifest | 填实 `M2-GO-PZ-MVP.md` §4 全部字段（OS/架构、PZ build ID、SteamCMD 版本、oracle、端口/网络、浏览器版本） |
| 真实副作用授权 | 用户对 SteamCMD 下载、PZ 安装/启动、端口绑定与网络探测、临时数据根写入逐项授权（含绝对路径、时间窗口、停止/恢复办法） |
| 独立审查 | 恢复 teammate 额度或指定替代审查渠道；当前仅有标注为非独立的复核 |
| M1-B 生产接管 | 不在本验收授权内；需具名主机 + 维护窗口 + 单独授权 |

## 3. 知识处置（本次新增的可复用经验）

四条经验已 stage 为候选（会话 `ksyn-17cc3f98d029250a`，状态 pending，晋升需用户确认），并在仓库内落地了**使用点可发现**的长期归宿：

| 经验 | 仓库内归宿（长期） | 候选状态 |
|---|---|---|
| 演练/验收脚本绝不修改仓库工作树 | `docs/acceptance/README.md` §不变量 + 两个脚本头部注释 | staged pending |
| HTTP 解码的 `json.Number` 会让适配器静默回退默认值 | `internal/adapters/pz/state_ini.go` 注释 + 回归测试 `TestManagedINIUpdatesAcceptsJSONNumberVariables` | staged pending |
| 错误映射 `default` 吞掉真实失败原因 | `internal/adapters/httpapi/server.go` 分支 + 表驱动测试 | staged pending |
| legacy 宽松权限根 → 首次启动失败关闭（`fix-permissions` 运行手册） | `docs/acceptance/README.md` §权限 + `docs/migration/WINDOWS-SETUP.md` | staged pending |

查看候选：`maestro knowledge review ksyn-17cc3f98d029250a --json`。

## 4. 实现侧 Windows 就绪改进（已在目标机验证的部分）

- **进程树终止**：原 `!linux && !darwin` 分支用 `os.Interrupt`（Windows 必失败）且只杀单进程；现新增 `process_windows.go` / `steamcmd/process_windows.go`，以 `taskkill /PID <pid> /T /F`（类型化 argv、不经 shell）终止整棵树，子进程以 `CREATE_NEW_PROCESS_GROUP` 创建。argv 形状在所有平台有单测（`TestTaskkillArgsStayTypedAndExact*`）。
- **SteamCMD 安装器接线**：原先 `steamcmd.New()` 未配置，真实安装请求必然失败；现从 `GAMESERVER_STEAMCMD_EXECUTABLE` / `GAMESERVER_STEAM_APP_ID`（缺省取模板 `steam.app_id`）解析，未配置时**失败关闭**并打印告警；SteamCMD 目录固定在 `<实例>/steamcmd`（不随 state promotion 移动）。
- **已在 Windows 目标机实测**：全部 Go 测试通过、`gofmt` 干净、HTTP 黑盒与端到端启停、双独立进程锁与恢复链路（见 §1 与两份 Windows 证据文件）。
- **仍需目标机/授权**：真实 SteamCMD 安装 PZ、真实启动与就绪 oracle、端口可达性、真机控制台闭环（`M2-PZ-LIVE`）；Linux 变体行。

## 5. 结论

离线可验证的部分已全部完成并有可复跑证据；**剩余 15 行与独立审查受外部依赖阻塞**。在真实主机证据出现前：不声明平台支持、不宣布 M1/M2 完成、不执行 M1-B。

## 5. 条件证据状态（2026-09-29 更新）

- **SteamCMD 安装 PZ（app 380870）已在 Windows 目标机真实执行并完成**：`install_task=COMPLETED progress=100`、`is_installed=true`、安装目录含 `java/ jre64/ license/ media/ natives/ ProjectZomboid64.json StartServer64.bat`（耗时约 4.5 分钟）。制品哈希见 `docs/acceptance/TARGET-MANIFEST-windows.md` 与 `m2-windows-artifacts.json`。
- **该记录为 Windows 条件证据，非 E-LIVE**：按 M2-GO-PZ-MVP §3.1，`M2-PZ-LIVE` 的 E-LIVE 目标是 Ubuntu LTS 主机；Windows 结果**不计入该 ID 的 7 行子案例**，也不产生平台支持声明。
- **启动子项仍为 BLOCKED（当时）**：Windows 服务端包不含 `ProjectZomboid64.exe`，原向量不可满足 → 现在由 `launcher-descriptor` 第三向量承接（见计划 r4 与 `TARGET-MANIFEST-windows.md` §3）。
- **授权偏差记录**：该次安装执行时未按 M2 §2 的"逐项授权并写入证据记录"流程先行落档；按门禁记为该动作的**偏差**，补救 = 在 `docs/acceptance/evidence/M2-windows-live-auth.md` 补齐授权记录后，在门内复跑 validate（见计划阶段 6）。

## 5. r3 收尾状态（2026-09-29，本计划执行后）

离线矩阵实测（`bash docs/acceptance/rehearsals/m2-offline.sh` → exit 0，共 116 行）：

- `M2-API`：离线 PASS **14** 行，BLOCKED **0** 行
- `M2-AUTH`：离线 PASS **6** 行，BLOCKED **0** 行
- `M2-BUILD`：离线 PASS **3** 行，BLOCKED **0** 行
- `M2-CONFIG`：离线 PASS **11** 行，BLOCKED **1** 行
- `M2-INSTALL`：离线 PASS **8** 行，BLOCKED **1** 行
- `M2-MIGRATE`：离线 PASS **6** 行，BLOCKED **2** 行
- `M2-PLATFORM`：离线 PASS **0** 行，BLOCKED **1** 行
- `M2-PROCESS`：离线 PASS **12** 行，BLOCKED **2** 行
- `M2-PZ-LIVE`：离线 PASS **0** 行，BLOCKED **1** 行
- `M2-RESTART`：离线 PASS **9** 行，BLOCKED **2** 行
- `M2-SECRET`：离线 PASS **7** 行，BLOCKED **1** 行
- `M2-SINGLEWRITER`：离线 PASS **9** 行，BLOCKED **1** 行
- `M2-UI`：离线 PASS **3** 行，BLOCKED **2** 行
- `M2-WS`：离线 PASS **13** 行，BLOCKED **1** 行

**Windows 条件性实机证据**（`docs/acceptance/evidence/M2-windows-live.md`，非支持声明）：下载→配置→启动→控制台→停止 五段闭环真机跑通（启动 PID 15848、端口 16261/16262 监听、停止后 0 残留），并修复三个真实缺陷（PZ 42.21 口令两 token、Windows 配置根在用户 profile、陈旧 cachedir 触发 QueuedQuit）。已记录的差异：冷启动超出 60s 就绪窗口；PZ 退出时以运行时状态重写 sandbox 文件。

**仍未解除**：`M2-PZ-LIVE`/`M2-PLATFORM`（E-LIVE = Ubuntu LTS 具名主机）保持 BLOCKED；独立审查 BLOCKED；M1-B 未授权；实例隔离方案（专用服务账户 / 共享 profile + `-servername` / junction）待用户定案。

## 6. r4 签核（2026-09-29）：声明边界落定

**声明对象（用户确认，D-R4-3）**：Windows 10 x64 + `launcher-descriptor` 向量 + PZ 42.21（buildid 25485538）。支持声明**仅对该目标成立**，且必须随附当前唯一未收敛的已记录差异：PZ 在退出时以运行时状态重写 `*_SandboxVars.lua`（文件回读与 API 回读可能短暂不一致）。原列差异①（冷启动超 60s 就绪窗口）经定位为两个代码缺陷（`Logs(limit<1)` 返回空导致探针读不到 marker；marker 窗口硬编码 60s 不可覆盖）叠加的假象，修复后实测冷启动 40s / warm 34s 均 `ready=true`（见 `evidence/M2-windows-live.md` §13，提交 `5cb4b3b`/`d06ba49`）。证据：`docs/acceptance/evidence/M2-windows-live.md`（§12 五段闭环）+ `TARGET-MANIFEST-windows.md`。

**未实测平台**：Linux（Ubuntu LTS）与 macOS 在本矩阵内保持**未验证**，不得因元数据或交叉编译而声称支持。

**本轮实测**：`bash docs/acceptance/rehearsals/m2-offline.sh` → exit 0，PASS **53** / FAIL **0** / BLOCKED **15**（行级 134 行；`m2-offline-latest.json` 已提交，commit 钉在 HEAD）。

**实例隔离**：定案专用服务账户（`docs/migration/WINDOWS-SETUP.md` §8），执行属系统级动作、需逐项授权；`GAMESERVER_PZ_HOME` 作为配置根对齐机制保留。

**范围修订（已确认归档）**：M2 子案例计数调整；CI/演练脚本 gofmt 口径扩到 `cmd internal tools`。

**仍未解除**：① 就绪窗口复测已闭环（冷 40s / warm 34s）；剩余为 sandbox 重写差异的下一轮复核；② 独立审查（teammate 额度受限，当前为非独立复核）；③ M1-B 生产接管（未授权）；④ Linux 实机验收（不阻塞 Windows 声明，阻塞 Linux 声明）。
