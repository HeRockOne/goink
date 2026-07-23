# Goink Build + Deploy Script
Write-Host "=== Goink Build + Deploy ===" -ForegroundColor Cyan

# Step 1: Build
Write-Host "[1/3] Building..." -ForegroundColor Yellow
$env:PATH = "C:\Program Files\Go\bin;C:\msys64\mingw64\bin;$env:PATH"
$env:CGO_ENABLED = "1"
$env:GOPROXY = "https://goproxy.cn,direct"
$env:CGO_CFLAGS = "-I$env:USERPROFILE\go\pkg\mod\github.com\mattn\go-sqlite3@v1.14.44"
Set-Location "$PSScriptRoot"
wails build -tags webkit2_41 -o build\bin\goink.exe 2>&1 | Select-String -NotMatch "KnownStructs|Not found"
if ($LASTEXITCODE -ne 0) { Write-Host "BUILD FAILED" -ForegroundColor Red; exit 1 }

# Step 2: Stop
Write-Host "[2/3] Stopping goink..." -ForegroundColor Yellow
Get-Process goink -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 2

# Step 3: Deploy
Write-Host "[3/3] Copying to D:\Goink\..." -ForegroundColor Yellow
$src = "build\bin\build\bin\goink.exe"
if (-not (Test-Path $src)) { $src = "build\bin\goink.exe" }
Copy-Item $src "D:\Goink\goink.exe" -Force
Start-Process "D:\Goink\goink.exe" -WindowStyle Minimized

Write-Host "=== Done! ===" -ForegroundColor Green
Start-Sleep -Seconds 2
