@echo off
setlocal
where go >nul 2>nul
if errorlevel 1 (
  echo Go 1.22 or later is required. Install it from https://go.dev/dl/
  exit /b 1
)
if not exist dist mkdir dist
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags="-s -w" -o dist\alpha-windows-amd64.exe .\cmd\resilience
if errorlevel 1 exit /b 1
echo Built dist\alpha-windows-amd64.exe
