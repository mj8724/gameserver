# Target Manifest（Windows 条件验收子块）

> **性质**：Windows **条件性**验收基线，**不产生平台支持声明**。ADR §6.4 只约束 `M2-PZ-LIVE`/`M2-PLATFORM`（E-LIVE = Ubuntu LTS）；本文件是其登记的例外子块，证据不计入 `M2-PZ-LIVE` 的 7 行子案例。
> **采集**：只读，2026-09-29T06:34:19Z，采集者 `root-agent via ssh wingame`，主机 `DESKTOP-9M8FOG7`（Windows 10 Pro 10.0.19045.6466 / AMD64 / 16 vCPU / 32 GB）。原始 JSON：`docs/acceptance/evidence/m2-windows-artifacts.json`。

## 1. 环境与路径

| 项 | 值 |
|---|---|
| 目标主机 | `DESKTOP-9M8FOG7`（192.168.31.60，经 Cloudflare 隧道 `winssh.liubaitech.cn`） |
| 工作根 | `G:\gameserver-work`（Go 1.27.1 于 `G:\gameserver-work\go`，SHA256 校验通过） |
| 实例数据根 | `G:\gameserver-work\data`（服务端口固定 `127.0.0.1:18769`） |
| 安装目录 | `G:\gameserver-work\data\servers\pz_01\server_files` |
| SteamCMD | `G:\steamcmd\steamcmd.exe`（工作目录 `G:\steamcmd`） |
| 游戏/app | Project Zomboid Dedicated Server，Steam app `380870`（匿名登录） |
| 内置 JRE | `openjdk version "25.0.1" 2025-10-21 LTS` |

## 2. 制品清单（向量相关，逐文件 SHA-256）

| 相对路径 | SHA-256 | 大小 | 角色 |
|---|---|---|---|
| `ProjectZomboid64.json` | `403263cf4f15461470bb1d36a76a6665c4b436ef35746bcb8d91f970a36eca52` | 468 B | 厂商启动描述（launcher descriptor） |
| `jre64/bin/java.exe` | `007422d785bb5e1cf3cef1d0ef03645c9423713c83aeb8e1ba78f123a35855e1` | 49760 B | 直接可执行目标 |
| `StartServer64.bat` | `e9d76cadbb74729100808cd6d8a6af1d2a499a774054191c5af515ffd992af82` | 367 B | 厂商批处理（**服务不使用**，仅一次性人工取证） |
| `StartServer64_nosteam.bat` | `bced99f54be9e424011aaf592f78f204efe3389579cdbc6a39348e06731d02db` | 361 B | 同上 |
| `ProjectZomboid64.exe` | **不存在（ABSENT）** | — | 直接可执行向量**不可用** |

**关键结论**：Windows 服务端包**不含** `ProjectZomboid64.exe`，ADR §5.1 原定首选向量在该平台**不可满足**；因此需要第三向量 `launcher-descriptor`（直接 `jre64\bin\java.exe` + 类型化 argv）。

## 3. 厂商启动描述（实值）

```json
{"mainClass": "zombie/network/GameServer",
 "classpath": ["java/.", "java/projectzomboid.jar"],
 "vmArgs": ["-Djava.awt.headless=true", "-Xmx3072m", "-Dzomboid.steam=1", "-Dzomboid.znetlog=1", "-Djava.library.path=natives/", "-XX:-CreateCoredumpOnCrash", "-XX:-OmitStackTraceInFastThrow"],
 "windows": {"7": {"vmArgs": ["-XX:+UseG1GC"]}, "10": {"vmArgs": ["-XX:+UseZGC"]}}}
```

厂商批处理（逐字，用于对照）：

```
".\jre64\bin\java.exe" -Djava.awt.headless=true -Dzomboid.steam=1 -Dzomboid.znetlog=1 -XX:+UseZGC -XX:-CreateCoredumpOnCrash -XX:-OmitStackTraceInFastThrow -Xms16g -Xmx16g -Djava.library.path=natives/ -cp %PZ_CLASSPATH% zombie.network.GameServer -statistic 0 %1 %2
PAUSE
```
（`SET PZ_CLASSPATH=java/;java/projectzomboid.jar`）

**由实值确定的实现要求（已写入计划 D-A）**：
1. **classpath 以批处理为准**：`java/;java/projectzomboid.jar`。descriptor 的 `classpath` 首项 `java/.` 是厂商笔误（`java/` 才是可加载目录），**不能原样透传**；实现采用确定性映射表（`java/. -> java/`），并在 JSON 指纹变化时拒绝启动并要求重新抽取。
2. **vmArgs 合并优先级**：通用 `vmArgs` + 平台键 `windows.<ver>.vmArgs` **追加**（Windows 10 → `-XX:+UseZGC`；批处理逐字印证）。
3. **内存参数**：以目录 `target: launch` 的 `Xms/Xmx` 覆盖（默认取 descriptor 的 `-Xmx3072m`；批处理的 `-Xms16g -Xmx16g` 与主机内存绑死，不采用）。
4. **`-statistic 0`**：descriptor 无此项，来源为厂商批处理 → 作为代码常量登记（理由：与厂商行为一致）。
5. **固定 WorkDir** = 安装目录（相对路径 `natives/`、`java/` 依赖它）。
6. **白名单**：`vmArgs` 仅接受本次登记的 token；拒绝 `-javaagent`/`-agentlib`/`-cp`/`-jar`/`@argfile` 等。

## 4. 待补齐（阶段 2）

| 项 | 状态 |
|---|---|
| oracle 证据（该 build 不支持 A2S + 逐字 marker `*** SERVER STARTED ***` + 匹配规则） | **待阶段 2 实机取证**（需先有可用向量） |
| 全量 `servertest.ini` / `servertest_SandboxVars.lua`（含 PZ 重写样本） | 待阶段 2 |
| 版本清单（`app_info_print 380870` 分支/build） | 待 #22 |
