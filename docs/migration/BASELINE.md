# M1 迁移基线证据

> 本文件是已批准 Plan（handoff key `86c3e976a061fc28b352f996708826a95c395a6600595d2b4cf9f13f1b8fc8f0`）阶段 0 的产出：
> 冻结改动前的工作区状态、Python 测试基线与 CI 证据。**本文件不代表 Go/PZ/Windows 已验证。**

## 1. 工作区保留清单

采集命令：`git status --short --branch`（分支 `main`，跟踪 `origin/main`）。

| 路径 | 状态 | 处理约束 |
|---|---|---|
| `docs/SYSTEM_DESIGN_AND_ARCHITECTURE.md` | 已修改（+153 / −117） | 用户既有改动。不得 reset/checkout/覆盖；本 Plan 的 ADR 与 M2 计划写入新路径。 |
| `.maestroignore` | 未跟踪 | 保留原样，不纳入本次提交范围。 |
| `.workflow/` | 未跟踪 | 保留原样（含 roadmap 会话、specs、knowhow）。 |
| `Frameworks` | 未跟踪（符号链接 → `MacOS/Frameworks`） | 保留原样。 |

`.gitignore` 已覆盖 `data/`、`.venv/`、`.pytest_cache/`、`__pycache__/`、`research_repos/`。

**结论：** 采集时刻不存在本次任务产生的改动；上述状态即“迁移前基线”，后续任何写入都必须与此清单核对且不得清理未跟踪路径。

## 2. Python 离线测试基线（本机）

| 项 | 值 |
|---|---|
| 解释器 | `.venv/bin/python` → Python 3.14.5（`.venv/bin/python3.14` → `/opt/homebrew/opt/python@3.14/bin/python3.14`） |
| pytest | 9.1.1 |
| 命令 | `.venv/bin/python -m pytest -q` |
| 结果 | **9 passed, 1 warning in 1.94s** |
| 警告 | `StarletteDeprecationWarning: Using httpx with starlette.testclient is deprecated` |
| 覆盖范围 | 认证/未授权拒绝、秘密脱敏、配置校验（路径穿越/未知字段/范围）、INI 写清空与非法名称、状态嵌套默认值合并、SteamCMD 初始化失败清理 busy、进程监督短命进程与停止等待 |

**边界：** 该结果仅为本机 Python 3.14 证据，既不是 Go 证据，也不是目标平台（PZ 实机/Windows）证据。

## 3. Ubuntu / Python 3.11 CI 证据

| 项 | 值 |
|---|---|
| 仓库 | `mj8724/gameserver`（`main`） |
| Run | `36383916726`（workflow `tests`，event `push`） |
| 结论 | **success**（job `pytest` success，9s；步骤含 `setup-python@v5` → `pip install -r requirements-dev.txt` → `python -m pytest -q`） |
| headSha | `ebacf29084e63afb505044ae69957c88a951e772` |
| 当前 HEAD | `ebacf29084e63afb505044ae69957c88a951e772`（**一致**） |
| 时间 | 2026-09-28T05:54:11Z |
| 平台 | `runs-on: ubuntu-latest` + `python-version: "3.11"`（依 `.github/workflows/tests.yml`） |

**结论：** 迁移前的 Python 3.11 CI 基线门槛已满足，且证据对应到当前提交。运行环境出现 Node.js 20 弃用与 `ubuntu-latest` 迁移提示（非失败原因），记录为后续 CI 维护项。

## 4. 尚未取得（缺口）

- Go 工具链：本机 `go` 不可用；`go.mod`/Go 源码不存在 → Go 侧任何“通过”声明均无证据。
- 真实 SteamCMD / PZ 实机安装、就绪、控制台、端口与停止验收：未执行。
- Windows 实机验收：未执行 → 依据质量规则与路线图，Windows 保持“未验证”。
- 跨进程单写者、promotion 中断恢复、备份/恢复演练：当前实现中不存在相关机制（详见 `LEGACY-CONTRACT.md` 与 ADR）。

## 5. 复现命令

```bash
git status --short --branch
git diff --stat
.venv/bin/python -m pytest -q
gh run view 36383916726 --json conclusion,headSha,workflowName
git rev-parse HEAD
```
