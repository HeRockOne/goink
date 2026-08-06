# Goink Build + Deploy Script
Write-Host "=== Goink Build + Deploy ===" -ForegroundColor Cyan

$ts = Get-Date -Format "yyyyMMdd-HH-mm"
$exeName = "goink-$ts.exe"

# Step 1: Build
Write-Host "[1/3] Building $exeName ..." -ForegroundColor Yellow
$env:PATH = "C:\Program Files\Go\bin;C:\msys64\mingw64\bin;$env:PATH"
$env:CGO_ENABLED = "1"
$env:GOPROXY = "https://goproxy.cn,direct"
$sqliteVer = (Select-String -Path "$PSScriptRoot\go.mod" -Pattern "mattn/go-sqlite3\s+(v[\d.]+)").Matches.Groups[1].Value
if (-not $sqliteVer) { Write-Host "go.mod 中未找到 mattn/go-sqlite3 版本" -ForegroundColor Red; exit 1 }
$env:CGO_CFLAGS = "-I$(go env GOMODCACHE)\github.com\mattn\go-sqlite3@$sqliteVer"
Set-Location "$PSScriptRoot"
wails build -tags webkit2_41 -o $exeName 2>&1 | Select-String -NotMatch "KnownStructs|Not found"
if ($LASTEXITCODE -ne 0) { Write-Host "BUILD FAILED" -ForegroundColor Red; exit 1 }

# Step 2: Stop
Write-Host "[2/3] Stopping goink..." -ForegroundColor Yellow
Get-Process goink* -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 2

# Step 3: Deploy
Write-Host "[3/3] Copying to D:\Goink\..." -ForegroundColor Yellow
$src = "build\bin\$exeName"
Copy-Item $src "D:\Goink\$exeName" -Force
Copy-Item $src "D:\Goink\goink.exe" -Force
Start-Process "D:\Goink\goink.exe" -WindowStyle Minimized

Write-Host "=== Done! $exeName ===" -ForegroundColor Green
Start-Sleep -Seconds 2
