# 验收与演练工具说明（M1-A / M2）

本目录下的脚本与测试是**可复跑的验收证据来源**，不是授权。任何真实副作用（下载、安装、启动 PZ、端口绑定/网络探测、目标机写入）都需要用户逐项授权并填实 Target Manifest。

## 脚本一览

| 脚本 | 用途 | 退出码 |
|---|---|---|
| `rehearsals/m2-offline.sh` | 按 `M2-GO-PZ-MVP.md` 矩阵执行所有离线行；单次 `go test -json ./...` 后按行判定，另查目标提交的 GitHub CI 结果；输出 JSON + 供归档的 Markdown | 0=无 FAIL；1=有 FAIL |
| `../migration/rehearsals/m1a-rehearsal.sh` | M1-A 切换就绪演练：启动、权限归一、配置落盘、跨进程锁 409、备份/篡改拒绝/恢复提升、`export-back` 对账、promotion 中断矩阵、SIGTERM | 0=全部 PASS |
| `../migration/rehearsals/smoke-baseline.sh` | 真实二进制冒烟（登录/状态/配置校验/日志/登出重放/优雅关闭） | 0=通过 |

## 不变量（违反即视为验收无效）

1. **脚本绝不修改仓库工作树**：禁止 `git checkout/reset/clean/restore`；需要回滚只能在 `mktemp`/`git worktree` 出来的副本内操作。历史事故：一条 `git checkout -- .` 抹掉了未提交的运维子命令，导致提交只剩 docs 变更。
2. **提交前后核对文件清单**：`git show --stat HEAD` 必须包含预期改动的源文件；“提交信息与文件清单不一致”即失败。
3. **只写 disposable 根**：仓库 `data/`、生产根、用户数据只读。权限归一（`fix-permissions`）只对显式传入的 `--data-root` 生效。
4. **离线证据不等于实机证据**：offline/fixture 结果不得用于平台支持声明或真实 PZ 就绪结论。
5. **端口优先不监听**：能用 `httptest`/注入 listener 的断言不要绑定真实端口（见 `cmd/gameserver/main_test.go` 的真实装配黑盒）。

## 已知陷阱（务必先读）

- **legacy 数据根的宽松权限**：Python 时代目录常为 0755/0644，Go 服务按 ADR §1.7 对秘密相关路径**失败关闭**（配置写入报“保存配置失败”）。首次启动前必须显式执行：
  `gameserver fix-permissions --data-root <root> --instance <id>`。演练需同时覆盖“未归一 → 失败关闭”和“归一后 → 成功”两个方向。
- **`json.Number`**：HTTP 层用 `json.Number` 解码请求体；把 `any` 直接交给格式化函数会导致**静默回退默认值**（配置“保存成功”但 INI 未更新）。适配器入口必须处理 `json.Number`，并且“使用默认值”要可观测。
- **错误映射**：HTTP 错误映射的 `default` 分支不得吞掉已包装的业务失败原因；排障时若只看到通用文案，先怀疑映射缺失而不是未知故障。
- **DS 权限门与 D8**：服务运行期不得静默 chmod；权限归一只能由显式运维命令完成。

## 证据归档位置

- M1-A：`docs/migration/M1-SIGNOFF.md`
- M2 状态：`docs/acceptance/M2-SIGNOFF.md`
- M2 逐行证据：`docs/acceptance/evidence/M2-darwin-pre-live.md`（+ 同名 JSON，含 commit、Go 版本、二进制 SHA-256）

## Windows / PZ 42.x 实测陷阱（2026-09-29 血泪清单）

1. **口令必须两 token**：`-adminpassword <值>` 才被接受；`-adminpassword=<值>` 会记为 `unknown option` 并让服务端卡在交互式口令提示，进程活着但永远不就绪。日志脱敏必须识别「标志 + 下一 token」形态。
2. **配置根不等于缓存目录**：PZ 在 Windows 忽略 `-cachedir` 的**配置**语义——INI/SandboxVars 固定写 `%USERPROFILE%\Zomboid\Server`，而存档/DB 才落在 `-cachedir`。用 `GAMESERVER_PZ_HOME` 把配置读写点对齐到游戏真正读取的位置（缓存目录仍归实例所有），并让安全门接受覆盖后的路径形状。
3. **陈旧 cachedir → `QueuedQuit`**：反复 `taskkill /F` 后 PZ 会**主动排队退出**（`ServerMap.QueuedQuit`），日志尾部只有 tier0 断言噪音、看起来像崩溃。排障时先去掉 `tier0|Assertion` 行看真实尾部；处理方式是清理/重建实例缓存目录，运行手册要求停止优先走优雅路径。
4. **厂商描述文件可能自相矛盾**：`ProjectZomboid64.json` 的 classpath 首项 `java/.` 是笔误（其自身批处理用 `java/`），必须确定性纠正并记录，不可原样透传。
5. **PZ 只在退出时以运行时状态重写 sandbox**：API 回读（文件）与游戏内生效值可能短暂不一致，验收需在停止后再次回读。

