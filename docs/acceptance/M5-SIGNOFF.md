# M5 签核：多实例产品化（2026-09-30）

> 口径：隔离数据根基线验收；平台声明仅 Windows 10 x64。

## 1. 计数

- `m4m6-offline.sh` → exit 0；M5 相关行：**PASS = 6 / FAIL = 0 / BLOCKED = 0 / NOT RUN = 0**（含 M5.4 按实例授权）
  - M5.1 注册表与端口对分配（5 项断言）PASS；实例创建/注销用例 PASS
  - M5.2 多实例清单聚合 PASS；双实例并行实机 PASS；**并行就绪数值已实测**：单实例 65s（冷缓存），并行 PZ 5s / Valheim 44s，停止后按 PID 核验无残留
  - M5.3 实例端点端到端（含 404 白名单缺陷修复验证）PASS
  - M5.4 **按实例授权** PASS：远程请求携带 `instance_id`，跨实例（或未知实例）请求在任何执行前被拒（审计 outcome=`instance_not_authorized`），活跃实例与空值（=活跃实例）放行（`TestRemoteRejectsForeignInstance`）
- 回归：M2/M3 矩阵未退化。

## 2. 交付与证据

| 能力 | 实现 | 证据 |
|---|---|---|
| 实例注册表（原子写 + 读回校验、legacy 自动发现、移除不删数据） | `internal/adapters/instanceregistry` | 5 项断言 |
| 端口对分配（跳过占用、成对不冲突） | `PortAllocator` | `TestPortAllocatorAvoidsUsedPorts` |
| 多实例控制面（注册表 + 活跃实例状态聚合、创建/注销、UI 选择器） | `GET/POST/DELETE /api/instances`、`ListInstances` | `TestM5OfflineInstancesEndpoint`、`TestM5OfflineInstanceLifecycle`、`TestListInstancesMergesRegistryAndLiveState` |
| **按实例授权** | `ControlService.authorizeInstance`：本进程仅管理自己的实例，跨实例一律拒绝并审计；远程请求的 `instance_id` 受同一边界约束 | `TestRemoteRejectsForeignInstance` |
| 双实例并行隔离 | 两进程各自实例/端口/数据根 | `M5-dual-instance.md`（PZ 16261/16262 + Valheim 2456/2457 同时监听、各 1 进程、停止后 0/0） |

## 3. 边界与后续项

1. **并行就绪数值已完成**（见 §1 与 `M3-residuals-live.md §8.2`）：单实例 65s、并行 PZ 5s / Valheim 44s；按 PID 跟踪停止后无残留。此前"残留 java 进程"经根因分析为**操作者本机的 Project Zomboid 游戏客户端**（非本项目进程），已降级为观测口径问题。
2. 明确**未启用**（计划明确的非目标，非缺陷）：RBAC/用户体系、真实计费、硬配额、自动归档删除。

## 4. 结论

M5 的隔离与产品化核心主张成立（双实例并行、互不干扰、零残留、控制面可创建/列举/注销、**按实例授权**），**并行就绪数值已完成**（单实例 65s；并行 PZ 5s / Valheim 44s），停止后按 PID 核验无残留（`M3-residuals-live.md §8.2`）。
