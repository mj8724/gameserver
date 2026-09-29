# 厂商配置抽取记录（阶段 2）

- **日期**：2026-09-29（UTC 07:05–07:13）
- **主机**：DESKTOP-9M8FOG7；命令（人工一次性取证，**不在服务启动路径内**）：
  `cmd.exe /c StartServer64.bat "-cachedir=G:\gameserver-work\data\servers\pz_01\Zomboid" "-servername=servertest"`（stdin 管道在就绪后发送 `quit`，日志出现 `Shutdown handling finished` 确认优雅退出）
- **PZ 版本**：`42.21`（`backups/last_server_version.txt`）
- **oracle marker（逐字）**：`*** SERVER STARTED ****`（注意末尾为 4 个星号；服务按子串 `*** SERVER STARTED ***` 匹配）
- **端口自述**：`Server is listening on port 16261 (for Steam connection) and port 16262 (for UDPRakNet connection)`

## 0. 关键发现（影响实例隔离设计）

`-cachedir=<实例>/Zomboid` **未生效**：PZ 仍把配置/存档写入 **`C:\Users\admin\Zomboid\`**（用户 profile），我们指定的实例目录下只留下一个陈旧 `Server/servertest.ini.bak`。
- 证据：抽取后 `G:\gameserver-work\data\servers\pz_01\Zomboid\Server\` 无新文件；`C:\Users\admin\Zomboid\Server\` 出现本次生成的 `servertest.ini`、`servertest_SandboxVars.lua`（mtime 15:09）。
- 影响：在解决重定向之前，**实例级隔离不成立**（多实例会共用同一份 Zomboid 目录，且 `VERSION`/`ServerPlayerID` 等键会互相覆盖）。
- 待验证的候选方案（阶段 3 前必须定论）：① `-cachedir` 的正确形态（`-cachedir=…\` 结尾、`-cachedir64`、参数位置）；② 子进程环境重定向（`USERPROFILE`/`APPDATA`/`LOCALAPPDATA` 指向实例目录）；③ 专用服务账户。

## 1. 抽取产物（逐字归档于 `docs/acceptance/evidence/vendor-config/`）

| 文件 | 大小 | 行数 | SHA-256 |
|---|---|---|---|
| `servertest.ini` | 15658 B | 421 | `cf8c8dd5fa609ecce4e7fa5a34539658e3681940c284a59652415f0598b6a934` |
| `servertest_SandboxVars.lua` | 45533 B | 1021 | `55a04f4eb34e4bcee4c403aa0e0219e850fb56aea153c8df0d6c477d1723ec6b` |
| `servertest_spawnpoints.lua` | 123 B | 8 | `c4224a595ecf2b8d91ca3108f1cef12fbd6427337e96e20a98421e2646b70d5c` |
| `servertest_spawnregions.lua` | 523 B | 11 | `f8dd1faf2175b564c9490e7e0ca19a0000affbed97d5794ea0dc94311a8def84` |

## 2. 规模（目录抽取的输入事实）

- INI 键（`KEY=` 形态）：**144** 个；去重后 **144** 个
- SandboxVars 顶层/嵌套赋值（`path =` 形态）：**276** 处（含嵌套表）
- 结论：目录规模与计划 D-H 的上限假设（≤500 项）相符；INI 全量键是 `options[]` 的第一批。

## 3. 待办（#23 剩余）

1. `tools/pzoptions` 解析上述两个文件 → `catalogs/project_zomboid.options.yaml`（含 `source{build_id, files[], sha256, extracted_at, command}`）
2. 集合相等校验（INI 键集合、SandboxVars 表路径集合与厂商文件逐一相等，差集为空）
3. 抽样 ≥20 项人工核对默认值（含 enum/float/secret 各 ≥1：`Password`/`RCONPassword` 属 secret）
4. 登记 `archtest` 的 `tools/pzoptions`、gofmt 口径扩到 `tools`（属范围修订，需用户确认）

## 4. 目录生成与校验（已完成）

工具：`tools/pzoptions`（新增，已登记 `internal/archtest` 分类）

```
go run ./tools/pzoptions -ini docs/acceptance/evidence/vendor-config/servertest.ini \
  -sandbox docs/acceptance/evidence/vendor-config/servertest_SandboxVars.lua \
  -template project_zomboid -build 42.21 -out catalogs/project_zomboid.options.yaml
