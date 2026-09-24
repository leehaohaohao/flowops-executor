<#
FlowOps Executor 启动脚本主逻辑（Windows）

设计依据: docs/2026-09-24-unified-startup-script-plan.md (v7)
由根目录薄入口 start.bat 调用；本脚本全程不调用任何 Go 工具链。

Windows 当前仅正式支持 native 模式；执行 docker 模式会明确拒绝（exit 3）。
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------- 常量

$DefaultAppEnv = 'prod'
$NativeBinRel = 'bin\windows-amd64\flowops-executor.exe'

# 退出码（计划 §D19）
$EXIT_OK = 0
$EXIT_USAGE = 1
$EXIT_PLATFORM = 2
$EXIT_MODE = 3
$EXIT_NO_DOCKER_CLI = 4
$EXIT_DOCKER_DAEMON = 5
$EXIT_BIN_MISSING = 6
$EXIT_ENTRY_MISSING = 7
$EXIT_CONTAINER_FAIL = 8

# ---------------------------------------------------------------- 日志

function Write-Info { param([string]$Message) Write-Host "[INFO] $Message" }
function Write-Warn { param([string]$Message) Write-Host "[WARN] $Message" }
function Write-Err { param([string]$Message) Write-Host "[ERROR] $Message" -ForegroundColor Red }

function Show-Usage {
    Write-Host @'
FlowOps Executor 启动脚本（Windows）

用法:
  start.bat [native] [选项] [executor 参数...]

模式:
  native   在宿主机前台运行 bin\windows-amd64\flowops-executor.exe（默认模式）
  docker   当前 Windows 版本不支持（exit 3）

选项:
  --check      只检查运行环境，不启动 executor、不修改文件
  -h, --help   显示本帮助

环境变量:
  APP_ENV      prod | dev（默认 prod）

说明:
  模式只能作为第一个位置参数；脚本选项需写在 executor 参数之前；
  遇到第一个未知参数后，其余参数原样透传给 executor。

退出码:
  0 成功                        1 参数 / APP_ENV / 目录错误
  2 平台不支持                  3 运行模式非法
  6 native 二进制缺失或不可执行
'@
}

# ---------------------------------------------------------------- 参数解析

$Mode = ''
$ModeExplicit = $false
$InvalidMode = ''
$Check = $false
$ExecutorArgs = @()

$remaining = New-Object System.Collections.Generic.List[string]
foreach ($a in $args) { $remaining.Add([string]$a) }

# 模式仅作为第一个位置参数：
# - native/docker → 显式模式
# - 脚本选项（--check/-h/--help）→ 模式用平台默认
# - 其它以 - 开头 → 未知选项，后续按模式透传
# - 其它不带 - 的值 → 非法模式（计划 §D4，在 [3/4] 处 exit 3）
if ($remaining.Count -gt 0) {
    $first = $remaining[0]
    if ($first -eq 'native' -or $first -eq 'docker') {
        $Mode = $first
        $ModeExplicit = $true
        $remaining.RemoveAt(0)
    }
    elseif ($first -eq '--check' -or $first -eq '-h' -or $first -eq '--help' -or $first -eq '--foreground') {
        # 脚本选项在前，模式用默认
    }
    elseif ($first.StartsWith('-')) {
        # 未知选项，交由后续按模式处理
    }
    else {
        $InvalidMode = $first
    }
}

# ---------------------------------------------------------------- 早期 help 预检

# 脚本选项区内出现 -h/--help 即显示用法（不依赖部署目录与平台）；
# 遇到第一个未知参数即停止扫描（其后参数归 executor）
$probe = @($remaining)
foreach ($a in $probe) {
    if ($a -eq '-h' -or $a -eq '--help') { Show-Usage; exit $EXIT_OK }
    if ($a -eq '--check' -or $a -eq '--foreground') { continue }
    break
}

# ---------------------------------------------------------------- [1/4] 定位部署目录

$Root = Split-Path -Parent $PSScriptRoot
if (-not (Test-Path (Join-Path $Root 'scripts') -PathType Container) -or
    -not (Test-Path (Join-Path $Root 'start.bat') -PathType Leaf)) {
    Write-Err "[1/4] 无法定位 FlowOps Executor 部署目录"
    Write-Err "期望目录包含: scripts\ 与 start.bat（推导路径: $Root）"
    exit $EXIT_USAGE
}
Write-Info "[1/4] 部署目录: $Root"

# ---------------------------------------------------------------- [2/4] 平台检测

$arch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) { $arch = $env:PROCESSOR_ARCHITEW6432 }
if ($arch -ne 'AMD64') {
    Write-Err "[2/4] 不支持的平台: windows/$arch"
    Write-Err "当前支持: Windows amd64 / Linux amd64"
    exit $EXIT_PLATFORM
}
Write-Info "[2/4] 平台: windows/amd64"

# ---------------------------------------------------------------- [3/4] 运行模式

if ($InvalidMode) {
    Write-Err "[3/4] 非法运行模式: $InvalidMode"
    Write-Err "Windows 可选: native"
    exit $EXIT_MODE
}

if ($ModeExplicit) {
    Write-Info "[3/4] 运行模式: $Mode（显式）"
}
else {
    $Mode = 'native'
    Write-Info "[3/4] 未指定运行模式，平台默认模式: native"
}

if ($Mode -eq 'docker') {
    Write-Err "[3/4] 当前版本 Windows 暂不支持 docker 模式"
    Write-Err "支持模式: native（Linux 上可使用 docker 模式）"
    exit $EXIT_MODE
}
if ($Mode -ne 'native') {
    Write-Err "[3/4] 非法运行模式: $Mode"
    Write-Err "Windows 可选: native"
    exit $EXIT_MODE
}

# 脚本选项（需写在 executor 参数之前）
while ($remaining.Count -gt 0) {
    $a = $remaining[0]
    if ($a -eq '--check') { $Check = $true; $remaining.RemoveAt(0); continue }
    if ($a -eq '-h' -or $a -eq '--help') { Show-Usage; exit $EXIT_OK }
    # 未知参数：其余全部原样透传给 executor
    $ExecutorArgs = $remaining.ToArray()
    break
}

Write-Info "[4/4] 进入 native 模式"

# ---------------------------------------------------------------- native 模式

# [native 1/3] 二进制存在性
$bin = Join-Path $Root $NativeBinRel
if (-not (Test-Path $bin -PathType Leaf)) {
    Write-Err "[native 1/3] 未找到 executor: $bin"
    Write-Err "请确认部署包包含当前平台二进制（bin\windows-amd64\flowops-executor.exe）"
    exit $EXIT_BIN_MISSING
}
Write-Info "[native 1/3] executor: $bin"

# [native 2/3] APP_ENV
if (-not $env:APP_ENV) { $env:APP_ENV = $DefaultAppEnv }
if ($env:APP_ENV -ne 'prod' -and $env:APP_ENV -ne 'dev') {
    Write-Err "[native 2/3] APP_ENV 非法: $($env:APP_ENV)"
    Write-Err "支持值: prod / dev"
    exit $EXIT_USAGE
}
Write-Info "[native 2/3] APP_ENV: $($env:APP_ENV)"

if ($Check) {
    Write-Info "[native 3/3] --check 完成：环境满足，未启动 executor"
    exit $EXIT_OK
}

# [native 3/3] 前台启动；$LASTEXITCODE 为 executor 退出码，原样透传
Write-Info "[native 3/3] 启动 executor（前台）"
if ($ExecutorArgs.Count -gt 0) {
    & $bin @ExecutorArgs
}
else {
    & $bin
}
exit $LASTEXITCODE
