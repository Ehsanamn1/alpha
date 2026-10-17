@echo off
setlocal
cd /d "%~dp0"
set ALPHA_EXE=alpha-windows-amd64.exe
if not exist "%ALPHA_EXE%" set ALPHA_EXE=dist\alpha-windows-amd64.exe
if not exist "%ALPHA_EXE%" (
  echo Alpha executable was not found. Run build_windows.bat or download the Windows package.
  pause
  exit /b 2
)
if not exist config.json (
  copy /Y config.example.json config.json >nul
  echo Created config.json. Review every command and argument before enabling process launch.
  pause
  exit /b 2
)
echo WARNING: Alpha will launch ONLY processes explicitly marked start_process=true in config.json.
echo Review command paths and arguments before continuing.
choice /M "Launch the configured processes"
if errorlevel 2 exit /b 0
"%ALPHA_EXE%" ui --config config.json --allow-process-launch
if errorlevel 1 (
  echo Alpha exited with an error. Check config.json and the console above.
  pause
)
endlocal