```

| 校验项 | 结果 |
|---|---|
| 集合相等（INI 键 / SandboxVars 路径） | **PASS**：`ini_keys=144 sandbox_paths=272 options=416`，差集为空（缺失即工具 exit≠0） |
| 类型分布 | bool 118 / int 167 / float 101 / string 30 |
| secret 分类 | `Password`、`RCONPassword`（值不落目录：`default: ""`） |
| 抽样核对（24 项，含 secret/float/enum-ish） | **0 不匹配**（与厂商文件逐项一致，脚本见提交说明） |
| 可复现性 | **PASS**：同参数重跑后 `git diff` 为空（时间戳由 `PZOPTIONS_TIMESTAMP` 固定） |
| 单写者标记 | 与模板变量绑定键（`Public/Password/MaxPlayers/PVP/Open/PauseEmpty/DefaultPort/UDPPort`）及 mods 键标记 `writable: ro`；`WorkshopItems/Mods` 归 mods API |
| 负向测试 | `TestOptionCatalogStaysOutOfTemplates`：目录文件不出现在 `/api/templates`，模板加载零 warning |

**范围修订待确认**：gofmt 口径需从 `cmd internal` 扩到 `cmd internal tools`（本地门禁已按新口径执行；CI 与 `m2-offline.sh` 待用户确认后同步）。

## 5. 重定向实验结论（2026-09-29 07:16–07:19 UTC）

三个变体（均在隔离目录、每轮 ~70s、事后 `taskkill /F /IM java.exe /T`）：

| 变体 | 参数 | 服务启动 | 实例 profile 内 INI | 真实 profile INI |
|---|---|---|---|---|
| A | `-cachedir=<实例>\Zomboid` | 未启动（MARKER=0，参数传递受 shell 影响） | 0 行 | 420 行 |
| B | `-cachedir=<实例>\Zomboid\`（尾反斜杠） | 未启动 | 0 行 | 420 行 |
| C | 无 `-cachedir`，仅 `USERPROFILE`/`APPDATA`/`LOCALAPPDATA` 重定向 | **启动成功（MARKER=1）** | 0 行 | 420 行 |

**结论**：PZ 42.21 的 Zomboid 目录由 **Windows 用户 profile** 决定；`-cachedir` 不改变配置写入位置，环境变量重定向也不影响（Java `user.home` 不取自这些变量）。

**可选隔离方案（需用户决策）**：
1. **专用服务账户**（每实例一个 Windows 用户）→ profile 天然隔离（最干净，运维成本最高）
2. **共享 profile + `-servername` 分实例**：PZ 原生支持同一 Zomboid 目录下多套 `<servername>.ini`/`<servername>_SandboxVars.lua`/存档；把「实例」映射为 `servername`，我们的文件管理作用域改为 `<home>/Server/<servername>*`（改动最小，但实例不再拥有独立目录树，需同步修订 ADR 的布局假设）
3. 目录联接（junction）把 profile 的 `Zomboid` 指向实例目录（全局切换，仅适合同时只跑一个实例）

## 6. 向量等价性判定（阶段 2 三分支之③，2026-09-29 07:2x–07:4x UTC）

对照运行（隔离目录、同一 `-servername=servertest`、同一 profile 目录）：

| 观测项 | 厂商 `.bat` 基线 | descriptor 向量（本服务产出的 argv 形状） | 差异 |
|---|---|---|---|
| 启动里程碑 `SERVER STARTED` | ✓（15:08 首轮） | **✓（MARKER=1）** | 无 |
| 端口自述 | `listening on port 16261 … 16262` | `listening on port 16261` | 无 |
| JVM 参数来源 | 批处理硬编码（含 `-Xms16g -Xmx16g`、`-statistic 0`） | 描述文件 `vmArgs` + `windows.10.vmArgs` + 配置化 `-Xms/-Xmx` + `-statistic 0` | 内存参数按计划由配置接管（已在 Manifest §3 登记） |
| `servertest.ini` 键集合 | 144 键 | 同左 | **0 / 0**（双向差集为空） |
| `servertest_SandboxVars.lua` 路径集合 | 272 路径 | 同左 | **0 / 0** |
| 产物字节 | `49bf30b5…` / `55a04f4e…` | 同左（运行窗口内未被重写） | 无 |

**判定：等价（branch ③）**，据此可进入阶段 2b（ADR 第三向量签核）。

**保留的注意点（写入 ADR 修订正文）**：
1. descriptor 运行未观察到配置重写（窗口内文件字节不变），因此"重生成等价"由「键集合双向相等 + 启动里程碑一致 + 端口自述一致」推定；阶段 6 将补一次带优雅退出的重写观测。
2. `-cachedir` 不改变配置位置（§5），等价性对照因此共用同一 profile 目录；实例隔离方案仍待用户决策。
3. 内存参数与厂商批处理的偏离（16g → 配置值）已登记；`-statistic 0` 作为代码常量保留。
