@echo off
echo === FlowOps Executor Build Script ===

echo.
echo [1/2] Building for Windows...
set GOOS=windows&& set GOARCH=amd64&& go build -o flowops-executor.exe .
if %errorlevel% neq 0 (
    echo Windows build failed!
    pause
    exit /b 1
)
echo Done: flowops-executor.exe

echo.
echo [2/2] Building for Linux...
set GOOS=linux&& set GOARCH=amd64&& go build -o flowops-executor .
if %errorlevel% neq 0 (
    echo Linux build failed!
    pause
    exit /b 1
)
echo Done: flowops-executor

echo.
echo === Build Complete ===
pause
