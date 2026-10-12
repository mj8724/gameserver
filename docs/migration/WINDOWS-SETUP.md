# Windows 测试主机接入与 G 盘安装手册

> 依据用户 2026-09-28 决定：**Windows 为首要支持平台**，测试工作目录建在 **G 盘**。
> 目标主机：`winssh.liubaitech.cn`（SSH，用户 `admin`）/ `wingame.liubaitech.cn`（RDP，自治代理不可用）。
> 本文是执行手册，不代表任何步骤已经执行；每项真实副作用仍需在操作前取得用户对该动作的授权。

## 运行配置（环境变量）

服务只读环境变量，不做系统级修改。Windows 目标机上至少需要：

| 变量 | 用途 | 示例 |
|---|---|---|
| `GAMESERVER_PORT` | 监听端口 | `8769` |
| `GAMESERVER_DATA_ROOT` | 数据根（含 `servers/`） | `G:\gameserver-work\data` |
| `GAMESERVER_ADMIN_PASSWORD` | 管理口令（缺失则登录/受保护操作 503） | 由你设置，勿写入日志或脚本 |
| `GAMESERVER_STATIC_DIR` | 静态 UI 目录 | `G:\gameserver-work\app\static` |
| `GAMESERVER_TEMPLATES_DIR` | 模板目录 | `G:\gameserver-work\app\templates` |
| `GAMESERVER_LAUNCH_EXECUTABLE` | 直接可执行制品名 | `ProjectZomboid64.exe` |
| `GAMESERVER_LAUNCH_DIRECT_EXEC` | 必须为 `1`（解释器向量被禁用） | `1` |
| `GAMESERVER_LAUNCH_EVIDENCE_REF` | 启动向量证据引用（Manifest 锚点） | `manifest#win-2026xx` |
| `GAMESERVER_STEAMCMD_EXECUTABLE` | SteamCMD 可执行文件绝对路径 | `G:\gameserver-work\data\servers\pz_01\steamcmd\steamcmd.exe` |
| `GAMESERVER_STEAMCMD_DIR` | SteamCMD 工作目录（缺省 `<实例>/steamcmd`） | 同上目录 |
| `GAMESERVER_STEAM_APP_ID` | 覆盖模板中的 app id（缺省取模板 `steam.app_id` = 380870） | `380870` |

未提供 SteamCMD 可执行文件或 app id 时，服务**失败关闭**：安装请求会返回错误而不是假装在装。

## Windows 进程语义（与 Linux 的差异）

- 停止/强杀都会终止**整棵进程树**（`taskkill /PID <pid> /T /F`，类型化 argv、不经 shell）；Windows 没有 SIGTERM，优雅停止依赖通过控制台 stdin 下发命令，超时后才升级为树终止。
- 子进程创建时使用 `CREATE_NEW_PROCESS_GROUP`。
- 存活探测无法枚举命令行：存在陈旧所有权记录时一律**失败关闭**，需显式 `gameserver recover`（默认拒绝活跃 PID，`--force` 才清除并留审计记录）。
- POSIX 权限位不适用：`fix-permissions` 在 Windows 上不做任何改动（由 ACL 决定）。

## 首次启动前：权限归一（必须）

legacy/复制过来的数据根通常保留 0755/0644。ADR §1.7 要求秘密相关路径权限不合规时**失败关闭**，服务不会静默 chmod，因此首次启动前必须显式执行一次：

```powershell
G:\gameserver-work\bin\gameserver.exe fix-permissions --data-root G:\gameserver-work\data --instance pz_01
```

未执行时的典型症状：配置保存返回 500（或“保存配置失败”），HTML/日志看起来一切正常。Windows 上该命令输出说明 POSIX 权限位不适用并**不做任何改动**（Windows 由 ACL 决定），但相同步骤在 Linux 目标机上必须执行。

## 0. 当前阻塞（必须先解决）

本机访问被 Cloudflare Access 拦截：

```
cloudflared access ssh --hostname winssh.liubaitech.cn
→ websocket: bad handshake
```

原因：本机 `~/.cloudflared` 不存在，即未完成 Access 客户端认证。**需要用户在本机执行一次**：

