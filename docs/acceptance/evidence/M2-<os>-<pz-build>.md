# M2 验收证据记录（模板）

> 复制本模板用于一次实际验收，并将文件名中的 `<os>`、`<pz-build>` 换成真实、可安全公开的标识。模板不代表测试已执行或通过。`待用户提供` 是未知值占位；测试状态须填 `PASS` / `FAIL` / `BLOCKED` / `NOT RUN`，不可把未知记作 PASS。记录不得包含任何密码、Cookie、session token、SSH 凭据或秘密原文。

## 1. 验收身份与范围

| 字段 | 本次记录 |
|---|---|
| 验收类别 | 隔离数据根基线验收（不得标作 M1-B/生产接管） |
| 验收日期/时区 | 待用户提供 |
| 目标主机安全别名（非凭据） | 待用户提供 |
| 目标 Go commit SHA | 待用户提供 |
| Go 版本 | go1.27.1（固定值；实际 `go version` 输出待记录） |
| Go 二进制 SHA-256 | 待用户提供 |
| OS / 发行版 / 版本 | 待用户提供 |
| 内核 / 架构 | 待用户提供 |
| PZ app id / 版本 / Steam build ID / 分支（若适用） | app id: 380870（模板契约值）；版本/build/分支：待用户提供 |
| SteamCMD 版本 | 待用户提供 |
| 浏览器名称 / 完整版本（UI） | 待用户提供 |
| 绝对隔离数据根 | 待用户提供 |
| 隔离根 inventory 摘要与非生产关系 | 待用户提供 |
| 网络路径、目标/来源、SERVER_PORT、DIRECT_PORT | 待用户提供 |
| 防火墙状态 / 获准变更范围 | 待用户提供 |
| Operator | 待用户提供 |
| Go 实现 owner | 待用户提供 |
| 独立 reviewer | 待用户提供 |
| 本次结论及范围 | 待记录；未执行实机时不得声明平台支持 |

## 2. 前置门、Manifest 与授权

### 2.1 门状态

| 门 | 状态 | 证据 / SHA / 说明 |
|---|---|---|
| 本次隔离 synthetic root 绝对路径、测试副作用范围、清理前 inventory | 待记录 | 待用户提供 |
| M1-A switch-ready 证据已复核 | 待记录 | 待用户提供 |
| 目标 commit Go CI 全绿（含 Ubuntu race 与 archtest） | 待记录 | Run/URL/SHA：待用户提供 |
| Target Manifest 完整并经 reviewer 复核 | 待记录 | 本节与 §2.2 |
| Disposable root 独立、空/fixture inventory 已存 | 待记录 | 待用户提供 |
| 用户已对本次每项真实副作用逐项授权 | 待记录 | 授权动作/范围/时间/证据引用：待用户提供 |
| 端口 bind/listener/目标网络探测授权（无授权不得默认执行） | 待记录 | 精确 host/port/protocol/source/time：待用户提供 |
| `data/` 与生产数据未触碰 | 待记录 | 只读 inventory / 差异证据：待用户提供 |

### 2.2 Target Manifest

| 字段 | 本次实值/证据 |
|---|---|
| Go commit / 版本 / 二进制 SHA-256 | 待用户提供 |
| PZ app id / 版本 / Steam build ID / 分支（若适用） | app id: 380870（模板契约值）；版本/build/分支：待用户提供 |
| SteamCMD 版本 | 待用户提供 |
| OS / 发行版 / 精确版本 / 架构 | 待用户提供 |
| 目标主机安全别名 | 待用户提供 |
| 数据隔离根绝对路径 | 待用户提供 |
| 根目录 inventory 与非生产关系 | 待用户提供 |
| 网络路径 / 端口 / 防火墙状态 | 待用户提供 |
| 浏览器名称 / 版本 | 待用户提供 |
| 制品检查（文件类型、shebang、摘要/路径） | 待用户提供 |
| 按制品证据选择的启动向量 | 待用户提供；须由目标 build 的实际 file/shebang 证据决定 |
| 各需外部可达端口的协议、预期范围、获准来源与探测法 | 待用户提供 |
| 实机 console 的无副作用命令、build 适用证据及授权 | 待用户提供 |
| Readiness oracle：A2S 或日志 marker | 待用户提供 |
| 若日志 fallback：不支持 A2S 证据、逐字 marker、规则 | 待用户提供（不适用时注明原因） |
| 用户逐项授权记录 | 待用户提供 |
| Operator / Reviewer / 复核时间 | 待用户提供 |

