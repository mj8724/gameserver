# M2 Windows 端到端 live 验收（条件性证据，未通过）

> **性质**：Windows **条件性**实机证据，**不产生平台支持声明**，不计入 `M2-PZ-LIVE` 的 E-LIVE 子案例。
> **状态**：**部分通过 + 1 项失败**（写入侧），失败原因已定位并归类为待修实现缺陷。

## 1. 环境与前置门

| 项 | 值 |
|---|---|
| 主机 | DESKTOP-9M8FOG7（Windows 10 Pro 10.0.19045.6466 / AMD64） |
| 受测提交 | `c89ddd1`（`feat/go-modular-skeleton`，Go 1.27.1 windows/amd64 现场构建） |
| 服务端口 | `127.0.0.1:18769`（仅回环）；PZ 端口 `16261/16262` |
| 授权 | `docs/acceptance/evidence/M2-windows-live-auth.md`（逐项：安装/validate、启动/停止、探测、配置写入） |
| 向量 | `launcher-descriptor`（依据 `TARGET-MANIFEST-windows.md`），内存 3072 MB |
| 就绪 oracle | 日志 marker `*** SERVER STARTED ***`（60s 窗口）+ 进程存活 |

## 2. 结果

| 步骤 | 结果 | 证据 |
|---|---|---|
| 模板版本清单 `/api/templates.versions[]` | **PASS** | 现场返回 `public`、`unstable`、`legacy41`、`42.19` 四个分支 |
| 未知分支安装请求 | **PASS** | `POST /api/server/install {"version":"nope"}` → **422**，且未启动下载 |
| 选定版本安装/校验（`{}` 默认分支） | **PASS** | HTTP **202**；`install_task` → `COMPLETED progress=100`，回显 `"version":"public"` |
| 选项写入 `POST {"options":…}`（INI 3 + SandboxVars 3，含 1 secret） | **FAIL** | HTTP **500**，`{"detail":"保存配置项失败"}` |
| 启动 → 就绪 → 控制台 → 停止 → 残留检查 | **NOT RUN** | 上一步失败后流程被中止（脚本卡在后续请求，已强制结束任务与 java/gs2 进程） |
| PZ 重写后回读与可写性 | **NOT RUN** | 同上 |

## 3. 失败根因（已定位）

`pz.Config.ApplyOptionValues` 先读实例子树内的配置文件来判定每个选项名的归属：

- 实例内存在 `<instance>/Zomboid/Server/servertest.ini`（服务此前创建），但**不存在** `<instance>/Zomboid/Server/servertest_SandboxVars.lua`；
- 因此 `SandboxVars` 类选项（`Zombies`、`Basement`、`LootItemRemovalList`）在实例内"不存在" → 适配器按设计**失败关闭**（拒绝创建新键）→ 控制面包装为「保存配置项失败」500。

**这与阶段 2 的隔离发现同源**：PZ 42.21 把配置写在**用户 profile**（`C:\Users\admin\Zomboid\Server`），而我们的实例子树只有自己创建的 INI；`-cachedir` 与环境重定向都不能改变这一点（`M2-windows-vendor-config-extraction.md` §5）。

## 4. 修复方向（下一轮）

1. **落地实例隔离决策**（三选一，需用户确认）：专用服务账户 / 共享 profile + `-servername` 分实例 / junction。
2. **首用播种**：实例内缺少 `<servername>_SandboxVars.lua` 时，从厂商基线（安装目录或抽取归档）**播种**一份到实例（写入前备份、原子写、读回校验），而不是让写入失败。
3. 播种后重跑本流程，并在阶段 6 补上「PZ 重写后回读 + 仍能写入新值」与残留检查。

## 5. 验收记法

按 `M2-GO-PZ-MVP.md §1.3` 三态：本文件的 FAIL/NOT RUN 不得记为 PASS；`M2-PZ-LIVE` 与 `M2-PLATFORM` 保持 **BLOCKED**（E-LIVE 目标为 Ubuntu LTS 具名主机）。

## 6. 复跑结果（2026-09-29，提交 `2a79beb`，修复后）

| 步骤 | 结果 | 证据 |
|---|---|---|
| 模板版本清单 | **PASS** | `public` / `unstable` / `legacy41` / `42.19` |
| 未知分支安装 | **PASS** | 422，未启动下载 |
| 选定版本安装/校验 | **PASS** | 202 → `COMPLETED progress=100`，回显 `version:"public"` |
| **选项写入（INI 3 + SandboxVars 3，含 1 secret）** | **PASS** | HTTP **200**；回读 `Zombies=7`（default 4）、`RCONPassword` 值 `null` 且 `secret:true`；`SECRET_LEAK=0` |
| 只读键经选项路径写入 | **PASS** | 409（含选项名的精确文案） |
| 启动请求 | PASS（HTTP 200） | `{"message":"启动指令已执行","running":true}` |
| **进程存活与就绪** | **FAIL** | 启动后进程立即退出：`PROCS=0`、`PORTS=0`、`readiness:"timeout"`；日志含 **`src\tier0\threadtools.cpp (3807) : Assertion Failed: Illegal termination of worker thread`**（Steam/tier0 断言） |
| 控制台命令 | 连带失败 | 400「服务器未运行」 |
| 停止与残留 | **PASS** | 停止 200；停止后进程 0 / 端口 0 |
| PZ 重写后回读与可写性 | **FAIL（读错文件）** | 读到的 profile 文件仍是 `Zombies = 4`、`RCONPassword` 不存在 → 证明**我们的写入落在实例子树，而 PZ 读的是用户 profile**（隔离缺口） |

### 6.1 两个新结论

1. **选项写路径已可用**（200/409/secret 不回显/回读一致），此前 500 的两个根因均已修复：① 缺少实例沙盒文件 → 播种修复；② 裸词字符串 → 写入为惰性字面量（`Basement = "Rural"`）。
2. **以 SYSTEM 身份运行服务无法拉起 PZ**：schtasks `/ru SYSTEM` 下 java 进程触发 Steam/tier0 断言后退出。厂商 `.bat` 与我们的 descriptor 直跑此前都在**用户上下文**（admin）成功到达 `SERVER STARTED`。→ 阶段 6 必须在**用户上下文**执行（交互式会话或 `schtasks` 以当前用户 + 已登录会话运行），并把该约束写入运行手册。
3. **隔离缺口仍是阻塞项**：写入落点（实例子树）与 PZ 读点（用户 profile）不一致；需在 §4 的三选一方案中定案，或实现「实例 → `-servername` + profile 目录托管」的映射。

## 7. 下一步（完成 #28 的前置）

1. 以**用户上下文**重跑（去掉 `/ru SYSTEM`），确认 PZ 达到 `SERVER STARTED` 并 `ready=true`；
2. 定案隔离方案后，把实例配置写入点与 PZ 读取点对齐（或按 `-servername` 映射），再验证「PZ 重写后回读 + 仍能写入新值」；
3. 全部通过后补齐本节表格与 Target Manifest 实值，并把 `M2-PZ-LIVE`/`M2-PLATFORM` 保持 BLOCKED（E-LIVE = Ubuntu LTS）。
