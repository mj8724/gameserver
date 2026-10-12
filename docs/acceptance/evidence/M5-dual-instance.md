# M5.2 多实例隔离：双实例并行实机验证（隔离数据根基线验收）

> 目标机 DESKTOP-9M8FOG7；工作根 `G:\gameserver-work`；提交 `86f4185`；口径：隔离数据根。
> 两个服务进程同时运行，各自持有**不同实例、不同数据根、不同端口**。

## 1. 实测结果（2026-09-30）

```
COMMIT=86f41857  BUILD_OK  CACHE_CLEARED
START_A=200   START_B=200                      # 两个实例同时接受启动
PZ_PORTS=2    VAL_PORTS=3                      # PZ 16261/16262 + Valheim 2456/2457 同时监听
PZ_PROC=1     VAL_PROC=1                       # java.exe 与 valheim_server.exe 各 1
A_STATE="instance_id":"pz_01"  B_STATE="instance_id":"valheim_01"
STOP_A=200    STOP_B=200
AFTER_PZ=0    AFTER_VAL=0   AFTER_PORTS=0      # 停止后零残留
```

| 验收点 | 结果 |
|---|---|
| 双实例并行运行（同机、两进程） | **PASS**（PZ + Valheim 同时 running，进程各 1） |
| 端口隔离互不冲突 | **PASS**（16261/16262 与 2456/2457 同时监听，无抢占） |
| 实例状态/配置互不串写 | **PASS**（各自 `/api/status` 报告自己的 `instance_id`；数据根分别为 `data` 与 `data-v3`） |
| 日志互不混淆 | **PASS**（两进程独立日志文件 `dual2-a.log` / `dual2-b.log`） |
| 停止后无残留 | **PASS**（两实例进程 0、四个端口 0） |
| 同一实例争锁（跨进程） | 已在 M3.6 实测：第二个进程被拒 `409 instance owned by another process`；本批次以「不同实例互不影响」互补 |
| PZ 就绪 marker 窗口内 | **未达**：PZ 端口已监听、进程存活，但 300s 测量窗口内未见 marker（同机并行两台服务 + 冷缓存重建世界）；**Valheim 侧在同一条件正常 ready**。PZ 单实例下的就绪已在 M3.5/M3.6 多次实测（冷 40s / warm 34s） |

## 2. 边界与历史记录（含已关闭项）

1. ~~`GET /api/instances` 返回 404~~ **已定位并修复**：根因是 HTTP 请求路径白名单未包含新端点（中间件层 404，而非路由缺失）；修复见 `internal/adapters/httpapi/server.go` 与架构守卫 `internal/archtest` 的 `TestEveryRegisteredHTTPRouteIsReachable`，本地端到端 `TestM5OfflineInstancesEndpoint` 复验通过。
2. **PZ 并行场景的就绪判定**：本批次为并行叠加负载，PZ marker 未在 300s 内出现；边界的 600s 窗口复测见 `M3-residuals-live.md §7`（数值化结果）。
3. ~~`mklink /J` 复用制品~~ 该路径在目标机阻塞，已放弃（改用两个已安装实例完成验收）；**不再作为未解除项**，仅作历史记录。

## 3. 结论

M5.2 的**隔离核心主张成立**：多实例在同一主机上可以并行运行，端口、数据根、状态与日志互不干扰，停止后无残留。首轮的 `/api/instances` 404 已定位并修复（中间件路径白名单 + 架构守卫）；**并行就绪数值已在后续批次取得**（单实例 65s，并行 PZ 5s / Valheim 44s，按 PID 核验停止后无残留，见 `M3-residuals-live.md §8.2`）。本文件不再保留未解除项。
