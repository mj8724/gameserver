# M2 Go + Project Zomboid MVP 验收计划

> **用途与状态：** 本文件是 M2 的执行计划，不是测试结果、部署授权或平台支持声明。验收对象为固定 Go 工具链上的目标提交，以及具名、隔离的 PZ 测试环境。用户已批准 ADR-001 r3；**截至本计划编写，M1-A switch-ready 尚无已记录的复核证据**，正式 M2 结果门尚未满足。目标 Linux 主机、PZ build 和 SteamCMD 版本尚待用户提供，实机项因此保持 **BLOCKED**。
>
> **规范优先级：** 已批准的 `docs/adr/ADR-001-go-backend.md`（ADR）高于本计划；`docs/migration/LEGACY-CONTRACT.md` 是 legacy wire/行为基线，但 ADR 登记的 D1–D8 是 Go 目标的明确变更，按 ADR 执行。本计划不得改变二者语义；遇到矛盾以 ADR 为准并先登记/修订 ADR，不以本计划默默覆盖。

## 1. 范围、一致性与结果判定

### 1.1 范围

覆盖 ADR §6.3 所列全部 14 个 M2 矩阵 ID：`M2-BUILD`、`M2-API`、`M2-UI`、`M2-AUTH`、`M2-WS`、`M2-SINGLEWRITER`、`M2-SECRET`、`M2-INSTALL`、`M2-RESTART`、`M2-CONFIG`、`M2-MIGRATE`、`M2-PROCESS`、`M2-PZ-LIVE`、`M2-PLATFORM`。离线兼容/故障注入使用合成夹具；只有获准的实机项才使用 SteamCMD/PZ 内容或目标主机。

本 M2 只验证所有权无法证明时失败关闭，不做 M3 的持久任务恢复/自动重试。M1-B 生产接管不在本验收授权内；未完成 M1-B 时，所有结果都必须标注“隔离数据根基线验收”，不得作为接管证据。darwin 仅开发/离线验证，Windows 未验证；本计划不会据此对 darwin 或 Windows 声称支持。

### 1.2 ADR/契约的落实与刻意细化

- 保持 ADR 的路由、字段追加规则、D1–D8、单写者位置/对账、state 子树 promotion、权限、秘密脱敏、启动向量和就绪判决；不把旧版缺失 Origin、logout replay 或 WS 未运行输入行为重新当作目标。
- `M2-MIGRATE` 将 ADR §4.3 的 `EXPORTED / IDLE` 合并格拆成两个独立测试现场，预期动作相同；另把同一 `COMMITTING × state 有 / prev 有 / staging 缺` 布局的 checksum 命中/不命中拆成显式子案例，覆盖 ADR 的两个结果分支，均不改变 ADR 语义。
- ADR §2.3 未规定 `/api/status.readiness` 的内部 schema。本计划只要求字段为追加字段、`ready` 与所选 oracle 一致，且不得伪造 readiness；**不擅自规定 `readiness` 的内部字段/枚举**。若实现或消费者需要固定 schema，须先由 ADR/契约补充。
- D2 将会话令牌升级为随机 `sid` + 签名 Cookie，而 LEGACY-CONTRACT 记录的是旧版 `unix_ts.signature` 格式及未来时间戳拒绝。本计划按 ADR 优先，不要求沿用旧 wire 编码；保留 12h 有效期/过期失效，若实现仍使用签名时间戳则也必须拒绝未来时间戳。
- ADR §4.2 有一处措辞歧义：一方面规定 `COMMITTING` 在首次 rename 前持久化，另一方面写“任何 rename 之前崩溃都只能观察到 VERIFIED 或更早”；§4.3 又明确列出 `COMMITTING × state 存在 / prev 缺失 / staging 存在`（屏障后、rename 前）并规定重试提升。本计划按更具体的屏障不变量和恢复表执行；验收暂以 §4.2 的持久化顺序及 §4.3 具体布局行为为准，建议下次维护 ADR 时澄清原句。不扩大 ADR 规定的恢复权限，其他状态仍失败关闭。
- D3 的错误帧与静态 UI 消费方存在契约差异：LEGACY-CONTRACT §4 记载 UI 的 WS handler 只消费 `type==="log"`，不会显示 `error` 帧；ADR 又要求 M2-WS 在未运行时新增 `{"type":"error","data":"server_not_running"}`。本计划不改旧 wire/UI，不把 error 当 log；由原始 WS client 断言 D3 帧、由 M2-UI 断言 UI 稳态且不把错误误显为日志。产品若要求 UI 呈现此错误，需另行明确前端范围，当前保持开放。
- 测试调用范围与副作用分开：M2-API/M2-UI 的静态和 synthetic 测试应优先用 `httptest`/注入 listener 不监听；任何本机 TCP listener、端口 bind、网络探测都是副作用门覆盖的动作，不因“loopback”自动豁免。实机 PZ/SteamCMD/目标网络均需专门授权。
- ADR 对 A2S 不支持时的日志回退没有另写独立超时。本计划将相同的 **启动后 60 秒**上限用于该回退，避免无界等待；在 Target Manifest 中必须预先登记实测 marker 与匹配规则。若 ADR 作者意图允许不同窗口，应先修订 ADR，而不是在执行时临场放宽。
- ADR/契约没有规定 PZ 实机 console 的具体无副作用命令，也没有规定外部网络可达范围。本计划增设两项可操作检查：只发送经该 build 核实、Manifest 预先记录并获准的无副作用命令；只探测用户在 Manifest 明确标为需外部可达的端口，逐个采用协议级证据；不自行要求开端口或改防火墙。
- 本计划增加的是可操作的夹具、注入点、证据字段、角色、PASS/BLOCKED 判定和复核步骤；这些细化不豁免 ADR 条件，也不授权真实副作用。

### 1.3 状态与测试类型

结果只能记为：`PASS`（该行所有断言均有证据）、`FAIL`（任一断言不成立）、`BLOCKED`（前置门/目标信息/授权缺失）、`NOT RUN`（尚未执行）。不得把 BLOCKED 或未执行写成 PASS。只有条件性行可记 `N/A` 并给出证据/原因：darwin/Windows 行因本次不声明支持可记 `N/A—无目标且不声明支持`；A2S 不支持时的日志 fallback 行仅在目标 build 已证明支持 A2S 时可记 `N/A—已有 A2S live 证据`。不得把必需的 Linux、CI、M1-A、授权或实机项记作 N/A。

类型：**unit**=无 OS/网络依赖的单元测试；**integration**=Go 服务、浏览器或多个真实本机组件配合但使用隔离夹具；**adversarial**=对抗输入、竞态或故障注入；**live**=真实 SteamCMD/PZ 或对目标主机/平台的实机验收。offline/fixture 结果绝不证明真实 PZ readiness 或平台支持。

## 2. 范围门槛、授权与执行顺序

M1-A 是切换就绪门槛，不等同于 M1 完成或 M1-B。按 ADR §6.3，M1-A 证据至少包括兼容矩阵、只读 dry-run、隔离副本备份/恢复、双进程单写者、promotion 中断恢复、非生产副本回滚/export-back 对账。备份依 ADR §4.6 写入 `<data_root>/backups/<instance>/<UTC ts>/` 并附 manifest（相对路径、大小、SHA-256、mtime）；默认含 `state/`、`Zomboid/Server/*.ini` 与实例元数据，存档及 `server_files/` 默认排除，显式 include 另授权。完成后逐文件重读并比对 hash；任一失败按 ADR 失败处理，改坏备份一个字节必须拒绝。恢复仅写入 staging 并走 promotion，保留最近 3 份成功备份，不自动清理，prune 仅显式 CLI。Go 写入后的非生产副本还须 `export-back`，再由 Python 基线读回并字段/字节对账。**截至本计划编制时，没有记录 M1-A 已通过的证据；它是本计划所有正式 M2 结果的入口门槛。** 若 M1-A 尚未完成，可在其自身验收范围内用 synthetic root 先执行重叠的兼容/单写者/promotion 案例并将记录归类为 M1-A 证据；reviewer 签署 switch-ready 前，这些记录不能作为正式 M2 PASS。完成 M1-A 后可经 SHA/环境/案例逐项核对复用，不必无故重复。

