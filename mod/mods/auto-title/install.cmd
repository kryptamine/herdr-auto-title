@echo off
rem Prefer the shared shell installer; PowerShell is only for machines without Git Bash.
setlocal DisableDelayedExpansion
pushd "%~dp0" || exit /b 1
set "bash="
set "BASH_ENV="
set "ENV="
for /f "delims=" %%G in ('where.exe git.exe 2^>nul') do (
    call :find_bash "%%~dpG.."
    call :find_bash "%%~dpG..\.."
)
if defined bash goto shell
for /f "delims=" %%G in ('git.exe --exec-path 2^>nul') do call :find_bash "%%G\..\..\.."
if defined bash goto shell
echo Git Bash is unavailable; using PowerShell.
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File install.ps1 "%~1" "%~2"
set "result=%errorlevel%"
if "%result%"=="0" goto done
if exist "%~2\status" goto done
>"%~2\status.tmp" echo 1
>>"%~2\status.tmp" echo Git Bash is unavailable and PowerShell could not run the installer. See the install tab for the policy or startup error.
move /y "%~2\status.tmp" "%~2\status" >nul
echo PowerShell could not run the installer. Its error is shown above.
pause
if defined HC_TERMINAL_ID hc kill "%HC_TERMINAL_ID%" >nul 2>&1
goto done

:shell
echo Using Git Bash.
for %%B in ("%bash%") do set "PATH=%%~dpB..\usr\bin;%PATH%"
"%bash%" --noprofile --norc install.sh "%~1" "%~2"
set "result=%errorlevel%"
:done
popd
exit /b %result%

:find_bash
if defined bash exit /b
if not exist "%~1\bin\bash.exe" exit /b
if not exist "%~1\usr\bin\uname.exe" exit /b
"%~1\bin\bash.exe" --noprofile --norc -c "case $(/usr/bin/uname -s) in MINGW*_NT-*|MSYS_NT-*) exit 0;; *) exit 1;; esac" >nul 2>&1
if not errorlevel 1 set "bash=%~1\bin\bash.exe"
exit /b
