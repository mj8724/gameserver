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

## 2. 未解除项（如实记录）

1. **`GET /api/instances` 在本次实机批次返回 404**，而同一提交的本地门禁与路由测试通过（路由注册见 `internal/adapters/httpapi/server.go:141`）。需在下一批次用**新构建的二进制**复验（疑似批次内二进制/包陈旧或端口复用到了旧进程）。
2. **PZ 并行场景的就绪判定**：本批次为并行叠加负载，PZ marker 未在 300s 内出现；不作为 M5.2 的失败项，但需在单实例与并行两种条件下分别记录就绪时间（下一批次补测）。
3. `mklink /J` 复用制品的第二实例方案在目标机上阻塞（已放弃，改用两个已安装实例），该路径未验证。

## 3. 结论

M5.2 的**隔离核心主张成立**：多实例在同一主机上可以并行运行，端口、数据根、状态与日志互不干扰，停止后无残留。就绪时间受并行负载影响（PZ 未在窗口内 ready）与 `/api/instances` 主机侧 404 属**未解除项**，不折算为通过。