| 门 | 可判定准入条件 | 适用范围 / 未满足时处理 |
|---|---|---|
| M1-A switch-ready | 有独立 reviewer 复核并签署的 M1-A 证据包，覆盖 ADR §6.3 全部项目；只用获准的合成夹具/非生产副本；没有未闭环的 Critical/High。 | 所有正式 M2 结果均以前置通过为准，M1-B 不由此推定完成。若需先产出重叠 M1-A 证据，只能在 M1-A 工作项下记录，不能提前记作 M2 PASS。 |
| 目标提交的 Go CI | `.github/workflows/go.yml` 对**同一 Go commit** 全绿：`gofmt -l cmd internal` 无输出、`go vet ./...`、`go test ./...`、Ubuntu CI `go test -race ./...`、`go build ./...`，并包含 ADR §1.8/§2.1 的 `internal/archtest` 直接导入边界检查及其负向探针。证据 SHA 必须与受测 Go 二进制 SHA 对应。 | 所有矩阵最终 PASS 的前置。仅本地测试不能替代目标提交 CI/race 证据。编写时的 `BASELINE.md` 未记录 Go workflow green；对 `.github/workflows/go.yml` 只读核查显示已包含 `go test ./internal/archtest/...` step。此配置不证明目标 commit 的实际 workflow 运行/通过；须记录运行结果、commit SHA 与二进制 SHA 后此门才可 PASS。 |
| 完整 Target Manifest | 本文件 §4 的每个适用字段已由目标环境 operator 填实，reviewer 复核；不留“待用户提供”或未知占位值。PZ 是否支持 A2S、oracle、制品类型/启动向量均有证据。 | 任何实机验收、目标端口操作或真实内容安装之前。缺失时不得猜值、不得 live。 |
| 一次性隔离数据根 | 使用目标文件系统上的新建、绝对路径、专用 disposable root；清单确认空/仅含本次 synthetic fixture、非符号链接、不是 `data/`、生产根或其父/子共享路径；记录读写前 inventory。`data/` 与生产根只读、不触碰。 | 写配置、promotion、权限修复、install/start 等测试。删除/清理该根也要另行授权，不做隐式 cleanup。 |
| 每项真实副作用的单独授权 | 用户对具体动作、主机/绝对路径、网络/端口、时间窗口、可写目录及停止/恢复办法逐项明确授权，写入证据记录；仅有通用“可以测试”不够。 | SteamCMD 下载/更新、PZ 安装/启动/停止、目标或本机 loopback 网络/端口监听与探测、防火墙/路由更改、改动目标文件或权限等。任一项未授权即 BLOCKED。可优先用 `httptest`/内存 fake 避免 listener；不得默认为获准 bind。此计划撰写不执行这些动作。 |
| 操作与复核责任人 | 证据中填写环境 operator、Go 实现 owner、独立 reviewer；用户授权方对每项真实副作用单独确认。 | 人名/身份未提供时用角色，不得编造。实现者不能单独自签 live 平台支持结论。 |

**执行顺序：** (1) 核对 ADR/契约和 M1-A 材料；如 M1-A 未通过，只在 M1-A 工作项及其授权 synthetic root 下补其所需离线证据，并标记为 M1-A、不报 M2 PASS；(2) 固定目标 commit 并通过 M2-BUILD；(3) 完成并由独立 reviewer 签署 M1-A switch-ready；(4) 执行其余正式 M2 离线矩阵，并按案例/SHA 复用已签 M1-A 证据；(5) 填写并复核 Target Manifest；(6) 用户逐项授权后才执行 live；(7) 完成结果表与独立复核。任何门失败即停止受影响部分，不扩大到真实 `data/` 或生产环境。

## 3. 环境配置与证据约定

### 3.1 环境配置代号

每个矩阵行均在“环境/版本”栏指定配置；实际版本必须记录在 evidence 文件，不可用这些代号代替真实填写。

- **E-OFF**：目标 Go commit；固定工具链 `go1.27.1`（`docs/migration/TOOLCHAIN.md`）；临时 synthetic root/进程/文件系统夹具；不连 Steam、不下载游戏、不读写仓库 `data/`。记录实际 `GOOS/GOARCH`。CI 检查另以 workflow 实际 `ubuntu-latest` 版本/Run/SHA 为证据。
- **E-BROWSER**：E-OFF + 实际浏览器名称/完整版本（填入 Manifest）；保留 `static/index.html`。后端依赖用测试 double；不连接真实 PZ。
- **E-OS**：被测 OS 的实际版本/架构、Go commit/toolchain、所选向量与安全 helper。Linux 为本次唯一支持声明门；darwin/Windows 仅在具名 runner 可用时作条件性 adversarial 测试，不赋予支持声明。
- **E-LIVE（r4 修订，D-R4-3）**：已完成且获授权的 Target Manifest 中的精确目标与 Go commit、PZ 版本/Steam build ID、SteamCMD 版本、数据根、网络/端口和浏览器版本。任一值未知即 BLOCKED。不得拿其它主机或离线夹具补位。
  - **r4 声明对象**：Windows 10 x64（`DESKTOP-9M8FOG7`，10.0.19045.6466）+ launcher-descriptor 向量 + PZ 42.21（buildid 25485538）——实机五段闭环证据见 `docs/acceptance/evidence/M2-windows-live.md`；**声明引用必须包含当前唯一未收敛差异**（PZ 退出时重写 sandbox 文件；就绪窗口差异已修复并实测冷 40s / warm 34s）。
  - **未实测平台**：Linux（Ubuntu LTS）与 macOS 在本矩阵内**保持未验证**；不得因本行定义变化或元数据而声称其受支持。

### 3.2 证据路径与秘密处理

每次验收复制 §5 模板为该次证据记录，文件名把占位符替换为真实 OS 与 PZ build 的安全 slug；路径规则为 `docs/acceptance/evidence/M2-OS-PZBUILD-TEMPLATE.md`。矩阵 evidence 栏的 `#case-results`、`#artifacts`、`#migrate-layouts`、`#migrate-interruptions`、`#migrate-failures`、`#migrate-boundaries`、`#backup-restore` 均指证据文件内标题/子案例锚点；迁移/备份专项结果在同文件 §4.1 的对应小节逐行填写，`#artifacts` 是 §3.1 制品索引，不代表独立目录。命令/CI/浏览器输出、权限与 journal 快照、带时间的 API/A2S 观察及制品 `file`/shebang 结论均引用在记录中，需附摘要或安全 artifact 路径与 SHA-256。

证据不得包含控制面口令、游戏口令、Cookie、session token、SSH 凭据或真实秘密。使用测试生成的一次性哨兵值并在进程日志、API、owner fingerprint 和归档中脱敏；记录 `已验证未泄露=true/false`，不记哨兵原文、命令行秘密原文或其可离线校验摘要。`-adminpassword=<value>` 在实际 argv 中可被同机同权限进程观察是 ADR 明示的残余风险，不得描述为绝对机密。

## 4. Target Manifest（每次实机验收一份）

本节字段是 ADR §6.4 的最小信息集并加可审核的来源/授权信息；此处是本计划的 Target Manifest 权威位置。当前计划保留 `待用户提供` 模板值，不表示目标已知。每次实际执行时，operator 必须在本节对应的验收计划副本/修订中将本次 Manifest 全部填写并附来源；把填写后的同一份字段快照复制到对应 §5 evidence 文件，并记录二者版本/SHA 关联。**所有实时/实机操作都禁止在计划内 Manifest 完整、证据附齐、reviewer 复核及用户对每项副作用授权之前开始。** 未知值统一写 `待用户提供`；未知不等于通过。

```text
验收记录文件：docs/acceptance/evidence/M2-OS-PZBUILD-TEMPLATE（模板名不使用 `<`/`>`：Windows 无法检出含这些字符的路径）.md
验收类别：隔离数据根基线验收（不得填写为 M1-B/生产接管）
目标主机安全别名：待用户提供（不填 SSH 私钥/口令）
目标 Go commit SHA：待用户提供
Go 版本：go1.27.1（应与 docs/migration/TOOLCHAIN.md、go.mod、CI 一致；实际 go version 输出：待记录）
目标二进制 SHA-256：待用户提供
PZ 版本：待用户提供
Steam build ID：待用户提供
SteamCMD 版本/来源：待用户提供
OS/发行版/版本（Ubuntu LTS 的具体版本）：待用户提供
内核版本：待用户提供
架构（如 amd64/arm64，以实际输出为准）：待用户提供
隔离数据根绝对路径：待用户提供
隔离根 inventory/空目录与非生产关系证据：待用户提供
网络路径与测试来源/目标：待用户提供
SERVER_PORT / DIRECT_PORT（实际配置值）：待用户提供
防火墙状态及所需规则/是否获准变更：待用户提供
浏览器名称/版本（静态 UI 实测）：待用户提供
PZ 制品检查：ProjectZomboid64、ProjectZomboid64.exe、start-server.sh、StartServer64.bat 的存在性及 file/shebang/类型证据：待用户提供
实际选用启动向量及选择理由：待用户提供（只能由制品证据决定，不由文件名/模板猜测）
每个配置 PZ 端口的协议、预期网络可达范围、获准测试来源与协议级探测法（SERVER_PORT 若不支持 A2S 则记录经该 build 验证的游戏协议探测）：待用户提供
真实 PZ 实机 console 测试所用无副作用命令、适用 build 依据与授权：待用户提供
Readiness oracle：A2S（SERVER_PORT）/日志 fallback（二选一）：待用户提供
若选日志 fallback：目标 PZ build 不支持 A2S 的证据、逐字 marker、匹配规则及样例日志：待用户提供
用户授权记录：每项动作、范围/目录/端口/窗口、明确授权时间与引用：待用户提供
Operator / Go 实现 owner / Reviewer：待用户提供
Manifest 填写时间与复核结论：待用户提供
```

没有 PZ build/Steam build ID、精确 Ubuntu LTS/架构、绝对隔离根、网络/端口、防火墙状态、浏览器版本（UI 保留时）、制品证据或已选 oracle 时，`M2-PZ-LIVE` 和 `M2-PLATFORM` 一律 BLOCKED；不能以“稍后补”边跑边填。

## 5. 执行矩阵（含可判定 PASS 与责任人）

<a id="matrix"></a>

| Required M2 ID | 矩阵条目数（子案例） |
|---|---:|
| M2-BUILD | 2 |
| M2-API | 3 |
| M2-UI | 2 |
| M2-AUTH | 3 |
| M2-WS | 6 |
| M2-SINGLEWRITER | 13 |
| M2-SECRET | 4 |
| M2-INSTALL | 7 |
| M2-RESTART | 3 |
| M2-CONFIG | 6 |
| M2-MIGRATE | 30 |
| M2-PROCESS | 6 |
| M2-PZ-LIVE | 7 |
| M2-PLATFORM | 3 |
| **合计** | **95** |

