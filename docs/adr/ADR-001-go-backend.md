# ADR-001：Go 后端迁移的模块边界、所有权与迁移安全

- 状态：**Approved r3**（用户依据 `docs/adr/ADR-001-decision.md` 批准；r1 NEEDS_REVISION → r2 修正 → 第 2 轮复审 NEEDS_REVISION（1 High RV-201）→ r3 修正 → 第 3 轮聚焦复审 + r3 RV-301 闭环）。后续实现发现只需变更架构时，应新增修订并说明。
- **日期**：2026-09-28
- **基线提交**：`ebacf29084e63afb505044ae69957c88a951e772`
- **依据**：已批准 Plan（handoff key `86c3e976a061fc28b352f996708826a95c395a6600595d2b4cf9f13f1b8fc8f0`）、修订版路线图 M1/M2、`docs/migration/BASELINE.md`、`docs/migration/LEGACY-CONTRACT.md`
- **范围**：M1（Go 迁移与兼容）与 M2（Go 基线 PZ 验收）所需决策。M3–M6 能力不在本 ADR 内。
- **r1 审查记录**：独立 reviewer 结论 NEEDS_REVISION（1 Critical / 9 High / 6 Medium / 4 Low），出版 ID `agent://64f802da-f1c7-4a7e-8c95-0fdf87632c1e`。本 r2 逐项闭环见 §10。

---

## 决策摘要

| # | 决策 | 关键理由 |
|---|---|---|
| 1 | HTTP 用 `net/http` + `http.ServeMux`（Go 1.22+ 方法模式）；WebSocket 用 `github.com/coder/websocket` | 路由集合固定且小；避免框架绑定；WS 需 context 感知且零传递依赖 |
| 2 | **保留**现有静态 `static/index.html`，不做 Vue 重建 | UI 仅 46 行、10 处调用点覆盖 13 个端点；M2 要求浏览器闭环验收；重建属产品决策 |
| 3 | 持久化继续用文件（`instance.json` + PZ INI），**不引入数据库** | 单实例；与 Python 基线 1:1 兼容；数据库属未批准的 ADR 范围 |
| 4 | 单二进制、**进程内**进程监督（无独立 daemon），保留适配器边界 | 单机目标；daemon 属 M6 |
| 5 | 平台按证据声明：**Linux 为第一候选**，darwin 仅开发/测试，**Windows 未验证**；模板元数据不构成支持声明 | 质量规则禁止无实机证据的平台声明 |
| 6 | 秘密静态存储：数据根 `0700` / 状态文件与 INI `0600`，明文（无 keyring/加密）；运行时仅经 argv 且禁止解释器重解析；进程指纹使用**脱敏后** argv | PZ 只接受 `-adminpassword` argv；锁定可见性边界 |
| 7 | 跨进程单写者：**锁与所有权记录放在被切换子树之外**；OS 级独占锁 + 所有权记录 + 启动对账 + 失败关闭；恢复经 CLI 且必须先持锁 | r1 的 C1：锁在会被 rename 的目录里会让两个进程同时持有 |
| 8 | 迁移提升只切换**状态子树**（`<instance_root>/state/`），存档/服务端文件/SteamCMD 原地不动；journal 状态机 + prev 轮转 + 幂等恢复 | 目录级原子切换且不搬运 GB 级数据，避免 `is_installed` 被破坏 |
| 9 | 启动向量：一律 exec + 类型化 argv；**代码侧 argv 为唯一权威**，模板 `start_arguments` 字符串永不解析；向量选择须先取得制品证据 | 三平台现状均经脚本中转；模板字符串插值是被禁止的形态 |
| 10 | 依赖图：`domain` ← `ports` ← `application` ← {`httpapi`, 各出站适配器}，`cmd` 唯一装配；`archtest` 强制 | 消除端口归属歧义与环 |
| 11 | 工具链版本由 `docs/migration/TOOLCHAIN.md` 单点固定（语言基线 `go 1.22`），该文件与 CI 就位前不得编码 | r1 M6：浮动选择会产生不可复现工具链 |

---

## 1. HTTP/WebSocket、UI、持久化、拓扑、平台、秘密、工具链

### 1.1 HTTP 框架：`net/http` + `http.ServeMux`

**决策**：使用标准库 `net/http`，路由用 Go 1.22+ 的 `ServeMux` 方法模式（`mux.HandleFunc("POST /api/server/install", ...)`）。中间件以显式函数包装实现（会话校验、Origin 校验、日志）。

**备选与否决理由**
- `chi`：路由/中间件生态成熟，但本服务路由固定（20 个），middleware 需求仅 3 类；引入依赖的收益不抵长期维护面。
- `gin`：自带绑定/渲染模型，会诱导入站 DTO 与领域类型耦合；与“domain/ports 不依赖框架”冲突。

**兼容要求（强制）**
1. 20 个路由/挂载点的方法、路径、状态码、响应字段逐项对齐 `LEGACY-CONTRACT.md` §2。
2. 错误信封固定为 `{"detail": <string>}`，并复现 `LEGACY-CONTRACT.md` §2.3 的全部字符串（含 UI 分支依赖的 `请先登录`）。
3. 容忍 UI 请求形态：GET 携带 `Content-Type: application/json`；无请求模型的 POST 接受 `{}`（`LEGACY-CONTRACT.md` §2.4）。
4. `StrictModel(extra="forbid")` 等价语义用 `json.Decoder.DisallowUnknownFields` 实现。
5. **追加字段规则**：允许新增响应字段（如 `/api/status` 的 `ownership`、`ready`），但不得重命名/移除/改变既有字段类型；每个追加字段必须登记在本 ADR 与 M2 计划中。

### 1.2 WebSocket：`github.com/coder/websocket`

**决策**：采用 `github.com/coder/websocket`，版本用 `go mod vendor` 固定。

**备选与否决理由**
- `gorilla/websocket`：成熟零依赖，但 API 非 context 优先；本项目需要“受控 shutdown 取消 + 读/写超时 + 断连清理不泄漏 goroutine”（M2-WS）。若实现中遇到阻塞性缺陷，允许改用 gorilla 并在此记录变更。
- `golang.org/x/net/websocket`：不再推荐用于新代码。

**契约要求**：accept 前完成会话与 Origin 校验，失败以 close code **1008** 结束；连接后回放最近 **100** 行；帧类型 `log`/`input`/`ping`/`pong` 与 `LEGACY-CONTRACT.md` §3 一致，并采用 D3 变更（新增 `error` 帧，见 §5.4）；断开时取消发送协程并移除 listener。

### 1.3 静态 UI：保留

**决策**：继续服务现有 `static/index.html`（含内联 CSS/JS），不做 Vue/Vite 重建。

