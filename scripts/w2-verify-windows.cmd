@echo off
setlocal
cd /d "%~dp0"
echo W2E - AINovel WEB-ONLY Browser Verification
echo.
ainovel-w2-verify.exe
set EXIT_CODE=%ERRORLEVEL%
echo.
if not "%EXIT_CODE%"=="0" echo W2E did not pass. Evidence was written locally when possible.
echo Press any key to close this window.
pause >nul
exit /b %EXIT_CODE%