> 上述仅统计各 canonical ID 的基本子案例行；`M2-MIGRATE` 的 `COMMITTING × state 有、prev 有、staging 缺` 行还必须跑校验和命中与不命中两个 fixture。端口可达性依每个配置端口逐行记录，M1-A 备份/恢复交叉证据必须与对应案例复核，不可因为引用同一 ID 而遗漏。

<a id="case-results"></a>
<a id="artifacts"></a>
<a id="migrate-layouts"></a>
<a id="migrate-interruptions"></a>
<a id="migrate-failures"></a>
<a id="migrate-boundaries"></a>
<a id="backup-restore"></a>

下表中的子案例共同使用其 canonical M2 ID；**每一个列出的断言均通过**才可把该 ID 记 PASS。contract 的旧版响应只允许 ADR/契约登记的差异（D1–D8 与追加字段），错误响应 envelope 仍为 `{"detail": <string>}`。

| 测试 ID | 案例 | 可判定 PASS 结果 | 类型 | 环境/版本 | 证据位置 | Owner |
|---|---|---|---|---|---|---|
| M2-BUILD | 固定工具链与目标提交 CI | `go version` 为 TOOLCHAIN 固定的 go1.27.1；目标 SHA 的 workflow 所有必需步骤全绿，SHA 与待测二进制一致；race 在 Ubuntu CI 通过。`gofmt` 无输出、vet/test/build 成功；archtest 直接导入规则与负向探针成功。 | integration | E-OFF；CI 的 go1.27.1 + Ubuntu `runs-on` 实际镜像版本 | `#case-results`、`#artifacts`（workflow URL/Run/SHA 与各 step 输出） | Go 实现 owner；reviewer 复核 |
| M2-BUILD | archtest 边界强制性负向探针 | 在一次性测试副本中注入 ADR 禁止的直接 import，archtest 必须失败并点名违规包；恢复干净工作树后测试通过。不得把被污染的副本合并。 | adversarial | E-OFF；Go 1.27.1；一次性工作树 | `#case-results`、`#artifacts`（干净/注入结果与 diff 摘要） | Go 实现 owner |
| M2-API | 20 个 API/挂载点与 legacy wire sweep | 对照 LEGACY-CONTRACT §2/§2.1–§2.4/§9，逐项断言 20 个挂载点的方法、路径、成功/错误状态码、既有响应字段名/类型与认证范围；`{"detail":<string>}` envelope 和 §2.3 指定错误字符串逐字一致。覆盖 StrictModel 未知字段 422、command 1..4096、months 1..24、workshop_id 数字、配置字段五类校验及 ports 1..65535；GET 带 `Content-Type: application/json` 可用，无模型 action POST 接受 `{}`。无无法解释的差异。 | integration | E-OFF；Go commit；synthetic state/templates | `#case-results`（附 20 项 checklist 与请求/响应摘要） | Go 实现 owner；验收操作员 |
| M2-API | 模板平台元数据与新增状态字段 | `/api/templates` 原样保留 `supported_os` 等模板元数据但不作为支持声明；`/api/status.ready`、`/api/status.readiness` 仅追加，不改变既有字段名/类型；`status` 枚举及 `INSTALLING` 覆盖和 `running` 语义保持，start body 不变。合成 readiness double 仅验证序列化追加、不提前伪报 ready；真实值另由 M2-PZ-LIVE 判定。 | integration | E-OFF；合成 GameReadiness double；Go commit | `#case-results`、`#artifacts`（脱敏 JSON golden diff） | Go 实现 owner |
| M2-API | API auth/Origin 边界完整性 | 对 LEGACY-CONTRACT §2 的所有 20 个挂载点逐一标记 public/protected/write；所有 protected route 经 `require_session`/等价 middleware，所有副作用方法覆盖 D1 Origin 缺失与跨源拒绝，只读 GET 缺 Origin 允许但跨源拒绝。提供 route→guard 单元表及请求级样本；不允许有未覆盖 route。 | unit + adversarial | E-OFF；测试路由注册表/handler middleware，不监听端口 | `#case-results`、`#artifacts`（route/guard 对照表、synthetic request 结果） | Go 实现 owner；reviewer |
| M2-UI | 静态 UI 登录与仪表盘闭环 | 从 `/` 加载仓库 `static/index.html`；浏览器通过登录进入仪表盘，状态轮询、配置字段、日志、安装/启停按钮及 WS 页面路径均可操作；核对 10 个调用点覆盖的 13 个 endpoint，配置/状态字段类型满足 UI，含 `install_task.progress` 为数字及密码字段为空/不回默认值；不以 Vue 重建替代。 | integration | E-BROWSER；浏览器完整版本记录；后端使用 fake installer/process | `#case-results`、`#artifacts`（浏览器版本、脱敏请求摘要/截图） | 验收操作员；Go 实现 owner |
| M2-UI | 未登录/会话过期界面分支 | 未授权 API 返回 `detail: "请先登录"`；UI 原有 `error.message === '请先登录'` 分支回到登录页，不显示已认证仪表盘；错误文案没有被重写成其它字符串。 | integration | E-BROWSER；合成会话；不连 PZ | `#case-results`（脱敏网络响应与页面状态） | 验收操作员 |
| M2-AUTH | 登录与 session 正常/失败路径 | 正确口令登录成功并设置 `gameserver_session` Cookie；Go D2 token 使用随机 `sid` + 签名，`sid` 只存服务端内存表（12h TTL）。检查 HttpOnly、SameSite Strict、Path、12h TTL 和配置驱动 Secure；错误口令为 401 指定 `detail`，未配置口令为 503 指定 `detail`，auth/status 未登录返回 authenticated=false；12h 到期后 session 无效；若实现使用签名时间戳，未来时间戳无效。用可控时钟验证 TTL 边界，不要求沿用 legacy `unix_ts.signature` wire 编码。 | unit + integration | E-OFF；注入 clock；仅测试口令 | `#case-results`、`#artifacts`（Cookie 属性脱敏，不含值） | Go 实现 owner |
| M2-AUTH | logout replay 与进程重启失效（D2） | 登录后保存仅存在内存中的测试 Cookie；logout 删除该 `sid` 后重放旧 Cookie 得 401 `{"detail":"请先登录"}`。服务重启后旧密钥/旧 sid 均失效；会话不写盘，不增加持久恢复行为。 | integration | E-OFF；同一隔离服务的可控重启 | `#case-results`（旧 Cookie 值绝不记录，仅记录结果） | 验收操作员；Go 实现 owner |
| M2-AUTH | Origin/CSRF（D1） | 同源副作用请求通过；POST/PUT/DELETE 缺 Origin 或跨源都为 403 `{"detail":"跨站请求已拒绝"}`；只读 GET 缺 Origin 可通过、跨源被拒；Host/Origin 对比包含端口。WS Origin 的 accept 前断言在 M2-WS。 | adversarial | E-OFF；请求头伪造；不连外网 | `#case-results`（方法/Origin/Host 与状态码矩阵） | Go 实现 owner |
| M2-WS | 接受前认证与 Origin 门 | 无/失效 session、缺失或跨源 Origin 时 WebSocket 不被 accept，关闭码 1008；同源有效 session 才能连接。 | integration + adversarial | E-OFF；本地 synthetic WS client | `#case-results`、`#artifacts`（握手/关闭码） | Go 实现 owner |
| M2-WS | 帧契约、回放与 D3 未运行输入 | 连接立即收到最近 100 行 `log`，新日志也为 `log`；JSON `input`/非 JSON 原文/ping→pong/未知帧按契约处理。实例未运行时 input 不转发、连接不断开并收到 `{"type":"error","data":"server_not_running"}`；REST command 仍为 400 `服务器未运行，无法发送控制台指令`；没有安装进度帧。 | integration | E-OFF；fake ProcessSupervisor；synthetic 1000 行环形缓冲 | `#case-results`（逐帧断言与 REST 对照） | Go 实现 owner |
| M2-WS | 断连取消与 listener 清理 | 客户端断开或服务 shutdown 后 sender context 已取消、listener 数回到基线、没有向已关闭连接继续写入；不以不稳定的全局 goroutine 数单独作为证据。 | integration + adversarial | E-OFF；可控 channel/计数器 | `#case-results`（取消确认与 listener 前/后计数） | Go 实现 owner |
| M2-WS | 读写超时与慢客户端隔离 | 注入不读帧/慢读客户端和无效/不结束的读帧；每个 WS read/write 均受已配置 deadline/context 限制，超时按约定关闭或取消该连接，不阻塞其他连接、HTTP 状态查询或受控 shutdown；断连后 sender 和 listener 清理。记录超时边界与两条连接的独立响应。 | adversarial | E-OFF；本地 synthetic WS clients；短可控 clock/deadline | `#case-results`（慢客户端/超时/第二连接/shutdown 时间线） | Go 实现 owner |
| M2-WS | D3 error frame 客户端互操作 | 按计划 UI 的现状核对：其 JS 只显示 `type==="log"` 的帧，不解析 D3 新增 `error` 帧。D3 仍须对 Go WS 原始 wire client 验证 `error/server_not_running` 且保持连接；UI 不因该新增帧而误显示日志或崩溃/重连循环，UI 不要求显示错误文本。只有产品后续要求 UI 明示该错误时，才另开前端范围决策。 | integration | E-BROWSER + E-OFF；静态 `static/index.html`；无真实 PZ | `#case-results`、`#artifacts`（脱敏 WS wire 与 UI 稳态） | 验收操作员；Go 实现 owner |
| M2-WS | UI 侧 D3 帧兼容分支探测 | 直接对照/测试 `static/index.html` WS `onmessage` handler：确认 `error` 帧没有进入 log 渲染，浏览器 console 无异常，连接不因未知帧自动关闭。该项只验证现有 UI 对追加帧的兼容，不要求 UI 新增提示；若当前 source 已无此行为则据证据报告差异，不修改文件。 | integration | E-BROWSER；仓库静态 UI；合成 WS server/frame | `#case-results`、`#artifacts`（只读 source 定位、脱敏 frame 与 console 摘要） | 验收操作员；Go 实现 owner |
| M2-SINGLEWRITER | 双独立进程争用同一实例 | 第一进程持外置实例锁；第二独立服务进程可提供只读查询，`/api/status.ownership="locked_by_other"`，变更 API 409 `instance owned by another process`；第二次 install/start 未创建任何子进程。用 PID/进程树和锁记录证明无双启动。 | adversarial | E-OS；Linux Ubuntu LTS 必跑；同一隔离根、两个 OS 进程 | `#case-results`、`#artifacts`（PID 计数、脱敏 owner/lock 状态） | 验收操作员；reviewer |
| M2-SINGLEWRITER | 启动对账：无记录、无匹配进程 | 持锁后无 owner record 且探测无匹配游戏/安装进程，服务正常进入可变更状态，无恢复误报。 | integration | E-OS；synthetic root 与进程表 fixture | `#case-results` | Go 实现 owner |
| M2-SINGLEWRITER | 启动对账：无记录但发现匹配进程 | 持锁后不存在记录但发现匹配进程，状态为 `RECOVERY_REQUIRED`，禁止 install/start，目录与进程均不被自动修复/清理。 | adversarial | E-OS；假匹配子进程 | `#case-results`（探测证据与无副作用核对） | Go 实现 owner |
| M2-SINGLEWRITER | 启动对账：owner PID 已死且无匹配进程 | 陈旧 owner 记录触发 `RECOVERY_REQUIRED`；install/start 被拒且不自动删除 owner。 | adversarial | E-OS；synthetic owner record | `#case-results` | Go 实现 owner |
| M2-SINGLEWRITER | 启动对账：PID 活着且指纹匹配 | 即便 PID 和脱敏 command fingerprint 匹配，也进入 `RECOVERY_REQUIRED`；不接管/杀进程、不启动第二个进程。 | adversarial | E-OS；受控假进程 | `#case-results`（指纹只记录脱敏形式） | Go 实现 owner |
| M2-SINGLEWRITER | 启动对账：PID 活着但指纹不匹配 | PID 复用/指纹不匹配时进入 `RECOVERY_REQUIRED`，变更被拒且不清理记录/进程。 | adversarial | E-OS；受控 PID/指纹 fixture | `#case-results` | Go 实现 owner |
| M2-SINGLEWRITER | 启动对账：session_token 不匹配 | 持锁后 token 不匹配触发 `RECOVERY_REQUIRED`；不 install/start，不改写 owner 以覆盖现场。 | adversarial | E-OS；synthetic owner record | `#case-results` | Go 实现 owner |
| M2-SINGLEWRITER | 启动对账：root 不匹配 | owner 记录绝对 root 与当前 root 不同则 `RECOVERY_REQUIRED`；任何变更均失败关闭。 | adversarial | E-OS；两个独立 synthetic roots | `#case-results` | Go 实现 owner |
| M2-SINGLEWRITER | 未取得锁时服务行为 | 第二实例服务可启动并只读；状态追加 `ownership="locked_by_other"`，所有变更请求 409 精确 detail；只读 API 可用；无 child、文件写或 owner 覆盖。 | integration + adversarial | E-OS；双进程、一次性数据根 | `#case-results` | 验收操作员 |
| M2-SINGLEWRITER | 启动时存在非终态 migration journal | `STAGED/VERIFIED/COMMITTING/ROLLING_BACK` journal 导致恢复必需；服务不创建/修复实例目录、不 install/start，也不删任何一侧。 | adversarial | E-OFF；synthetic journal/filesystem | `#case-results`（与 M2-MIGRATE 现场交叉引用） | Go 实现 owner |
| M2-SINGLEWRITER | promotion 后锁仍外置且相互排斥 | staging/manifest 不含 `.locks`、`.owners`；提升不搬锁/owner；提升后原持有者与第二持有者仍互斥；新 state 不带陈旧 owner 副本，外置 owner 只属于当前持锁者。 | adversarial | E-OS + synthetic promotion；Linux 必跑 | `#case-results`、`#artifacts`（rename 前后路径 inventory） | Go 实现 owner；reviewer |
| M2-SINGLEWRITER | CLI recovery 也受同一把实例锁保护 | `gameserver recover --data-root <隔离根> --instance <id>` 无法非阻塞取得实例锁时立即拒绝且不改任何 owner/journal/实例文件；取得锁后仅按 ADR 对账规则清理“PID 已死且无匹配进程”的 stale owner。发现存活匹配进程时默认拒绝；`--force` 必须二次确认、先展示 owner 与进程证据并写审计记录。全部行为由一次性 synthetic fixture 验证，不执行真实恢复。 | adversarial | E-OS；Linux 必跑；隔离 root、可控 helper PID 与 fake terminal | `#case-results`、`#artifacts`（lock 状态、文件 hash 前后、确认/审计摘要） | Go 实现 owner；reviewer |
| M2-SINGLEWRITER | 共享 SteamCMD/migration 锁串行化 | 不同实例共享目录写操作在 `.migration.lock` 持有期间互斥；提升同时持实例锁与数据根 migration lock；锁释放后可由唯一下一写者前进。 | adversarial | E-OS；多个假实例/进程 | `#case-results`（锁竞争时间线） | Go 实现 owner |
| M2-SECRET | POSIX 秘密目录/文件权限 D8 | Linux 创建/验证 `<data_root>/servers/`、`<data_root>/steamcmd/`、`<instance_root>/state/` 与 `<instance_root>/Zomboid/` 为 0700；`<instance_root>/state/instance.json`、`<instance_root>/Zomboid/Server/<name>.ini` 与 legacy export-back `<instance_root>/instance.json` 为 0600；实际 `stat` 证明。宽权限既存根允许只读但阻止 install/start/config 写；Windows ACL 不据此声称通过。 | integration | E-OS；Linux Ubuntu LTS；synthetic root | `#case-results`、`#artifacts`（仅路径/模式，不含内容） | 验收操作员；Go 实现 owner |
| M2-SECRET | 不静默修权限 | 对宽权限 synthetic 文件，服务不擅自 chmod；变更返回 `RECOVERY_REQUIRED`/明确失败。只有经隔离根与授权的 `fix-permissions` CLI 后，重读模式符合要求并留下审计结果。 | adversarial | E-OS；synthetic files；CLI 每次写入须在授权范围 | `#case-results`（前后 mode、审计摘要） | 验收操作员 |
| M2-SECRET | 持久秘密数据边界与 API/备份脱敏 | synthetic `instance.json`/受管 state 中仅保留应用所需的 `ADMIN_PASSWORD`，INI 中 `SERVER_PASSWORD` 按 legacy data boundary 存放，模式分别符合 0600；配置/status API 对 `SERVER_PASSWORD` 恒输出空串、`ADMIN_PASSWORD` 真值输出 null，日志/状态/WS/证据不含原值。备份可包含受管状态/INI，但 artifact/evidence 只留 hash/脱敏摘要，不外发秘密内容；控制面口令只来自运行环境，不写配置/日志/文件。 | adversarial + integration | E-OFF/E-OS；一次性合成哨兵；不使用真实口令 | `#case-results`、`#artifacts`（脱敏 API 对照、模式与 hash；不附文件明文） | Go 实现 owner；reviewer |
| M2-SECRET | Secret 脱敏、argv 与指纹（D7/D8） | 一次性合成秘密在 `-adminpassword=<value>` 中是一个 argv 元素；不经 shell 再解析；日志/API/配置响应/WS/owner fingerprint/证据均无原值；fingerprint 由 redacted argv 计算。允许并记录同权限进程可观察 argv 的 ADR 残余风险；不保存哨兵原文。 | adversarial | E-OS；安全 helper、合成秘密；无真实口令 | `#case-results`（仅未泄露断言与 redacted argv） | Go 实现 owner；reviewer |
| M2-INSTALL | 安装成功 | POST install 对首个请求返回契约 `202 {message,status:"INSTALLING"}`；fake Installer 成功后进度有界且单调，GET install 观察到 `COMPLETED`，busy/owner 清理且只创建预期 child；不触发真实 SteamCMD/下载。 | integration | E-OFF；fake Installer/child；合成 root | `#case-results`（POST/GET 脱敏摘要、事件序列/child 计数） | Go 实现 owner |
| M2-INSTALL | 安装失败 | 首个 POST 遵循既有异步接口接受语义；fake Installer 非零退出/模拟错误后 GET install 进入 `FAILED`、error 可见且脱敏，绝不呈成功；busy/安装所有权清除且后续状态查询/重试可进行。 | integration + adversarial | E-OFF；假命令退出/错误注入 | `#case-results`（POST/GET 状态、detail 与 busy/owner 摘要） | Go 实现 owner |
| M2-INSTALL | 并发/运行中冲突 | 已有安装任务时第二 install 返回 409 `已有安装任务在运行中`；游戏运行时 install 返回 409 `服务器运行中，请先关机后再更新`；两种情况下均无第二 child/额外写入。 | integration + adversarial | E-OFF；fake installer/process；并发请求 | `#case-results`（两分支状态码/detail/PID） | Go 实现 owner |
| M2-INSTALL | 受控取消（D4） | context cancel 或受控 shutdown 可取消任务，不新增公开 cancel route；子进程树终止并 `wait` 回收，install ownership 清理；状态不再表示安装中，且未把取消当作成功。 | adversarial | E-OFF；可控 fake 子进程树 | `#case-results`（cancel 时间线、所有 PID 已退出/回收） | Go 实现 owner |
| M2-INSTALL | deadline timeout（D4） | 超过 deadline 后同样终止整棵子进程树、wait/reap、释放 busy/owner；无存活 child、无假成功。 | adversarial | E-OFF；短测试 deadline、假子进程 | `#case-results` | Go 实现 owner |
| M2-INSTALL | 正常结束/取消后的 reap | 子进程正常退出、失败、取消、shutdown 各路径均调用 wait；在平台进程探测中 PID/后代均消失，无 zombie/句柄残留，状态可继续查询。 | integration + adversarial | E-OS；helper process；按 OS 记录版本 | `#case-results`（父子 PID 与 wait 结果） | Go 实现 owner |
| M2-INSTALL | 终止后 retry | 取消/timeout/reap 完成后可发起一次新 install；只启动一个新 child，不继承旧 progress/error，不因 busy 锁残留 409；并发第二次仍被拒绝。 | integration + adversarial | E-OFF；fake Installer；重复请求 | `#case-results`（task ID 序列/child 计数） | Go 实现 owner |
| M2-RESTART | 有序服务重启生命周期 | shutdown 先 drain/cancel 安装并 reap，再优雅停 PZ child、核验进程树已结束、清 owner 并释放锁；重启后单写者可重取锁、持久 JSON/INI 可读且未被重置；内存日志/会话/安装状态按契约丢失/失效。 | integration | E-OS；synthetic root + helper process；Go commit | `#case-results`（shutdown 顺序、前后 inventory、PID/lock） | 验收操作员；Go 实现 owner |
| M2-RESTART | 控制进程异常退出后的启动对账 | 构造服务异常退出与匹配存活 child/陈旧 owner；新服务不接管、不清理、不产生第二 child，呈 `RECOVERY_REQUIRED` 并拒绝变更。不开启 M3 持久任务恢复。 | adversarial | E-OS；isolated helper child；synthetic owner | `#case-results`、`#artifacts` | Go 实现 owner；reviewer |
| M2-RESTART | PZ child 意外退出观察与重启隔离 | 运行中的 helper child 自然非零退出时，Go 后端不伪称 running/ready；记录退出状态并允许只读状态查询，锁/owner 按 ADR 所有权对账规则保留或清理，不能被另一控制进程并发接管。重新启动只有在现有 M2 ownership gate 确认安全后才创建一个 child；不验证跨重启自动复活、持久重试或 M3 task recovery。 | integration + adversarial | E-OS；一次性 helper child；隔离 root | `#case-results`（退出码、状态、lock/owner、重启前后 PID） | Go 实现 owner；reviewer |
| M2-CONFIG | JSON/INI 原子写 round-trip | 修改/读回 JSON 与 INI 一致；临时文件→fsync→受校验 backup→rename→dir fsync→readback 顺序有注入点证据；保留一层嵌套合并、SERVER_NAME guard、INI 注释/未知键；清空 mods 写空串；API 脱敏。 | integration | E-OFF；synthetic JSON/INI；E-OS 文件系统复核 | `#case-results`、`#artifacts`（仅脱敏 diff/哈希） | Go 实现 owner |
| M2-CONFIG | target/backup 损坏与 RECOVERY_REQUIRED | 损坏 target 或缺失/截断/校验失败 `.bak` 时，恢复前先验完整性/可解析性；拒绝截断备份、返回失败或 `RECOVERY_REQUIRED`，不以不可信 backup 覆盖、不假成功、不删现场。 | adversarial | E-OFF；人为损坏的合成副本 | `#case-results`（字节/校验和现场与结果） | Go 实现 owner |
| M2-CONFIG | 原子写失败注入 | 对 temp 创建/写、file fsync、backup copy/fsync/校验、rename、dir fsync、readback 各注入失败；每次 API/CLI 明确失败或 recovery required，绝无 success 2xx；原/备份现场符合 ADR，失败不破坏证据。 | adversarial | E-OFF；注入文件系统/故障点；只对 disposable root | `#case-results`（故障点→HTTP/CLI→文件现场矩阵） | Go 实现 owner；reviewer |
| M2-CONFIG | 变更操作的 D8 权限门 | 宽权限受管路径下配置写请求被拒且不静默改权限；权限修复需显式 CLI 并有隔离根授权/审计；修复后可读回。 | adversarial | E-OS；Linux synthetic root | `#case-results`（与 M2-SECRET 权限证据互链） | 验收操作员 |
| M2-CONFIG | 配置校验边界与用户可编辑字段 | 使用合成模板逐项验证：只接受模板声明的 `user_editable` 键；string 类型/长度≤256 与 `SERVER_NAME` 正则、required password、number 不接受 bool 且遵循 min/max、boolean 严格 bool、port 键必须声明且 1..65535；拒绝 `SERVER_NAME` 路径穿越及未知字段。成功与失败均读回对比，不写真实 `data/`。 | unit + adversarial | E-OFF；合成模板和 disposable JSON/INI | `#case-results`（规则→输入类→状态/detail 与 readback） | Go 实现 owner |
| M2-CONFIG | INI/状态读取兼容与重启持久化 | 使用代表性的合成 Python-era `instance.json` 测一层嵌套默认值合并、SERVER_NAME guard；INI 只更新受管键，保留注释/未知键、空 Mods 写空串。关闭并重启 Go 服务后 state/INI 仍可读回一致；不把内存日志/安装任务/会话写成持久数据。 | integration | E-OFF；Python-era 合成夹具；disposable root | `#case-results`、`#artifacts`（输入/输出脱敏 hash 与 diff） | Go 实现 owner |
| M2-MIGRATE | IDLE × 任意现场 | `journal=IDLE`，state/prev/staging 任意；按 ADR 丢弃 staging（幂等重建），不删除 state/prev，journal 停在 EXPORTED；没有服务自动创建/修复目录。 | adversarial | E-OFF；synthetic promotion fixture；不读写 `data/` | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | EXPORTED × 任意现场 | `journal=EXPORTED`，state/prev/staging 任意；与 ADR 同：幂等丢弃 staging、保留 state/prev，停在 EXPORTED。 | adversarial | E-OFF；synthetic promotion fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | STAGED × state 原状、当前 promotion prev 尚无、staging 有 | 合法 staging 校验通过则进入 VERIFIED；夹具中的 staging 不匹配时按 ADR 丢弃 staging、源/target 不被改写。分别构造 target state 存在与首次提升时 target state 缺失两种原状；`prev/—` 解释为当前 journal sequence 尚无 prev target，历史 `prev/<instance>.<seq>` sibling 可保留且不得覆盖。 | adversarial | E-OFF；checksum 可控 fixture；含/不含 state 与历史 prev sibling 的 disposable root | `#migrate-layouts`（journal sequence、state/current/历史 prev 路径/hash） | Go 实现 owner |
| M2-MIGRATE | VERIFIED × state 有、当前 promotion prev 尚无、staging 有 | 幂等丢弃 staging（以便重建），journal 仍 VERIFIED；state checksum 不变，不触碰 server_files/Zomboid。`prev/—` 解释为当前 journal sequence 尚无 prev target；历史 prev sibling 可保留且不得覆盖。 | adversarial | E-OFF；synthetic state 与 manifest；带历史 prev sibling 的 disposable root | `#migrate-layouts`（journal sequence、当前/历史 prev 路径/hash） | Go 实现 owner |
| M2-MIGRATE | VERIFIED × state 缺、prev 无、staging 任意 | 与 ADR 一致：`state/` 缺失且无 `prev/` 来源属于未列出的异常现场；严格进入 `RECOVERY_REQUIRED`，禁止服务自动重建实例目录或自动删除/覆盖 staging。 | adversarial | E-OFF；synthetic missing-state fixture | `#migrate-layouts`（state/prev/staging 前后 inventory） | Go 实现 owner |
| M2-MIGRATE | VERIFIED × state 缺、prev 有、staging 有 | 异常现场严格转 `RECOVERY_REQUIRED`；不自动 rename、删除或重建任一侧。 | adversarial | E-OFF；合成三路径现场 | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | COMMITTING × state 缺、prev 有、staging 缺 | 执行 `prev → state` 回滚并记 ROLLED_BACK；rename 消费 prev，`journal.last_rolled_back_from` 留审计来源；无数据删除。 | adversarial | E-OFF；模拟第一次 rename 后中断 | `#migrate-layouts`（路径/hash/journal） | Go 实现 owner |
| M2-MIGRATE | COMMITTING × state 有、prev 有、staging 缺 | state 校验和等于新 manifest 且完整才前滚记 COMMITTED；哈希不符则 `RECOVERY_REQUIRED`；两种样本都执行，不依据目录存在 alone。 | adversarial | E-OFF；新 manifest 命中/不命中两组 fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | COMMITTING × state 有、prev 有、staging 有 | 两侧同时存在必须 `RECOVERY_REQUIRED`；不自动删除 state/prev/staging 任一侧。 | adversarial | E-OFF；合成三路径现场 | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | COMMITTING × state 有、prev 缺、staging 有 | 覆盖 COMMITTING 屏障后/步骤 1 尚未执行或失败现场；按规则重试步骤 1→2。成功后新 state 校验和匹配；任一步失败则按 §4.2 失败关闭，不删任一侧。 | adversarial | E-OFF；rename/fault-injection fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | COMMITTING × state 缺、prev 有、staging 有 | 覆盖两次 rename 之间；staging 校验通过才前滚到 state/COMMITTED；校验不通过则 `RECOVERY_REQUIRED` 且保留 prev/staging。 | adversarial | E-OFF；有效/无效 staging 两组 fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | COMMITTING × state 缺、prev 缺、staging 有（首次提升，无 prev） | staging hash 与新 manifest 匹配则首次前滚到 COMMITTED；不匹配则 `RECOVERY_REQUIRED` 并保留 staging。没有 prev 时绝不进入不适用的 ROLLING_BACK/rename 回滚。 | adversarial | E-OFF；首次 promotion fixture（无旧 state/无 prev） | `#migrate-layouts` | Go 实现 owner；reviewer |
| M2-MIGRATE | COMMITTING × state 有、prev 缺、staging 缺 | 无可证实的新状态或回滚源，进入 `RECOVERY_REQUIRED`；不创建空 state、不假定成功。 | adversarial | E-OFF；合成三路径现场 | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | ROLLING_BACK × state 缺、prev 有、staging 任意 | 继续 `prev → state`，记录 ROLLED_BACK 和消费的 prev 来源；保留 staging，不做额外删除。 | adversarial | E-OFF；rollback corridor fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | ROLLING_BACK × state 有、prev 缺、staging 任意 | 只有 state 内容是完整旧版本时幂等记 ROLLED_BACK，不再 rename；其它内容不得伪装为成功，应进入 recovery-required。 | adversarial | E-OFF；旧版本命中/损坏两组 fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | ROLLING_BACK × state 有、prev 有、staging 任意 | 不一致现场 `RECOVERY_REQUIRED`；不自动删任一侧。 | adversarial | E-OFF；rollback corridor fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | ROLLED_BACK × state 有、prev/staging 任意 | 无操作、重复执行幂等；prev 可不存在（回滚 rename 已消费），审计来源由 `last_rolled_back_from` 保留。 | adversarial | E-OFF；重复启动/recovery fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | ROLLED_BACK × state 缺、prev/staging 任意（异常现场） | ADR §4.3 仅将 `state/` 存在列为无操作行。state 缺失时按失败关闭规则记 `RECOVERY_REQUIRED`，不根据残留 prev/staging 猜测并自动修复。 | adversarial | E-OFF；synthetic missing-state fixture | `#migrate-layouts`（不自动修复/删除断言） | Go 实现 owner |
| M2-MIGRATE | LEGACY_ACTIVE × state 缺、prev 有、staging 任意 | 识别有意回滚终态，无操作；不会把它误报损坏或自动重新提升；重提必须从 STAGED 开始。 | integration | E-OFF；synthetic legacy rollback fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | COMMITTED × state 有且校验通过、prev 有、staging 缺 | 重复 promotion 幂等无操作，state checksum 不变、prev 不覆盖；不复制/移动 `server_files/`、`Zomboid/`、`steamcmd/`。 | integration + adversarial | E-OFF；checksum/metadata fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | COMMITTED × state 校验失败 | 损坏即 `RECOVERY_REQUIRED`，不自动回滚旧 prev、不丢弃现场。 | adversarial | E-OFF；损坏 state fixture | `#migrate-layouts` | Go 实现 owner |
| M2-MIGRATE | COMMITTED × state 缺失 | ADR §4.3 只将 state 存在且校验通过定义为幂等现场，缺失不满足 commit 证明；失败关闭进入 `RECOVERY_REQUIRED`，不使用 prev 自动替代。 | adversarial | E-OFF；synthetic missing-state fixture | `#migrate-layouts`（journal 与完整路径 inventory） | Go 实现 owner |
| M2-MIGRATE | journal 边界中断与 rollback corridor | 在每个状态转换边界及 COMMITTING 首次 rename 前屏障、步骤 1 后、步骤 2 后、步骤 1/2 失败处注入进程终止；重启严格落入上列布局规则。终态须与 ADR 定义的完整 `COMMITTED`、`ROLLED_BACK` 或有意 `LEGACY_ACTIVE` 一致，来源 state/legacy 数据按 manifest 或字段对账可证；rollback 的 `prev→state` 消费语义与 LEGACY_ACTIVE/export-back→Python 读回字段对账均通过。 | adversarial | E-OFF；非生产 synthetic copy；只模拟进程中断，不做真实掉电 | `#migrate-interruptions`（转换/注入点/恢复现场表） | Go 实现 owner；独立 reviewer |
| M2-MIGRATE | 步骤 1 rename(state→prev) 失败 | 注入 ENOTEMPTY、权限或跨卷等失败；journal 标为 `RECOVERY_REQUIRED`，原 target state 保持原状，prev 只允许按 journal 记录为不存在或完整存在；任何一侧均不删除或覆盖，install/start 保持阻断。 | adversarial | E-OFF；合成文件系统故障点；只写 disposable root | `#migrate-failures`（错误类型、state/prev hash、journal 与无副作用断言） | Go 实现 owner；reviewer |
| M2-MIGRATE | 首次提升 rename(staging→state) 失败（无 prev） | 在无既有 state/prev 的 fixture 注入 staging→state rename 失败；无 prev 时不进入不适用的 ROLLING_BACK；保留 staging、source 不变且失败关闭为 `RECOVERY_REQUIRED`，要求人工处理。 | adversarial | E-OFF；首次 promotion、fault injection；只写 disposable root | `#migrate-failures`（state/prev/staging hash、journal、未删除断言） | Go 实现 owner；reviewer |
| M2-MIGRATE | 步骤 2 rename(staging→state) 失败且立即回滚成功 | 注入步骤 2 rename 失败；立即 `prev→state` 成功后记 `ROLLED_BACK`，state 是完整旧版本、prev 已被消费且 `journal.last_rolled_back_from` 记录来源；不丢弃 staging 或其它现场。 | adversarial | E-OFF；合成 rename 故障；有旧 state/prev 的提升 | `#migrate-failures`（故障点、旧 state hash、journal 来源字段） | Go 实现 owner |
| M2-MIGRATE | 步骤 2 失败且回滚 rename 也失败 | 注入步骤 2 和 `prev→state` 回滚双失败；结果为 `RECOVERY_REQUIRED`，完整旧数据仍留在 prev，staging/target 现场不自动清理、不得假报成功。 | adversarial | E-OFF；合成双 rename 故障；只写 disposable root | `#migrate-failures`（prev/staging/state hash 与 journal） | Go 实现 owner；reviewer |
| M2-MIGRATE | staging 边界、同卷与锁 | staging 与 target 同卷；staging/manifest 仅含 state 内容且排除 `.locks/.owners`；dry-run 对 source/target 均只读且二者 inventory/hash 不变，dry-run manifest/hash 可复核；VERIFIED 前重新校验 source 未变；server_files/Zomboid/steamcmd 仅记录元数据、不搬动；提升同时持实例与 `.migration.lock`。 | adversarial | E-OS；一次性 synthetic filesystem | `#migrate-boundaries`（设备号/目录清单/源与 target 前后 hash/manifest/锁） | 验收操作员 |
| M2-MIGRATE | 仅因权限宽松拒绝提升 | 对 synthetic 源目录/JSON/INI 设置违反 D8 的 mode 后，dry-run 可报告但不修改源；导入/提升被阻止，返回可判定的权限项与修复指引，不静默 chmod 或写入 staging/target。执行一次仅针对 disposable root 的授权 `fix-permissions` 后，模式实测符合 ADR §1.7，重新校验后才允许继续 promotion；记录前后 stat 和 source hash。 | adversarial | E-OS；Linux synthetic root；CLI 写权限需用户逐项授权 | `#migrate-boundaries`（源 hash、前后 mode、拒绝原因、授权与复验） | Go 实现 owner；reviewer |
| M2-MIGRATE | 备份/损坏拒绝/恢复全链路（M1-A 交叉证据） | 对隔离 synthetic 副本按 ADR §4.6 备份 `state/`、`Zomboid/Server/*.ini`、实例元数据；逐文件重读 SHA-256 与 manifest 一致。人为篡改一个字节的备份必须被拒绝。完整备份只恢复进 staging 并走 journal promotion，读回逐项一致；默认不含 saves/server_files，未显式获授权不 include。最近 3 份保留，任何 prune 只可显式 CLI。 | integration + adversarial | E-OFF；合成数据根与一次性备份副本；只对 disposable tree 写入 | `#backup-restore`、`#artifacts`（manifest/hash/损坏拒绝/promotion 后读回） | Go 实现 owner；reviewer |
| M2-PROCESS | 进程生命周期 helper | 用安全 helper 验证启动、参数传递、stdin/stop、graceful→force、wait/reap 和进程树清理；该测试不启动 PZ；模板 `environments.*` 的 executable/start_arguments 不进入 argv 构造路径。 | integration + adversarial | E-OS；Linux 必跑；safe helper 与 synthetic template | `#case-results`（argv redacted、父子 PID、模板恶意元数据断言） | Go 实现 owner |
| M2-PROCESS | Linux Ubuntu LTS 启动向量对抗 | 先将目标 PZ 制品 `ProjectZomboid64`/`start-server.sh` 的 `file`/shebang 检查写入 Manifest；若有可直接执行的 binary，使用 direct exec；否则才测 `/bin/bash start-server.sh` + 独立 argv 数组。所选实际向量必须由制品证据决定；safe helper 接收 ADR §5.3 中列出的全部特殊字符、空格、换行、`$(...)`、`%VAR%` 后，argv 与预期逐字节一致；无额外命令/参数分裂；`-adminpassword=<synthetic>` 是单 token 且脱敏。失败则不得声明 Linux 支持。 | adversarial | E-OS：Manifest 中精确 Ubuntu LTS/arch；实际选择向量；Go 1.27.1 | `#case-results`、`#artifacts`（制品类型证据、helper 接收结果；无哨兵原文） | 验收操作员；reviewer |
| M2-PROCESS | darwin 启动向量对抗（不作支持声明） | 若提供具名 darwin runner，则先按实际制品检查选择 direct exec；只有无法直接执行时才按 ADR Linux/macOS 次选规则测试 `/bin/bash start-server.sh`。helper 对全套恶意参数逐字节相等、无注入且模板字符串未被解析；否则记 `N/A—无目标且不声明支持`。任何结果只表示开发验证，不表示服务端支持。 | adversarial | E-OS；darwin 具体版本/架构须实际填写；未提供则不执行 | `#case-results`（含 N/A 理由或该 OS 的完整 helper 证据） | Go 实现 owner；reviewer |
| M2-PROCESS | Windows 启动向量对抗（不作支持声明） | 若提供具名 Windows runner，先检查实际制品类型（实测 PZ 专用服务端包不含 `ProjectZomboid64.exe`，只有 `ProjectZomboid64.json` + `jre64\bin\java.exe`）：含 `ProjectZomboid64.exe` 时才用 direct-executable，否则使用 ADR §5.1 第三向量 `launcher-descriptor`（解析厂商描述→代码侧 vmArgs 白名单→类型化 argv 直执内置 JRE）。**禁止 `.bat`/`cmd.exe` 回退**（负向测试：记录型 runner 断言被执行可执行文件集合 == {`jre64\bin\java(.exe)`}）；相同恶意值逐字节进入 argv、无额外命令、秘密不入日志；描述文件单字节篡改必须被拒。否则记 `N/A—无目标且不声明支持`。不外推 Windows 支持。 | adversarial | E-OS；Windows 具体版本/架构须实际填写；未提供则不执行 | `#case-results`（含 N/A 理由或该 OS 的完整 helper 证据、向量与白名单证据） | Go 实现 owner；reviewer |
| M2-PROCESS | OS 启动向量 negative control | 确认 OS 对抗用例的“helper 实际收到的 argv”来自该 runner 上由启动适配器创建的真实 helper child，而不是将预期 argv 直接传给断言；对该 child 收到的字节序列与每个 fixture 输入逐元素、逐字节相等，并证明 shell 元字符未触发额外进程/命令。Linux Ubuntu LTS 为必测；darwin/Windows 按对应行的条件性范围与声明限制。 | adversarial | E-OS；对应具名 OS/架构、启动向量、编译后的 safe helper | `#case-results`、`#artifacts`（helper pid/父 pid、实际 argv 的安全编码比较、额外进程观察） | Go 实现 owner；reviewer |
| M2-PROCESS | 环境变量与模板插值隔离 | 用合成 user-controlled 值覆盖 `environments.*` 和模板 `start_arguments`，证明它们既不决定 executable/argv，也不进入进程创建的环境变量；只继承服务进程环境并显式覆盖 ADR 允许的必需变量（例如依据平台向量的 `LD_LIBRARY_PATH`），不把用户可控值注入 env。记录 helper 看到的 env allowlist/差异和 argv，敏感值不落日志/证据。 | adversarial | E-OS；各支持声明平台 runner 与 safe helper；synthetic template | `#case-results`、`#artifacts`（模板 payload、parent/child env redacted diff、argv 对照） | Go 实现 owner；reviewer |
| M2-PZ-LIVE | 真实 SteamCMD 安装 PZ build | 经单项用户授权后在隔离 root 使用 Manifest 中精确 SteamCMD 安装契约模板所列 PZ app id `380870`；SteamCMD 命令退出成功，实际 Steam build ID/PZ 版本与 Manifest 一致，目标文件 inventory 与制品检查可复核，安装目录仅在隔离 root。分支/版本选择须由 Manifest/用户提供，不猜版本。证据是带时间的 SteamCMD 结果、app/build 标识和目录/hash 摘要。**fake Installer/离线夹具绝不能替代此 live case。** | live | E-LIVE；Steam app id 380870；SteamCMD 版本/分支/build 以实际 Manifest 为准 | `#case-results`、`#artifacts`（SteamCMD 输出脱敏、app/build ID、隔离目录 inventory） | 目标环境 operator；用户授权；独立 reviewer |
| M2-PZ-LIVE | 真实启动与就绪 oracle（r4：判定对象=声明目标） | 对声明目标（Windows 10 x64 + descriptor 向量）在启动后向实际配置 `SERVER_PORT` 发出 A2S/日志 marker 探测；记录 start/T0、查询目标/时间、同刻 PID 存活。**实测（2026-09-29）**：冷启动 40s、warm 34s 达 `ready=true`（默认 60s 窗口内；曾因 `Logs(limit<1)` 返回空 + 窗口硬编码的两处缺陷误判为超窗，已修复）；窗口可用 `GAMESERVER_READINESS_TIMEOUT` 由 Manifest 放宽 |
| M2-PZ-LIVE | A2S 不支持时的日志 fallback | 仅当 Manifest 在运行前记录该 PZ build 不支持 A2S 的证据、精确可复现 marker 与匹配规则时适用；marker 在 T0+60s 内出现且当时进程存活，API `ready` 才可为 true。若支持 A2S，此行记 N/A 并引用 A2S 证据，不可借 fallback 放宽失败。**本地模拟日志不能替代目标 PZ 实机 marker。** | live | E-LIVE；仅 Manifest oracle=日志 fallback 时执行 | `#case-results`、`#artifacts`（build、逐字 marker/规则、带时间原始日志片段、PID/API 观察） | 目标环境 operator；reviewer |
| M2-PZ-LIVE | 真 PZ 控制台闭环 | PZ 实机 ready 后，使用 Manifest/evidence 中预先注明、经该 build 核实且获准的无副作用 console command，经真实 REST/WS 控制路径发送；收到成功响应，且匹配的服务端回显/日志或 WS log 可关联到该命令；无授权命令不发送。证据含命令标识（不含秘密）、请求/帧结果、对应日志时间线。**fake process 或 mock 日志不能替代此 live case。** | live | E-LIVE；真实 PZ build、Manifest 中已核实命令及获准控制面 | `#case-results`、`#artifacts`（脱敏 REST/WS 与服务端日志时间线） | 目标环境 operator；用户授权；独立 reviewer |
| M2-PZ-LIVE | PZ 端口可达性 | 从 Manifest 中已授权的测试来源对所有明确标为需外部可达的配置 PZ 端口做目标协议级探测；若该 build 支持 A2S，`SERVER_PORT` 要收到成功 A2S info；若 Manifest 证明不支持 A2S，则对 `SERVER_PORT` 使用预先登记、该 build 支持的游戏协议客户端/连接验证（日志 marker 只证明 readiness，不证明网络可达）；其他端口以各自协议可验证的连接或游戏客户端观察为准。证据记录来源/目标/协议/端口、逐项预期与观测、时间戳和防火墙状态；只有 socket bind、端口扫描的 UDP “open” 猜测或本机回环结果不算外部可达。目标网络探测不可由 mock、容器或离线夹具替代。 | live | E-LIVE；按 Manifest 的实际端口/协议/防火墙；逐项授权网络探测 | `#case-results`、`#artifacts`（按端口的协议级结果、来源与时间戳） | 目标环境 operator；用户授权；独立 reviewer |

