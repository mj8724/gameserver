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
