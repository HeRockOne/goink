# Goink Build + Deploy Script
Write-Host "=== Goink Build + Deploy ===" -ForegroundColor Cyan

# Step 1: Build
Write-Host "[1/3] Building..." -ForegroundColor Yellow
$env:PATH = "C:\Program Files\Go\bin;C:\msys64\mingw64\bin;$env:PATH"
$env:CGO_ENABLED = "1"
$env:GOPROXY = "https://goproxy.cn,direct"
# sqlite-vec 的 C 代码 include "sqlite3.h"，头文件由 mattn/go-sqlite3 包自带。
# 动态从 go.mod 解析版本 + GOMODCACHE，避免硬编码路径/版本在升级后失效。
$sqliteVer = (Select-String -Path "$PSScriptRoot\go.mod" -Pattern "mattn/go-sqlite3\s+(v[\d.]+)").Matches.Groups[1].Value
if (-not $sqliteVer) { Write-Host "go.mod 中未找到 mattn/go-sqlite3 版本" -ForegroundColor Red; exit 1 }
$env:CGO_CFLAGS = "-I$(go env GOMODCACHE)\github.com\mattn\go-sqlite3@$sqliteVer"
Set-Location "$PSScriptRoot"
wails build -tags webkit2_41 -o goink.exe 2>&1 | Select-String -NotMatch "KnownStructs|Not found"
if ($LASTEXITCODE -ne 0) { Write-Host "BUILD FAILED" -ForegroundColor Red; exit 1 }

# Step 2: Stop
Write-Host "[2/3] Stopping goink..." -ForegroundColor Yellow
Get-Process goink -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 2

# Step 3: Deploy
Write-Host "[3/3] Copying to D:\Goink\..." -ForegroundColor Yellow
$src = "build\bin\goink.exe"
if (-not (Test-Path $src)) { $src = "build\bin\goink.exe" }
Copy-Item $src "D:\Goink\goink.exe" -Force
Start-Process "D:\Goink\goink.exe" -WindowStyle Minimized

Write-Host "=== Done! ===" -ForegroundColor Green
Start-Sleep -Seconds 2
