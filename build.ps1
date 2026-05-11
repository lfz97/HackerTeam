# 交叉编译脚本 - 在 PowerShell 中运行
$ErrorActionPreference = "Stop"

$OutputDir = "release"
$LDFLAGS = "-s -w"

Write-Host "开始交叉编译..." -ForegroundColor Green

# linux-arm64
Write-Host "构建 linux-arm64..." -ForegroundColor Yellow
$env:GOOS = "linux"
$env:GOARCH = "arm64"
go build -ldflags $LDFLAGS -o "$OutputDir/linux-arm64/HackerTeam"
if ($LASTEXITCODE -ne 0) { exit 1 }

# linux-x64
Write-Host "构建 linux-x64..." -ForegroundColor Yellow
$env:GOARCH = "amd64"
go build -ldflags $LDFLAGS -o "$OutputDir/linux-x64/HackerTeam"
if ($LASTEXITCODE -ne 0) { exit 1 }

# macos-arm64
Write-Host "构建 macos-arm64..." -ForegroundColor Yellow
$env:GOOS = "darwin"
$env:GOARCH = "arm64"
go build -ldflags $LDFLAGS -o "$OutputDir/macos-arm64/HackerTeam"
if ($LASTEXITCODE -ne 0) { exit 1 }

# macos-x64
Write-Host "构建 macos-x64..." -ForegroundColor Yellow
$env:GOARCH = "amd64"
go build -ldflags $LDFLAGS -o "$OutputDir/macos-x64/HackerTeam"
if ($LASTEXITCODE -ne 0) { exit 1 }

# windows-x64
Write-Host "构建 windows-x64..." -ForegroundColor Yellow
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags $LDFLAGS -o "$OutputDir/windows-x64/HackerTeam.exe"
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "所有平台构建完成!" -ForegroundColor Green

# 验证构建结果
Write-Host "`n构建产物:" -ForegroundColor Cyan
Get-ChildItem -Path $OutputDir -Recurse -File | Select-Object FullName, Length