> **安全门：** §2.2 有未知或占位值、授权范围不覆盖某项动作、Manifest 未经复核，均不得开始对应的 live 操作。不要在执行后补填前置条件。

## 3. 执行摘要

| 项 | 结果 / 证据 |
|---|---|
| 总结（通过/失败/阻塞项数量） | 待记录 |
| 未执行/阻塞原因 | 待记录；未知主机/build 时 M2-PZ-LIVE 与 M2-PLATFORM 为 BLOCKED |
| 最终支持声明范围 | 待记录；仅满足 ADR 的精确 Linux Ubuntu LTS 实机门后方可声明；darwin 仅开发验证、Windows 未验证 |
| 证据文件索引 | 待记录；使用安全、可访问的 artifact 路径与 SHA-256，不归档秘密 |

### 3.1 制品索引

<a id="artifacts"></a>

| 制品类别 | 安全路径/引用（不含秘密） | SHA-256 | 采集时间/工具/版本 | 脱敏复核 |
|---|---|---|---|---|
| Go CI Run / test output | 待记录 | 待用户提供 | 待用户提供 | 待记录 |
| API/UI/WS 摘要或截图 | 待记录 | 待用户提供 | 待用户提供 | 待记录 |
| 日志/A2S/进程/文件系统现场 | 待记录 | 待用户提供 | 待用户提供 | 待记录 |
| promotion/backup manifest 与 hash | 待记录 | 待用户提供 | 待用户提供 | 待记录 |

## 4. Case Results

<a id="case-results"></a>

> 按 `docs/acceptance/M2-GO-PZ-MVP.md` §5 的每个矩阵行/子案例分别记录；同一 ID 多行测试必须逐个复制表格行，不能用单个聚合 PASS 掩盖失败子案例。证据位置要能定位到命令输出、请求/响应摘要、hash、截图或时间线；不得写秘密值。

| 测试 ID | 矩阵案例/子案例（精确名称） | 状态（PASS/FAIL/BLOCKED/NOT RUN） | 可判定结果 / 偏差 | 证据位置、时间戳、SHA-256 | Operator | Reviewer |
|---|---|---|---|---|---|---|
| M2-BUILD | 固定工具链/CI；archtest 负向探针 | 待记录 | 目标 commit 的 Run/各 step 与二进制 SHA；负向 import 注入失败并恢复干净树 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-API | REST/错误信封/校验；模板元数据与追加状态；route auth/Origin guard | 待记录 | 20 项 method/path/status/fields 与契约对应；状态字段只加法式追加，route→guard 覆盖完整 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-UI | 静态 UI 闭环；未登录/会话过期分支 | 待记录 | 浏览器版本与 10 调用点/13 endpoint 证据、未登录返回登录页 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-AUTH | login/session；logout replay/restart；Origin/CSRF | 待记录 | cookie 属性/TTL；logout 与重启旧 session 401；Origin×method 状态矩阵 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-WS | 鉴权/Origin；frames/replay/D3；cleanup/timeout/slow client；UI D3 frame compatibility | 待记录 | 握手 code 1008、帧对照、清理计数、deadline 与 UI 未把 error 帧误显示为 log | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-SINGLEWRITER | 双进程、各 owner 对账行、journal、recovery CLI、promote 与共享锁 | 待记录 | 409/只读/no child、RECOVERY_REQUIRED、锁 fencing 与审计现场均有逐案结果 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-SECRET | D8 mode；不静默 chmod；data boundary；redaction/argv/fingerprint | 待记录 | 权限位与授权修复；synthetic secret 未泄漏到日志/API/指纹/证据 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-INSTALL | success/failure/conflict/cancel/timeout/reap/retry | 待记录 | POST/GET task 状态、409 冲突、进程树退出与重试均逐案可判定 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-RESTART | graceful shutdown/restart；crash reconciliation；unexpected child exit | 待记录 | drain/reap/lock 清理及 restart 前后状态/进程对照，无自动 task recovery | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-CONFIG | atomic round-trip/corruption/write-failure/permissions/validation/restart | 待记录 | JSON/INI 读回；各故障点无假成功；权限、校验、持久状态逐案证据 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-MIGRATE | 状态布局/COMMITTING 全集/hash 双分支/rollback/failures/backup | 待记录 | 逐行引用 4.1；staging、first-promotion 无 prev、失败关闭与 backup hash 对账 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PROCESS | helper lifecycle；Linux vector；darwin/Windows 条件测试；真实 argv negative control | 待记录 | 制品证据选向量；runner helper 逐字节 argv、无注入；声明边界符合 ADR | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PZ-LIVE | 安装/A2S 或 marker/console/端口可达或 N/A/stop | 待记录 | 精确 build 的实时证据；端口逐个记录；无副作用授权与 offline 替代限制 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PLATFORM | 实机 OS/ABI/权限与支持范围；最终证据一致性签核 | 待记录 | 仅通过的精确 Ubuntu LTS/架构，或明确 BLOCKED；darwin/Windows 不声称支持 | 待用户提供 | 待用户提供 | 待用户提供 |

