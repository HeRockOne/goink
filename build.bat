@echo off
echo === Goink Build + Deploy ===
cd /d "%~dp0"
set PATH=C:\Program Files\Go\bin;C:\msys64\mingw64\bin;%PATH%
set CGO_ENABLED=1

echo [1/3] Building...
wails build -tags webkit2_41 -o build\bin\goink.exe
if %ERRORLEVEL% NEQ 0 (echo BUILD FAILED & pause & exit /b 1)

echo [2/3] Stopping goink...
taskkill /f /im goink.exe >nul 2>&1
timeout /t 2 /nobreak >nul

echo [3/3] Copying to D:\Goink\...
if exist "build\bin\build\bin\goink.exe" (
  copy /y "build\bin\build\bin\goink.exe" "D:\Goink\goink.exe" >nul
) else (
  copy /y "build\bin\goink.exe" "D:\Goink\goink.exe" >nul
)
start "" "D:\Goink\goink.exe"
echo === Done! ===
timeout /t 2 /nobreak >nul