| M2-PZ-LIVE | PORTS 无外部可达需求（仅符合计划 N/A 条件时） | 若 Manifest 中每个配置端口都明确为仅本机、无外部可达需求，证据逐项列出端口/范围；reviewer 复核后该端口可达性子案例为 N/A。只要任一端口需外部可达，则此 N/A 子案例不适用，必须对需外部可达端口经授权做协议级实机探测；offline/mock 不可替代，未授权/无外部来源记 BLOCKED。 | live | E-LIVE；逐端口网络需求来自 Manifest | `#case-results`、`#artifacts`（仅本机范围证据或逐端口 BLOCKED/探测证据） | 目标环境 operator；reviewer |
| M2-PZ-LIVE | 真 PZ 停止闭环 | 获准的实机控制/就绪检查完成后验证 stop 成功、PID/进程树退出、锁/owner 清理；`status/running/start` 语义未被 readiness 字段改变。证据包括带时间 API/日志摘要、启动向量、停止结果及进程退出。**假进程 lifecycle 不能替代此 live case。** | live | E-LIVE；仅隔离根和获准端口 | `#case-results`、`#artifacts`（脱敏 API/日志、PID/锁前后状态） | 目标环境 operator；用户授权；独立 reviewer |
| M2-PLATFORM | 平台资格（r4：声明对象而非 Linux 专用） | 在声明目标上记录 OS 标识（Windows：`winver`/`os-version`）、架构、运行二进制 commit/toolchain/hash、数据根权限/锁与原子写验收结果；确认目标是 Manifest 声明对象；Linux 资格行在 Ubuntu 实机前保持未验证 |
| M2-PLATFORM | 平台声明边界 | 结果报告只对通过本矩阵的精确 Linux Ubuntu LTS 目标作有证据的陈述；darwin 明确“开发验证”、Windows 明确“未验证”，不因模板 `supported_os` 元数据或交叉编译而声称支持。若 Linux live 门失败，不发布 Linux 支持声明；支持结论须由本次具名目标的 M2-PZ-LIVE 与 E-LIVE 主机/平台证据支持，离线测试、CI、容器和交叉编译均不能替代。 | live | E-LIVE + ADR-001-decision.md 的平台决策 | `#case-results`、`#review`（reviewer 结论、实机证据引用与明确排除项） | 独立 reviewer；用户 |
| M2-PLATFORM | Manifest/准入证据自洽与签核 | 执行版计划中的 Target Manifest、对应 evidence 文件快照、CI/二进制 SHA、逐项授权、现场结果及 reviewer 签核相互一致；没有必填未知字段、凭空假设、范围外 side-effect 或 offline→live 推断。最终支持声明仅限同一具名 Linux Ubuntu LTS/架构且 M2-PZ-LIVE 与 Linux M2-PROCESS 必测通过的结论；否则记 BLOCKED/FAIL，不发布支持声明。 | live | E-LIVE；本次完整计划副本与证据包，reviewer 交叉核对 | `#review`、`#artifacts`（Manifest/证据 SHA 关联、授权索引、排除项与签核） | 独立 reviewer；用户 |