### 4.1 Promotion 中断/现场矩阵

<a id="migrate-layouts"></a>

> `M2-MIGRATE` 的每一个现场布局、匹配/不匹配校验和分支、首次提升无 prev 路径、失败注入和 rollback corridor 均须按计划逐行记录。可复制以下行；`state/`、`prev/`、`staging/` 只记录存在性、合成夹具标识与哈希，不记录敏感内容。

| Journal / 注入点 | state / prev / staging 现场 | 输入校验和 | 预期/实际恢复动作 | 状态 | 证据位置/hash | Reviewer |
|---|---|---|---|---|---|---|
| 待记录 | 待记录 | 待记录 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |

#### 4.1.1 Journal 转移中断

<a id="migrate-interruptions"></a>

| 转移边界/注入时点 | 计划中的 journal / 文件布局 | 故障注入方法 | 重启实际结果（journal 与路径） | 完整旧/新 manifest 校验 | 状态 | 证据/hash | Reviewer |
|---|---|---|---|---|---|---|---|
| 待记录 | 待记录 | 待记录 | 待记录 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |

#### 4.1.2 Rename/回滚失败注入

<a id="migrate-failures"></a>

| 步骤/错误点（state→prev / staging→state / prev→state） | 注入错误 | state/prev/staging 前后现场 hash | 预期失败关闭/回滚 | 实际结果与 journal | 状态 | 证据/hash | Reviewer |
|---|---|---|---|---|---|---|---|
| 待记录 | 待记录 | 待记录 | 待记录 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |

#### 4.1.3 Staging、同卷、权限与锁边界

<a id="migrate-boundaries"></a>

| 检查项 | 预期证据 | 实际结果 | 状态 | 证据/hash | Operator/Reviewer |
|---|---|---|---|---|---|
| dry-run source/target inventory 不变 | 待记录 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |
| staging 仅含 state，排除 .locks/.owners，同卷 | 待记录 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |
| D8 宽权限拒绝/显式修复审计 | 待记录 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |
| promotion 持 instance 与 .migration.lock | 待记录 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |
| server_files/Zomboid/steamcmd 未移动 | 待记录 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |

#### 4.1.4 Backup/Restore（M1-A 交叉证据）

<a id="backup-restore"></a>

