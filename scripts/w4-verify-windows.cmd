@echo off
setlocal
cd /d "%~dp0"
echo W4 - AINovel WEB-ONLY Real Gemini Local Tool-Call E2E Verification
echo.
echo This uses the latest passed W3 browser profile.
echo Gemini Web must request one local verification Tool; ainovel executes it locally and returns the result to Gemini.
echo No AI API or API key is used. The web page never executes the local Tool directly.
echo.
ainovel-w4-verify.exe
set EXIT_CODE=%ERRORLEVEL%
echo.
if not "%EXIT_CODE%"=="0" echo W4 did not pass. Check the message above.
echo Press any key to close this window.
pause >nul
exit /b %EXIT_CODE%
