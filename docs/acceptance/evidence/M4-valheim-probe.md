# M4.3 Valheim 第二游戏：制品取证与实机闭环（隔离数据根基线验收）

> 目标机 DESKTOP-9M8FOG7（Windows 10 x64）；工作根 `G:\gameserver-work`；提交 `5dd16f8`。
> 口径：隔离数据根（`G:\gameserver-work\data-v3`），非生产路径；平台声明仅 Windows 10 x64。

## 1. 制品取证（只读探测，steamcmd exit 0）

| 项 | 结论 |
|---|---|
| 安装源 | Steam app **896660**（Valheim Dedicated Server），`+login anonymous` 即可 |
| 顶层制品 | `valheim_server.exe`（直接可执行，**无 launcher descriptor**）、`UnityPlayer.dll`、`start_headless_server.bat`、`steam_appid.txt` |
| 依赖 | Unity/Mono 运行时随包分发；`steamclient*.dll` 齐备 |
| 厂商 argv（`start_headless_server.bat`） | `valheim_server -nographics -batchmode -name "<name>" -port 2456 -world "<world>" -password "<pw>" -crossplay`；脚本 export `SteamAppId=892970` |
| 端口 | 2456（游戏）/2457（查询）/2458，UDP |
| 就绪 oracle | **stdout 日志标记 `Game server connected`**；实测首次生成世界约 40s 后出现（`-logpath` 参数不生效，日志只在 stdout） |
| 对账素材 | `steamapps/appmanifest_896660.acf`（与 PZ 同一套 build id + manifest SHA + 字节数对账机制） |

## 2. 插件实现（M4.3）

- `internal/plugins/valheimplugin`：描述符（app 896660、端口 2456/2457、readiness=log-marker + 180s 窗口、`ConfigTargets=["argv"]`、`InstallMarkers=["valheim_server.exe"]`）+ 厂商 argv 构造（值逐元素传参、密码 <5 字符失败关闭、**可执行文件绝对路径**）+ `SteamAppId` 环境变量。
- 共享编排零游戏分支：组合根按模板 id 解析插件；`archtest` 强制 adapter/共享编排不得导入 plugin。
- 新增 `templates/valheim.yaml`（版本清单/端口/变量/环境，argv 由插件拥有）。

## 3. 实机闭环（2026-09-30，服务端口 18795）

```
PLUGIN=game plugin valheim
INSTALL=202 → INSTALL_STATE status=COMPLETED progress=100 version=public
IS_INSTALLED=true
START={"message":"启动指令已执行","running":true}
READY="ready":true "readiness":"ready" EL=68s
PROCS=1 PORTS=3
STOP=200
AFTER_STOP_PROCS=0
```

| 阶段 | 结果 |
|---|---|
| 安装（真实 SteamCMD 896660 入实例目录） | **PASS**（202 → COMPLETED，`version:"public"`） |
| 安装检测（插件声明的制品标记） | **PASS**（`is_installed=true`；修复前用 PZ 候选文件判定，Valheim 恒判未安装） |
| 配置同步 | **按设计跳过**（`ConfigTargets=["argv"]`，无受管 INI 文件；修复前会错误地写 PZ INI 并导致启动 500） |
| 启动 | **PASS**（200 → running:true） |
| 就绪（log-marker，180s 窗口） | **PASS**（68s 达 ready；与探测结论一致） |
| 进程/端口 | **PASS**（valheim_server 1 进程、2456/2457 监听） |
| 停止 | **PASS**（200 → 残留 0 进程） |

## 4. 本次修复的三个真实缺陷（均已入库）

1. **启动路径解析**：进程适配器按 PATH 解析可执行文件 → 插件返回裸名必然失败（实测 `exec: "valheim_server.exe": executable file not found in %PATH%`）；修为绝对路径（`5dd16f8`）。
2. **安装检测游戏绑定**：`IsInstalled` 硬编码 PZ 候选文件 → 第二游戏恒判未安装；改为由插件描述符 `InstallMarkers` 提供（PZ 保留原候选集，等价性已验）。
3. **配置同步强绑 PZ**：`ApplyGameConfig` 无条件写 PZ INI → argv 型游戏启动 500；改为按描述符 `ConfigTargets` 决定（`53dfe0d`）。

## 5. 边界与未覆盖项（如实记录）

- **控制台**：Valheim 服务端不提供 stdin 控制台协议，插件 `ConfigTargets` 为 `argv`；`POST /api/server/command` 的行为保持既有契约（未运行 400），本次未作为 M4.3 的通过条件。
- **Linux**：未实测（Valheim Linux 服务端制品未下载）；不得声称 Linux 支持。
- **`-logpath` 不可用**：就绪只能依赖 stdout 捕获，长日志场景依赖环形缓冲（与 PZ 相同的观测限制）。
