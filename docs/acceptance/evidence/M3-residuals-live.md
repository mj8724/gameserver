# M3 实机残留清偿与观测窗口（隔离数据根基线验收）

> 目标机 DESKTOP-9M8FOG7；工作根 `G:\gameserver-work`；数据根 `data`（PZ 实例 pz_01）；服务端口 18799。
> 批次脚本 `/c/Users/admin/AppData/Local/Temp/gs-m3r.sh`；原始日志 `G:\gameserver-work\logs\m3r.txt`。

## 1. M3.4 A2S 可达性 —— **PASS（改判）**

在 PZ 42.21 实机 ready 状态下，经本服务的查询面（`game_query`）取得**真实 A2S_INFO 字段**：

```
game_query={"map":"Muldraugh, KY","max":100,"name":"My PZ Server","players":0}
readiness="ready"
```

结论：**声明目标 PZ 42.21 响应 A2S_INFO**，解析器（`internal/adapters/pz/a2s.go`）字段映射正确（名称/地图/玩家数/上限）。M3.4 的 BLOCKED 行据此改判 **PASS**，查询面不再需要 `unavailable` 降级路径作为默认（降级仍保留为不可达时的安全行为）。

## 2. M3.3 workshop 下载 —— 缺陷已定位并修复，实机复验待做

首次实机运行暴露**两个真实缺陷**（均已修复并入库）：

1. **端点不可达**：`/api/server/mods/download` 未在 HTTP 请求路径白名单中 → 405 `Method Not Allowed`（与 `GET /api/instances` 同类缺陷）。修复：补齐白名单与方法表，并新增**路由覆盖守卫测试** `TestEveryRegisteredRouteIsWhitelisted`（解析 `server.go` 中全部 `HandleFunc` 注册，逐一断言 `knownPath` 与 `allowedMethods` 覆盖），从机制上防止该类缺陷再次出现（提交 `10d8737`）。
2. **下载命令错误**：`DownloadWorkshopItem` 复用了 `+app_update` 的 argv，**从未发出 `+workshop_download_item`**，因此内容目录永不出现（实测返回 `workshop item … did not land under …`）。修复：`InstallSpec.WorkshopID` + `BuildArgs` 在 workshop 模式下只发 `+workshop_download_item <appid> <id>` 且**不与 app_update 同批**，新增断言 `TestBuildArgsWorkshopDownload`（提交 `014d847`）。

**复验状态**：修复后尚未重跑实机（观测窗口占用同一数据根/SteamCMD 槽，避免干扰）。**在该行复验通过前保持 BLOCKED**。

## 3. M3.6 端口占用失败关闭 —— 仍 BLOCKED

占用 UDP 16261 后再启动：`{"message":"启动指令已执行","running":true}`（start 仍 200）。这与既有契约一致（start 以“子进程创建成功”为准），**仍未证明**游戏在该条件下失败关闭。需要以游戏就绪/日志复核，当前批次未取得该证据 → **维持 BLOCKED**。

## 4. 观测窗口（进行中 → 已达 99 样本 ≈ 1h40m）

| 指标 | 结果 |
|---|---|
| `running` | true（全程） |
| `readiness` | `ready`（全程） |
| PID | 3508（恒定，无重启） |
| **句柄** | 5449 → 5436 → **5344 后恒定**（无增长趋势） |
| 工作集 | 246–3520 MB 区间波动（GC 正常回收） |
| 关键事件 marker | 恒为 1（环形缓冲可检索，无静默丢失） |

已达时长约 1 小时 40 分；**目标 ≥4h（理想 5h）**。在达到窗口前该行记 `BLOCKED(时间窗口)`，**不记 PASS**；已收集证据表明无泄漏与无崩溃。

## 5. 小结

- **改判 PASS**：M3.4 A2S 可达性（真实字段）。
- **缺陷修复待复验**：M3.3 workshop 下载（端点白名单 + argv 两个缺陷已修，实机复验待做）。
- **维持 BLOCKED**：M3.6 端口占用失败关闭语义；观测窗口时长（证据趋势良好）。
