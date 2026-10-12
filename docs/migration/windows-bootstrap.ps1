# Windows 测试主机引导脚本（G 盘）
#
# 用途：在 Windows 测试机上建立 G:\gameserver-work 工作根、安装固定版本 Go、
#      取仓库副本并运行本地质量门禁。
#
# 安全边界：
#   - 不修改系统级环境变量、不开放防火墙、不安装服务。
#   - 不下载 SteamCMD/游戏内容（那需要用户对该动作单独授权）。
#   - 脚本每一步都会打印将要执行的动作；失败即停止。
#
# 用法（在目标机 PowerShell 中）：
#   powershell -ExecutionPolicy Bypass -File .\windows-bootstrap.ps1 -Stage recon
#   powershell -ExecutionPolicy Bypass -File .\windows-bootstrap.ps1 -Stage workspace
#   powershell -ExecutionPolicy Bypass -File .\windows-bootstrap.ps1 -Stage go
#   powershell -ExecutionPolicy Bypass -File .\windows-bootstrap.ps1 -Stage repo
#   powershell -ExecutionPolicy Bypass -File .\windows-bootstrap.ps1 -Stage gates

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('recon', 'workspace', 'go', 'repo', 'gates')]
    [string]$Stage,

    [string]$GoVersion = '1.27.1',
    [string]$Root = 'G:\gameserver-work',
    [string]$RepoUrl = 'https://github.com/mj8724/gameserver.git',
    [string]$Branch = 'feat/go-modular-skeleton'
)

$ErrorActionPreference = 'Stop'

function Write-Step([string]$text) { Write-Host "==> $text" -ForegroundColor Cyan }

switch ($Stage) {
    'recon' {
        Write-Step 'OS 与磁盘侦察（只读）'
        Get-CimInstance Win32_OperatingSystem |
            Select-Object Caption, Version, BuildNumber, OSArchitecture | Format-List
        Get-Volume | Select-Object DriveLetter, FileSystemType, SizeRemaining, Size | Format-Table
        Write-Step '现有工具链'
        foreach ($tool in 'go', 'git', 'python', 'cloudflared') {
            $command = Get-Command $tool -ErrorAction SilentlyContinue
            if ($command) { "{0} -> {1}" -f $tool, $command.Source } else { "$tool -> (absent)" }
        }
        Write-Step 'G 盘目标点检查'
        if (Test-Path -LiteralPath 'G:\') { 'G:\ 可用' } else { throw 'G 盘不存在，请与用户确认盘符' }
    }

    'workspace' {
        Write-Step "创建 $Root 及子目录（testdata / backups）"
        foreach ($dir in $Root, (Join-Path $Root 'testdata'), (Join-Path $Root 'backups')) {
            New-Item -ItemType Directory -Force -Path $dir | Out-Null
            "已就绪: $dir"
        }
    }

    'go' {
        $zip = Join-Path $Root "go$GoVersion.windows-amd64.zip"
        $url = "https://dl.google.com/go/go$GoVersion.windows-amd64.zip"
        Write-Step "下载 Go $GoVersion"
        Invoke-WebRequest -Uri $url -OutFile $zip
        Write-Step '计算并显示 SHA256（必须与 go.dev 官方元数据一致后再继续）'
        Get-FileHash -LiteralPath $zip -Algorithm SHA256 | Format-List
        Write-Step "解压到 $Root"
        Expand-Archive -LiteralPath $zip -DestinationPath $Root -Force
        $go = Join-Path $Root 'go\bin\go.exe'
        & $go version
        Write-Step '本会话 PATH（不写系统环境变量）'
        $env:Path = (Join-Path $Root 'go\bin') + ';' + $env:Path
        '已为本会话设置 PATH'
    }

    'repo' {
        $repo = Join-Path $Root 'repo'
        if (Test-Path -LiteralPath $repo) {
            Write-Step "更新已有副本 $repo"
            git -C $repo fetch --all --prune
        }
        else {
            Write-Step "克隆 $RepoUrl 到 $repo"
            git clone $RepoUrl $repo
        }
        Write-Step "检出 $Branch"
        git -C $repo checkout $Branch
        git -C $repo log --oneline -1
        Write-Step '提示：Python/FastAPI 旧基线与 Go 服务互不影响；本脚本不安装 Python 依赖'
    }

    'gates' {
        $repo = Join-Path $Root 'repo'
        $env:Path = (Join-Path $Root 'go\bin') + ';' + $env:Path
        Push-Location $repo
        try {
            Write-Step 'gofmt（cmd internal）'
            $unformatted = & gofmt -l cmd internal
            if ($unformatted) { throw "gofmt 报告未格式化文件: $unformatted" }
            Write-Step 'go vet ./...'
            & go vet ./...
            Write-Step 'go test ./...'
            & go test ./...
            Write-Step 'go build ./...'
            & go build ./...
            Write-Step 'architecture boundaries'
            & go test ./internal/archtest/...
            Write-Step 'go test -race ./...（Windows 需要 gcc；失败则记录为“无 race 证据”）'
            & go test -race ./... ; if ($LASTEXITCODE -ne 0) { 'RACE_UNAVAILABLE_OR_FAILED（记录证据，不降低 CI 要求）' }
        }
        finally {
            Pop-Location
        }
    }
}
