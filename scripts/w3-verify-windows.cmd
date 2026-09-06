@echo off
setlocal
cd /d "%~dp0"
echo W3 - AINovel WEB-ONLY Real Gemini Prompt Verification
echo.
echo This uses the latest passed W2 browser profile and sends ONE verification prompt through Gemini Web.
echo No AI API or API key is used.
echo.
ainovel-w3-verify.exe
set EXIT_CODE=%ERRORLEVEL%
echo.
if not "%EXIT_CODE%"=="0" echo W3 did not pass. Check the message above.
echo Press any key to close this window.
pause >nul
exit /b %EXIT_CODE%