**理由**：UI 与 API 契约耦合面已完整枚举：**10 处调用点、13 个具体端点**，且存在两处硬编码依赖（`data.detail` 错误信封、`'请先登录'` 字符串分支）。保留静态 UI 才能在 M2-UI 中验证“迁移未破坏既有控制路径”。Vue 重建属产品/前端决策，列入未决项（§7）。

### 1.4 持久化：文件 + 原子写（含日志缓冲区）

**决策**：保持文件持久化，引入统一原子写协议；不引入 SQLite/ORM。

**受管文件与协议**

| 文件 | 写协议 |
|---|---|
| `<instance_root>/state/instance.json` | 临时文件 → `fsync(file)` → 将当前 target 复制为同目录 `instance.json.bak` 并 `fsync` → `os.Rename(tmp, target)` → `fsync(dir)` → 读回校验 |
| `<instance_root>/Zomboid/Server/<name>.ini` | 同上（`.ini.bak`），保留注释与未知键；空 Mod 列表写空串 |

- 失败语义：写失败必须返回失败（5xx）或明确 `recovery required`，**不得**先响应成功（对齐 D5）。
- 读回校验失败、`.bak` 缺失且 target 损坏 → `RECOVERY_REQUIRED`。
- **`.bak` 完整性（r2 RV-206）**：`.bak` 由复制产生，可能被中断撕裂；**恢复前必须先通过完整性与可解析性校验**（大小/校验和/JSON 或 INI 解析），校验不通过一律 `RECOVERY_REQUIRED`，不得把截断的旧版本当作权威。
- JSON 读取保留一层嵌套合并 + SERVER_NAME 守卫（`core/instance_manager.py:69-83`、`:79-81`）。
- **严格性边界（r1 L3）**：运行时读取保持 Python 的宽容语义；**严格校验只作用于迁移导入与提升门**（缺必需秘密字段等拒绝并要求显式 override 参数）。
- **平台说明（r1 L2）**：darwin 的普通 `fsync` 不刷新驱动器缓存（需 `F_FULLFSYNC`）；掉电级别的原子性只在通过演练的平台声明，darwin 仅限开发验证。

**日志缓冲区（r1 M4 关闭）**：`ProcessSupervisor` 维持**内存环形缓冲，容量 1000 行，重启即清空**；WS 回放最近 100 行；`GET /api/server/logs` 的 `limit` 夹在 1..1000（默认 150）。这是兼容面的一部分。

### 1.5 进程拓扑：单二进制 + 进程内监督

**决策**：控制面与执行面同进程（同 Python 基线），通过 `ports` 隔离 `steamcmd` 与 `process` 适配器，为未来 daemon 留替换点但**不实现**独立 daemon/gRPC。**否决**：现在拆 daemon（引入网络分区、任务幂等、节点身份等未批准范围）。

### 1.6 目标平台与部署

| 平台 | 状态 | 允许的声明 |
|---|---|---|
| **Windows（第一候选）** | 用户于 2026-09-28 明确要求优先支持；测试主机 `winssh.liubaitech.cn` / `wingame.liubaitech.cn` | 仅当实机 M2-PZ-LIVE（SteamCMD Windows、PZ 服务端安装/就绪/控制台/端口/停止）与启动向量对抗测试全部通过后才可声明支持；在此之前保持“未验证” |
| Linux（Ubuntu LTS） | 第二候选；需 M2-PZ-LIVE 实机 + 启动向量对抗测试通过 | 通过后可声明支持 |
| macOS（darwin） | 开发/离线测试平台（掉电级原子性不保证，见 §1.4） | 仅“开发验证”；服务端支持需实机 PZ 验收 |

**模板元数据的例外（r1 追加矛盾）**：`/api/templates` 必须按原样返回模板中的 `supported_os: [windows, linux, darwin]` 等元数据（兼容数据），但该字段**不作为平台支持声明**；Go 版可另行通过 `/api/status` 的追加字段表达本服务的验证状态。此例外必须在 M2-API 记录中显式标注。