```bash
cloudflared access login winssh.liubaitech.cn   # 浏览器登录，令牌随后缓存在 ~/.cloudflared
```

登录后可用（本地 `~/.ssh/config` 可选，便于人工使用；自动化侧由 SSH 管理器/ProxyCommand 承担）：

```sshconfig
Host win-game
  HostName winssh.liubaitech.cn
  User admin
  ProxyCommand cloudflared access ssh --hostname %h
```

非交互访问还需要 SSH 密钥（口令登录无法在自动化中完成）；若只有口令，需要用户自行 `ssh-copy-id` 或改用密钥。

## 1. 目标目录布局（G 盘）

| 用途 | 路径 |
|---|---|
| 工作根 | `G:\gameserver-work\` |
| Go 工具链 | `G:\gameserver-work\go\`（解压 `go1.27.1.windows-amd64.zip`） |
| 仓库副本 | `G:\gameserver-work\repo\` |
| 隔离测试数据根 | `G:\gameserver-work\testdata\<instance>\`（一次性，非生产） |
| 备份目录 | `G:\gameserver-work\backups\` |
| SteamCMD（后续授权后） | `G:\gameserver-work\steamcmd\` |

## 2. 接入后的第一步：只读侦察（无需授权，先取证）

```powershell
[System.Environment]::OSVersion.Version
Get-CimInstance Win32_OperatingSystem | Select-Object Caption,Version,BuildNumber,OSArchitecture
Get-Volume | Where-Object DriveLetter -eq 'G' | Select-Object DriveLetter,FileSystem,SizeRemaining
Get-PSDrive -PSProvider FileSystem | Select-Object Name,Used,Free
# 已有工具链
$env:Path = "$env:Path"; Get-Command go,git,python,cloudflared -ErrorAction SilentlyContinue
```

记录到 `docs/acceptance/evidence/` 的对应 Windows 证据文件；此步只读，不改系统。

## 3. 建立 G 盘工作根（写操作，需授权）

```powershell
New-Item -ItemType Directory -Force -Path 'G:\gameserver-work\','G:\gameserver-work\testdata\','G:\gameserver-work\backups\' | Out-Null
```

## 4. 安装 Go 1.27.1（写操作，需授权）

固定版本见 `docs/migration/TOOLCHAIN.md`（go1.27.1，官方校验和 `ee215d57…` 为 darwin 包；Windows 包需另取官方 zip 的 sha256 并记录）。

```powershell
$V='1.27.1'
Invoke-WebRequest "https://dl.google.com/go/go$V.windows-amd64.zip" -OutFile "G:\gameserver-work\go$V.zip"
Get-FileHash "G:\gameserver-work\go$V.zip" -Algorithm SHA256   # 与 go.dev 官方元数据比对
Expand-Archive "G:\gameserver-work\go$V.zip" -DestinationPath 'G:\gameserver-work\' -Force
& 'G:\gameserver-work\go\bin\go.exe' version
```

PATH 仅在本会话设置，不改系统环境变量：

```powershell
$env:Path = 'G:\gameserver-work\go\bin;' + $env:Path
```

## 5. 取仓库副本并跑门禁（写操作，需授权）

```powershell
git clone https://github.com/mj8724/gameserver.git G:\gameserver-work\repo
cd G:\gameserver-work\repo
git checkout feat/go-modular-skeleton
$env:Path = 'G:\gameserver-work\go\bin;' + $env:Path
go vet ./...
go test ./...
go build ./...
go test ./internal/archtest/...
```

说明：
- **Windows 的 `-race` 需要 C 工具链（cgo/gcc）**；目标机若无 gcc，则记录“Windows 不具备 race 证据”，race 门继续由 Ubuntu CI 承担（ADR §1.8），不得因此降低 M2-BUILD 要求。
- 当前 Windows 侧的 `oslock`（`LockFileEx`）与 `process`（`cmd.exe /c` / 直接 `.exe`）分支尚无实机证据，本轮结果直接决定它们是“已验证”还是保持“未验证”。

## 6. 后续（每项都需单独授权）

1. M1-A 隔离副本演练：在 `G:\gameserver-work\testdata\` 上用迁移工具跑 dry-run→import→promote→备份/恢复/中断注入。
2. 启动向量制品取证：检查目标 PZ 安装产物（`ProjectZomboid64.exe` 是否为真 PE 可执行、`StartServer64.bat` 内容），据此选择 direct-exec 或 `cmd.exe /c`，并写入 Target Manifest。
3. SteamCMD + PZ 安装/就绪/控制台/端口/停止实机验收（M2-PZ-LIVE）。
4. 防火墙/端口可见性变更：仅在用户明确要求且记录规则后执行；本代理不自行改防火墙。

## 7. 边界与红线

- 不在该机执行生产接管（M1-B 未授权）。
- 不把 Windows 结果声明为“已支持”，直到 M2-PZ-LIVE 与启动向量对抗测试在该机通过。
- 不打印/记录口令、令牌、私钥；RDP 通道本代理不使用。
- 所有隔离数据根仅用于测试，删除/清理需另行授权。

### 启动向量（2026-09-29 实机取证后新增）

Windows 服务端包**不含** `ProjectZomboid64.exe`（制品清单见 `docs/acceptance/TARGET-MANIFEST-windows.md`），因此该平台使用第三向量 `launcher-descriptor`：直接执行 `jre64\bin\java.exe` + 类型化 argv（不经 shell、不用 `.bat`）。

| 变量 | 用途 | 取值 |
|---|---|---|
| `GAMESERVER_LAUNCH_VECTOR` | 选择启动向量；**缺省不探测**（沿用 direct-executable） | `launcher-descriptor` |
| `GAMESERVER_SERVER_MEMORY_MB` | JVM 堆（`-Xms==-Xmx`）；缺省取厂商描述文件的 `-Xmx` | 例如 `4096` |
| `GAMESERVER_LAUNCH_EVIDENCE_REF` | 必须非空（向量证据引用） | `manifest#win-live-1` |

