# Windows 测试主机接入与 G 盘安装手册

> 依据用户 2026-09-28 决定：**Windows 为首要支持平台**，测试工作目录建在 **G 盘**。
> 目标主机：`winssh.liubaitech.cn`（SSH，用户 `admin`）/ `wingame.liubaitech.cn`（RDP，自治代理不可用）。
> 本文是执行手册，不代表任何步骤已经执行；每项真实副作用仍需在操作前取得用户对该动作的授权。

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
