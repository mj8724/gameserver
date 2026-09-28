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