`GAMESERVER_LAUNCH_EXECUTABLE` / `GAMESERVER_LAUNCH_DIRECT_EXEC` 仅对 direct-executable 生效。启用 descriptor 时同时需要显式向量 env 与证据引用，缺一即拒绝启动；`vmArgs` 只接受 Target Manifest 逐字登记的 token，`-Xmx/-Xms` 由本服务接管，`java/. → java/` 是厂商笔误的确定性纠正。

## 8. 实例隔离方案（r4 定案：专用服务账户）

**背景**：PZ 在 Windows **忽略 `-cachedir` 的配置语义**——INI 与 `*_SandboxVars.lua` 固定写入运行账户的 `%USERPROFILE%\Zomboid\Server`，而存档/DB 才落在 `-cachedir`。共享 profile 会让不同实例的配置互相覆盖，且与"实例化"语义冲突。用户于 2026-09-29 定案采用**专用服务账户**（记录见 `docs/acceptance/M2-PLAN-DECISION.md` r4 D-R4-2）。

### 8.1 目标形态

| 项 | 取值 | 说明 |
|---|---|---|
| 运行账户 | `gs-pz`（本地账户，非管理员） | 每个实例一个账户；禁止用 `SYSTEM`/管理员账户运行游戏进程 |
| 账户 profile | `C:\Users\gs-pz\` | PZ 配置根 = `C:\Users\gs-pz\Zomboid`（游戏自己选的路径，非我们指定） |
| `GAMESERVER_PZ_HOME` | `C:\Users\gs-pz\Zomboid` | 把我们的 INI/SandboxVars 读写点对齐到游戏真正读取的位置 |
| 缓存目录 | `<data_root>\servers\<id>\Zomboid`（**仍归实例所有**） | `-cachedir`；存存档/DB；不得与配置根混淆 |
| 数据根 | `G:\gameserver-work\data` | ACL：`gs-pz` 与运维账户**修改**，其它用户无权限；不继承 `Users` 组 |
| 服务 | `gameserver-<id>`（若以服务方式运行） | 登录身份 = `gs-pz`；`GAMESERVER_*` 环境变量随服务配置下发 |

### 8.2 落地步骤（每步需授权）

```powershell
# 1) 建账户（一次性）
$pw = Read-Host -AsSecureString "gs-pz password"
New-LocalUser -Name gs-pz -Password $pw -PasswordNeverExpires -AccountNeverExpires
# 2) 数据根 ACL：仅 gs-pz 与运维账户，去掉继承
icacls G:\gameserver-work\data /inheritance:r
icacls G:\gameserver-work\data /grant "gs-pz:(OI)(CI)M" "Administrators:(OI)(CI)F"
# 3) 以该账户首次登录一次，生成 profile 与 Zomboid 目录（或由安装流程以该身份运行一次）
# 4) 启动时注入配置根对齐
setx /m GAMESERVER_PZ_HOME C:\Users\gs-pz\Zomboid
# 5) 服务身份（若用服务运行）
sc.exe config gameserver-pz_01 obj= ".\gs-pz" password= "<pw>" start= auto
```

**权限检查**：Windows 路径不检查 POSIX 位（ADR §1.7），仍以 ACL 为准；`fix-permissions` 在 Windows 只做存在性/归属核对，不改 ACL。

### 8.3 验证与回滚

- **验证**：启动后确认 ① `C:\Users\gs-pz\Zomboid\Server\<name>.ini` 与 `*_SandboxVars.lua` 被真正读取（改一个值 → 游戏内生效）；② 存档/DB 落在 `<data_root>\servers\<id>\Zomboid`；③ 另一实例的配置不受影响；④ `tasklist /v` 显示 java 进程属主为 `gs-pz`。
- **回滚**：停服 → 恢复原运行身份与环境变量 → 数据根 ACL 复原（`icacls /reset`）→ 删除账户；配置根回退时把 `gs-pz` profile 下的 INI/SandboxVars 复制回目标位置（保留 `.bak1..3`）。
- **副作用声明**：涉及本机账户/ACL 变更，属系统级动作，必须逐项授权；不修改防火墙与系统服务策略（除上述服务注册）。

### 8.4 对 r3 记录差异的影响

| 差异 | 在专用账户下的处理 |
|---|---|
| 冷启动地图生成超出 60s 就绪窗口 | 与账户无关（属 PZ 冷启动特性）；按 Manifest 记录的窗口与首次/再次启动分别判定 |
| PZ 退出时以运行时状态重写 sandbox 文件 | 与账户无关；隔离后仍需在停止后复读并核对（配置根单一写入者已由 `gs-pz` 独占强化） |

`GAMESERVER_READINESS_TIMEOUT`（秒，缺省 60，上限 3600）：Manifest 记录的就绪窗口覆盖，用于 marker 到达晚于 60s 的机器。

**容量策略（M3.5，可选）**：`GAMESERVER_CAPACITY_SOFT_PERCENT`（告警）/`GAMESERVER_CAPACITY_HARD_PERCENT`（拒绝新增写入，409），按实例 `quota_gb` 计算；缺省 0 = 禁用（保持历史行为）。超限恢复路径=调阈值或清旧备份（**系统永不自动删除数据**）。用量口径=实例根递归（不含备份与 `#locks/#owners`）。

**SteamCMD 凭据（可选，默认匿名）**：`GAMESERVER_STEAM_LOGIN` / `GAMESERVER_STEAM_PASSWORD`。**默认不设置**——本项目出厂边界为 `+login anonymous` 且不存储任何 Steam 凭据（ADR §1.7）。仅当需要**下载 Steam Workshop 内容**时才必须配置（匿名会被 Steam 以 `Failure` 拒绝，实测见 `docs/acceptance/evidence/M3-residuals-live.md §2/§6.3`）：配置后 SteamCMD 以该账号登录，密码作为独立 argv 元素传入并在日志中脱敏（`internal/adapters/steamcmd`：`progressReader.extraSecrets`，断言 `TestBuildArgsCredentialedLoginIsOptionalAndInert`、`TestProgressReaderRedactsConfiguredPassword`）。该通道属"凭据管理"产品决策（决策点 B），启用即自担账号风险。
