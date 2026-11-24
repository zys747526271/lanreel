# 一键构建（Windows PowerShell）：前端 → 内嵌资源 → 单二进制 lanreel.exe
#
# 用法：.\scripts\build.ps1 [-Version 0.1.0]
param(
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

function Require-Command($name, $hint) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        throw "未找到 $name，请安装 $hint"
    }
}

Require-Command node "Node.js 20+"
Require-Command go "Go 1.25+"

Write-Host "==> [1/4] 安装前端依赖" -ForegroundColor Cyan
Push-Location web
npm install --no-audit --no-fund
if ($LastExitCode -ne 0) { throw "npm install 失败" }

Write-Host "==> [2/4] 构建前端" -ForegroundColor Cyan
npm run build
if ($LastExitCode -ne 0) { throw "npm run build 失败" }
Pop-Location

Write-Host "==> [3/4] 同步前端产物到内嵌目录" -ForegroundColor Cyan
$embed = "server/internal/webui/dist"
if (Test-Path "$embed/assets") { Remove-Item "$embed/assets" -Recurse -Force }
if (Test-Path "$embed/index.html") { Remove-Item "$embed/index.html" -Force }
Copy-Item "web/dist/*" -Destination $embed -Recurse -Force

Write-Host "==> [4/4] 编译后端（版本 $Version）" -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path dist | Out-Null
Push-Location server
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w -X main.version=$Version" -o "../dist/lanreel.exe" ./cmd/lanreel
if ($LastExitCode -ne 0) { throw "go build 失败" }
Pop-Location

$size = [math]::Round((Get-Item "dist/lanreel.exe").Length / 1MB, 1)
Write-Host ""
Write-Host "构建完成：dist/lanreel.exe（$size MB，含内嵌前端）" -ForegroundColor Green
Write-Host "启动：.\dist\lanreel.exe --config config.yaml"
Write-Host "首次访问：按网页引导创建管理员（无浏览器环境可设 ADMIN_INIT_PASSWORD 预置）"
