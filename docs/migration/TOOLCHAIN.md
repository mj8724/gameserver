# Go 工具链固定记录

> 依据 `docs/adr/ADR-001-go-backend.md` §1.8：**精确工具链版本必须在本文件、本机 `go.mod` 的 `toolchain` 指令与 `.github/workflows/go.yml` 三处一致**；`go.mod` 的 `go 1.27` 是语言/最低版本基线，不要求等于 `toolchain go1.27.1`。任何精确版本不一致都视为配置错误。
> 本文件是 Go 版本的**唯一事实来源**。未完成本文件与 CI 之前不得编写 Go 实现，也不得声称任何 Go 验证通过。

## 固定版本

| 项 | 值 |
|---|---|
| 版本 | **go1.27.1** |
| 平台（本机开发） | darwin/arm64 |
| CI 平台 | ubuntu-latest（`.github/workflows/go.yml`，`go-version: "1.27.1"`） |
| 语言基线 | `go 1.27`（语言版本）；toolchain 精确固定为 `toolchain go1.27.1`。两者合用可让 go directive 表达语言最低版本，同时 pin 实际工具链 |
| 安装方式 | 官方发行包解压到项目外目录（用户授权：不使用系统包管理器） |
| 安装位置 | `$HOME/.local/go`（`$HOME/.local/go/bin/go`） |
| 官方校验和 | `ee215d57e0ec269c60cc9ceca68e6bda321ba9ee5afe24f4b0988703c2d87d12`（`go1.27.1.darwin-arm64.tar.gz`，来自 `https://go.dev/dl/?mode=json`） |
| 本地校验和 | 与官方一致（2026-09-28 安装时验证） |
| 来源 URL | `https://dl.google.com/go/go1.27.1.darwin-arm64.tar.gz` |

## GitHub Actions 绿证

| 项 | 值 |
|---|---|
| Workflow | `go` |
| Run ID | **36421008298**（success） |
| Job | `go` / 108923436601（success，30s） |
| Commit | `79de6c080bbc05e6c7619070ef26c5cefa9ea4cd` |
| Run ID（最新） | **36497938863**（success，commit `1ec48ca`：Windows 进程树终止 + SteamCMD 安装器接线） |
| Run ID（离线执行器落地） | `36456593530`（success，commit `dbb7c57`：运维 CLI + M2 离线执行器 + 缺陷修复） |
| URL | <https://github.com/mj8724/gameserver/actions/runs/36421008298> |
| 通过步骤 | gofmt、go vet、go test、go test -race、go build、architecture boundaries |
| Python 基线 workflow | `tests` run 36421008088，对同一 commit success |

**说明**：`go.dev/dl/...` 在本机经重定向下载失败（curl 35 TLS 错误），改用其最终地址 `dl.google.com` 成功；校验和使用 go.dev 官方元数据比对通过。CI annotations 中的 Node.js 20 弃用与“无 go.sum 因此未恢复 module cache”均为非阻塞 warning，所有工作流步骤通过。

## 复现步骤（新机器）

```bash
V=go1.27.1
curl -fL -o /tmp/${V}.darwin-arm64.tar.gz "https://dl.google.com/go/${V}.darwin-arm64.tar.gz"
shasum -a 256 /tmp/${V}.darwin-arm64.tar.gz   # 必须等于上表官方校验和
mkdir -p "$HOME/.local" && tar -C "$HOME/.local" -xzf /tmp/${V}.darwin-arm64.tar.gz
"$HOME/.local/go/bin/go" version               # 期望：go version go1.27.1 darwin/arm64
```

`PATH` 未全局修改；本仓库命令使用绝对路径 `$HOME/.local/go/bin/go`，或自行 `export PATH="$HOME/.local/go/bin:$PATH"`。

## 已验证的命令（本机 go1.27.1）

| 命令 | 用途 |
|---|---|
| `gofmt -l cmd internal` | 格式检查（必须无输出；只扫描本模块目录，不扫 `research_repos/`） |
| `go vet ./...` | 静态检查 |
| `go test ./...` | 单元测试 |
| `go test -race ./...` | 并发竞态检查（CI 必跑环境 = ubuntu-latest） |
| `go build ./...` | 构建 |

**race 检查的例外规则**：仅在 CI 基础设施故障时允许降级，且必须提交失败/重试日志并由 reviewer 复核；本机不支持 race 不构成例外。

## 架构边界检查

- `internal/archtest` 已加入 `.github/workflows/go.yml` 的 `architecture boundaries` step：`go test ./internal/archtest/...`。
- 检查使用 `go list -e -json -test ./...`，并同时校验普通包与 test imports；合成违规 import 的证伪测试通过。详见 `internal/archtest/boundaries_test.go`。

## 已知坑（2026-09-28 实测）

1. **不要令 `toolchain` 与 `go` 指令 patch 版本相同**：实测 `go 1.27.1` + `toolchain go1.27.1` 会被 `go mod tidy` 移除冗余 toolchain 行。当前正确组合是 `go 1.27`（语言基线）+ `toolchain go1.27.1`（精确工具链 pin）；`go mod tidy -diff` 无差异。
2. **`gofmt -l .` 会误报**：仓库内的 `research_repos/` 含多个下游参考模块（各自有 `go.mod`），其中少数文件未格式化。格式检查与质量口径限定为 `cmd internal`。
3. **`go.dev/dl/...` 直链在本机可能 TLS 失败**（curl 35）；改用最终地址 `dl.google.com/go/...` 可正常下载，校验和仍以 go.dev 官方元数据为准。