**Windows 优先决定的实现含义（2026-09-28）**：
1. `oslock` 的 Windows 路径（`LockFileEx`）从“未验证分支”升级为**必测路径**：需实机验证非阻塞互斥、进程退出后锁释放、陈旧所有权对账与 `recover` 行为。
2. `internal/adapters/process` 的 Windows 分支（`cmd.exe /c` 与直接 `.exe`）、路径分隔符/大小写不敏感、PID/进程树回收都需要实机证据。
3. Go 的 `-race` 在 Windows 需要 C 工具链（cgo/gcc）；若目标机无 gcc，则记录“Windows 不具备 race 证据”，race 门继续由 Ubuntu CI 承担（§1.8），不得因此降低 M2-BUILD 要求。
4. 目标机工作目录按用户指示使用 **G 盘新建目录**：`G:\gameserver-work\`（见 `docs/migration/WINDOWS-SETUP.md`）。

**部署**：单二进制 + `static/` 资源，默认监听 `127.0.0.1:8769`（沿用 `GAMESERVER_HOST/PORT`）；对外暴露须经 HTTPS 反向代理；`GAMESERVER_COOKIE_SECURE=1` 时 Cookie 仅 HTTPS 发送。

### 1.7 秘密：静态与运行时

| 边界 | 决策 |
|---|---|
| 传输到游戏进程 | PZ 仅支持 `-adminpassword` argv，故**必然对同机同权限进程可见**；接受为**残余风险**并记录，绝不声称绝对机密 |
| 解释器重解析 | **禁止**：argv 经 exec 直接传递，不得经 shell 字符串、`sh -c`、`cmd /c` 拼接 |
| 命令指纹 | `.owner.json` 的 `command_fingerprint` 必须对**脱敏后**的 argv 计算（`-adminpassword=[REDACTED]`，与日志同一规则），避免成为密码离线校验器（r1 L1） |
| 日志/响应 | 沿用脱敏规则（`LEGACY-CONTRACT.md` §2.2）；迁移 dry-run/summary 不含秘密值 |
| 会话密钥 | 启动即轮换（旧 Cookie 失效）；不落盘 |

**文件权限（r1 M1 关闭，登记为 D8）**

| 路径 | POSIX 模式 | 时机 | 不合规处理 |
|---|---|---|---|
| `<data_root>/servers/`、`<data_root>/steamcmd/` | `0700` | 创建时设置 | 若既存且宽松：**不阻塞只读**，变更类操作前要求修复 |
| `<instance_root>/state/`、`<instance_root>/state/instance.json` | `0700` / `0600` | 创建与写入后校验 | 宽松 → `RECOVERY_REQUIRED`，阻止 start/install/config 写入 |
| `<instance_root>/Zomboid/`、`.../Server/<name>.ini` | `0700` / `0600` | 写入后校验 | 同上（INI 含 `SERVER_PASSWORD`，必须纳入） |
| `<instance_root>/instance.json`（legacy，由 `export-back` 写回，含明文秘密） | `0600` | 写入后校验 | 不合规 → `RECOVERY_REQUIRED`（r2 RV-203） |
| Windows | 不适用（不检查 POSIX 位） | — | 依赖默认 ACL；在取得 Windows 实机证据前不声称支持 |

修复入口：`gameserver fix-permissions --data-root <path>`（CLI，记录审计日志）；服务本身**不静默 chmod 用户文件**。

**未决项**：是否引入 OS keyring 或静态加密（owner=用户；触发条件：多用户或备份外发）。

### 1.8 Go 版本与 CI

- **语言基线**：最低 Go 1.22（`ServeMux` 方法模式要求）；当前用户批准的实际版本见 `docs/migration/TOOLCHAIN.md`。
- **版本单点固定（r1 M6 关闭）**：实际工具链版本写入 `docs/migration/TOOLCHAIN.md`，Go 语言基线写入 `go.mod` 的 `go` 指令（当前 `1.27`），精确 patch 版本写入 `toolchain` 指令（当前 `go1.27.1`）并与 `.github/workflows/go.yml` 的 `go-version` 一致；三处不一致即视为配置错误。**`TOOLCHAIN.md`、`go.mod`、CI 工作流三者就位并且 CI 至少成功一次之前，不得编写 Go 实现，也不得声称任何 Go 验证通过**；禁止使用浮动 `latest`。该 `go`/`toolchain` 分层避免同值冗余 toolchain 行被 `go mod tidy` 删除。
- **CI 工作流**（新增 `.github/workflows/go.yml`，不改动现有 Python 工作流）：
  1. `gofmt -l` 无输出；
  2. `go vet ./...`；
  3. `go test ./...`；
  4. **`go test -race ./...` 必须在 ubuntu-latest 上对目标提交为 green**；不得以非 race 证据替代。唯一例外：CI 基础设施故障时，须提交失败/重试日志并由 reviewer 复核；本地无 Go 或本地不支持 race **不构成例外**；
  5. `go build ./...`；
  6. `archtest`（导入边界检查，见 §2.1）。
- 本地无 Go 时不得声称任何 Go 验证通过（`BASELINE.md` §4）。

---

## 2. 模块边界与依赖规则

### 2.1 包图（唯一允许的方向）

```
cmd/gameserver            (组合根：读配置、装配、启动；唯一 import 所有适配器的位置)
   ├── internal/adapters/httpapi        (HTTP/WS/Cookie/Origin；不含业务规则)
   ├── internal/adapters/staticassets   (只读静态资源，供 httpapi 使用)
   ├── internal/adapters/localstate     (实例状态文件 + 原子写)
   ├── internal/adapters/pz             (模板、INI、安装/启动规格、就绪探测)
   ├── internal/adapters/steamcmd       (SteamCMD 获取与调用)
   ├── internal/adapters/process        (进程创建/停止/日志/指标)
   ├── internal/adapters/systemclock    (ports.Clock 的 stdlib 实现)
   └── internal/adapters/oslock         (跨进程文件锁，build tags 分平台)
                │  以上均 import：ports, domain
                ▼
internal/application     (用例编排：认证/状态/配置/安装/启停/控制台)
                │  import：ports, domain
                ▼
internal/ports           (出站端口接口 + 仅引用 domain 的 DTO)
                │  import：domain
                ▼