**此 live case 的替代规则：** 不可由 offline/mock 替代；只有当该实例每个配置端口都在 Manifest 明确为“无外部可达需求/仅本机”时，才可记 `N/A—Manifest 明确为无外部可达需求` 并由 reviewer 复核。任一端口要求外部可达时须获授权做协议级实机探测；未授权或无外部来源则 BLOCKED，不记 PASS/N/A。


### 5.1 `M2-MIGRATE` 现场核验说明

上表覆盖 ADR §4.3 的逻辑恢复现场：IDLE、EXPORTED、STAGED、VERIFIED、COMMITTING 七种 ADR 列出的 `state/ × prev/ × staging/` 布局、ROLLING_BACK 三种走廊、ROLLED_BACK、LEGACY_ACTIVE、COMMITTED。计划表将同一 `COMMITTING × state 有/prev 有/staging 缺` 布局的 checksum 命中/不命中作为同一个子案例的两个夹具运行，不重复计矩阵 ID 行。表中 `prev/` 指 journal 当前 promotion sequence 对应的 prev target；历史 `prev/<instance>.<seq>` sibling 单独记录并不得覆盖。COMMITTING 每个布局的动作必须与 ADR §4.3 逐行一致；state checksum 分支需分别构造匹配与不匹配输入。额外列出的缺失 state 布局按 §3.2 强制失败关闭，并非 ADR 新增的自动恢复分支。每个 `RECOVERY_REQUIRED` 行都要求保留现场，不把“进程能启动”当作恢复成功。另独立注入 §4.2 步骤 1、步骤 2 和回滚 rename 失败；首次提升无 prev 时只允许校验前滚或失败关闭。备份行同时作为 M1-A 证据候选，正式 M1-A 完成后按 §2 复核/交叉引用。中断测试用 disposable synthetic fixture 和可重复故障点，不对生产根做 kill/power-cut 演练。

