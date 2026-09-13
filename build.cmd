@echo off
setlocal EnableExtensions
cd /d "%~dp0"

set "OUTPUT_DIR=%~dp0dist"
set "REPOSITORY=%VIEWER_LAUNCHER_REPOSITORY%"
set "BUILD_VERSION=%VIEWER_LAUNCHER_VERSION%"

if not defined REPOSITORY set "REPOSITORY=AntaresTechno/Viewer-Launcher"
if not defined BUILD_VERSION set "BUILD_VERSION=dev"

if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"
if errorlevel 1 goto :failed

echo [1/5] Formatting Go sources...
go fmt ./...
if errorlevel 1 goto :failed

echo [2/5] Running desktop tests...
go test ./...
if errorlevel 1 goto :failed

echo [3/5] Running CLI tests...
go test -tags cli ./...
if errorlevel 1 goto :failed

set "LINK_FLAGS=-s -w -X main.repository=%REPOSITORY% -X main.version=%BUILD_VERSION% -X main.releaseTag=preview"

echo [4/5] Building desktop app...
go build -trimpath -ldflags "%LINK_FLAGS%" -o "%OUTPUT_DIR%\viewer-launcher.exe" .
if errorlevel 1 goto :failed

echo [5/5] Building CLI app...
go build -tags cli -trimpath -ldflags "%LINK_FLAGS%" -o "%OUTPUT_DIR%\viewer-launcher-cli.exe" .
if errorlevel 1 goto :failed

echo.
echo Build completed:
echo   %OUTPUT_DIR%\viewer-launcher.exe
echo   %OUTPUT_DIR%\viewer-launcher-cli.exe
exit /b 0

:failed
echo.
echo Build failed with exit code %errorlevel%.
exit /b %errorlevel%