| 备份/恢复步骤 | ADR §4.6 预期 | 实际结果 / hash | 状态 | 证据位置 | Reviewer |
|---|---|---|---|---|---|
| 默认备份范围与 manifest | state、Server/*.ini、instance metadata；按默认规则排除 saves/server_files | 待记录 | 待记录 | 待用户提供 | 待用户提供 |
| 逐文件重读 checksum | 全部与 manifest 相符；任一失败则拒绝 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |
| 损坏备份负向验证 | 篡改一字节必须失败、不恢复 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |
| restore staging → promotion → readback | 只经 promotion；读回一致 | 待记录 | 待记录 | 待用户提供 | 待用户提供 |
| 保留/清理政策 | 最近 3 份成功备份；无自动 prune | 待记录 | 待记录 | 待用户提供 | 待用户提供 |

### 4.2 PZ 实机与平台 live 子案例

| 测试 ID | 案例 | 状态（PASS/FAIL/BLOCKED/NOT RUN/N/A） | 精确 live 证据/引用 | 时间戳/来源/协议或 command 标识 | Operator | Reviewer |
|---|---|---|---|---|---|---|
| M2-PZ-LIVE | 真实 SteamCMD 安装 | 待记录 | 待用户提供 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PZ-LIVE | A2S readiness 或日志 fallback（仅一个适用） | 待记录 | 待用户提供 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PZ-LIVE | 真 PZ console 闭环 | 待记录 | 待用户提供 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PZ-LIVE | 端口协议级可达性（逐端口列行） | 待记录 | 待用户提供 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PZ-LIVE | 真 PZ 停止闭环 | 待记录 | 待用户提供 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PZ-LIVE | PORTS 无外部可达需求（仅符合计划 N/A 条件时） | 待记录 | Manifest 全部配置端口与仅本机目标证明；否则该行不适用且不得标 N/A | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PLATFORM | 同一实机 OS/内核/架构/二进制/权限与 rename 证据 | 待记录 | 待用户提供 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PLATFORM | 最终声明边界（Linux 精确目标；darwin/Windows 排除） | 待记录 | 待用户提供 | 待用户提供 | 待用户提供 | 待用户提供 |
| M2-PLATFORM | Manifest/准入证据自洽与签核 | 待记录 | 待用户提供 | 待用户提供 | 待用户提供 | 待用户提供 |

> 上方 M2-PZ-LIVE 与 M2-PLATFORM 行须按计划 §5 的所有子案例逐行复制，端口按配置逐个列明；不得聚合成一个 PASS。

## 5. Ready Oracle 时间线（仅真实 PZ 结果可满足 live 验收）

| 事件 | 时间戳（含时区） | 观察 / 证据 |
|---|---|---|
| T0：start 请求 / child 创建 | 待用户提供 | child-created 不等同于 ready |
| A2S 成功或已授权的逐字日志 marker | 待用户提供 | 目标 oracle/`SERVER_PORT` 或 marker 规则：待用户提供 |
| 成功 oracle 同时刻 PID 存活 | 待用户提供 | 待用户提供 |
| `/api/status.ready` 与 `.readiness` 摘要 | 待用户提供 | 只记录追加字段，不包含 Cookie/秘密 |
| T0 到 oracle 的耗时（必须 ≤60 秒） | 待用户提供 | 超时不自动终止进程；不能用 mock 代替 |
| 停止结果与进程树退出 | 待用户提供 | 仅在用户授权的 live case 记录 |

## 6. 秘密与清理确认

- [ ] 未将任何控制面/游戏口令、Cookie、session token、SSH 凭据写入本记录或 artifact。
- [ ] 测试秘密为临时合成值；日志、响应、状态、owner fingerprint 与 evidence 均已扫描确认未泄露（只记断言结果，不记原文）。
- [ ] argv 可能被同机同权限进程观察的 ADR 残余风险已记录，没有声称绝对机密。
- [ ] 未清理或删除隔离 root；如需清理，另行取得对该确切路径的授权并记录独立操作结果。
- [ ] 未触碰仓库 `data/`、生产数据或未授权目录。

## 7. 复核与最终结论

<a id="review"></a>

Reviewer 结论：待用户提供  
Critical/High 发现及闭环证据：待记录  
未解决问题/偏差/阻塞：待记录  
用户对最终结果/支持声明确认：待用户提供  
签署时间：待用户提供
