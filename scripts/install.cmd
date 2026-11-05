@echo off
rem Keep installation in install.sh; this entry only locates Git Bash or Go.
setlocal DisableDelayedExpansion
pushd "%~dp0.." || exit /b 1
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
where.exe /q go.exe
if errorlevel 1 (
    echo herdr-auto-title: install Git for Windows or Go 1.24 or newer >&2
    set "result=1"
    goto done
)
echo herdr-auto-title: Git Bash is unavailable; building from source >&2
go.exe build -o herdr-auto-title.exe ./cmd/herdr-auto-title
set "result=%errorlevel%"
if not "%result%"=="0" echo herdr-auto-title: it could not be built from source, which needs Go 1.24 or newer >&2
goto done

:shell
"%bash%" --noprofile --norc -c "export PATH=/usr/bin:$PATH; exec /usr/bin/sh scripts/install.sh"
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
