# M4 签核：游戏插件契约与第二游戏（2026-09-30）

> 口径：隔离数据根基线验收；平台声明仅 Windows 10 x64（Linux/macOS 未验证）。
> 独立审查：**非独立复核**（ADR §11.3 豁免留痕）。

## 1. 计数（先跑脚本后写数字）

- 离线矩阵：`bash docs/acceptance/rehearsals/m4m6-offline.sh` → **exit 0**；`docs/acceptance/evidence/m4m6-offline-latest.json`
- M4 相关行：**PASS = 4 / FAIL = 0 / BLOCKED = 0 / NOT RUN = 0**（M4-BUILD 契约与注册表、M4.2 PZ 插件等价、M4.3 Valheim argv/就绪 5 项断言 + 实机闭环）
- 回归不变式：`m2-offline.sh` exit 0（53/0/15 未退化）、`m3-offline.sh` exit 0（23/0/4 未退化）

## 2. 交付

| 能力 | 实现 | 证据 |
|---|---|---|
| 插件契约（编译期注册、未知模板失败关闭） | `internal/ports/plugin.go`、`internal/plugins/registry.go` | `TestRegistryLookupFailsClosed` 等 4 项 |
| 层级约束（共享编排不得含游戏分支） | `internal/archtest` 新增 `plugin` 层 | archtest 全绿 |
| PZ 作为首个插件（等价性） | `pluginSteamAppID`/`pluginReadinessMarker`/描述符驱动配置同步 | M2/M3 矩阵零退化 |
| Valheim 第二游戏 | `internal/plugins/valheimplugin`、`templates/valheim.yaml` | 5 项 argv/就绪断言 + **实机闭环**（`M4-valheim-probe.md`：安装 COMPLETED、启动 200、**ready@68s**、停止 200、残留 0） |

## 3. 结论

**M4 判定为通过（离线 + 实机闭环）**；Valheim 控制台（无 stdin 协议）按插件边界记为未覆盖项而非失败；Linux 未实测，不产生支持声明。
