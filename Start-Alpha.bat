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
  echo Created config.json from config.example.json.
  echo Edit config.json and enable only an engine you have independently tested.
  echo After editing config.json, run Start-Alpha.bat again.
  pause
  exit /b 0
)
"%ALPHA_EXE%" ui --config config.json
if errorlevel 1 (
  echo Alpha exited with an error. Check config.json and the console above.
  pause
)
endlocal