internal/domain          (实例/配置/任务/端口/秘密策略的值类型与规则；仅 stdlib)
```

**规则**
1. `domain` 不 import 任何其他 internal 包。
2. `ports` 只 import `domain` 与 Go 标准库；**所有出站接口声明在此**（消除“端口归用例所有 vs 适配器不得依赖用例”的歧义）。
3. `application` import `domain`、`ports`；**不** import 任何 adapter 或 httpapi。
4. 适配器 import `ports`、`domain`；**不** import `application`、**不**互相 import、不共享可变全局状态。
5. `httpapi` import `application`、`domain`（仅 DTO 映射）与 `ports`；**不得直接 import 文件系统与进程执行包**（如 `os`、`io/fs`、`path/filepath`、`os/exec`），更不得访问实例数据文件系统（`state/`、`Zomboid/`、`server_files/`、`steamcmd/`）；静态资源必须经 `ports.StaticAssets`（由 `staticassets` 适配器实现）只读获取。
6. `domain` 与 `application` 不得 import OS/文件/网络/进程/数据库包；`cmd` 是唯一组合根，装配所有适配器与端口实现。

**强制手段（r1 M5 关闭）**：`internal/archtest` 是 Go 测试，执行 `go list -e -json -test ./...`，读取每个包的 `.Imports`、`.TestImports` 与 `.XTestImports`，断言上述规则基于**直接导入**（非传递）判定；同时拒绝 domain/application 的 OS/IO/网络/DB 标准库依赖及 httpapi 的直接 filesystem/process 依赖。违规即失败。该检查必须证伪：人为注入违规 import 时测试失败。

### 2.2 端口清单（首批）

| 端口 | 职责 | 适配器 |
|---|---|---|
| `StateStore` | 读/写实例状态（原子、版本化、失败关闭） | localstate |
| `GameConfig` | 读/写 PZ INI（保留未知键与注释） | pz |
| `Installer` | 获取/更新服务端文件（类型化 argv、进度、取消、reap） | steamcmd |
| `ProcessSupervisor` | 启动/停止/强杀/日志环形缓冲/指标 | process |
| `GameReadiness` | 就绪判定（见 §2.3） | pz |
| `InstanceLock` | 获取/释放所有权、读所有权记录 | oslock |
| `StaticAssets` | 只读静态资源 | staticassets |
| `Clock` | 时间源（测试可注入） | systemclock |

### 2.3 就绪判定（r1 M3 关闭）

- **判决**：`GameReadiness` 的默认真相源是模板声明的 A2S 查询（`templates/project_zomboid.yaml:120-122`，`port_key=SERVER_PORT`）：在启动后**最多 60 秒**内收到一次成功的 A2S info 响应，且进程仍存活，即 `ready=true`。
- **回退**：若目标 PZ build 在 Target Manifest 中被记录为不支持 A2S，则改用**日志就绪标记**（该 build 实际输出的可复现字符串），标记内容与判定规则写入 Target Manifest 与 M2 计划后才能执行 M2-PZ-LIVE。
- **兼容性约束（加法式）**：就绪状态**只以追加字段**暴露（`/api/status.ready` 与 `/api/status.readiness`），**不得**改变既有 `status`（仍为 `STOPPED/STARTING/RUNNING/CRASHED/STOPPING`，且 `steamcmd.is_busy` 时沿用 **`INSTALLING`** 覆盖值，见 `core/instance_manager.py:264-265`）与 `running` 的语义，也不改变 start 响应体。超时本身不终止进程；`start` 的返回仍与基线一致（子进程创建成功即 200）。

---

## 3. 跨进程单写者

### 3.1 机制（位置由 r1 C1 修正）

| 项 | 位置 |
|---|---|
| 实例锁 | `<data_root>/servers/.locks/<instance>.lock` |
| 所有权记录 | `<data_root>/servers/.owners/<instance>.json` |
| 迁移/共享资源串行化锁 | `<data_root>/servers/.locks/.migration.lock` |

**为什么不在实例目录内**：`flock` 绑定 inode，而提升会对实例目录做 rename；锁文件若位于被切换子树内，会出现“旧持有者仍持有被移动的 inode、新路径上的锁文件可被第二个进程获取”的双写者窗口。**staging 目录不得包含任何锁/所有权文件**，迁移必须显式排除 `.locks`、`.owners`。

**加锁实现**
- Unix（linux/darwin）：`flock(LOCK_EX|LOCK_NB)`（`golang.org/x/sys/unix`）
- Windows：`LockFileEx`（`golang.org/x/sys/windows`）
- 不使用第三方锁库；`oslock` 适配器按 build tag 分平台，接口统一。

**所有权记录**（持锁后原子写 + fsync）：
```json
{"schema":"owner/1","service_id":"...","pid":12345,"session_token":"...","started_at":"ISO8601","root":"<abs path>","command_fingerprint":"sha256(redacted argv)","operation_id":"..."}
```

**持锁范围**：所有会改变实例状态的操作（install/start/stop/restart/kill/config 写入/mods/**迁移提升**）必须先持实例锁；读取类查询（status/logs/templates）不要求。**共享资源规则（r1 L4）**：`steamcmd/` 与 `.migration/` 是跨实例共享目录，对其的写操作（SteamCMD 自举/更新、迁移提升）必须先持有 `.migration.lock`（数据根级）；提升同时持实例锁与数据根锁。

### 3.2 启动对账（**本节为权威表**，r1 H1 关闭）

| 启动时现场 | 行为 |
|---|---|
| 取得锁 + **无所有权记录** + 进程探测未发现匹配的游戏/安装进程 | 正常启动 |
| 取得锁 + **无所有权记录** + 探测发现匹配进程 | `RECOVERY_REQUIRED`；禁止 install/start |
| 取得锁 + 记录 PID 已死 + 无可匹配进程 | `RECOVERY_REQUIRED`（陈旧所有权） |
| 取得锁 + 记录 PID 存活 + 指纹匹配 | `RECOVERY_REQUIRED`（疑似遗留运行进程）；禁止 install/start |
| 取得锁 + 记录 PID 存活但**命令指纹不匹配** | `RECOVERY_REQUIRED`；禁止 install/start（PID 复用风险） |
| 取得锁 + 记录 PID 存活但 **session_token 不匹配** | `RECOVERY_REQUIRED`；禁止 install/start |
| 取得锁 + 记录 **root 不匹配**当前数据根 | `RECOVERY_REQUIRED`；禁止 install/start |
| **未取得锁** | 服务可启动、只读可服务；所有变更类操作返回 409，`/api/status.ownership = "locked_by_other"` |
| 迁移 journal 处于非终态（`STAGED`/`VERIFIED`/`COMMITTING`/`ROLLING_BACK`） | `RECOVERY_REQUIRED`；**服务不得创建或修复实例目录**，禁止 install/start |

**失败关闭原则**：任何“所有权不可证明”的情形都不允许回退为“尽力而为”。跨进程并发变更一律返回 **409 `instance owned by another process`**。

### 3.3 显式恢复（r1 H2 关闭）

**决策**：恢复动作由同一二进制的 CLI 子命令提供，不新增 HTTP 端点：

```
gameserver recover --data-root <path> --instance <id> [--force]
```

- **必须先以非阻塞方式取得同一把实例锁**：取不到锁（另一进程在运行）即**拒绝**并退出，不得修改任何所有权文件。
- 取得锁后：仅在“记录 PID 已死且无匹配进程”时清理陈旧所有权；若探测到匹配进程存活，默认拒绝（要求先 `stop`/`kill`，或显式 `--force` 并二次确认）。
- `--force` 需交互确认，执行前打印当前所有权记录与进程证据。
- 每次恢复写入审计日志（时间、旧记录、判定依据、操作者、结果）。

### 3.4 验收（M2-SINGLEWRITER / M2-RESTART）

- 两个独立服务进程：第二个进程的 install/start 被拒且**不产生第二个子进程**（PID 计数 + 锁状态为证据）。
- §3.2 表中每一行都是可执行测试用例，且断言“变更类操作被拒 + 无副作用”。
- **提升后仍冲突（r1 C1 回归用例）**：完成一次提升后，原持有者与新持有者（第二进程）仍互相排斥，且新数据根内**不存在**陈旧的 `.owner.json`。
- 正常关闭路径：drain 安装任务并 reap 子进程 → 优雅停止 PZ → 核验进程树结束 → 释放锁并清理所有权记录。

---

## 4. 迁移与提升的中断安全

### 4.1 目录布局（**只切换状态子树**，r1 H3 关闭）

```
<data_root>/servers/
  <instance>/
    state/                    ← 切换单元：Go 管理的状态（instance.json 等，KB 级）
    instance.json             ← legacy Python 状态文件：仅导入期读取，运行期不动
    Zomboid/                  ← 存档/缓存：原地不动（Go 就地写 Server/<name>.ini）
    server_files/             ← 游戏服务端：原地不动（不复制、不删除、不切换）
  .locks/<instance>.lock      ← 锁（永不参与切换）
  .owners/<instance>.json     ← 所有权记录（永不参与切换）
  .migration/
    staging/<instance>/       ← 仅含 state/ 内容
    prev/<instance>.<seq>/    ← 被替换掉的旧 state（保留，不覆盖）
    journal.json              ← 状态机
    manifest.json             ← 基线清单 + 校验和 + 版本