### 5.2 `M2-PROCESS` 对抗字符集

传给 safe helper 的测试数据须覆盖 ADR §5.3 全集：`'`、`"`、反引号、`$`、`;`、`&`、`|`、`(`、`)`、`<`、`>`、`^`、`%`、`!`、空格、换行、`$(...)`、`%VAR%`。逐字节比较实际收到的 argv 与预期；各值只在隔离内存/临时夹具中使用。除确认没有被执行/拆分外，日志、status、owner 和 evidence 中均不得出现秘密原文。

## 6. Ready oracle（不可由 child-created 代替）

1. **默认 oracle：A2S。** 使用 Target Manifest 中真实模板/配置选出的 `SERVER_PORT`（端口值来自该次实例配置，不从样例猜），启动后以 T0 为起点最多 60 秒；目标端口返回一次成功 A2S info，且成功响应时 game process 仍存活，才判 `ready=true`。
2. **受限 fallback：日志 marker。** 仅在 Manifest 有该实际 PZ build 不支持 A2S 的证据时选择；在运行前记录实际 marker 原文、匹配规则和样例。沿用同一个 T0+60 秒窗口；进程仍须存活。没记录就不准在测试失败后临时改用 marker。
3. **未 ready 的处理。** 60 秒无 oracle 信号则记录未 ready/失败；不因 readiness 超时自动 kill/stop 子进程。`start` 仍按 ADR/legacy 行为：child 创建成功可返回原 200 响应；这不表示服务器已经 ready。`status`/`running` 原语义不变。
4. **交叉验证。** 同一时间线保存独立 A2S query 或真实 marker 日志、PID 存活观察、`/api/status` 的追加 `ready` 与 `readiness` 字段。`ready` 不得早于 oracle 成功；`readiness` 的内部 schema 暂不由本计划强定。状态字段只允许加法式扩展，不可改变现有 `status`（含 `INSTALLING` 覆盖）、`running` 或 start response body。
5. **证据门。** live readiness 的成功 A2S/marker 与存活 PID 时间戳必须处于 60 秒窗口内；fake UDP responder、合成日志、child PID、端口仅处于 LISTEN 或 HTTP 200 均不构成 oracle 证据，不能替代 M2-PZ-LIVE。

