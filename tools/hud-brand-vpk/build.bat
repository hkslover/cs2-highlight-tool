@echo off
setlocal
cd /d "%~dp0"
if exist ".venv\Scripts\python.exe" (
    set "PYTHON=.venv\Scripts\python.exe"
    goto dependencies
)
py -3 --version >nul 2>&1
if errorlevel 1 (
    set "PYTHON=python"
) else (
    set "PYTHON=py -3"
)
%PYTHON% -c "import PIL" >nul 2>&1
if not errorlevel 1 goto dependencies
%PYTHON% -m venv .venv
if errorlevel 1 goto failed
set "PYTHON=.venv\Scripts\python.exe"

:dependencies
%PYTHON% -c "import PIL" >nul 2>&1
if not errorlevel 1 goto build
%PYTHON% -m pip install -r requirements.txt
if errorlevel 1 goto failed

:build
%PYTHON% build_vpk.py %*
if errorlevel 1 goto failed
pause
exit /b 0

:failed
echo Build failed. Check Python installation and the error above.
pause
exit /b 1