<data_root>/backups/<instance>/<UTC ts>/   ← 备份（见 §4.6）
```

**切换 = `rename(.migration/staging/<instance>, <instance_root>/state)`**（如需替换既有 `state/`，先 `rename(state, .migration/prev/<instance>.<seq>)`）。因为切换单元只含 KB 级状态文件，存档与服务端文件**从不移动**，`is_installed`（文件存在性判定，`core/instance_manager.py:103-110`）不受影响。

**排除规则**：staging 与 manifest **必须排除** `.locks/` 与 `.owners/`，且**仅允许包含 `state/` 的内容**；`server_files/`、`Zomboid/`、`steamcmd/` 仅以“存在性 + 元数据”记录进 manifest（不复制、不切换）。

### 4.2 状态机与原子切换

```
IDLE → EXPORTED(dry-run 记录) → STAGED → VERIFIED → COMMITTING → COMMITTED
                                            ↘ ROLLING_BACK → ROLLED_BACK → LEGACY_ACTIVE(有意回滚终态)
```

**不变量（r1 H4-1）**：`COMMITTING` 必须在**第一次 rename 之前**原子写入并 fsync。任何 rename 之前崩溃都只能观察到 `VERIFIED` 或更早状态。

**切换步骤**
1. 若 `<instance_root>/state` 存在 → `rename(state, .migration/prev/<instance>.<seq>)`，`seq` 取 journal 单调计数器（**prev 永不覆盖**，r1 H5）。
2. `rename(.migration/staging/<instance>, <instance_root>/state)`。

**失败语义（r1 H5 关闭）**
- 步骤 1 失败（含 `ENOTEMPTY`、权限、跨卷）：**失败关闭**，记 `RECOVERY_REQUIRED`，现场 = target 保持原状、prev 未创建或已完整创建（按 journal 记录判定），**不删任何一侧**。
- 步骤 2 失败：立即回滚 `rename(prev → state)`；回滚成功记 `ROLLED_BACK`；回滚失败记 `RECOVERY_REQUIRED`（数据保留在 `prev/`，绝不删除）。
- **回滚原语语义（r3 RV-301）**：`rename(prev → state)` **消费** prev 目录；被消费的路径与序号记入 `journal.last_rolled_back_from` 供审计。因此 `ROLLED_BACK` 现场的 prev 可能已不存在，§4.3 按“任意”判定。
- 重复提升：若 journal 为 `COMMITTED` 且 manifest 校验通过 → 幂等无操作；若 staging 已被消费则重新导入至 `VERIFIED` 后再提升。
- **首次提升没有 prev（r2 RV-201）**：当 `<instance_root>/state` 不存在时跳过步骤 1，因此 `ROLLING_BACK` 与步骤 2 的回滚原语**不适用**；该路径下的失败只能“校验通过则前滚”或“失败关闭并保留 staging 供人工处理”。

### 4.3 中断恢复规则（启动与命令入口均执行；r1 H4 关闭）

| journal | state/ | prev/ | staging/ | 动作 |
|---|---|---|---|---|
| `EXPORTED` / `IDLE` | 任意 | 任意 | 任意 | 丢弃 staging（幂等重建）；停在 `EXPORTED` |
| `STAGED` | 不变 | — | 存在 | 校验 staging 后进入 `VERIFIED`；不一致则丢弃 staging |
| `VERIFIED` | 存在 | — | 存在 | 丢弃 staging（幂等重建）；停在 `VERIFIED` |
| `VERIFIED` | **缺失** | 存在 | 存在 | `RECOVERY_REQUIRED`（异常现场，禁止自动修复） |
| `COMMITTING` | 缺失 | 存在 | 缺失 | 回滚：`prev → state`，记 `ROLLED_BACK` |
| `COMMITTING` | 存在 | 存在 | 缺失 | **前滚判定**：仅当 state 内容校验和 == 新 manifest 且完整才能记 `COMMITTED`；否则 `RECOVERY_REQUIRED` |
| `COMMITTING` | 存在 | 存在 | 存在 | `RECOVERY_REQUIRED`（两侧同时存在，禁止自动删） |
| `COMMITTING` | 存在 | 缺失 | 存在 | **步骤 1 尚未执行（COMMITTING 屏障后）或步骤 1 失败**：重试步骤 1→2（幂等）；任一步失败按 §4.2 失败语义处理（r2 RV-201） |
| `COMMITTING` | 缺失 | 存在 | 存在 | **两次 rename 之间**：校验 staging 校验和 → 通过则前滚 `staging → state` 并记 `COMMITTED`；不通过则失败关闭（保留 prev，不删） |
| `COMMITTING` | 缺失 | 缺失 | 存在 | **首次提升中被中断**：无 prev 可回滚；校验 staging 通过则前滚并记 `COMMITTED`，否则 `RECOVERY_REQUIRED`（保留 staging，禁止删除） |
| `COMMITTING` | 存在 | 缺失 | 缺失 | `RECOVERY_REQUIRED`（无回滚源） |
| `ROLLING_BACK` | 缺失 | 存在 | 任意 | 继续回滚 `prev → state`，记 `ROLLED_BACK` |
| `ROLLING_BACK` | 存在 | 缺失 | 任意 | state 已是完整旧版本 → 直接记 `ROLLED_BACK`（幂等，无需再次 rename） |
| `ROLLING_BACK` | 存在 | 存在 | 任意 | 回滚后现场不一致 → `RECOVERY_REQUIRED`（不自动删任一侧） |
| `ROLLED_BACK` | 存在 | 任意 | 任意 | 无操作（幂等）；prev 可能已被回滚 rename 消费，审计信息在 `journal.last_rolled_back_from` |
| `LEGACY_ACTIVE` | 缺失 | 存在 | 任意 | 无操作（有意回滚后的终态，见 §6.2）；重新提升从 `STAGED` 入口重新开始（r2 RV-204） |
| `COMMITTED` | 存在 + 校验通过 | 存在 | 缺失 | 无操作（幂等） |
| `COMMITTED` | 存在 + **校验失败** | 任意 | 任意 | `RECOVERY_REQUIRED`（视为损坏，不做自动回滚） |

**强制规则**
- **禁止前滚误判**：即使实例目录会被服务在初始化时创建，`state/` 的内容也必须与 manifest 校验和一致才允许记 `COMMITTED`；非终态 journal 存在时服务不得创建/修复实例目录（§3.2 末行）。
- 所有 `RECOVERY_REQUIRED` 都需要 `gameserver recover --migration` 显式处理（打印现场与建议动作，不自动删数据）。

### 4.4 前置条件与失败关闭

- staging 必须与 target **同卷**（比较设备/卷标识）；不同卷直接拒绝（跨卷 rename 非原子）。
- `dry-run` 只读源数据（不写源、不写 target），输出清单/版本/校验和/拒绝项；`VERIFIED` 阶段必须重新校验源未变，否则拒绝提升。
- 导入遇到损坏状态、缺失必需秘密字段、未知 schema 版本 → 拒绝并保留源数据；缺 `ADMIN_PASSWORD` 等宽容场景需显式 `--accept-missing-secret` 并记录警告。
- 提升期间同时持有实例锁与 `.migration.lock`（§3.1）。

### 4.5 验收（M2-MIGRATE）

每个状态转移边界至少注入一次中断（进程被杀/掉电模型），**覆盖 §4.3 中 `COMMITTING` 的全部（state, prev, staging）现场组合、首次提升（无 prev）路径与 `ROLLING_BACK` 回滚走廊**；终态必须是**完整旧版本**或**完整新版本**且 manifest 校验一致；重复提升幂等；步骤 1/步骤 2 失败路径各自可复现并落在 §4.3 的确定行。

### 4.6 备份/恢复（r1 M2 关闭，M1 范围）

| 项 | 决策 |
|---|---|
| 范围 | **默认**：`state/` + `Zomboid/Server/*.ini` + 实例元数据（manifest）。`Zomboid/` 存档与 `server_files/` **默认排除**，需显式 `--include-saves` / `--include-server-files` 选择 |
| 目标位置 | `<data_root>/backups/<instance>/<UTC ts>/` + `manifest.json`（相对路径、大小、sha256、mtime） |
| 校验 | 备份完成后逐文件重新读取并比对 sha256；任一失败即删除该备份并返回失败 |
| 恢复 | **只恢复到 staging 并通过 §4 状态机提升**；禁止就地覆盖活动数据根 |
| 保留 | 保留最近 3 份成功备份；删除只经显式 CLI（`gameserver backup prune --keep 3`），**无自动清理**（产品化归 M3） |
| 验收 | 备份→校验→恢复→读回 全链路在隔离副本上通过；损坏备份被拒绝（篡改一个字节后必须失败） |

---

## 5. 启动向量与注入防护

### 5.1 向量选择（r1 H9 关闭）

**前置证据步骤（实施前必做）**：对目标平台的实际安装产物做制品检查——`file`/shebang 判定 `ProjectZomboid64`、`ProjectZomboid64.exe`、`start-server.sh`、`StartServer64.bat` 的真实类型（可执行二进制 vs 解释器脚本），结果写入 Target Manifest。**向量选择必须依据该证据**，不能依据文件名推测。

| 平台 | 首选向量 | 次选（需对抗测试通过） |
|---|---|---|
| Linux | 若制品证据表明存在可直接执行的二进制入口 → 直接执行 + 类型化 argv | `/bin/bash start-server.sh` + argv 数组（**不得**拼接为字符串）；若该脚本本身是包装器，仍视为解释器中转，须通过对抗测试 |
| macOS | 同上（legacy 已有直接执行 `ProjectZomboid64` 的分支） | 同上 |
| Windows | 直接执行 `ProjectZomboid64.exe` + argv（legacy 已有该分支） | `cmd.exe /c StartServer64.bat`（**仅当无法直接执行时**） |

**通用规则**
- 一律 `execve`/`CreateProcess` 直接传递 argv；禁止 `sh -c "..."`、`cmd /c "..."` 字符串拼接与插值。
- 环境变量：沿用“继承服务进程环境 + 显式覆盖必需变量（如 `LD_LIBRARY_PATH`）”（`core/instance_manager.py:211-213`）；禁止把**用户可控值**注入环境变量。
- `-adminpassword=<value>` 作为**单个 argv 元素**传递（此形态为权威；不要拆成两个 token）；密码值永不进入日志。

### 5.2 反模板插值（r1 H8 关闭）

**决策**：模板 `environments.*`（`executable`、`working_directory`、`start_arguments`）整体是 **legacy 元数据**，Go 实现**不得解析、不得插值、不得用于构造命令行或选择可执行文件**；启动规格由 `pz` 适配器在代码中以类型化 argv 构造（与 `core/instance_manager.py:186-208` 的实际行为一致）。**已登记不一致（r2 RV-207）**：模板 darwin 的 `executable` 为 `ProjectZomboid64`，而 legacy darwin 在 `start-server.sh` 存在时优先使用它（`core/instance_manager.py:200-202`）——以 §5.1 的制品证据与代码侧向量为权威。

**强制测试（M2-PROCESS）**：断言模板中的 `start_arguments` 字符串**从未**进入进程创建路径（例如：以包含 shell 元字符的模板文本运行启动规格构造，断言产出的 argv 与模板文本无关，且进程收到的 argv 与预期逐字节一致）。

### 5.3 对抗测试（M2-PROCESS）

对**每个声明支持的平台**与其**实际选用的向量**执行：
- 参数含 `'` `"` `` ` `` `$` `;` `&` `|` `(` `)` `<` `>` `^` `%` `!`、空格、换行、`$(...)`、`%VAR%`；
- 断言目标进程收到的参数与输入**逐字节一致**（helper 可执行文件回显 argv 验证），无额外命令执行、无参数分裂；
- 断言日志与状态输出不含密码值。
- 任一向量未通过 → 改用可直接执行的向量；两者皆失败 → **该平台不得声明支持**。

**与 Python 基线的差异**：legacy 的 Windows 与 macOS 分支已有直接执行回退（`core/instance_manager.py:196-197,204-205`），Linux 分支**始终**经 `/bin/bash start-server.sh`（`:206-208`）；三平台均未做注入对抗测试。本 ADR 的升级属**显式安全改进**（D7），不作为兼容承诺。

### 5.4 legacy 偏差采用表（r1 H6 / M1 关闭，权威）

| ID | 基线与目标 | 目标线格式（wire） |
|---|---|---|
| **D1** Origin/CSRF | 基线缺失 `Origin` 即放行 | 有副作用请求（POST/DELETE/PUT）缺 `Origin` 或跨源 → **403 `{"detail":"跨站请求已拒绝"}`**；只读 GET 允许缺 `Origin`、拒绝跨源；WS 必须匹配 `Origin`，否则 close **1008** |
| **D2** 登出/重放 | 基线登出只删 Cookie，令牌仍有效 | 会话令牌 = 随机 `sid` + 签名 Cookie；`sid` 存在于**服务端内存会话表**（TTL 12h）。登出 = 移除该 `sid` → 旧 Cookie 请求 → **401 `{"detail":"请先登录"}`**；进程重启 = 密钥轮换 + 空表 → 全部失效。不引入持久会话存储（单进程 + 本地部署） |
| **D3** WS 未运行输入 | 基线静默失败 | 未运行时**不转发**，回 `{"type":"error","data":"server_not_running"}`（**追加帧类型，已登记**）；连接不断开。REST `/api/server/command` 保持 400 `服务器未运行，无法发送控制台指令` |
| **D4** 安装取消/超时/回收 | 基线无取消语义，无 reap 证据 | 安装受 `context` 与 deadline 控制；取消/关闭时终止子进程树并 `wait` 回收；`install_state` 所有权清理。**不新增公开 cancel 路由**（保持 20 路由兼容面）；取消仅由 deadline/受控 shutdown 触发 |
| **D5** 持久化原子性 | 基线直写 | §1.4 原子写协议；失败不得报成功 |
| **D6** 跨进程单写者 | 基线无 | §3 全套；变更类 API 返回 **409 `instance owned by another process`** |
| **D7** 启动向量注入 | 基线脚本中转、未测 | §5.1–5.3；密码不经解释器重解析 |
| **D8** 文件权限 | 基线不检查权限 | §1.7 权限表；不合规 → 变更类操作被阻止 |

---

## 6. 迁移与回滚方案、M1-A/M1-B 边界

### 6.1 迁移步骤（M1）

1. **导出/盘点**：只读 inventory（legacy `instance.json`、`Zomboid/`（含 INI）、`server_files/` 元数据、`steamcmd/` 处理策略、模板），生成 `manifest.json`（版本 + 逐文件校验和 + 大小 + 权限）。
2. **dry-run**：报告保留原位/导入/拒绝项；不写源、不写 target。
3. **导入**：写入 `.migration/staging/<instance>/`（仅 state 内容）；校验和一致；权限按 §1.7 归一化。**同时**对原地保留的 `<data_root>/servers/`、`<instance_root>/`、`Zomboid/`、`Server/*.ini` 执行权限归一化（或显式运行 `gameserver fix-permissions --data-root`），并在 M1-A 清单与 M1-B runbook 中列为**首次 Go 启动前的必经步骤**——遗留 0755/0644 会触发 §1.7 的变更类操作阻断（r2 RV-202）。
4. **提升**：按 §4 状态机切换（只动 `state/`）；源数据在 `VERIFIED` 前保持只读，切换后**保留**（不删除）。
5. **对账**：Go 服务读取新数据根后，逐项比对 `LEGACY-CONTRACT.md` §9 兼容清单（API/WS/INI/脱敏/错误信封）。

**`server_files/` 与 `steamcmd/`**：原地保留、不复制、不删除，仅在 manifest 中记录存在性与元数据；`steamcmd/` 的写入受 `.migration.lock` 串行化（§3.1）。

### 6.2 回滚

- **回滚 = 撤销状态子树切换**：将 `<instance_root>/state` 移出（`.migration/prev/` 保留供审计），并将 journal 置为终态 **`LEGACY_ACTIVE`**（r2 RV-204），使 Python 基线继续以其 `instance.json`/INI 为权威；重新提升从 `STAGED` 入口重新开始，避免 §4.3 将有意回滚误判为损坏。
- **Go 写入后的回滚必须显式导出**：Go 运行期可能更新 INI（原地，Python 可直接读）与 Go state；回滚前执行 `gameserver export-back --to-legacy`，把 Go state 写回 legacy `instance.json`（等价字段映射；不可映射字段记录为 `unmappable`），否则 Python 会读到过期 variables。该步骤必须在演练中验证。
- 运行中进程**不跨版本接管**：切换前必须停止游戏进程并回收安装任务；检测到存活进程则中止并进入 `RECOVERY_REQUIRED`。
- 演练必须覆盖：Go 写入状态 → `export-back` → Python 启动并读回 → 字节级/字段级对账通过。

### 6.3 M1-A / M1-B 与 M2 验收 ID（r1 M2 关闭）

- **M1-A（切换就绪）**：ADR 批准 + 兼容矩阵（M2-API/M2-AUTH/M2-WS/M2-UI/M2-CONFIG/M2-SECRET）+ dry-run + 备份/恢复（§4.6）+ 双进程单写者（M2-SINGLEWRITER）+ promotion 中断恢复（M2-MIGRATE）+ 非生产副本回滚演练（§6.2）全部通过。
- **M1-B（用户授权后接管）**：具名目标（主机、数据根、实例、维护窗口）+ 用户单独授权 → 停服、切换、对账、回滚待命。未经授权不得执行，也不得表述为“已迁移接管”。
- **M2 基线验收**：`M2-BUILD / M2-API / M2-UI / M2-AUTH(登录/登出/Origin) / M2-WS / M2-SINGLEWRITER / M2-SECRET / M2-INSTALL(成功失败/取消回收) / M2-RESTART / M2-CONFIG(持久化/损坏写失败) / M2-MIGRATE / M2-PROCESS / M2-PZ-LIVE / M2-PLATFORM`（ID 集与已批准 Plan 一致，计划正文见 `docs/acceptance/M2-GO-PZ-MVP.md`，由任务 #12 产出）。
- 未执行 M1-B 时，M2 结果必须标注“隔离数据根基线验收”，不得作为接管证据。

### 6.4 Target Manifest（r1 M2 关闭）

- **位置**：`docs/acceptance/M2-GO-PZ-MVP.md` 内的 “Target Manifest” 章节（执行时填写），逐次验收一份；证据文件 `docs/acceptance/evidence/M2-OS-PZBUILD-TEMPLATE.md`。
- **最小字段**：Go commit/版本（来自 `TOOLCHAIN.md`）；PZ 版本与 Steam build ID；SteamCMD 版本；OS/发行版/版本/架构；数据隔离根目录绝对路径；网络/端口与防火墙状态；浏览器版本（若保留 UI）；**启动向量的制品证据**（§5.1）；就绪 oracle 选择（A2S 或日志标记）。
- **owner**：目标环境 operator 填写；reviewer 复核；用户对“真实副作用”单独授权后方可执行。

---

## 7. 未决项（owner = 用户）

| 项 | 触发条件 | 影响的决策 |
|---|---|---|
| 静态加密/OS keyring 保护游戏密码 | 多用户或备份外发 | §1.7 |
| 前端重建（Vue/Vite） | 产品要求多页面/组件化 | §1.3 |
| 引入数据库 | 多实例/查询/审计需求（M5） | §1.4 |
| 独立 daemon/远程节点 | M6 阶段且完成身份/幂等设计 | §1.5 |
| 备份保留策略产品化（自动清理） | M3 可靠性阶段 | §4.6 |

上述未决项**不阻塞** M1/M2 实现路径；任一项被选择时必须新增 ADR。

---

## 8. 决策与验收矩阵的对应

| 决策 | 对应验收 ID |
|---|---|
| §1.1–1.2 HTTP/WS | M2-API、M2-WS |
| §1.3 静态 UI + §1.1 错误信封 | M2-UI、M2-API |
| §1.4 持久化原子写 + 日志缓冲 | M2-CONFIG、M2-WS |
| §1.7 秘密与权限（D8） | M2-SECRET、M2-CONFIG |
| §1.8 工具链/CI | M2-BUILD |
| §2 依赖边界 + archtest | M2-BUILD |
| §2.3 就绪判决 | M2-PZ-LIVE、M2-API |
| §3 单写者 | M2-SINGLEWRITER、M2-RESTART |
| §4 promotion + 备份 | M2-MIGRATE、M1-A |
| §5 启动向量 + D1–D8 | M2-PROCESS、M2-AUTH、M2-INSTALL |

## 9. 后果与风险

- **正面**：依赖面最小（stdlib + 1 个 WS 库 + `x/sys`）；模块可独立测试；所有权与迁移中断语义可判定、可失败关闭；状态子树切换使 GB 级数据免于搬移。
- **代价**：需自行实现中间件、原子写与跨平台锁；Windows 锁语义与文件原子性需单独测试（当前无 Windows 证据）。
- **残余风险**：游戏管理员密码经 argv 对同机同权限进程可见（PZ 限制）；若目标平台脚本向量无法通过对抗测试，该平台无法声明支持。

## 10. r1 审查闭环记录

| r1 发现 | 级别 | 本 r2 处理 |
|---|---|---|
| C1 锁/所有权在被 rename 的目录内 | Critical | §3.1 迁移到 `<data_root>/servers/.locks|.owners`，staging 排除锁文件，§3.4 增加提升后回归用例 |
| H1 对账表自相矛盾 + 缺行 | High | §3.2 明确为权威表，缺记录按失败关闭，新增指纹/token/root 三个不匹配行 |
| H2 `recover` 未持锁 | High | §3.3 要求先取同一非阻塞独占锁，取不到即拒绝 |
| H3 整目录切换与“不复制 server_files”矛盾 | High | §4.1 改为只切换 `state/`；存档/服务端文件/SteamCMD 原地不动 |
| H4 journal 顺序与恢复漏洞（可达空根假 `COMMITTED`） | High | §4.2 不变量（COMMITTING 先落盘）、前滚需内容校验和、非终态 journal 禁止创建实例目录；§4.3 补全状态×现场表 |
| H5 prev 冲突导致重复提升失败/步骤 1 语义缺失 | High | §4.2 prev 带 `seq` 永不覆盖；步骤 1/2 失败语义与落表明确 |
| H6 D1/D2/D4 未采用 | High | 新增 §5.4 权威采用表，D2 明确内存会话表 + 登出吊销 + 重启失效 |
| H7 UI 依赖的错误信封与字符串未记录 | High | `LEGACY-CONTRACT.md` 新增 §2.3/§2.4；§1.1 强化 `{"detail"}` 与追加字段规则；§1.3 计数更正为 10 调用点/13 端点 |
| H8 模板 `start_arguments` 字符串插值冲突 | High | §5.2 明确模板字符串永不解析/插值 + 强制断言测试 |
| H9 Linux 直接可执行假设缺证据 | High | §5.1 增加制品证据前置步骤；§5.3 更正 legacy 描述（Win/macOS 有直接回退，Linux 恒为脚本） |
| M1 权限规则不可判定 | Medium | §1.7 权限表（路径→模式→时机→处理）+ D8 + `fix-permissions` |
| M2 备份/工具链/ID 引用未定义 | Medium | §4.6 备份决策；§1.8 `TOOLCHAIN.md` 单点固定；§6.3/§6.4 内联 ID 与 Target Manifest |
| M3 就绪判定缺标准 | Medium | §2.3 A2S/日志标记 + 加法式字段约束 |
| M4 日志保留缺决策 | Medium | §1.4 环形缓冲 1000 行/重启清空 |
| M5 规则 5 与静态资源矛盾、archtest 弱 | Medium | §2.1 规则 5 收窄为“实例数据文件系统”，新增 `staticassets`/`StaticAssets`；archtest 使用 `-test` 与直接导入 |
| M6 工具链浮动 + race 例外 | Medium | §1.8 固定语言基线 + `TOOLCHAIN.md` 三处一致 + race 必跑 ubuntu CI |
| L1 指纹泄漏风险 | Low | §1.7 指纹基于脱敏 argv |
| L2 `.bak` 顺序/INI/darwin | Low | §1.4 明确协议与平台限制 |
| L3 严格性边界 | Low | §1.4 边界声明 + override 参数 |
| L4 共享目录未串行化 | Low | §3.1 数据根级 `.migration.lock` 规则 |
| 追加矛盾：模板 `supported_os` vs 不声称支持 | — | §1.6 模板元数据例外 + M2-API 记录要求 |

---

## 11. r2 复审闭环记录

第 2 轮独立复审（`agent://9dbff804-ab94-4743-9830-170a7c92ba2e`）结论：**19 CLOSED / 2 PARTIAL**，判定 NEEDS_REVISION（1 High）。本 r3 对全部残留项的处理：

| 发现 | 级别 | 处理位置 |
|---|---|---|
| RV-201 §4.3 缺 `COMMITTING` 三个现场行与首次提升路径 | High | §4.3 新增三行（COMMITTING 共 7 行）+ §4.2 首次提升说明 + §4.5 改为“全部现场组合” |
| RV-202 遗留目录权限阻断首次启动 | Medium | §6.1 步骤 3 增加归一化要求与 M1-A/M1-B 必经步骤 |
| RV-203 D8 缺 legacy `instance.json` | Low | §1.7 权限表新增行 |
| RV-204 有意回滚无终态 | Low | §4.2 状态图 + §4.3 + §6.2 引入 `LEGACY_ACTIVE` |
| RV-205 缺 `INSTALLING` 枚举 | Low | §2.3 补全并标注 `steamcmd.is_busy` 映射 |
| RV-206 `.bak` 未校验即恢复 | Low | §1.4 增加 `.bak` 完整性门 |
| RV-207 模板 `executable` 与 legacy darwin 不一致 | Low | §5.2 扩展到 `environments.*` 并登记不一致 |
| RV-208 §4.1 排除规则措辞歧义 | Low | §4.1 改为“仅允许 `state/` 内容” |
| RV-209 契约行号/计数漂移 | Low | `LEGACY-CONTRACT.md` §2.3/§9 已修正 |
| RV-301 回滚走廊现场未成行/与回滚原语矛盾（第 3 轮复审新增） | High | §4.2 回滚原语语义 + §4.3 `ROLLED_BACK`/`ROLLING_BACK` 三行 + §4.5 覆盖回滚走廊 |
