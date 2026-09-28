# 游戏服务器 Web 控制面系统架构与详细功能设计规范
### —— 跨平台（Windows / Linux）、Steam 游戏驱动、Mod自动化、容量边界防护、用户与续费生命周期、主控预留架构

---

## 目录
1. [项目愿景与核心定位](#1-项目愿景与核心定位)
2. [专家团视角分析与业界主流系统深度解构](#2-专家团视角分析与业界主流系统深度解构)
   - 2.1 [Agency-Agents 专家团评审阵容与分析维度](#21-agency-agents-专家团评审阵容与分析维度)
   - 2.2 [WindowsGSM 核心机制解构 (C# / Win32)](#22-windowsgsm-核心机制解构-c--win32)
   - 2.3 [GameAP & GameAP Daemon 核心机制解构 (Go / gRPC)](#23-gameap--gameap-daemon-核心机制解构-go--grpc)
   - 2.4 [PufferPanel 核心机制解构 (Go / Vue / WebSocket)](#24-pufferpanel-核心机制解构-go--vue--websocket)
   - 2.5 [CubeCoders AMP 与 TCAdmin 商业级架构剖析](#25-cubecoders-amp-与-tcadmin-商业级架构剖析)
   - 2.6 [业界标杆横向对比分析矩阵](#26-业界标杆横向对比分析矩阵)
3. [系统总体架构设计](#3-系统总体架构设计)
   - 3.1 [核心设计原则：单机先驱部署，主控微内核解耦](#31-核心设计原则单机先驱部署主控微内核解耦)
   - 3.2 [总体架构拓扑与数据流图](#32-总体架构拓扑与数据流图)
   - 3.3 [技术栈选型与理由矩阵](#33-技术栈选型与理由矩阵)
   - 3.4 [系统各功能子系统极度解耦架构 (Hexagonal & Event-Driven)](#34-系统各功能子系统极度解耦架构-hexagonal--event-driven)
4. [Steam 游戏运维引擎与 Mod 获取自动化体系](#4-steam-游戏运维引擎与-mod-获取自动化体系)
   - 4.1 [SteamCMD 全自动管理流水线](#41-steamcmd-全自动管理流水线)
   - 4.2 [声明式游戏服务器模板引擎 (Template Engine)](#42-声明式游戏服务器模板引擎-template-engine)
   - 4.3 [跨平台终端控制台与日志双向流 (ConPTY / PTY)](#43-跨平台终端控制台与日志双向流-conpty--pty)
   - 4.4 [非侵入式游戏状态探测 (Valve A2S UDP 协议)](#44-非侵入式游戏状态探测-valve-a2s-udp-协议)
   - 4.5 [游戏 Mod 自动化获取与生态适配矩阵](#45-游戏-mod-自动化获取与生态适配矩阵)
   - 4.6 [Mod 依赖树解析与开机前增量热更新机制](#46-mod-依赖树解析与开机前增量热更新机制)
   - 4.7 [跨平台操作系统硬性兼容准则 (明确拒绝 Wine 陷阱，必须 Windows 绝不强行 Linux)](#47-跨平台操作系统硬性兼容准则-明确拒绝-wine-陷阱必须-windows-绝不强行-linux)
   - 4.8 [GSLT 登录令牌托管与自动化池化分配](#48-gslt-登录令牌托管与自动化池化分配)
   - 4.9 [Steam Guard 2FA 交互式流式认证](#49-steam-guard-2fa-交互式流式认证)
   - 4.10 [Steam Beta 分支与版本密码切换管理](#410-steam-beta-分支与版本密码切换管理)
5. [磁盘容量配额与边界超限纵深防护体系](#5-磁盘容量配额与边界超限纵深防护体系)
   - 5.1 [业务风险与爆炸半径 (Blast Radius) 分析](#51-业务风险与爆炸半径-blast-radius-分析)
   - 5.2 [四层纵深防护架构体系](#52-四层纵深防护架构体系)
   - 5.3 [超容有限状态机 (Over-Quota State Machine)](#53-超容有限状态机-over-quota-state-machine)
   - 5.4 [操作系统底层硬隔离实现方案 (Linux vs Windows)](#54-操作系统底层硬隔离实现方案-linux-vs-windows)
6. [前台 Web 控制面全功能模块与交互界面设计](#6-前台-web-控制面全功能模块与交互界面设计)
   - 6.1 [视觉风格与全局交互骨架 (Layout & Dark Gaming Aesthetic)](#61-视觉风格与全局交互骨架-layout--dark-gaming-aesthetic)
   - 6.2 [概览仪表盘 (Dashboard / Overview)](#62-概览仪表盘-dashboard--overview)
   - 6.3 [工业级 Web 终端控制台 (Terminal / xterm.js Console)](#63-工业级-web-终端控制台-terminal--xtermjs-console)
   - 6.4 [游戏 Mod 模组中心 (Mod Center & Workshop Manager)](#64-游戏-mod-模组中心-mod-center--workshop-manager)
   - 6.5 [双模式可视化参数配置中心 (Visual Config: Form & Monaco)](#65-双模式可视化参数配置中心-visual-config-form--monaco)
   - 6.6 [在线文件管理器与内置 SFTP (Web File Manager & SFTP)](#66-在线文件管理器与内置-sftp-web-file-manager--sftp)
   - 6.7 [在线玩家管理与实时踢封 (Player Management & Rules)](#67-在线玩家管理与实时踢封-player-management--rules)
   - 6.8 [计划任务与智能自动化运维 (Scheduled Tasks & Smart Automation)](#68-计划任务与智能自动化运维-scheduled-tasks--smart-automation)
   - 6.9 [备份与快照管理 (Backups & Snapshots)](#69-备份与快照管理-backups--snapshots)
   - 6.10 [商业续费、升降配与收银台 (Billing, Renewal & Upgrades)](#610-商业续费升降配与收银台-billing-renewal--upgrades)
   - 6.11 [Steam OpenID 2.0 快捷登录与多渠道 Webhook 报警通知中心](#611-steam-openid-20-快捷登录与多渠道-webhook-报警通知中心)
7. [用户管理体系与细粒度 RBAC 权限设计](#7-用户管理体系与细粒度-rbac-权限设计)
   - 7.1 [多级租户与用户角色模型](#71-多级租户与用户角色模型)
   - 7.2 [细粒度权限作用域 (Permission Scopes)](#72-细粒度权限作用域-permission-scopes)
   - 7.3 [API Key 与免密协作令牌体系](#73-api-key-与免密协作令牌体系)
8. [计费系统、续费与实例生命周期状态机](#8-计费系统续费与实例生命周期状态机)
   - 8.1 [商业套餐规格与定价模型](#81-商业套餐规格与定价模型)
   - 8.2 [实例生命周期有限状态机 (FSM)](#82-实例生命周期有限状态机-fsm)
   - 8.3 [续费、宽限期与自动化停机/归档/回收工作流](#83-续费宽限期与自动化停机归档回收工作流)
   - 8.4 [支付网关集成与幂等性对账机制](#84-支付网关集成与幂等性对账机制)
   - 8.5 [主机商容量超卖水位与节点熔断控制 (Overcommit Ratios)](#85-主机商容量超卖水位与节点熔断控制-overcommit-ratios)
   - 8.6 [营销优惠券、促销码与推广分销返利系统](#86-营销优惠券促销码与推广分销返利系统)
9. [跨平台底层进程监督与安全沙箱](#9-跨平台底层进程监督与安全沙箱)
   - 9.1 [进程保活、崩溃恢复与防死循环熔断看门狗](#91-进程保活崩溃恢复与防死循环熔断看门狗)
   - 9.2 [跨平台计算资源配额 (Windows Job Objects & Linux cgroups v2)](#92-跨平台计算资源配额-windows-job-objects--linux-cgroups-v2)
   - 9.3 [目录越权 (Path Traversal) 与安全隔离沙箱](#93-目录越权-path-traversal-与安全隔离沙箱)
   - 9.4 [全局端口池分配引擎与多网卡 IP 绑定 (Port Pool & Multi-IP)](#94-全局端口池分配引擎与多网卡-ip-绑定-port-pool--multi-ip)
   - 9.5 [操作系统防火墙规则自动化联动 (Windows netsh / Linux nftables)](#95-操作系统防火墙规则自动化联动-windows-netsh--linux-nftables)
   - 9.6 [底层依赖静默自愈医生 (VC++ Redistributable, 32-bit glibc)](#96-底层依赖静默自愈医生-vc-redistributable-32-bit-glibc)
   - 9.7 [非特权用户运行沙箱与 RCON 暴力破解防御 (Fail2ban)](#97-非特权用户运行沙箱与-rcon-暴力破解防御-fail2ban)
   - 9.8 [全链路不可篡改审计追踪 (Audit Trail)](#98-全链路不可篡改审计追踪-audit-trail)
10. [数据库设计与核心实体关系 (ERD)](#10-数据库设计与核心实体关系-erd)
11. [从单机控制到分布式主控演进实施路径](#11-从单机控制到分布式主控演进实施路径)

---

## 1. 项目愿景与核心定位

本项目旨在打造一个**面向中小型主机商、游戏社区、私人服主的高性能现代化游戏服务器 Web 控制面**。

### 核心业务特征：
1. **多端跨平台支持**：原生无缝运行于 **Windows** (Windows Server 2016/2019/2022、Windows 10/11) 与 **Linux** (Ubuntu, Debian, CentOS, Rocky Linux, Alpine) 操作系统。
2. **深度聚焦 Steam 专用服务器生态**：提供对 Valve SteamCMD 生态的开箱即用自动化支持（一键安装、自动鉴权、验证修复、创意工坊 Mod 订阅管理、分支版本切换、A2S 状态探针、RCON 交互）。
3. **商业化用户管理与续费生命周期**：内建完整的多租户权限体系（超级管理员、服主、协管员、访客）与商业计费订阅状态机（套餐配额、自动续费、到期预警、欠费宽限、锁定停机、冷备份归档与安全回收）。
4. **单机自洽落地，架构原生预留主控**：
   - **当前落地目标**：做到**单台服务器极简交付与控制**（Web 控制面与执行代理同机共存，零外部依赖，极速部署）。
   - **架构设计要求**：系统内核采用 **Master（控制面脑）** 与 **Daemon（执行面手）** 的解耦通信模型。未来向多节点、分布式机房横向扩展时，无需重构任何业务逻辑与数据库结构，仅需将 Daemon 部署至远程节点并接入主控即可。

---

## 2. 专家团视角分析与业界主流系统深度解构

为了确保架构设计的先进性、高可靠性与避免走弯路，我们引入 `agency-agents` 专家团视角，对开源界与商业界 5 大知名游戏服务器管理系统进行了深度源码级解构。

### 2.1 Agency-Agents 专家团评审阵容与分析维度

| 专家角色 | 对应角色定义 (agency-agents) | 核心审查与把关维度 |
| :--- | :--- | :--- |
| **🏛️ 系统架构专家** | `engineering-software-architect.md` | 审查主控/节点架构解耦、单机自洽向集群演进的可扩展性、IPC/gRPC 通信性能、分层清晰度 |
| **🎮 游戏基础设施专家** | `engineering-desktop-app-engineer` + SRE | 审查 SteamCMD 自动化流程、跨平台进程生命周期、Win32 Console/ConPTY 终端流、A2S/RCON 协议接入 |
| **💳 计费与订阅生命周期专家** | `engineering-payments-billing-engineer.md` | 审查订阅状态机（创建/续费/宽限/挂起/归档/销毁）、幂等性设计、防超卖、账单与支付回调对账 |
| **🛡️ 安全与沙箱架构师** | `security-architect.md` + AppSec | 审查跨平台进程隔离（Windows Job Objects vs Linux cgroups）、目录越权与防遍历（Path Traversal）、Token 轮转、WebSocket 劫持防御 |
| **🎯 产品与运营专家** | `product-manager.md` | 审查用户开箱即用体验、管理员控制台、租户前台、自服务续费体验、降低运维人力成本 |

---

### 2.2 WindowsGSM 核心机制解构 (C# / Win32)

WindowsGSM 是目前 Windows 环境下最流行的桌面型游戏服务器管理工具之一。

```
WindowsGSM 核心架构:
[WPF UI (C#)]
  └── [Engine (Source, Unity, UnrealEngine)]
        ├── [SteamCMD.cs] -> 命令行参数拼接 (+force_install_dir, +login, +app_update)
        ├── [ServerConsole.cs] -> Win32 user32.dll (PostMessage / SendKeys) 绕过输入阻断
        ├── [ProcessManagement.cs] -> Win32_Process WMI 命令行与 PID 关联
        └── [Query/A2S.cs] -> 原生 UDP Client 实现 Valve A2S 协议心跳探测
```

#### 关键源码机制与发现：
1. **SteamCMD 参数构建管道 (`Installer/SteamCMD.cs`)**：
   - 自动拉取官方 `steamcmd.zip` 并静默解压到 `./bin/steamcmd/`。
   - 严格构建参数链：`+force_install_dir "<path>" +login <user> <pass> +app_set_config <appId> mod "<mod>" +app_update <appId> -beta <branch> validate +quit`。
   - **痛点修复**：针对 GoldSource 引擎（如 CS 1.6 / HL1，AppID 90），源码中发现其必须硬编码重复执行 4 次 `+app_update 90` 才能完整下载所有依赖文件的诡异 Bug。
2. **Win32 控制台输入崩溃难题 (`Engine/Source.cs` & `Functions/ServerConsole.cs`)**：
   - **致命陷阱**：在 Windows 上，当使用 C# 或常规进程管理尝试重定向 `srcds.exe` 的标准输入时（`RedirectStandardInput = true`），Source 引擎直接抛出致命崩溃：`CTextConsoleWin32::GetLine: !GetNumberOfConsoleInputEvents`。
   - **WindowsGSM 的妥协做法**：它无法使用标准管道输入，被迫利用 `user32.dll` 中的 `SetForegroundWindow` + `SendKeys` 或 `PostMessage(WM_KEYDOWN)` 向主窗口发送虚拟按键。
   - **专家团评估意见**：生产环境 Web 控制台绝不能依赖 `SendKeys`（会抢占系统剪贴板和窗口焦点，且在 Windows Service / Session 0 隔离模式下彻底失效）。**现代化解法必须采用 Windows 10/Server 2019+ 引入的 Windows Pseudo Console (ConPTY) 原生伪终端技术**。
3. **A2S UDP 探针协议 (`GameServer/Query/A2S.cs`)**：
   - 直接构造二进制 UDP 包（前导 4 字节 `0xFF 0xFF 0xFF 0xFF` 拼接 `TSource Engine Query\0`），接收响应处理 `0x41` (Challenge 握手)，直接获取服务器实时名称、地图、在线玩家数、最大玩家数与 Ping。无需解析游戏日志。

---

### 2.3 GameAP & GameAP Daemon 核心机制解构 (Go / gRPC)

GameAP 展现了极其成熟的 Master-Daemon 分布式架构设计。

```
GameAP 体系架构:
[GameAP Web (Master)]
      │
   (gRPC + mTLS 双向流通道)
      │
[GameAP Daemon (Node/Agent)]
      ├── [Enrollment] 一键安全配对 (CA根证书 + 客户端证书双向签发)
      ├── [gdaemon_scheduler] 异步任务队列 (Install, Update, Restart, Chained RunAftID)
      └── [ProcessManager] 跨平台插件化进程监管:
            ├── Linux: tmux (支持控制台附加), systemd (系统级/用户级服务), docker, podman
            └── Windows: winsw (Windows Service Wrapper), shawl, scm, simple
```

#### 关键源码机制与发现：
1. **主控与节点安全通信 (gRPC + mTLS)**：
   - 彻底摒弃高风险的明文 HTTP API，默认使用双向 TLS（mTLS）。Master 签发专属 CA，节点通过一次性 Setup Key 执行 `gameap-daemon enroll` 自动拉取证书完成认证绑定。
2. **跨平台进程管理抽象 (`internal/processmanager`)**：
   - Linux 环境提供 `tmux` 适配器：将游戏进程置于独立的 tmux 会话内，Daemon 通过 `tmux send-keys` 注入指令，通过捕获 buffer 提取日志，解决 Linux 下需要控制台交互的问题。
   - Windows 环境提供 `winsw` 与 `shawl` 适配器：将任何控制台可执行文件封装为 Windows 核心服务（SCM），支持 `NT AUTHORITY\NetworkService` 或指定独立低权限本地账户运行，系统重启后自动恢复。
3. **状态化异步任务队列 (`domain/gdaemon_task.go` & `gdaemon_scheduler/task_manager.go`)**：
   - 游戏安装与更新是长时间运行的操作（如 Ark/Rust 动辄 30GB+）。GameAP 在 Master 与 Node 间维护了异步任务模型：`TaskWaiting` -> `TaskWorking` -> `TaskSuccess / TaskError`。
   - 支持 `RunAftID` 依赖调度链：例如先执行 `gsstop`（停止服务器）-> 成功后再自动触发 `gsupd`（SteamCMD更新）-> 成功后再触发 `gsstart`（重新拉起）。支持输出实时分段回传。

---

### 2.4 PufferPanel 核心机制解构 (Go / Vue / WebSocket)

PufferPanel 展现了极高雅的单二进制架构（All-In-One Monolith），兼具轻量与强大。

```
PufferPanel 体系架构:
[Single Go Binary: PufferPanel]
  ├── [Web Engine (Gin)]
  │     ├── OAuth2 / JWT 细粒度权限作用域 (Scope)
  │     ├── 嵌入式 Vue 3 + Vite 前端静态资产
  │     └── 内置 SFTP 服务 (与面板用户鉴权联动)
  ├── [Template Engine] 声明式 JSON 配置模板
  └── [Server Instance Engine]
        ├── [operations/steamgamedl] SteamCMD + DepotDownloader 双模式
        ├── [connections/rcon.go, rconws.go] 协议级控制台与 WebSocket RCON
        └── [Multi-stream WebSocket] 单套接字复用 Console / Stats / Status
```

#### 关键源码机制与发现：
1. **声明式服务器模板 (`models/template.go` & `pufferpanel/server.go`)**：
   - 任何一款游戏（无需修改核心代码）均定义为一个结构化 JSON 文件：
     - `data`: 暴露给前端用户填写的变量（端口、地图名、密码、RCON端口）。
     - `install`: 声明式安装指令流水线（`steamgamedl`、`download`、`writefile`、`extract`、`command`）。
     - `run`: 运行命令、环境参数、优雅退出指令（`stop` 命令或退出信号）。
2. **DepotDownloader 与 SteamCMD 双轨设计 (`operations/steamgamedl`)**：
   - 除了支持原生 `steamcmd`，源码中创新性地集成了 `DepotDownloader`（开源 C# SteamKit 工具）。
   - **核心优势**：原生 SteamCMD 在无交互终端环境下偶发假死或因更新检查挂起，而 DepotDownloader 纯基于 HTTP/Depot 协议并发分块下载，支持 Lancache 局域网缓存代理，极速且不锁死。
3. **多路复用全双工 WebSocket (`web/daemon/server.go: openSocket`)**：
   - 前端单一 WebSocket 连接，通过 URL 查询参数（`?console=1&stats=1&status=1`）完成三个频道的事件复用：
     - `console`: 实时吸收进程 STDOUT 并下发 STDIN。
     - `stats`: 周期（5秒）推送 CPU 占用百分比、物理内存占用、网络 I/O 速率。
     - `status`: 状态机跃迁广播（`running`, `stopped`, `installing`, `crashed`）。

---

### 2.5 CubeCoders AMP 与 TCAdmin 商业级架构剖析

AMP 与 TCAdmin 代表了全球游戏服务器商业面板的巅峰标杆，重点提供了企业级多租户与计费集成模型。

```
商业化运营模型:
[WHMCS / 发卡网 / 计费中心]
       │ (REST / Webhook 驱动)
       ▼
[Master 控制台 (ADS / TCAdmin Core)]
       ├── 资源配额池 (CPU Cores, RAM, Disk, Ports)
       ├── 租户与服务器所有权绑定 (User ID -> Server Instance)
       └── 订阅周期监控定时器 (Cron Engine)
             ├── 到期前 3 天: 邮件 / Webhook 催缴
             ├── 到期当天: 标记 GRACE_PERIOD 宽限期
             ├── 逾期 48 小时: 执行 Suspend (强制关闭实例，锁定启动，保留数据)
             ├── 续费成功: 执行 Unsuspend (解锁权限，恢复运行)
             └── 逾期 7 天: 执行 Terminate (冷备份导出，物理擦除，回收端口)
```

#### 关键商业级设计提炼：
1. **CubeCoders AMP 的 ADS (Application Deployment Service) 控制器哲学**：
   - 控制器自身也是一个独立的轻量进程。主控（ADS Target）只负责协调，每一个游戏服务器是独立的隔离环境（Instance），彼此网络端口、配置环境完全隔离，崩溃单点绝不波及主控。
2. **TCAdmin 的生命周期命令标准**：
   - 其与 WHMCS 商业计费系统的对接接口规范已成为行业事实标准：
     - `Create`: 创建服务，根据套餐规格（内存限额、CPU优先级、槽位）开辟容器/目录，自动分配可用端口池中的端口。
     - `Suspend`: 扣费失败触发，立即下发停机指令，面板端将实例置灰，禁止任何开机与控制台操作。
     - `Unsuspend`: 补缴账单后触发，恢复所有操作权限。
     - `Change Package`: 补差价升配（例如 4 核 8G 升级至 8 核 16G），无需重装游戏，直接热修改底层沙箱配额。
     - `Terminate`: 最终清理，回收分配的公网/内网端口至端口池。

---

### 2.6 业界标杆横向对比分析矩阵

| 对比维度 | WindowsGSM | GameAP | PufferPanel | CubeCoders AMP | TCAdmin | **本项目设计方案** |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **开源/闭源** | 开源 (C#) | 开源 (Go) | 开源 (Go+Vue) | 商业闭源 | 商业闭源 | **开源自主可控 (Go+Vue3)** |
| **部署架构** | 单机桌面 GUI | 分布式 (Master-Daemon) | 单机一体化 (支持多节点) | ADS 控制器 + 实例 | 主控 Master + 远程 Node | **单机一体化优先，内核严格解耦** |
| **操作系统支持** | 仅 Windows | Linux + Windows | Linux + Windows (部分) | Linux + Windows | Linux + Windows | **全功能原生支持 Win + Linux** |
| **SteamCMD 支持** | 深度集成 (GUI配置) | 脚本化任务调用 | 声明式模板 (支持DepotDL)| 模块化插件 | XML 游戏模板 | **SteamCMD + DepotDL 双引擎** |
| **控制台终端方案**| Win32 SendKeys (脆弱) | tmux (Linux) / winsw (Win) | PTY + WebSocket 流 | Web PTY 终端 | 轮询/日志捕获 | **ConPTY (Win) + PTY (Linux) + xterm.js** |
| **用户与权限管理**| 无 (单机个人用) | RBAC 细粒度权限 | OAuth2 Scopes 权限 | 细粒度 ACL | 多租户/分销商体系 | **RBAC 租户与协管员隔离体系** |
| **商业计费与续费**| 无 | 无 (需第三方插件) | 无 | 支持 WHMCS 模块 | 原生深度支持 WHMCS | **内建原生计费状态机 + 外部 Webhook**|
| **资源限制配额** | 手动设置 CPU 亲和度 | 系统级机制依赖 | Docker 隔离依赖 | Docker / 进程限额 | Job Objects / cgroups | **Windows Job Objects + Linux cgroups**|

---

## 3. 系统总体架构设计

### 3.1 核心设计原则：单机先驱部署，主控微内核解耦

> **🏛️ 软件架构师评述**：
> "本项目采用 **'单机一体化二进制交付，逻辑分层微内核严格解耦'** 的架构策略：在逻辑上，代码严格区分为 **Master（调度控制中心）** 与 **Daemon（节点执行代理）**；在当前单机控制目标下，Master 与 Daemon 运行在同一个进程空间中，用户只需要执行一条启动命令即可运行。未来跨机器扩容时，只需将 Daemon 编译为独立二进制放上远端服务器，秒级蜕变为分布式架构。"

```
+-----------------------------------------------------------------------------------+
|                           单服务器部署模式 (当前目标：All-In-One)                     |
|                                                                                   |
|   +-----------------------+                    +------------------------------+   |
|   |   Web 前端 (Vue 3)     |                    |   外部支付网关 / 运营管理    |   |
|   +-----------+-----------+                    +--------------+---------------+   |
|               | (HTTP REST / WebSocket)                       | (Webhook 回调)    |
|               ▼                                               ▼                   |
|   +---------------------------------------------------------------------------+   |
|   |                        Master 控制面内核 (Go Master Core)                  |   |
|   |  - 用户认证 & RBAC 权限系统               - 商业套餐 & 订阅计费引擎        |   |
|   |  - 游戏声明式模板库 (Steam AppID/Config)   - 订单流水 & 续费生命周期状态机  |   |
|   |  - 节点拓扑管理 (当前指向 Localhost)       - 端口资源分配池 (Port Pool)     |   |
|   +-------------------------------------+-------------------------------------+   |
|                                         |                                         |
|                 +-----------------------+-----------------------+                 |
|                 | (In-Process Direct Bus 或 Loopback gRPC 通信)  |                 |
|                 ▼                                               ▼                 |
|   +---------------------------------------------------------------------------+   |
|   |                        Daemon 节点执行引擎 (Local Daemon)                  |   |
|   |  - SteamCMD / DepotDownloader 运行时下载与验证器                          |   |
|   |  - 进程监管执行器 (Process Supervisor: 启动/停止/自动拉起/优雅关机)        |   |
|   |  - 跨平台伪终端引擎 (Windows ConPTY / Linux POSIX PTY)                    |   |
|   |  - 资源配额沙箱 (Windows Job Objects / Linux cgroups v2)                   |   |
|   |  - 游戏状态监测探针 (A2S UDP Poller / RCON Client)                         |   |
|   |  - 内置文件引擎 (Web File Browser / 嵌入式 SFTP Server)                   |   |
|   +-------------------------------------+-------------------------------------+   |
|                                         |                                         |
|                  +----------------------+----------------------+                  |
|                  | (OS Native Process Spawning)                |                  |
|                  ▼                                             ▼                  |
|        +--------------------+                       +--------------------+        |
|        | 游戏实例 #1 (Palworld) |                      | 游戏实例 #2 (CS2)    |        |
|        +--------------------+                       +--------------------+        |
+-----------------------------------------------------------------------------------+

    ======================== 未来平滑演进形态 ========================

+--------------------------+                  +--------------------------+
|  Master 控制服务器 (云端)  | <== mTLS gRPC ==> |   Remote Daemon 节点 #2  |
|  - 统一结算与用户控制中心 |                  |   (Windows Server 实体机)|
+--------------------------+                  +--------------------------+
             ▲
             ║ mTLS gRPC
             ▼
+--------------------------+
|   Remote Daemon 节点 #1  |
|   (Linux 高防物理机)     |
+--------------------------+
```

---

### 3.2 总体架构拓扑与数据流图

系统的整体数据流转遵循 **"配置声明化、控制异步化、监控事件化"**：
1. **用户操作流**：用户在 Vue 3 前端发起安装/启停操作 -> Master 鉴权并校验租户配额/续费状态 -> 生成调度指令 -> 推送给 Daemon 任务队列。
2. **终端流 (Terminal Stream)**：前端 xterm.js 建立 WebSocket 连接 -> Master 校验 Token -> 穿透直连 Daemon 伪终端（ConPTY/PTY）-> 双向流式传输字符与 ANSI 颜色代码。
3. **状态探针流 (Monitoring Stream)**：Daemon 后台协程周期性（5秒）向本地实例发送 A2S UDP 探针获取在线玩家，并采集 OS 性能指标 -> 汇聚至本地环形缓冲区，向 Master 广播心跳包。

---

### 3.3 技术栈选型与理由矩阵

| 技术层 | 推荐选型 | 选型理由与架构优势 |
| :--- | :--- | :--- |
| **后端语言** | **Go (Golang 1.22+)** | 1. 跨平台静态编译为单个自包含二进制，**无需在宿主机安装 Python/Node/.NET 运行库**。<br>2. 原生协程模型轻松支撑上千个高并发 WebSocket 与长连接。<br>3. 原生支持 Windows Win32 API 绑定 (ConPTY, Job Objects) 与 Linux syscall (PTY, cgroups)。 |
| **Web 框架** | **Gin / Chi** | 轻量无冗余，路由性能极致，中间件机制成熟，与 WebSocket 及 gRPC 无缝融合。 |
| **前端技术** | **Vue 3 + Vite + TailwindCSS + Pinia** | 现代化 SPA，响应式状态管理，TailwindCSS 快速适配暗色系（Dark Mode）专业游戏控制面风格，开箱即用。 |
| **Web 终端** | **xterm.js + xterm-addon-fit** | 工业级前端虚拟终端，完整支持 ANSI 颜色代码、光标控制、按键拦截、UTF-8 字符集渲染。 |
| **数据库** | **SQLite (WAL 模式) + GORM**<br>*(支持无缝切换 MySQL/PostgreSQL)* | 1. 单机部署模式下，SQLite 零配置、零外部端口暴露，自包含在单个 `.db` 文件中。<br>2. 开启 WAL (Write-Ahead Logging) 模式后，读写并发性能足以支撑千级并发。<br>3. 借助 GORM 抽象，未来升级分布式多机部署时，仅修改配置文件即可无缝连接 MySQL / Postgres。 |
| **通信传输** | **RESTful (业务) + WebSocket (终端/遥测) + gRPC (节点互联)** | 标准化协议，兼容所有反向代理（Nginx, Caddy, Cloudflare），安全性与抓包调试体验极佳。 |

---

### 3.4 系统各功能子系统极度解耦架构 (Hexagonal & Event-Driven)

> **🏛️ 软件架构专家核心把关**：
> "在构建同时承载‘Web 业务’与‘底层 OS 进程调度’的复杂系统时，最致命的架构反模式是**模块间网状交叉调用（Spaghetti Coupling）**：例如计费模块直接调用进程管理器的关机函数，或者文件管理器直接操作游戏服务器实例数据库。
> 为此，系统确立严格的**模块解耦三原则**：
> 1. **单向无环依赖 (DAG)**：上层依赖下层抽象，业务模块严禁循环导入；
> 2. **事件总线解耦 (Event-Driven Pub/Sub)**：跨领域业务流转禁止硬编码直接调用，一律通过事件发布与订阅异步触发；
> 3. **端口与适配器驱动隔离 (Ports & Adapters)**：所有与外部系统（操作系统底层、第三方支付、外部通知、存储驱动、节点通信）交互的逻辑全部收敛为抽象接口（Interface），实现可插拔替换。"

```
               +--------------------------------------------------------+
               |                  Interfaces 入口层                     |
               |   [HTTP REST Handlers]  [WebSocket]  [gRPC Service]    |
               +---------------------------+----------------------------+
                                           │ 调用 (Inbound Ports)
                                           ▼
               +--------------------------------------------------------+
               |                 Application 应用服务层                 |
               |   - InstanceAppService       - BillingAppService       |
               |   - ModAppService            - TemplateAppService      |
               +---------------------------+----------------------------+
                                           │ 编排 / 聚合
                                           ▼
               +--------------------------------------------------------+
               |                   Domain 纯核心领域层                  |
               |   - Instance / Package / Order / Template 实体与业务规则  |
               |   - 【全局领域事件总线 (EventBus)】                       |
               +---------------------------+----------------------------+
                                           │ 依赖反转 (Outbound Ports)
                                           ▼
               +--------------------------------------------------------+
               |                Infrastructure 底层实现层                |
               |   [GORM / SQLite / MySQL 仓储实现]                      |
               |   [ProcessManager 驱动: WinJobObject / LinuxCgroups]   |
               |   [NodeDriver 驱动: InProcessDriver / RemoteGrpcDriver] |
               |   [PaymentProvider 驱动: Stripe / Wechat / Alipay]      |
               |   [StorageEnforcer 驱动: ProjectQuota / VHDX]           |
               |   [NotificationSender 驱动: Discord / WeCom / Email]    |
               +--------------------------------------------------------+
```

#### 3.4.1 跨领域业务事件总线 (EventBus Pub/Sub) 范式

系统内建高性能内存事件总线（单机运行时为 Go 原生通道广播，分布式集群时可透明切换为 NATS / Redis Pub-Sub），所有跨子系统的联动完全基于松散事件响应：

| 领域事件 (Domain Event) | 发布者模块 | 独立订阅者与动作响应 (完全解耦，互不导入) |
| :--- | :--- | :--- |
| `EventSubscriptionSuspended`<br>`{InstanceID, Reason}` | **Billing 计费引擎**<br>(宽限期欠费) | 1. **Instance 运维模块**：收到事件，执行安全停机并置灰启动按钮。<br>2. **Notification 告警模块**：收到事件，自动向服主推送“欠费停机”通知。<br>3. **Audit 审计模块**：记录计费状态机跃迁流水日志。<br>*(注：Billing 模块完全不需要知道 Instance 模块的存在！)* |
| `EventSubscriptionRenewed`<br>`{InstanceID, NewExpireAt}` | **Billing 计费引擎**<br>(支付网关回调成功) | 1. **Instance 运维模块**：解锁实例启动限制，状态恢复 ACTIVE。<br>2. **Notification 模块**：推送“续费成功”确认卡片。 |
| `EventInstanceCrashed`<br>`{InstanceID, ExitCode, Time}` | **Process 监管驱动**<br>(底层检测到非正常退出) | 1. **Watchdog 看门狗模块**：计算 5 分钟内崩溃频次，决定是否熔断自启。<br>2. **WebSocket 广播模块**：实时向下游客户端推送前端终端警报。<br>3. **Notification 模块**：向服主绑定的 Discord/微信群推送崩溃日志前 10 行。 |
| `EventHardOverQuota`<br>`{InstanceID, UsedBytes, Limit}`| **Storage 配额驱动**<br>(磁盘占用达到 100%) | 1. **Instance 模块**：拦截开机操作，若运行中进入 5 分钟优雅关机倒计时。<br>2. **Mod 模块**：禁用新增下载任务。<br>3. **Web 端**：弹出全局“红色容量硬超限”警报。 |

#### 3.4.2 核心可插拔抽象驱动接口体系 (Interface Isolation)

系统将所有环境差异和外部供应商全部抽象为微内核驱动，业务代码仅面向接口编程：

1. **节点执行驱动接口 (`NodeDriver`)**：
   - 当前单机阶段实现：`InProcessNodeDriver`（主控直接在当前进程调用 Daemon 协程，零网络开销，单文件运行）；
   - 未来集群阶段实现：`RemoteGrpcNodeDriver`（自动打包 gRPC + mTLS 远端调用，业务层代码零修改）；
2. **底层进程监督驱动接口 (`ProcessManager`)**：
   - Windows 环境实现：`WinConptyJobManager`（基于 ConPTY 虚拟终端 + Win32 Job Objects 内存/CPU 配额）；
   - Linux 环境实现：`LinuxPtyCgroupManager`（基于 POSIX openpty + cgroups v2 配额）；
   - 测试环境实现：`MockProcessManager`（纯内存模拟，单元测试无需真实开服）；
3. **第三方支付网关接口 (`PaymentGateway`)**：
   - 统一接口：`CreateCheckoutSession(order) (PayURL, QR, error)`、`VerifyWebhook(req) (*PaymentReceipt, error)`；
   - 适配器：`WeChatPayAdapter`、`AlipayAdapter`、`StripeAdapter`、`BalanceWalletAdapter`；
4. **外部告警推送接口 (`NotificationSender`)**：
   - 统一接口：`SendAlert(ctx, event, payload) error`；
   - 适配器：`DiscordWebhookSender`、`WeComSender`、`FeishuSender`、`DingTalkSender`、`EmailSMTPSender`。

#### 3.4.3 清晰严格的代码工程目录分层规范 (Go Clean Architecture)

```
gameserver/
├── cmd/
│   ├── gameserver/               # 统一主入口 (单机 All-In-One 编译目标)
│   └── gameserver-daemon/        # 独立节点守护进程 (未来分布式集群编译目标)
├── templates/                    # 管理员官方游戏模板目录 (YAML)
├── internal/
│   ├── domain/                   # 纯核心领域模型 (零框架依赖)
│   │   ├── instance/             # 游戏实例聚合根、状态机、实体
│   │   ├── billing/              # 订单、套餐、优惠券、分销
│   │   ├── template/             # UGTS 模板规范结构体
│   │   ├── user/                 # 用户、角色、RBAC
│   │   └── events/               # 全局领域事件定义
│   ├── application/              # 应用用例编排层 (Use Cases)
│   │   ├── instance_service.go
│   │   ├── billing_service.go
│   │   └── template_service.go
│   ├── infrastructure/           # 外部适配器与基础设施实现
│   │   ├── persistence/          # GORM 数据库实体与仓储实现
│   │   ├── process/              # 跨平台进程管理 (Win ConPTY / Linux PTY)
│   │   ├── sandbox/              # 资源配额驱动 (JobObjects / cgroups)
│   │   ├── steamcmd/             # SteamCMD / DepotDownloader 驱动
│   │   ├── payments/             # 微信/支付宝/Stripe 支付适配器
│   │   ├── notifications/        # Discord / 飞书 / 邮件推送适配器
│   │   └── eventbus/             # 内存事件总线实现
│   └── interfaces/               # 协议入口交互层
│       ├── http/                 # REST API 控制器 (Gin/Chi)
│       ├── websocket/            # 终端流、状态流、遥测复用通道
│       └── sftp/                 # 嵌入式 SFTP 服务挂载
└── web/                          # 前端 Vue 3 + Vite 单页应用源码
```

#### 3.4.4 前端 (Vue 3) 领域状态与组件解耦设计

前端同样杜绝“巨石组件”与“巨石状态”，按业务领域严格划分 Pinia Store 与功能卡片组件：
- **领域独立 Store**：
  - `useAuthStore`：仅负责用户凭证、Token 刷新与个人 Profile；
  - `useServerStore`：仅管理实例列表、启停状态、当前选定实例元数据；
  - `useTerminalStore`：专门管理 xterm.js 实例、WebSocket 终端连接与滚动锁；
  - `useBillingStore`：管理套餐列表、收银台弹窗、续费长轮询与优惠券计算；
  - `useModStore`：管理创意工坊检索、已安装 Mod 清单与依赖关系。
- **页面视图高度组件化**：
  - 控制台页面 (`/server/:id`) 由独立可插拔卡片组成：`<TerminalPanel />`、`<QuickActionToolbar />`、`<MetricCharts />`、`<LivePlayerList />`。
  - 每个组件通过 Props 与 Event 通信，任意组件报错绝不引发整页白屏（Error Boundary 兜底隔离）。

---

## 4. Steam 游戏运维引擎与 Mod 获取自动化体系

### 4.1 SteamCMD 全自动管理流水线

系统将 SteamCMD 封装为非阻塞的异步任务引擎，整体流水线设计如下：

```
[任务触发: 安装 / 更新 / 验证]
        │
        ▼
[环境自检阶段]
  ├── 检测宿主机是否存在 SteamCMD 二进制
  │     ├── Linux: 检查 ~/.local/share/Steam/steamcmd.sh (缺失则自动拉取官方 tar.gz 并注入 32位动态链接库依赖)
  │     └── Windows: 检查 bin/steamcmd/steamcmd.exe (缺失则自动下载官方 zip 并静默解压)
  └── 检测系统环境依赖:
        ├── Linux: 校验 glibc.i686, libstdc++.i686, lib32z1
        └── Windows: 校验 Visual C++ 2015-2022 Redistributable
        │
        ▼
[参数构建阶段]
  ├── 动态生成专属安装目录: --dir <instance_working_dir>
  ├── 注入登录鉴权凭证:
  │     ├── 匿名游戏 (大部分): +login anonymous
  │     └── 需要商业授权的游戏: +login <steam_account> <steam_password> (支持主控全局配置池)
  ├── 注入目标游戏参数: +app_update <AppID> [-beta <Branch>] [-betapassword <Pass>] [validate]
  └── 终结符: +quit
        │
        ▼
[执行与日志解析阶段]
  ├── 启动非阻塞子进程，标准输出与标准错误合并
  ├── 正则表达式引擎实时捕获下载进度:
  │     匹配: `Update state \(0x(?P<state>[0-9a-fA-F]+)\) downloading, progress: (?P<percent>\d+\.\d+) \((?P<current>\d+) / (?P<total>\d+)\)`
  │     -> 换算为百分比与瞬时速度，通过 WebSocket 向前端发送平滑进度条
  └── 异常模式识别:
        ├── `0x202 / Disk full`: 磁盘空间不足，直接终止并告警
        ├── `0x402 / Invalid password`: Steam 凭据错误
        └── `0x602 / Connection lost`: 网络断连，触发自动退避重试 (最多 3 次)
```

---

### 4.2 平台预置确定性游戏与通用模板引擎规范 (Universal Game Template Specification - UGTS)

> **🎯 产品与系统架构专家核心把关**：
> "在商业化游戏服务器控制台中，**绝不能让普通小白租户自行去网上查 Steam AppID、摸索复杂的启动命令行或手动改写系统服务**。
> 系统的核心设计模式是：**平台管理员负责准备并维护经过严格测试、调优好的标准游戏通用配置文件（Universal Template），用户可以部署的游戏范围是完全确定的（Curated Catalog）。** 用户只需要在前端如同逛‘应用市场’一样选择想开的游戏，填入几个最基本的个性化变量（如服务器名、密码、最大人数），底层即可全自动完成端口绑定、SteamCMD 下载、配置文件自动修改与沙箱拉起！"

```
+----------------------------------------------------------------------------------------------------+
|  【平台管理员 / 运维团队视角】                                                                      |
|  - 编写并维护通用的 YAML 配置文件: ./templates/*.yaml                                              |
|  - 声明游戏 Steam AppID、启动参数、端口规则、配置插值映射、生命周期与 Mod 规范                     |
|  - 随时将新游戏 YAML 丢入 ./templates/ 目录，系统后台自动热重载 (Hot-Reload)，无需改代码或重启服务   |
+--------------------------------------------------+-------------------------------------------------+
                                                   | 自动化编译解析与验证
                                                   ▼
+----------------------------------------------------------------------------------------------------+
|  【租户 / 玩家服主视角】                                                                            |
|  - 打开面板进入“可开通游戏列表”，呈现管理员预置好的游戏库 (如 幻兽帕鲁, CS2, DayZ, Rust, 方舟)       |
|  - 点击某游戏，弹出基于该模板动态生成的极简向导表单 (仅暴露：服务器名称、进服密码、PvP开关等安全项)  |
|  - 点击“确认开通”：系统自动从端口池扣端口、注入启动参数、自动改写游戏配置文件、秒级完成部署！       |
+----------------------------------------------------------------------------------------------------+
```

#### 4.2.1 通用游戏模板规范核心 Schema (UGTS v1.0 标准)

管理员只需遵循以下结构编写单份 YAML 文件，即可完美适配任何一款 Steam 游戏：

```yaml
schema_version: "1.0"

# 1. 基础元数据
metadata:
  id: "palworld"                          # 游戏唯一标识符
  name: "Palworld (幻兽帕鲁)"             # 前台展示名称
  category: "Survival"                   # 游戏分类 (Survival, FPS, RPG, Sandbox)
  icon: "https://cdn.example.com/pal.png"# 封面大图
  author: "Official Team"                 # 维护者
  version: "1.0.0"                        # 模板版本
  description: "缝合怪开放世界生存制作游戏，支持多人联机与帕鲁捕捉。"
  supported_os: ["windows", "linux"]      # 严格限制运行系统 (如声明 ["windows"] 则 Linux 节点严禁部署)

# 2. Steam 专服下载与更新策略
steam:
  app_id: "2394010"                       # 目标专用服务端 AppID
  anonymous_login: true                  # 是否支持匿名下载 (若 false，系统自动进入 2FA 互动流)
  require_auth_account: false             # 是否需要商业购买账号
  validate_on_install: true               # 首次安装是否校验完整性
  default_branch: "public"                # 默认分支 (支持切换 experimental / previousversion)

# 3. 运行环境与启动参数模板 (支持 Go 模板语法动态插值)
environments:
  windows:
    executable: "PalServer.exe"
    working_directory: ""
    start_arguments: "-port={{ .Ports.SERVER_PORT }} -players={{ .Variables.MAX_PLAYERS }} -useperfthreads -NoAsyncLoadingThread -UseMultithreadForDS"
  linux:
    executable: "PalServer.sh"
    working_directory: ""
    start_arguments: "-port={{ .Ports.SERVER_PORT }} -players={{ .Variables.MAX_PLAYERS }} -useperfthreads -NoAsyncLoadingThread -UseMultithreadForDS"

# 4. 端口声明清单 (与全局端口池自动挂钩)
ports:
  - key: "SERVER_PORT"
    label: "游戏主通信端口 (UDP)"
    type: "UDP"
    default: 8211
    is_primary: true                      # 决定对外展示给玩家连接的主端口
  - key: "RCON_PORT"
    label: "RCON 管理与监控端口 (TCP)"
    type: "TCP"
    default: 25575
    is_primary: false

# 5. 用户端暴露变量 (系统根据此处自动渲染前台设置表单)
variables:
  - key: "SERVER_NAME"
    label: "服务器房间名称"
    type: "string"
    default: "My Palworld Dedicated Server"
    required: true
    user_editable: true                   # 是否允许租户自行修改
    description: "将在游戏公网大厅中搜索到的房间标题"

  - key: "SERVER_PASSWORD"
    label: "入服密码"
    type: "password"
    default: ""
    required: false
    user_editable: true
    description: "为空则表示公开无密码开放进入"

  - key: "ADMIN_PASSWORD"
    label: "管理员超级特权密码"
    type: "password"
    default: ""
    required: true
    user_editable: true
    description: "在游戏内通过 /AdminPassword 认证管理员使用的密码"

  - key: "MAX_PLAYERS"
    label: "最大玩家容纳人数"
    type: "number"
    default: 32
    validation:
      min: 1
      max: 64
    user_editable: true

  - key: "PVP_ENABLED"
    label: "启用公会与玩家 PvP 伤害"
    type: "boolean"
    default: false
    user_editable: true

  - key: "EXP_RATE"
    label: "玩家与帕鲁经验倍率"
    type: "number"
    default: 1.0
    validation:
      min: 0.1
      max: 10.0
    user_editable: true

# 6. 配置文件自动双向映射与插值引擎 (Config Binding)
config_bindings:
  - file_path: "Pal/Saved/Config/WindowsServer/PalWorldSettings.ini"
    platform: "windows"
    format: "ini"                         # 支持 ini, cfg, json, yaml, properties
    section: "/Script/Pal.PalGameWorldSettings"
    mappings:
      ServerName: "{{ .Variables.SERVER_NAME }}"
      ServerPassword: "{{ .Variables.SERVER_PASSWORD }}"
      AdminPassword: "{{ .Variables.ADMIN_PASSWORD }}"
      ServerPlayerMaxNum: "{{ .Variables.MAX_PLAYERS }}"
      bIsPvP: "{{ .Variables.PVP_ENABLED }}"
      ExpRate: "{{ .Variables.EXP_RATE }}"
      RCONEnabled: "True"
      RCONPort: "{{ .Ports.RCON_PORT }}"

  - file_path: "Pal/Saved/Config/LinuxServer/PalWorldSettings.ini"
    platform: "linux"
    format: "ini"
    section: "/Script/Pal.PalGameWorldSettings"
    mappings:
      ServerName: "{{ .Variables.SERVER_NAME }}"
      ServerPassword: "{{ .Variables.SERVER_PASSWORD }}"
      AdminPassword: "{{ .Variables.ADMIN_PASSWORD }}"
      ServerPlayerMaxNum: "{{ .Variables.MAX_PLAYERS }}"
      bIsPvP: "{{ .Variables.PVP_ENABLED }}"
      ExpRate: "{{ .Variables.EXP_RATE }}"
      RCONEnabled: "True"
      RCONPort: "{{ .Ports.RCON_PORT }}"

# 7. 探针与实时玩家监控配置
query:
  type: "a2s"                             # 采用 Valve A2S UDP 探针
  port_key: "SERVER_PORT"                 # 绑定的查询端口

# 8. 进程生命周期与优雅退出策略
lifecycle:
  stop_method: "rcon"                     # rcon / stdin / signal
  stop_command: "Shutdown 15 Server_Restarting..."
  stop_timeout_seconds: 30                # 超时 30s 自动升级为强制 Kill 保护

# 9. Mod 生态与装配规范
mods:
  engine_type: "unreal_pak"               # 支持: unreal_pak, sourcemod, bepinex, umod, dayz_pbo
  workshop_support: false                 # 是否支持 Steam 创意工坊
  install_directory: "Pal/Content/Paks/~mods"
```

---

#### 4.2.2 动态变量与配置文件双向插值引擎 (File Interpolation & Config Sync)

游戏配置文件的手动修改是服主最大的操作门槛。UGTS 引擎实现了**“一次填写，开机前自动打补丁”**机制：
1. **开机前拦截钩子 (Pre-start Config Hook)**：
   - 每次服主点击启动服务器时，Daemon 会在拉起可执行文件前的毫秒级时间内，执行 `ApplyConfigBindings()`。
2. **基于 AST 语法的保护性修改**：
   - 系统绝非简单粗暴地用文本覆写整个配置文件（避免冲掉服主在高级文件中手工微调的其他小参数）；
   - 系统使用对应格式的解析器（如 INI/CFG 语法树），精准读取现存文件，只定向更新 `config_bindings.mappings` 里声明的键值，并完好保留文件中的注释与未声明字段。
3. **动态模板注入**：
   - 支持动态取值：如 `{{ .Ports.RCON_PORT }}` 会被自动替换为系统分配给该服务器的实际网络端口；
   - 彻底免去服主自己配置端口映射的痛苦。

---

#### 4.2.3 零依赖内置模板 (embed.FS) 与外部模板动态热重载 (Hot-Reload)

为了兼顾“开箱即用”与“主机商自定义扩展”，模板系统采用双层加载器：

```
                              [Master 模板引擎管理器]
                                         │
                   ┌─────────────────────┴─────────────────────┐
                   ▼                                           ▼
      【内置官方认证模板 (Embed FS)】             【外部自定义模板目录 (External)】
      - 编译时通过 //go:embed 打包入二进制        - 位于本地目录: ./templates/*.yaml
      - 包含预置主流游戏:                        - 管理员可随时放入全新游戏 YAML
        (Palworld, CS2, DayZ, Rust, Valheim)    - 支持覆盖同名内置模板
                   │                                           │
                   └─────────────────────┬─────────────────────┘
                                         │
                                         ▼
                        【fsnotify 文件事件监听器 (Hot-Reload)】
                        - 毫秒级感知 ./templates/ 下的增删改事件
                        - 自动重载至内存结构，校验 YAML 语法
                        - 通过 WebSocket 向管理员后台广播更新事件
                        - 【无需重启服务，无需重新编译二进制，零停机生效！】
```

---

#### 4.2.4 典型主流游戏官方标准模板矩阵速览

为了让系统开箱即用，内置模板库全面覆盖主流 Steam 联机专服：

| 游戏名称 | 游戏 AppID | 运行平台声明 | 默认端口集 | 关机方式 | Mod 生态适配器 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Palworld (幻兽帕鲁)** | `2394010` | Windows, Linux | UDP 8211 (Game), TCP 25575 (RCON) | RCON | `unreal_pak` (`~mods`) |
| **CS2 (反恐精英 2)** | `730` | Windows, Linux | UDP 27015 (Game/Query), TCP 27015 (RCON) | STDIN (`quit`) | `sourcemod` (`+host_workshop`) |
| **Rust (腐蚀)** | `258550` | Windows, Linux | UDP 28015 (Game), UDP 28016 (Query), TCP 28016 (RCON) | STDIN (`quit`) | `umod` (`oxide/plugins/*.cs`) |
| **DayZ Standalone** | `223350` | Windows, Linux | UDP 2302 (Game), UDP 27016 (Query), TCP 2304 (RCON) | STDIN (`#shutdown`) | `dayz_pbo` (`@ModDir` + `keys/*.bikey`) |
| **Valheim (英灵神殿)** | `896660` | Windows, Linux | UDP 2456-2458 (Game/Query) | Signal | `bepinex` (Thunderstore API) |
| **Space Engineers (太空工程师)** | `298740` | **仅限 Windows** (`["windows"]`) | UDP 27016 (Game) | STDIN | 游戏内置工坊 |

> **注**：如上表所示，**Space Engineers** 等官方不提供 Linux 服务端二进制的游戏，模板严格硬性标记 `supported_os: ["windows"]`。在 Linux 节点上部署时，系统直接给出明晰友好的阻断提示，绝不给服主造成运行时困扰。

---

### 4.3 跨平台终端控制台与日志双向流 (ConPTY / PTY)

1. **Windows 平台：ConPTY (Windows Pseudo Console)**
   - 调用 Win32 原生 API `CreatePseudoConsole()` 创建成对的双向虚拟终端设备。
   - 通过 `InitializeProcThreadAttributeList` 将 `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` 属性附加至 `CreateProcessW`。
   - 彻底解决 WindowsGSM 依赖 `SendKeys` 导致在 Windows 服务中失效以及 Source 引擎 `!GetNumberOfConsoleInputEvents` 崩溃的问题。
2. **Linux 平台：POSIX PTY (`openpty` / `forkpty`)**
   - 使用 Go 的 `creack/pty` 库为游戏进程分配 master/slave PTY 设备，原生支持 ANSI 控制字符。
3. **前端渲染与交互 (xterm.js 流水线)**：
   - Daemon 读取 PTY 输出，通过 WebSocket 以二进制格式推送到客户端，前端 `xterm.js` 零延迟渲染。

---

### 4.4 非侵入式游戏状态探测 (Valve A2S UDP 协议)

系统内置 Valve A2S 探针采集器，每隔 5 秒向服务器的 Query 端口发送查询：
- **A2S_INFO (`0x54`)**：获取服务器名称、当前地图、在线人数、最大人数、Ping。
- **A2S_PLAYER (`0x55`)**：获取在线玩家昵称、击杀得分、连入时长（秒）。
- **A2S_RULES (`0x56`)**：获取服务器当前的完整 cvars 规则参数表。

---

### 4.5 游戏 Mod 自动化获取与生态适配矩阵

不同游戏引擎与生态的 Mod 管理逻辑存在巨大差异，系统提供插件化的 Mod 适配引擎：

```
                                  [用户在 Web 前端添加 Mod]
                                              │
                      ┌───────────────────────┴───────────────────────┐
                      ▼                                               ▼
         【Steam 创意工坊模式】                            【非工坊 / 第三方生态模式】
          (输入 Workshop ID / 链接)                       (CurseForge / Thunderstore / 直链)
                      │                                               │
                      ▼                                               ▼
         ISteamRemoteStorage API 预检                     三方 REST API 预检 (获取体积与依赖)
         (验证 AppID, 标题, 体积, 依赖)                               │
                      │                                               ▼
                      ▼                                    HTTP/HTTPS 分块下载校验
         SteamCMD / DepotDownloader 下载                              │
         (下载至共享缓存目录 content/<appid>/<modid>)                  │
                      │                                               │
                      └───────────────────────┬───────────────────────┘
                                              │
                                              ▼
                             【多引擎自动化装配与文件映射引擎】
         ┌───────────────────┬────────────────────┬───────────────────┬───────────────────┐
         ▼                   ▼                    ▼                   ▼                   ▼
    [Source 引擎]       [Unity 引擎]          [Unreal Engine]       [DayZ 引擎]        [通用插件]
  (CS2, GMod, L4D2)   (Rust, Valheim)      (Palworld, ARK:SA)   (Bohemia Enforce)  (Minecraft等)
         │                   │                    │                   │                   │
         ├─ VPK 挂载         ├─ BepInEx / uMod    ├─ ~mods/*.pak      ├─ @ModDir 软链接   └─ mods/*.jar
         ├─ addons 目录      ├─ 插件热重载        ├─ UE4SS Lua 注入   ├─ *.bikey 钥匙同步
         └─ +host_workshop   └─ Harmony 补丁      └─ 配置文件生成     └─ 启动参数追加 -mod
```

#### 4.5.1 多引擎 Mod 接入规范与技术适配矩阵

| 游戏与引擎类型 | 代表游戏 | Mod 来源与协议 | 部署路径与文件类型 | 核心自动化技术细节 |
| :--- | :--- | :--- | :--- | :--- |
| **Valve Source / Source 2** | CS2, GMod, L4D2, TF2 | Steam Workshop / AlliedMods | `addons/sourcemod/plugins/` (`.smx`)、`cfg/sourcemod/` | 1. 启动参数动态拼接 `+host_workshop_collection <ID>`。<br>2. 自动拉取 SourceMod / MetaMod 最新稳定包解压挂载。 |
| **Unity 引擎生态** | Rust | uMod / Oxide / Carbon | `oxide/plugins/*.cs` | 1. 自动编译 C# 脚本，无需重启游戏即可热加载（Hot-reload）。<br>2. 配置文件自动在 `oxide/config/*.json` 生成映射。 |
| **Unity + BepInEx 生态** | Valheim, V Rising, 雾锁王国 | Thunderstore / NexusMods | `BepInEx/plugins/` (`.dll`) | 1. 自动化安装 BepInEx 框架引导器（`winhttp.dll` 或 Linux `doorstop_config.ini`）。<br>2. 从 Thunderstore API 递归解析 `package.json` 中的前置依赖并自动下载。 |
| **Unreal Engine 4/5** | Palworld (幻兽帕鲁), ARK: Survival Evolved | Steam Workshop / Nexus / 外部 | `Pal/Content/Paks/~mods/*.pak`、`Pal/Binaries/Win64/Mods/` (UE4SS) | 1. 自动创建 `~mods` 特殊优先级目录，将 Pak 文件软链注入。<br>2. 集成 UE4SS 脚本系统，支持蓝图与 Lua 增强 Mod。 |
| **Bohemia Interactive** | DayZ Standalone | Steam Workshop 专有 | `@ModName/Addons/*.pbo`、`keys/*.bikey` | 1. **双向密钥认证机制**：自动遍历 Mod 目录下的 `keys/*.bikey` 并复制至服务端根目录 `keys/` 中。<br>2. 启动参数自动规范化拼接 `-mod=@mod1;@mod2;@mod3`。 |

---

### 4.6 Mod 依赖树解析与开机前增量热更新机制

1. **依赖树自动解析 (Dependency Resolver)**：
   - 当用户在前端点击安装某个高级 Mod（例如 Valheim 的某款大型装备扩展包）：
   - 系统调用 Thunderstore / Steam Web API，分析该 Mod 的 `dependencies` 字段（如 `["denikson-BepInExPack_Valheim-5.4.2202", "ValheimModding-Jotunn-2.18.0"]`）。
   - 系统自动在前台弹出前置依赖安装提示，并支持“一键全选级联安装”，杜绝因缺少基础库导致的服务器黑屏崩溃。
2. **开机前自动同步与增量检查 (Pre-flight Mod Sync)**：
   - 每次用户点击“启动服务器”时，Daemon 异步执行 `CheckModUpdates()`：
   - 提取实例当前已装的所有 Workshop PublishedFileId，批量调用 Steam API 获取其 `time_updated`。
   - 对比本地记录的上次同步时间戳；若检测到作者发布了新版本，自动调用 SteamCMD 增量拉取最新内容并刷新挂载，彻底解决游戏客户端与服务端因 Mod 版本差导致的拒连问题。

---

### 4.7 跨平台操作系统硬性兼容准则 (明确拒绝 Wine 陷阱，必须 Windows 绝不强行 Linux)

> **🏛️ 架构师与系统工程师核心决策**：
> "在业界实践中，部分面板试图在 Linux 上通过 Wine / Proton 来运行仅有 Windows 服务端二进制的游戏（如 Space Engineers、Medieval Dynasty、部分老旧生存游戏）。但这在合租宿主机上是巨大的灾难：Wine 带来不可预测的虚拟句柄泄漏、内存暴涨、DirectX/Audio 虚拟化死锁，以及游戏崩溃时无法捕获真实堆栈。
> **本项目确立最高原则：极简、务实、拒绝不可控黑盒！需要特定 Windows 环境的游戏直接明确标注必须 Windows，绝不在 Linux 上通过 Wine 强行转译！**"

1. **模板级硬性平台声明 (`supported_os`)**：
   - 游戏模板显式声明兼容的操作系统列表：
     - 双平台原生支持（如 Palworld, CS2, Rust）：`supported_os: ["windows", "linux"]`
     - 仅支持 Windows（如 Space Engineers）：`supported_os: ["windows"]`
     - 仅支持 Linux（部分专用分支）：`supported_os: ["linux"]`
2. **实例部署准入强校验 (Hard Compatibility Gate)**：
   - 用户在 Web 端开通实例或选择节点时，系统严格执行系统版本匹配：
   - 若用户试图在 Linux 节点上部署 `supported_os: ["windows"]` 的游戏，Master API 立即硬性拦截并报错：
     `"【平台不兼容】该游戏服务端官方仅提供 Windows 原生版本，当前节点为 Linux 系统，无法部署。请切换至带有 Windows 标记的服务器节点。"`
   - 从根本上消除跨平台虚拟化转译带来的无穷运维陷阱与客诉！

---

### 4.8 GSLT 登录令牌托管与自动化池化分配

针对 Valve 旗下游戏（CS2, TF2, L4D2）及部分要求官方认证的游戏（如 Rust）：
1. **GSLT 的必要性**：此类游戏服务端若未配置 `+sv_setsteamaccount <Token>`，会被 Valve 判定为“匿名未受信任”服务器，官方大厅拒绝列出，正版玩家无法直接搜到。
2. **双模式令牌生命周期管理**：
   - **用户私有令牌模式**：服主在控制台界面直接粘贴其个人 Steam 开发者账号生成的 32 位 GSLT 秘钥。
   - **主机商全自动令牌池 (GSLT Token Pool)**：超级管理员可在后台一次性录入一批通用 GSLT 令牌。
   - 实例创建时，系统自动从未使用的池中 Claim 一个有效 Token 注入启动参数；
   - 实例销毁或删除时，系统自动释放（Release）该 Token 回池，实现全自动零配置开服。

---

### 4.9 Steam Guard 2FA 交互式流式认证

针对不可匿名下载、必须登录特定 Steam 商业/私人账号的游戏（如 Arma 3、部分附带 DLC 专服）：
1. **异步交互通道**：
   - SteamCMD 在命令行要求双重认证时，输出固定特征字符：`Enter the current code from your Steam Guard Mobile Authenticator app:`。
2. **双向流式通信闭环**：
   - Daemon 的进程捕获器识别到该正则特征，立即暂停等待，并向 Master 发送 `EventSteam2FARequired{InstanceID, Account}`；
   - Master 通过 WebSocket 实时向正在观看安装界面的用户浏览器弹出模态对话框：*“请输入 Steam 手机令牌 5 位动态验证码 (倒计时 60s)”*；
   - 用户输入验证码后，前端通过 API 回写给 Daemon，Daemon 将字符写入进程 STDIN 并追加 `\n`，SteamCMD 顺利鉴权并继续下载安装，彻底告别后台无休止假死！

---

### 4.10 Steam Beta 分支与版本密码切换管理

1. **版本锁定与兼容性分支**：
   - 许多游戏在强制大版本更新时会导致第三方 Mod 体系全面报错崩溃。服主必须能够锁定旧版本（如 DayZ 实验分支、Palworld 旧版本）。
2. **模板与启动参数集成**：
   - 实例配置界面提供“测试分支 (Beta Branch)”输入项，支持配置分支名称（如 `experimental`, `public-beta`）及私有分支保护密码（`-betapassword <pwd>`）。
   - 用户点击“切换分支并更新”时，后台自动执行 `+app_update <AppID> -beta <branch> -betapassword <pwd> validate +quit`，实现版本自由升降级。

---

## 5. 磁盘容量配额与边界超限纵深防护体系

> **🛡️ 安全与平台架构师核心把关**：
> "在多租户/合租游戏云环境中，**磁盘打满（ENOSPC / Disk Full）是最高危的灾难性事件**。一旦物理磁盘被单个恶意或粗心的用户下满，宿主机的 SQLite/MySQL 数据库会立即死锁甚至文件损坏，其他正常缴费用户的游戏服务器会全部遭遇崩溃掉档。因此，磁盘容量界限绝不能只停留在前端口头提示，必须构建**四层纵深防御体系**！"

```
[用户触发 Mod 下载 / 文件上传 / 游戏更新]
                     │
                     ▼
       【第一层：事前准入预检 (Pre-flight Admission)】
       - 调用 Steam API 获取 Mod 真实解压体积: ModSize
       - 计算可用空间: FreeSpace = Quota - UsedSpace
       - 断言: FreeSpace >= ModSize * 1.25 (预留25%冗余)
       - 失败: 直接拦截，抛出 HTTP 400 友好错误，零磁盘 I/O 消耗！
                     │ 通过
                     ▼
       【第二层：事中流式熔断与自动垃圾回收 (In-flight Streaming)】
       - 采用 QuotaEnforcedWriter 实时字节计量监控
       - 触发限额立即发送 SIGKILL 终止下载进程
       - 自动触发 GC 清理 steamapps/downloading/* 临时残片
                     │ 未触发熔断
                     ▼
       【第三层：事后超限有限状态机 (Post-event FSM)】
       ┌──────────────────────┬──────────────────────┐
       ▼                      ▼                      ▼
  [正常状态 (<90%)]     [软上限警告 (90%-99%)]    [硬上限熔断 (>=100%)]
  - 允许所有操作         - 黄色警示横幅           - 红色紧急横幅
                         - 限制新下载/上传         - 锁定启动，开机拦截
                         - 允许游玩与清理         - 开放“一键安全清理”
                                                     (清日志/Dump/缓存)
                     │
                     ▼
       【第四层：操作系统底层硬隔离与沙箱 (OS-level Hard Quota)】
       - Linux: 目录级 Project Quota (ext4/xfs) 或 独立稀疏 Loopback 镜像
       - Windows: 动态可扩展 VHDX 虚拟硬盘目录挂载 或 NTFS 配额
       (内核级拦截，物理层面绝不可能溢出至宿主机系统盘！)
```

### 5.1 业务风险与爆炸半径 (Blast Radius) 分析

| 超限原因 | 产生场景 | 潜在危害与爆炸半径 |
| :--- | :--- | :--- |
| **巨型 Mod / 集合订阅** | 用户一次性订阅含有几十款大地图的 Workshop 合集（如 GMod/DayZ 50GB+） | 吞食母机全部空闲磁盘，导致同机其他用户无法写盘。 |
| **下载中断遗留垃圾** | 下载到 99% 时网络中断或用户强行取消 | SteamCMD `steamapps/downloading/` 目录堆积数十 GB 孤儿缓存。 |
| **游戏运行时日志暴走** | Mod 冲突引发每秒上万条死循环报错，`output_log.txt` 瞬间膨胀到几十 GB | 几个小时内耗尽磁盘空间，导致母机崩溃。 |
| **崩溃转储文件 (Crash Dump)** | 游戏反复崩溃，生成大量 `.dmp`、`core` 文件 | 静默消耗存储空间，服主无感知。 |

---

### 5.2 四层纵深防护架构体系

#### 第一层：事前准入预检 (Pre-flight Admission Control)
- 用户在 Web 界面点击“安装 Mod”时，Master 绝不直接调用下载命令，而是先执行探针查询：
  ```
  ModFileSize = SteamAPI.GetPublishedFileDetails(modId).file_size
  CurrentUsed = Daemon.GetInstanceDiskUsage(instanceId)
  DiskQuota   = Instance.Package.disk_limit_bytes

  RequiredSpace = ModFileSize * 1.25  // 增加 25% 解压与构建冗余
  if (CurrentUsed + RequiredSpace) > DiskQuota:
      return Error("磁盘配额不足: 当前可用空间为 %.2f GB，该模组及安装解压需要 %.2f GB。请先清理文件或升级套餐。", FreeGB, ReqGB)
  ```
- **架构收益**：在未产生任何网络流量与宿主机磁盘写入前，完成安全拦截。

#### 第二层：事中流式熔断与自动回滚垃圾回收 (In-flight Guard & Rollback GC)
- 针对非工坊直链下载或大小未知的动态压缩包，Daemon 采用封装的 `QuotaEnforcedReader / Writer` 管道：
  - 实时累计写入字节数。
  - 一旦写入累积值导致实例目录达到 $100\%$ 配额，后台立即杀死下载子进程（`SIGKILL`）。
  - **自动垃圾回收 (Auto-Rollback GC)**：系统立刻清空当前下载的未完成碎片文件、删除 `<server_dir>/steamapps/downloading/` 下的对应临时缓存，将磁盘空间瞬时恢复至下载前的健康状态，杜绝死锁。

#### 第三层：事后超限有限状态机 (Over-Quota State Machine)
系统在后台维持对实例目录的定期扫描（每 60 秒轮询或基于文件变动事件监听）：

```
+-----------------------------------------------------------------------------------+
| 正常状态 (< 90% 容量)                                                              |
| - 所有功能正常可用。                                                               |
+-----------------------------------------------------------------------------------+
                                         │ 达到 90%
                                         ▼
+-----------------------------------------------------------------------------------+
| 软上限预警 (SOFT_OVER_QUOTA: 90% - 99%)                                            |
| - Web 控制台顶部常驻黄色警告横幅：“服务器存储空间已达 92%，即将写满！”              |
| - 发送邮件 / Webhook 提醒服主。                                                    |
| - 【受限行为】：禁用新 Mod 下载、禁用 Web 文件上传、禁用创建新全量备份。            |
| - 【允许行为】：游戏允许正常开机运行、允许控制台操作、允许在文件管理器中删除文件。|
+-----------------------------------------------------------------------------------+
                                         │ 达到或超过 100%
                                         ▼
+-----------------------------------------------------------------------------------+
| 硬上限锁定 (HARD_OVER_QUOTA: >= 100%)                                              |
| - Web 控制台顶部常驻红色醒目危险横幅：“存储空间已超限 (100.8%)，保护性锁定中！”    |
| - 【强制保护】：游戏若正在运行，等待 5 分钟后优雅关机（避免强制损坏数据库存档）；   |
| - 【开机拦截】：禁止启动服务器，防止游戏因为无法写入日志而发生未知损坏；           |
| - 【专属通道】：开放“一键安全清障”按钮（系统自动扫描并安全删除 .log、.dmp、缓存）；|
| - 【升配快捷通道】：提供一键补差价升级磁盘配额选项，付款后秒级恢复 ACTIVE。         |
+-----------------------------------------------------------------------------------+
```

---

### 5.3 超容有限状态机代码级实现参考

```go
// DiskQuotaEnforcer 实现实例磁盘配额裁决
func (e *DiskQuotaEnforcer) EvaluateInstance(ctx context.Context, instance *Instance) QuotaStatus {
    usedBytes, _ := e.dirScanner.GetTotalBytes(instance.Path)
    quotaBytes := instance.Package.DiskLimitBytes

    usageRatio := float64(usedBytes) / float64(quotaBytes)

    if usageRatio >= 1.0 {
        // 硬超限: 触发写保护与关机防护
        instance.QuotaState = StateHardOverQuota
        e.masterBus.Publish(EventInstanceHardOverQuota{InstanceID: instance.ID})
        return QuotaStatusHardExceeded
    } else if usageRatio >= 0.90 {
        // 软超限: 警告并禁用上传下载
        instance.QuotaState = StateSoftOverQuota
        e.masterBus.Publish(EventInstanceSoftOverQuota{InstanceID: instance.ID})
        return QuotaStatusSoftExceeded
    }

    instance.QuotaState = StateNormal
    return QuotaStatusHealthy
}
```

---

### 5.4 操作系统底层硬隔离实现方案 (Linux vs Windows)

为了防止极端情况下应用层检查失效（如游戏自身疯狂刷出 100GB 日志），必须依赖操作系统内核层强制沙箱兜底：

#### Linux 平台：目录级项目配额 (Project Quotas)
- 基于 `ext4` 或 `XFS` 文件系统的 `prjquota` 特性：
- 为每个实例的数据目录分配全局唯一的 `Project ID`（例如 `ProjectID = 10000 + InstanceID`）。
- 执行配额设定：
  ```bash
  # 关联目录与项目 ID
  echo "10001:/srv/gameservers/instance_1001" >> /etc/projects
  echo "instance_1001:10001" >> /etc/projid
  # 设定 30GB 硬限制
  xfs_quota -x -c 'limit -p bhard=30g instance_1001' /srv/gameservers
  ```
- **效果**：一旦该目录文件总和达到 30GB，操作系统内核直接对任何写操作返回 `EDQUOT`（Disk quota exceeded），从物理层面上 100% 杜绝超标可能。

#### Windows 平台：动态 VHDX 虚拟硬盘目录挂载
- Windows 原生对文件夹配额的支持较弱（NTFS Quota 默认是针对用户 SID 而非文件夹）。
- **专业级解法**：为每个实例创建一个**动态可扩展 VHDX 虚拟磁盘文件**（例如 `D:\vhd_storage\instance_1001.vhdx`）：
  - 声明最大容量上限为 30GB（初始创建只占几兆字节）。
  - 通过 PowerShell / Win32 Virtual Disk API 将该 VHDX 挂载为一个 NTFS 虚拟卷，并以 **NTFS 文件夹装载点 (Folder Mount Point / Junction)** 的形式映射到 `C:\gameservers\instance_1001\`。
- **效果**：游戏进程在这个文件夹里写入数据，对它而言这就是一个物理上限刚好为 30GB 的独立磁盘驱动器。一旦写满，Windows 操作系统内核直接报错 `ERROR_DISK_FULL`，**绝对不可能溢出到母机的 C 盘或 D 盘**！

---

## 6. 前台 Web 控制面全功能模块与交互界面设计

> **🎯 产品与用户体验专家核心把关**：
> "很多传统游戏控制面板（如早期 TCAdmin 或 LinuxGSM）界面充斥着杂乱的代码和晦涩的术语，对普通服主和租户极不友好。前台控制面必须遵循**现代电竞暗色风格（Dark Gaming Aesthetic）**，打造直观、低心智负担、所见即所得的 Web 交互体验。"

```
+----------------------------------------------------------------------------------------------------+
|  [Logo] STEAM HOST PANEL  |  实例: [ Palworld #1001 (幻兽帕鲁) ▼ ]  |  ● 运行中 (24/32人)  |  到期: 18天  |  [用户头像] |
+----------------------------------------------------------------------------------------------------+
|  [侧边导航栏]   |                                                                                  |
|  - 概览看板     |   【实时监控指标】                                                                 |
|  - 终端控制台   |   CPU: [==== 28% ====]   RAM: [======== 8.2G / 16G ========]   Disk: [== 12G/30G ==] |
|  - 模组中心     |                                                                                  |
|  - 可视化配置   |   【快捷操作】                                                                     |
|  - 文件管理     |   [ 优雅重启 ]  [ 安全关机 ]  [ 强制终止 (Kill) ]  [ 检查游戏更新 ]  [ 一键备份 ]      |
|  - 在线玩家     |                                                                                  |
|  - 计划任务     |   【服务器连接信息】                                                               |
|  - 备份管理     |   公网连接地址: 124.221.88.99:8211  [复制]   [ steam://一键直连游戏客户端 ]        |
|  - 网络与端口   |   当前运行地图: World_Survival_01    游戏版本: v0.2.4.0 (最新)   Ping: 22ms         |
|  - 续费与套餐   |                                                                                  |
|  - 协管员管理   |   【最近在线玩家列表 (A2S实时)】                                                   |
|                 |   1. Player_Alex (连入 45m, 得分 1200)   [ 踢出 ] [ 封禁 ]                          |
|                 |   2. Master_Chief (连入 12m, 得分 340)   [ 踢出 ] [ 封禁 ]                          |
+----------------------------------------------------------------------------------------------------+
```

### 6.1 视觉风格与全局交互骨架 (Layout & Dark Gaming Aesthetic)

- **主色调与设计系统**：基于 TailwindCSS 构建。背景采用高级深灰黑（`bg-slate-950` / `bg-zinc-900`），主交互强调色采用电竞青绿（`text-emerald-400` / `bg-emerald-500`），危险操作采用警示赤红（`text-rose-400`）。
- **全局顶部状态条 (Global Header)**：
  - 实例切换器（支持多服玩家快速无刷新切换）。
  - 实时状态微指示灯（绿点：运行中；黄点：安装/更新中；红点：已关机；紫点：崩溃自检中）。
  - 到期倒计时徽章（低于 3 天自动变黄并闪烁，点击直达续费收银台）。
  - 快捷电源按钮组（开机、关机、重启）。

---

### 6.2 概览仪表盘 (Dashboard / Overview)

- **核心实时图表 (ECharts / Chart.js)**：
  - CPU 实时占用折线图（平滑展示最近 15 分钟波动）。
  - 内存实时水位计量表（直观标注：已用物理内存 / 套餐限定内存 / 宿主机整体内存）。
  - 存储空间容量条（分色指示：游戏基础文件、Mod 模组文件、备份文件、剩余可用）。
  - 网络流量速率仪表盘（当前接收/发送 KB/s 速率与累计消耗带宽）。
- **公网连接卡片**：
  - 显式展示玩家需要连接的公网 IP:Port。
  - 提供 `steam://connect/<ip>:<port>` 深度链接按钮：玩家点击后可直接唤醒本地 Steam 客户端并自动启动游戏加入服务器！

---

### 6.3 工业级 Web 终端控制台 (Terminal / xterm.js Console)

- **终端体验**：
  - 集成 `xterm.js` 与 `xterm-addon-fit`，支持自适应窗口缩放。
  - 原生渲染 ANSI 256 色代码与文本加粗下划线。
  - 提供“自动锁屏滚屏”开关（方便追查历史日志时不被刷屏打断）。
- **交互控制增强**：
  - 命令输入框支持 **Up/Down 方向键历史翻阅**（本地存储最近 50 条已发指令）。
  - 底部预设“常用快捷指令按钮群”：
    - `[ 保存世界 (Save) ]`、`[ 广播消息 (Broadcast) ]`、`[ 查看状态 (Status) ]`、`[ 清屏 (Clear) ]`。
  - 提供“下载当前完整日志 (Raw Text)”与“日志关键字实时搜索高亮”功能。

---

### 6.4 游戏 Mod 模组中心 (Mod Center & Workshop Manager)

- **工坊一键安装器**：
  - 用户只需在输入框粘贴 Steam 创意工坊 URL 或纯数字物品 ID，系统自动通过 Steam Web API 异步拉取模组封面大图、标题、作者、体积大小与更新日期。
  - 自动执行容量预检：容量充足显示绿勾“允许安装”，不足显示红字“超额拦截，差额 X GB”。
- **已安装模组卡片列表**：
  - 采用网格卡片布局展示每一个已装 Mod。
  - 每个卡片包含：缩略图、版本号、占用磁盘体积。
  - **一键启用/禁用 Switch 开关**：无需删除文件，切换开关即可在后台完成符号链接建立或解绑，重启游戏即生效。
  - **批量检查更新**：一键检测并更新所有过期的 Mod。

---

### 6.5 双模式可视化参数配置中心 (Visual Config: Form & Monaco)

针对配置文件的维护，提供两种心智模型的自由切换：
1. **向导表单模式 (Form Mode, 适合新手服主)**：
   - 系统根据游戏模板将复杂的 `.ini`、`.cfg` 转换为友好的 UI 控件：
     - 服务器名称、密码输入框（带显示/隐藏眼睛）。
     - PvP / PvE 模式单选卡片。
     - 掉落倍率、经验倍率、采集倍率滑动条（Slider，带安全步长）。
   - 用户修改表单后，系统自动反向编译并写入目标配置文件。
2. **专业源码模式 (Raw Editor Mode, 适合资深服主)**：
   - 嵌入微软官方 Monaco Editor（VS Code 核心）。
   - 具备语法高亮、括号匹配、搜索替换、代码折叠。
   - 提供快捷键 `Ctrl+S`（Mac `Cmd+S`）即时保存，且在切换页面前若有未保存改动弹出阻止确认框。

---

### 6.6 在线文件管理器与内置 SFTP (Web File Manager & SFTP)

- **文件浏览器**：
  - 树状面包屑导航、文件大小、最后修改时间、操作菜单。
  - 支持快捷多选、批量删除、文件重命名、权限修改。
  - **文件拖拽直传**：用户可将本地文件或文件夹直接拖入浏览器窗口，支持多线程分片上传与断点续传。
  - **在线压缩与解压**：支持一键将选中的文件打包为 `.zip` 下载，或在服务器端一键解压 `.zip`、`.tar.gz`（内建 Zip Slip 漏洞安全防御）。
- **SFTP 连接指引抽屉**：
  - 点击右上角“SFTP”，弹出抽屉面板，显示主机 IP、端口（默认 2022）、用户名、一键重置 SFTP 独立连接密码。
  - 提供 FileZilla / WinSCP 快捷连接配置下载。

---

### 6.7 在线玩家管理与实时踢封 (Player Management & Rules)

- **实时玩家动态看板**：
  - 通过 A2S 探针实时拉取在线玩家清单。
  - 展示玩家昵称、在线时长（格式化为时分秒）、击杀得分、网络延迟。
- **管理治理动作**：
  - **踢出 (Kick)**：输入原因，通过 RCON / 控制台执行踢人命令。
  - **封禁 (Ban)**：封禁玩家 SteamID / IP，并自动同步写入黑名单配置文件。
  - **白名单管理 (Whitelist)**：支持启用白名单准入机制，只允许在名单中的 Steam 账号连入服务器。

---

### 6.8 计划任务与智能自动化运维 (Scheduled Tasks & Smart Automation)

- **可视化 Cron 任务定制器**：
  - 支持服主配置日常自动化流程：
    - `每日凌晨 04:00 重启服务器`（支持提前 10 分钟在游戏内自动广播多条倒计时预警）。
    - `每隔 2 小时自动创建世界存档快照`。
    - `每日凌晨 05:00 自动检查游戏与 Mod 增量更新`。
- **空服智能节能休眠 (Smart Idle)**：
  - 可选开启“当在线玩家连续 30 分钟为 0 人时，自动触发存档并转入低功耗待机”；当外部玩家重新通过客户端发起连接时快速热唤醒。

---

### 6.9 备份与快照管理 (Backups & Snapshots)

- **备份分级策略**：
  1. **游戏世界存档快照 (Save Snapshot)**：仅备份世界存档文件（体积仅几 MB 到几十 MB），速度极快，可在游戏运行时无感热备。
  2. **服务器全量镜像 (Full Backup)**：备份游戏程序、Mod、配置文件与存档全量，生成单文件归档。
- **一键回滚与下载**：
  - 备份列表展示创建时间、备份类型、文件大小。
  - 点击“恢复备份”：弹出二次安全验证，系统先自动为当前状态打一个保护快照，再安全解压历史快照回滚。
  - 点击“下载”：直接通过浏览器下载到服主本地电脑，实现双重异地容灾。

---

### 6.10 商业续费、升降配与收银台 (Billing, Renewal & Upgrades)

- **服务状态看板**：
  - 清晰展示当前实例规格：“4核 CPU / 16G 物理内存 / 50G 高速存储”。
  - 距离到期日倒计时条（例如“服务剩余：12天18小时，到期时间：2026-03-15”）。
- **一键续费收银台 (Checkout Modal)**：
  - 提供周期选择卡片：`1个月 (原价)`、`3个月 (9折优惠)`、`12个月 (7.5折大促)`。
  - 支付方式聚合选择：微信支付、支付宝、Stripe / 信用卡、站内预充值钱包余额抵扣。
  - 页面实时渲染二维码，前端使用长轮询 / WebSocket 监听付款状态，一旦付款成功，页面撒花提示，实例到期时间即时顺延，状态即时解锁！
- **无缝升降配 (Upgrade / Downgrade)**：
  - 服主若觉得人数变多服务器卡顿，可点击“升级配置”，选择更高规格（如 8核32G）。
  - 系统根据剩余有效天数，按天自动计算**折算补缴差价**。补缴完成后，无需重装游戏，系统仅需在下一次重启时热修改 Windows Job Objects 或 Linux cgroups 配额即可生效！

---

### 6.11 Steam OpenID 2.0 快捷登录与多渠道 Webhook 报警通知中心

1. **Steam OpenID 2.0 原生登录绑定**：
   - 游戏玩家天然拥有 Steam 账号。面板支持基于 Valve OpenID 2.0 规范的一键登录与授权；
   - 首次登录自动创建租户，同步抓取 Steam 昵称、头像与 64 位 SteamID；
   - 便于在服主管理后台直接将当前用户的 SteamID 一键设为游戏管理员（Admin）或加入服务器白名单。
2. **多渠道事件告警推送引擎 (Notification Engine)**：
   - 服主可为自己的服务器绑定专属外部机器人 Webhook：
     - **国内常用**：企业微信群机器人、飞书自定义机器人、钉钉群机器人、邮件、短信；
     - **海外常用**：Discord Webhook (Rich Embed 富文本卡片)、Telegram Bot；
   - **关键触发事件矩阵**：
     - `SERVER_CRASHED`: 游戏异常崩溃并触发熔断，推送崩溃日志前 10 行；
     - `EXPIRING_WARNING`: 实例距离到期不足 3 天 / 24 小时，推送账单续费直达链接；
     - `OVER_QUOTA_ALERT`: 存储空间超过 90% 预警，提醒及时清理；
     - `HIGH_LOAD_SPIKE`: CPU 或物理内存持续 5 分钟超过 95%，提醒可能被压测或 Mod 冲突。

---

## 7. 用户管理体系与细粒度 RBAC 权限设计

### 7.1 多级租户与用户角色模型

```
+-------------------------------------------------------------------------------+
|                             超级管理员 (Super Admin)                           |
|  - 拥有平台全部物理资源掌控权                                                   |
|  - 全局系统配置、支付网关配置、套餐制定、全局模板维护、用户增删改查              |
+---------------------------------------+---------------------------------------+
                                        | 创建 / 租售配额
                                        ▼
+-------------------------------------------------------------------------------+
|                       租户 / 服务器服主 (Server Owner)                         |
|  - 购买/拥有一台或多台游戏服务器实例                                            |
|  - 对自己名下的服务器拥有完整的管理、配置、续费、备份权力                        |
|  - 可将自己的服务器授权给技术员/协管员共同维护                                  |
+---------------------------------------+---------------------------------------+
                                        | 细粒度授权委派
                                        ▼
+-------------------------------------------------------------------------------+
|                      协作协管员 (Collaborator / Admin)                         |
|  - 由服主按需分配权限 (如：只允许在特定时间重启游戏、查看控制台、上传地图)        |
|  - 严禁执行危险操作 (无权删除服务器、无权申请退款、无权转移实例)                 |
+-------------------------------------------------------------------------------+
```

---

### 7.2 细粒度权限作用域 (Permission Scopes)

借鉴 PufferPanel 与 GameAP 的权威权限定义，系统构建了严密的树状权限空间：

```
server:
  ├── view                  # 查看服务器基础概览与在线状态
  ├── lifecycle:
  │     ├── start           # 开机
  │     ├── stop            # 关机
  │     ├── restart         # 重启
  │     └── kill            # 强制拔电 (Kill Process)
  ├── console:
  │     ├── view            # 实时查看控制台输出流
  │     └── send            # 向控制台输入指令 (敏感权限)
  ├── files:
  │     ├── list            # 浏览文件目录
  │     ├── read            # 查看与下载配置文件
  │     ├── write           # 保存编辑、上传文件
  │     ├── delete          # 删除文件 (高危)
  │     └── sftp            # 允许使用独立 SFTP 客户端登录
  ├── config:
  │     ├── view            # 查看变量设置与启动参数
  │     └── edit            # 修改启动端口、地图、最大人数等
  ├── workshop:
  │     └── manage          # 添加、删除或更新创意工坊 Mod
  ├── backup:
  │     ├── create          # 创建快照备份
  │     ├── restore         # 恢复备份 (高危)
  │     └── delete          # 删除备份
  └── billing:
        └── renew           # 执行续费与升降配操作 (服主独占)
```

---

### 7.3 API Key 与免密协作令牌体系

- **个人 API Token (Personal Access Tokens, PAT)**：支持用户生成具备特定 Scope 和过期时间的 API 密钥，可方便集成外部脚本、QQ/Discord 游戏群机器人实现群内指令一键查服/重启。
- **免密协管链接 (Invite Tokens)**：服主可在面板生成有效期为 24 小时的临时协助链接，受邀技术人员点击后直接进入受控控制台，无需注册全平台主账号，大幅降低协作门槛。

---

## 8. 计费系统、续费与实例生命周期状态机

### 8.1 商业套餐规格与定价模型

Master 提供套餐模板管理器，超级管理员可定义多款标准商业套餐：

| 套餐字段 | 说明与约束 | 示例值 |
| :--- | :--- | :--- |
| **套餐名称** | 展示给用户的前台名称 | "帕鲁畅玩 4核16G 专享版" |
| **底层规格配额** | CPU 限制百分比 / 限制核心数 | 4 Cores (400%) |
| | 物理内存配额 (Memory Limit) | 16384 MB (16 GB) |
| | 磁盘空间上限 (Disk Quota) | 50 GB NVMe |
| | 备份插槽数量 (Backup Slots) | 3 个快照位 |
| **计费周期与价格** | 月付 / 季付 / 年付价格 (存储为整型分) | 9900 (99.00 CNY / 月) |
| **支持游戏类型** | 该套餐允许部署的游戏模版 | `["palworld", "valheim", "ark"]` |

---

### 8.2 实例生命周期有限状态机 (FSM)

实例的生命周期状态转换具有单向性与强校验，杜绝幽灵开机与数据丢失：

```
                    +--------------------+
                    |  PENDING_PAYMENT   |
                    |   (等待首次付款)    |
                    +---------+----------+
                              |
                     支付成功 | 超时 30m 未付 -> CANCELED (取消)
                              ▼
                    +--------------------+
              +---> |       ACTIVE       | <---+
              |     |     (正常服务中)    |     |
              |     +---------+----------+     |
              |               |                |
              |      到期日前 3 天 发送续费邮件  |
              |               |                |
              |      到达到期时间 (Expire At)   | 宽限期内补缴续费
              |               ▼                |
              |     +--------------------+     |
              |     |    GRACE_PERIOD    | ----+
              |     |   (宽限缓冲期: 3天)  |
              |     +---------+----------+
              |               |
              |      宽限期结束，依然未续费
              |               ▼
              |     +--------------------+
              |     |     SUSPENDED      |
              |     |   (欠费停机冷冻)    |
              |     +---------+----------+
              |               |
              +---------------+ 停机 7 天内补缴续费 (解冻并恢复 ACTIVE)
                              |
                     逾期超过 7 天 未处理
                              ▼
                    +--------------------+
                    |  ARCHIVE_PENDING   |
                    | (系统自动压缩冷备份) |
                    +---------+----------+
                              |
                     备份归档完成
                              ▼
                    +--------------------+
                    |     TERMINATED     |
                    |  (物理释放目录端口)  |
                    +--------------------+
```

---

### 8.3 续费、宽限期与自动化停机/归档/回收工作流

系统通过 Master 内置的高精度定时任务（Cron Worker，每 5 分钟扫描一次）：
1. **预警通知 (Warning)**：
   - 距过期 72 小时与 24 小时：触发短信/邮件/Webhook 发送账单缴费通知。
2. **进入宽限期 (Grace Period, 默认 3 天)**：
   - 到期当日：实例状态变更为 `GRACE_PERIOD`。面板首页弹出显著告警红条，但**不立刻拔电关机**，保障正在进行重要副本或公会战的游戏玩家免于瞬间掉线。
3. **欠费停机 (Suspended)**：
   - 宽限期届满：系统立即向 Daemon 下发安全停机指令（`Soft Stop` 30 秒超时后强制 `Kill`）。
   - **锁定防护**：数据库中锁定实例的 `lifecycle:start` 权限，前端禁用所有启动按钮。**但保留全部用户数据、游戏存档与自定义配置**。
4. **自动续费解封 (Unsuspend)**：
   - 用户任何时候只要完成续费账单，系统立刻将到期时间顺延（基于原到期日顺延，而非支付日，防止薅羊毛），状态瞬时恢复为 `ACTIVE`，解锁启动权限。
5. **冷备份与终结销毁 (Termination)**：
   - 停机冻结超过 7 天（可配置）：系统最后一次触发备份引擎，将整个服务器目录打包为 `archived_server_{id}.tar.gz` 暂存冷存储库（保留 30 天可供管理员赎回），随后安全物理擦除游戏工作目录，并将分配的端口归还至全局端口池。

---

### 8.4 支付网关集成与幂等性对账机制

```
[用户前端]                     [Master 支付网关]                  [第三方收银台/Stripe/微信]
    │                                │                                    │
    │ 1. 点击“立即续费” (1个月)       │                                    │
    ├───────────────────────────────>│                                    │
    │                                │ 2. 创建本地待支付账单 (Status=PENDING) │
    │                                │    生成业务幂等键:                     │
    │                                │    `IDEMP_{ServerID}_{ExpireAt}_{Period}`
    │                                │ 3. 向收银台预下单                   │
    │                                ├───────────────────────────────────>│
    │                                │ <──────────────────────────────────┤
    │ 4. 弹出支付二维码 / 跳转收银台   │    返回跳转支付凭据                  │
    │<───────────────────────────────┤                                    │
    │                                │                                    │
    │ 5. 用户扫码完成扣款            │                                    │
    │═══════════════════════════════>│═══════════════════════════════════>│
    │                                │                                    │
    │                                │ 6. 异步发送已付款通知 (Webhook)      │
    │                                │ <──────────────────────────────────┤
    │                                │    a. 验签 (RSA/HMAC-SHA256)       │
    │                                │    b. 幂等键消费检验 (防重复投递)    │
    │                                │    c. 事务更新: 订单标记 SUCCESS     │
    │                                │       顺延实例 ExpiryDate          │
    │                                │       状态机: SUSPENDED -> ACTIVE  │
    │                                ├───────────────────────────────────>│
    │                                │ 7. 响应 HTTP 200 SUCCESS           │
```

---

### 8.5 主机商容量超卖水位与节点熔断控制 (Overcommit Ratios)

> **💳 计费与主机商运营专家核心把关**：
> "商业机房物理服务器成本昂贵，由于绝大多数游戏服不会在同一时间满载，主机商普遍需要进行合理的超卖（Overcommit）以摊薄硬件成本。但绝不能盲目超卖，必须建立**硬件超卖比率与动态熔断水位线**。"

1. **超卖配置维度 (Node Overcommit Configuration)**：
   - 超管可在节点属性上定义最大超额分配比率：
     - `CPU 超卖率`: 如 200%（即 16 核物理机允许累计售出 32 虚拟核心）；
     - `RAM 超卖率`: 如 125%（即 64GB 物理机允许累计售出 80GB 虚拟内存）；
     - `Disk 超卖率`: 严格锁定在 100%（**磁盘禁止超卖**，杜绝 ENOSPC 物理写满崩溃）。
2. **母机负载动态熔断保护机制 (Dynamic Overload Circuit Breaker)**：
   - 系统设置双重硬熔断防线：
     - 若节点的**物理内存真实消耗达到 85%**，即使该节点在账面上仍有超卖名额，Master 也会自动将该节点状态瞬时置为 `SOLD_OUT (售罄)`，阻止新订单创建；
     - 自动将后续新购买的用户订单转移调度至负载较低的其他节点，确保母机永不出现 OOM 导致内核蓝屏或进程被杀。

---

### 8.6 营销优惠券、促销码与推广分销返利系统

1. **优惠券引擎 (`coupons`)**：
   - 支持设置满减券（如满 100 减 20）、折扣券（如首月 8 折）、单游戏专用券（如仅限帕鲁专服使用）；
   - 支持限制每人领取次数与全平台发放总库存量。
2. **服主与公会分销推广返利 (`referrals`)**：
   - 每个注册用户拥有专属推广链接与邀请码；
   - 被邀请人首充或持续续费时，系统按比例（如 10%）自动将佣金划拨至邀请人的站内余额钱包，支持直接抵扣服主自己的服务器续费，实现自发性社交裂变营销。

---

## 9. 跨平台底层进程监督与安全沙箱

### 9.1 进程保活、崩溃恢复与防死循环熔断看门狗

游戏服务器进程经常因为 Mod 冲突、内存溢出（OOM）或游戏自身 Bug 崩溃。Daemon 内置双层看门狗（Watchdog）：
1. **进程退出事件监听**：
   - Linux: `syscall.Wait4` 阻塞监听退出信号与 Exit Code。
   - Windows: 挂载 `RegisterWaitForSingleObject` 监听进程句柄。
2. **崩溃重试熔断器 (Crash Loop Circuit Breaker)**：
   - 若服务器异常退出（ExitCode != 0），且用户开启了“崩溃自启”：
   - 系统检测在最近 5 分钟内的崩溃次数。
   - 若 5 分钟内连续崩溃超过 **5 次**，系统自动触发熔断，阻止盲目重启（避免陷入磁盘死循环占满 CPU），向面板抛出严重告警：“服务器启动反复崩溃，已自动熔断保护，请检查最新日志”。

---

### 9.2 跨平台计算资源配额 (Windows Job Objects & Linux cgroups v2)

#### Windows 实现：Windows Job Objects (作业对象)
- 创建专属作业对象：`CreateJobObjectW(nil, "GameServer_{ID}")`。
- 设置限制信息：`JOBOBJECT_EXTENDED_LIMIT_INFORMATION`：
  - `JobMemoryLimit`: 严格限制物理内存上限（超过自动拒绝分配内存，保护母机系统不蓝屏）。
  - `ActiveProcessLimit`: 限制该作业内允许派生的子进程最大数，杜绝 Fork 炸弹。
- 绑定 CPU 限制：`JOBOBJECT_CPU_RATE_CONTROL_INFORMATION`：
  - 限制最大 CPU 利用率百分比（例如限制最多占整机 CPU 的 25%）。
- 进程绑定：在启动游戏时传入 `CREATE_SUSPENDED`，将新进程句柄通过 `AssignProcessToJobObject` 压入作业后，再调用 `ResumeThread` 恢复执行。

#### Linux 实现：cgroups v2 (Control Groups)
- 为每个实例建立控制组：`/sys/fs/cgroup/gameserver/{id}/`。
- 限制内存：写入 `memory.max = 17179869184` (16GB)。
- 限制 CPU 配额：写入 `cpu.max = "400000 100000"` (最多 4 个物理核心)。
- 将游戏进程 PID 写入 `cgroup.procs`。

---

### 9.3 目录越权 (Path Traversal) 与安全隔离沙箱

```go
func SafeResolvePath(baseRoot string, requestedRelative string) (string, error) {
    cleanRel := filepath.Clean(requestedRelative)
    fullPath := filepath.Join(baseRoot, cleanRel)
    evalRoot, err := filepath.EvalSymlinks(baseRoot)
    if err != nil {
        return "", err
    }
    evalTarget, err := filepath.EvalSymlinks(fullPath)
    if err != nil && !os.IsNotExist(err) {
        return "", err
    }
    if !strings.HasPrefix(evalTarget, evalRoot) {
        return "", errors.New("security violation: path traversal detected")
    }
    return fullPath, nil
}
```

---

### 9.4 全局端口池分配引擎与多网卡 IP 绑定 (Port Pool & Multi-IP)

1. **多端口原子化分配引擎**：
   - 现代游戏通常需要 2~4 个不同协议的端口（如 Palworld 需要主通信 UDP 8211 + RCON TCP 25575；CS2 需要游戏 UDP 27015 + Query UDP 27015 + RCON TCP 27015 + GOTV UDP 27020）；
   - 系统在节点层维护全局端口池表（`port_allocations`），预留安全业务端口段（如 `7000-8999`）；
   - 创建实例时，通过数据库行级排他锁原子化申请可用连续/离散端口并打上实例归属标记；若无可分配端口直接友好拦截并提示联系管理员扩充端口段；
   - 实例销毁时，事务性释放对应端口，从根本上杜绝合租用户间的“端口冲突与抢占”。
2. **多网卡多公网 IP 绑定 (Multi-IP Binding)**：
   - 商业机房服务器经常绑定多个公网 IP 地址；
   - 系统支持在创建实例时指定其绑定的网卡 IP 地址，并在启动参数中注入 `+ip <allocated_ip>` 或 `-ip=<allocated_ip>`，实现单台物理机上多个合租实例使用各自独立公网 IP 的企业级隔离。

---

### 9.5 操作系统防火墙规则自动化联动 (Windows netsh / Linux nftables)

1. **自动生命周期编排**：
   - **游戏启动时**：Daemon 自动扫描实例分配的端口列表，向宿主机防火墙下发精准入站允许规则：
     - Windows: `netsh advfirewall firewall add rule name="GS_{UUID}_{Port}" dir=in action=allow protocol={UDP/TCP} localport={Port}`；
     - Linux: 动态添加 `nftables` / `iptables` 规则；
   - **游戏关机或删除时**：Daemon 自动精准撤销该规则。
2. **最小暴露面原则**：
   - 彻底告别运维手动在宿主机 RDP/SSH 加规则的落后方式，确保未运行的实例端口不暴露在外网，大幅降低被端口扫描和渗透的风险。

---

### 9.6 底层依赖静默自愈医生 (VC++ Redistributable, 32-bit glibc)

1. **Windows 宿主机环境自愈**：
   - 全新纯净 Windows Server 经常缺少 C++ 运行库，导致 SteamCMD 或游戏闪退且无日志；
   - Daemon 自带环境诊断器，自检注册表键值 `HKLM\SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\X64`；若缺失，自动静默下载并安装 `vc_redist.x64.exe /quiet /norestart`。
2. **Linux 宿主机环境自愈**：
   - SteamCMD 核心依赖 32 位 ELF 运行库；
   - Daemon 自检系统是否存在 `/lib/ld-linux.so.2`，若缺失则自动发出告警并调用系统包管理器自动安装 `lib32gcc-s1`、`lib32stdc++6`。

---

### 9.7 非特权用户运行沙箱与 RCON 暴力破解防御 (Fail2ban)

1. **剥离系统管理员特权 (Drop Privileges)**：
   - 严禁以 `root` 或 Windows `Administrator` 启动任何第三方游戏进程或 Mod；
   - Linux: Daemon 统一以非特权用户 `gameserver:gameserver`（UID/GID 1000）创建并切换 UID 运行，剥离 `CAP_SYS_ADMIN` 等全部敏感 Linux Capabilities；
   - Windows: 游戏进程均在非特权用户上下文中执行，禁止访问系统关键驱动器。
2. **RCON 暴力破解与凭据防护**：
   - 创建实例时系统强制生成 16 位高强度随机 RCON 密码；
   - 内置 **Fail2ban 速率限制防御器**：针对 RCON 交互端口进行监控，若单一来源 IP 连续 5 次认证失败，系统自动将该 IP 封禁 1 小时，杜绝暴力字典爆破。

---

### 9.8 全链路不可篡改审计追踪 (Audit Trail)

1. **全景操作审计 (`audit_logs`)**：
   - 对平台内所有敏感事件进行不可篡改的流水留痕：
     - 操作主体（用户 ID、租户角色、真实来源 IP、User-Agent）；
     - 操作对象（服务器实例 UUID、关联账单 ID）；
     - 动作类型（`server.start`, `server.stop`, `server.kill`, `file.delete`, `config.edit`, `billing.renew`）；
     - 变动快照（Diff: 记录修改前旧值与修改后新值）；
   - 为服主与协管员团队提供透明、可追溯的权限审计看板，责任到人。

---

## 10. 数据库设计与核心实体关系 (ERD)

系统使用 GORM 构建关系型模型，数据字典设计如下：

```
[ users (用户表) ]
  ├── id (PK, uint64)
  ├── username (varchar(64), unique)
  ├── email (varchar(128), unique)
  ├── steam_id64 (varchar(32), unique, nullable) -- Steam OpenID 2.0 绑定
  ├── password_hash (varchar(255))
  ├── role (enum: 'admin', 'user')
  ├── balance (bigint, 单位分)
  └── created_at, updated_at

[ nodes (节点服务器表 - 预留主控架构) ]
  ├── id (PK, uint64)
  ├── name (varchar(64)) -- 默认为 "Local Master Node"
  ├── host (varchar(128)) -- 默认为 "127.0.0.1"
  ├── grpc_port (int) -- 默认为 31718
  ├── os_type (enum: 'windows', 'linux')
  ├── cpu_overcommit_percent (int) -- 如 200 (允许超卖 200%)
  ├── mem_overcommit_percent (int) -- 如 125 (允许超卖 125%)
  ├── auth_token (varchar(255))
  ├── is_local (bool) -- 是否为同机本地节点 (当前为 true)
  └── status (enum: 'online', 'offline', 'sold_out')

[ packages (套餐规格表) ]
  ├── id (PK, uint64)
  ├── name (varchar(128))
  ├── cpu_limit_percent (int)
  ├── memory_limit_mb (int)
  ├── disk_limit_gb (int)
  ├── price_per_month (bigint, 单位分)
  └── allowed_games (json)

[ server_instances (游戏服务器实例表) ]
  ├── id (PK, uint64)
  ├── uuid (uuidv4, 唯一公开标识符)
  ├── name (varchar(128))
  ├── owner_id (FK -> users.id)
  ├── node_id (FK -> nodes.id)
  ├── package_id (FK -> packages.id)
  ├── template_id (varchar(64)) -- 如 "palworld", "cs2"
  ├── current_status (enum: 'installing', 'running', 'stopped', 'crashed')
  ├── billing_status (enum: 'active', 'grace_period', 'suspended', 'terminated')
  ├── quota_state (enum: 'normal', 'soft_over_quota', 'hard_over_quota')
  ├── bound_ip (varchar(64)) -- 绑定的物理公网 IP
  ├── allocated_ports (json) -- 如 {"SERVER_PORT": 8211, "RCON_PORT": 25575}
  ├── custom_variables (json)
  ├── expire_at (datetime)
  └── created_at, updated_at

[ gslt_tokens (Steam GSLT 官方登录令牌池) ]
  ├── id (PK, uint64)
  ├── app_id (varchar(32)) -- 如 "730" (CS2), "252490" (Rust)
  ├── token (varchar(64), unique)
  ├── memo (varchar(128))
  ├── is_used (bool)
  ├── assigned_instance_id (FK -> server_instances.id, nullable)
  └── created_at, updated_at

[ port_allocations (节点端口占用分配表) ]
  ├── id (PK, uint64)
  ├── node_id (FK -> nodes.id)
  ├── port (int)
  ├── protocol (enum: 'udp', 'tcp', 'both')
  ├── instance_id (FK -> server_instances.id)
  └── purpose (varchar(32)) -- 如 "SERVER_PORT", "RCON_PORT"

[ server_collaborators (协管授权表) ]
  ├── id (PK, uint64)
  ├── server_id (FK -> server_instances.id)
  ├── user_id (FK -> users.id)
  └── permissions (json) -- 允许的 Scopes 清单

[ billing_orders (充值与续费账单表) ]
  ├── id (PK, uint64)
  ├── order_no (varchar(64), unique)
  ├── user_id (FK -> users.id)
  ├── server_id (FK -> server_instances.id, nullable)
  ├── idempotency_key (varchar(128), unique)
  ├── coupon_id (FK -> coupons.id, nullable)
  ├── amount (bigint, 单位分)
  ├── months (int) -- 购买/续费月数
  ├── payment_channel (enum: 'balance', 'wechat', 'alipay', 'stripe')
  ├── payment_status (enum: 'pending', 'paid', 'canceled', 'refunded')
  ├── paid_at (datetime, nullable)
  └── created_at, updated_at

[ coupons (营销优惠券表) ]
  ├── id (PK, uint64)
  ├── code (varchar(32), unique)
  ├── discount_type (enum: 'percent', 'fixed') -- 折扣率 或 满减固定金额
  ├── discount_value (int) -- 如 80 代表 8 折，2000 代表减 20.00 元
  ├── min_order_amount (bigint) -- 最低消费门槛
  ├── max_uses (int)
  ├── used_count (int)
  ├── expire_at (datetime)
  └── created_at

[ notification_channels (外部告警机器人渠道表) ]
  ├── id (PK, uint64)
  ├── user_id (FK -> users.id)
  ├── instance_id (FK -> server_instances.id, nullable)
  ├── channel_type (enum: 'discord', 'wecom', 'feishu', 'dingtalk', 'telegram', 'email')
  ├── webhook_url (varchar(512))
  ├── secret_token (varchar(255), nullable)
  ├── enabled_events (json) -- 启用的事件类型清单
  └── created_at, updated_at

[ audit_logs (全链路不可篡改审计流水表) ]
  ├── id (PK, uint64)
  ├── operator_user_id (FK -> users.id)
  ├── operator_ip (varchar(64))
  ├── instance_id (FK -> server_instances.id, nullable)
  ├── action (varchar(64)) -- 如 "server.start", "file.delete", "config.update"
  ├── details (json) -- 变动详情快照
  └── created_at (datetime)
```

---

## 11. 从单机控制到分布式主控演进实施路径

当前仓库已落地的产品范围是 Python/FastAPI 的 Project Zomboid 单实例原型。以下路线以可运行代码为准；Go/Vue、Palworld/CS2 等原始设想属于未来方案，不表示已经实现。

```
+-------------------------------------------------------------------------------+
| 阶段一：受保护的 PZ 单机可操作 MVP（当前交付目标）                              |
+-------------------------------------------------------------------------------+
  - [ ] 管理员会话认证，保护 HTTP 管理接口与 WebSocket 控制台。
  - [ ] 浏览器单页控制台：安装/更新状态、启停、配置和日志。
  - [ ] 提供可查询的 SteamCMD 安装任务结果，修复重复安装竞争与失败反馈。
  - [ ] 验证单个 PZ 实例的配置持久化、进程生命周期和密码脱敏。
  - [ ] 建立不依赖真实 SteamCMD/PZ 进程的自动化测试及 CI。
  - [ ] 在目标 Windows 环境完成真实安装和启动联调后再声明 Windows 支持。

+-------------------------------------------------------------------------------+
| 后续阶段：运维能力与产品扩展（尚未实现）                                      |
+-------------------------------------------------------------------------------+
  - [ ] Workshop Mod 实际下载、备份恢复、A2S 状态探针和资源隔离。
  - [ ] 多游戏模板、多实例持久化、用户体系及商业计费。
  - [ ] 远程 Daemon、多节点集群、调度与迁移。
```

当前控制台对 Workshop Mod 只登记配置，不执行下载；续费接口为本地模拟；CPU/内存与磁盘用量是观测数据，不是系统级配额。未完成目标能力不可作为现有产品功能宣传。

---

## 12. 总结

本设计规范通过吸收 **WindowsGSM** 在 Windows 游戏专用控制台和 A2S 探针方面的宝贵实战经验，采纳 **GameAP** 的 Master-Daemon 分布式通信与跨平台服务封装范式，借鉴 **PufferPanel** 极其现代化优雅的声明式模板、DepotDownloader 与多路复用 WebSocket 设计，并深度融合 **CubeCoders AMP** 与 **TCAdmin** 工业级商业生命周期与续费管理标准，通过 `agency-agents` 五维专家团的严格推演，输出了一套**兼具当前单机落地极简性与未来集群扩展性**的最佳系统工程设计方案。