6. **日志源的非正 limit 返回空**：`Supervisor.Logs(limit)` 对 `limit < 1` 返回空切片，因此「读取全部缓冲」必须显式传正数（如 `scanAll = 1000`）。就绪探针曾用 `Recent(0)`，导致 marker 永远读不到、`ready` 永不翻转，而 HTTP 日志端点（传正 limit）同时能看到 marker——排查此类问题必须对比"探针读取路径"与"展示路径"的同一数据源。
7. **超时窗口不要硬编码**：PZ marker 到达时间随机器/存档波动（本机实测冷 40s / warm 34s），oracle 窗口需可经 Manifest 覆盖（`GAMESERVER_READINESS_TIMEOUT`，缺省 60s），否则慢机器会出现"服务已就绪但 ready 永远 false"。

## M3（单实例可靠性）：矩阵与执行器

- 矩阵：`docs/acceptance/M3-GO-PZ-RELIABILITY.md`（四态判定与门禁规则）。
- 执行器：`bash docs/acceptance/rehearsals/m3-offline.sh` → `docs/acceptance/evidence/m3-offline-latest.json`。
- 定位：`m2-offline.sh` = **回归不变式**（M2 不退化）；`m3-offline.sh` = M3 里程碑验收。
- 计数：PASS/FAIL/BLOCKED/NOT RUN；FAIL>0 阻断；BLOCKED/NOT RUN 允许存在但必须逐 ID 列出。
- 证据口径：实机证据一律标注「隔离数据根基线验收」。

8. **`grep -c ... || echo 0` 在命令替换里会产生两行**：`$(cmd | grep -c x || echo 0)` 在计数为 0 时输出 `0\n0`，跨行匹配的断言会失配——验收脚本的残留检查曾因此误报 10/10 轮失败（真实 procs=0/ports=0）。写断言用 `|| true` 或先规范化输出。
9. **JSON 证据里的 Windows 路径必须转义**：直接把 `G:\path\x` 拼进 `printf '{"evidence":"%s"}'` 会产生非法转义（`\g`）使整个证据文件不可解析；用正斜杠或写进 Markdown 证据文件并在 JSON 中引用文档。
10. **start=200 只代表子进程创建成功**（既有契约）：端口被占用时 start 仍可能返回 200，判断"是否可用"必须看 readiness/日志，不能只看状态码。
11. **本机 `/api/status` 的 `memory_mb`/`cpu_percent` 在 Windows 恒为 0**：长时间观测的内存趋势需另行取数（PowerShell `Get-Process java | Handles/WorkingSet`），并在证据里注明口径差异。

12. **路由注册 ≠ 可达**：HTTP 中间件的请求路径白名单/方法表若没同步新端点，`mux` 里注册好的路由仍会 404/405（本项目连发两次：`/api/instances`、`/api/server/mods/download`）。现由 `internal/archtest` 的守卫测试解析 `HandleFunc` 注册源并逐一断言覆盖；新增端点后必须让该测试通过。
13. **可执行文件必须给绝对路径**：进程适配器按 `PATH` 解析裸文件名，插件返回 `valheim_server.exe` 会得到 `executable file not found in %PATH%`；插件应在 install 目录内拼好绝对路径（本项由 `process.LoggingSupervisor` 记录底层错误才得以定位）。
14. **argv 复用的隐性错**：实现"下载 Workshop 内容"时若复用安装的 `+app_update` argv，命令看起来成功（exit 0）却什么都没下；应显式发 `+workshop_download_item <appid> <id>` 且不与 app_update 同批。
15. **平台账号约束要先用原始 CLI 定性**：匿名 SteamCMD 下载 Workshop 会被 Steam 以 `Failure` 拒绝（需已认证且拥有该游戏的账号）。拿到直连 CLI 原始输出再判断，能避免在 argv/路径上反复徒劳修复——并把结论写成产品决策点，而不是"待实现"。
