@echo off
setlocal
cd /d "%~dp0"
echo W2E - AINovel WEB-ONLY Browser Verification
echo.
echo The verifier first checks AUTH_REQUIRED in read-only inspection mode.
echo DO NOT sign in during that first inspection window.
echo It will then reopen the same profile in NORMAL Chrome without DevTools.
echo Sign in there. When Gemini is fully signed in, CLOSE that Chrome window.
echo The verifier will continue automatically and verify READY + restart persistence.
echo.
ainovel-w2-verify.exe
set EXIT_CODE=%ERRORLEVEL%
echo.
if not "%EXIT_CODE%"=="0" echo W2E did not pass. Evidence was written locally when possible.
echo Press any key to close this window.
pause >nul
exit /b %EXIT_CODE%