## 7. 本计划明确不覆盖

- 不实现或验收 M3 的持久任务恢复、任务自动重试、跨重启安装任务续跑。
- 不对 darwin 或 Windows 作平台支持声明；模板 `supported_os` 只是兼容元数据。
- 不包含或暗示 M1-B 生产 takeover 授权；无具名主机/窗口/单独授权时不切换真实数据、不宣布迁移接管。
- 不把 M1-A、Go CI、离线/合成测试、helper process、mock A2S 当作真实 PZ 安装/ready/平台支持证据。
- 不引入数据库/daemon、静态加密/keyring、Vue 重建或任何 ADR 未批准设计。

## 8. 执行前已知阻塞/待确认项

- `docs/migration/BASELINE.md` 是迁移前的快照（当时尚无 Go CI）；后续 Go workflow 已取得绿证（`TOOLCHAIN.md` 记录 run 36421008298 / 36436902986 / 36437730487 / 36438544535）。实际执行仍须先填写本计划 §4，再把同一 Manifest 快照写入对应 evidence 文件；只填其中一个位置不满足 ADR §6.4。
- ADR §1.8 要求 CI 纳入 `internal/archtest`；`.github/workflows/go.yml` 已含 `go test ./internal/archtest/...` step，且 `TOOLCHAIN.md` 已记录多次全绿 run（含 race 与 architecture boundaries）。**但这些 run 属于已推送的提交**；正式 M2-BUILD PASS 仍需针对本次受测 commit 跑绿并记录 Run/SHA，使 CI SHA 与被测二进制 SHA 对应（旧 run 不得替代新提交）。
- 用户决策记录明确：M2 实机为“另给一台”，但尚无主机/数据根/build 信息；SteamCMD 版本、网络/端口/防火墙、浏览器版本也未提供。M2-PZ-LIVE/M2-PLATFORM 当前 BLOCKED，Target Manifest 必须待用户提供并逐项授权。
- **测试主机与平台声明的区分：** 用户提供了一台通过 Cloudflare Tunnel 可到达的 Windows 主机（`winssh.liubaitech.cn` / `wingame.liubaitech.cn`）。该主机可用于 M2-PROCESS 的 Windows 条件行与 M1-A 隔离副本演练，但**不产生 Windows 支持声明**（ADR §1.6：Windows 未验证）；支持声明目标仍为 Linux Ubuntu LTS。接入前需先完成 Cloudflare Access 客户端认证，并对每次真实副作用取得逐项授权。详见 `docs/acceptance/M2-PLAN-DECISION.md`。
- **审查状态：** 本计划的审查为非独立复核（teammate 额度耗尽，用户同意替代）并已获用户确认范围；独立审查在恢复审查渠道后应补做，详见 `docs/acceptance/M2-PLAN-REVIEW.md`。
- `/api/status.readiness` 内部 schema 未在 ADR/契约固定；当前按 §1.2 只验存在及与 oracle 的事实一致，不自造 wire contract。若代码/消费者出现更严格需求，须 ADR/契约先决策。
- ADR §4.2 的“rename 之前崩溃”句与 COMMITTING 持久化屏障和 §4.3 具体布局行存在上述措辞冲突；按 §1.2 的窄化解释执行，不把更宽松的恢复行为判 PASS。建议后续仅澄清 ADR 文句，不改变已批准恢复规则。

## 9. 审查记录与执行报告

本文件逐项依 ADR §6.3 ID 集与 LEGACY-CONTRACT §9/§2/§3 对照，并把每个矩阵行要求映射到 §5 evidence 模板。计划文本本身没有执行测试，也没有获得实机副作用授权。按 todo #12 的批准工作流，**下一步为独立 reviewer 对本计划做只读审查，修复任何 Critical/High 后交用户确认计划范围与 Target Manifest 门槛**；在独立审查及用户范围确认完成前，不开始执行 M2 用例。执行 owner、reviewer 与用户确认必须写入实际 evidence 记录；任何 Linux 支持声明前还须由 reviewer 检查 Critical/High 并签结论。
