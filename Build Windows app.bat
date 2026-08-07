@echo off
rem ===========================================================
rem  Sablewright - build the Windows app
rem
rem  Double-click this. It checks for Go, installs the Wails
rem  build tool, compiles the app, and puts the finished .exe
rem  on your Desktop.
rem
rem  First run takes a few minutes (it downloads Go packages).
rem  Later runs take seconds.
rem ===========================================================
title Sablewright - build
setlocal enabledelayedexpansion
set "HERE=%~dp0"
if "%HERE:~-1%"=="\" set "HERE=%HERE:~0,-1%"
cd /d "%HERE%"

echo.
echo   Sablewright - Windows build
echo   =====================================
echo.

if not exist "%HERE%\main.go" (
    echo   [X] This script must sit in the project folder, next to main.go.
    echo.
    pause
    exit /b 1
)

rem ---------------------------------------------------------------
rem  1. Go
rem     Verified by actually running it, not by trusting a path.
rem ---------------------------------------------------------------
echo   [1/4] Checking for Go...
set "GOEXE="
for /f "delims=" %%I in ('where go.exe 2^>nul') do (
    if not defined GOEXE call :trygo "%%I"
)
if not defined GOEXE if exist "%ProgramFiles%\Go\bin\go.exe" call :trygo "%ProgramFiles%\Go\bin\go.exe"
if not defined GOEXE if exist "%LOCALAPPDATA%\Programs\Go\bin\go.exe" call :trygo "%LOCALAPPDATA%\Programs\Go\bin\go.exe"

if defined GOEXE goto :gofound

echo         Go isn't installed yet.
echo.
where winget >nul 2>&1
if errorlevel 1 goto :manualgo

echo         Installing it with winget (this may take a few minutes)...
echo         A Windows permission prompt may appear - please accept it.
winget install --id GoLang.Go --silent --accept-source-agreements --accept-package-agreements
echo.
echo         Re-checking...
for /f "delims=" %%I in ('where go.exe 2^>nul') do (
    if not defined GOEXE call :trygo "%%I"
)
if not defined GOEXE if exist "%ProgramFiles%\Go\bin\go.exe" call :trygo "%ProgramFiles%\Go\bin\go.exe"
if defined GOEXE goto :gofound

echo.
echo   [!] Go was installed but this window can't see it yet, because
echo       Windows only refreshes PATH for new windows.
echo.
echo       Close this window and double-click this script again.
echo.
pause
exit /b 0

:manualgo
echo   [X] Go is needed to build the app, and winget isn't available
echo       to install it automatically.
echo.
echo       1. Install Go from the page about to open (take the MSI).
echo       2. Close this window, then run this script again.
echo.
echo       That's the only prerequisite - no Visual Studio, no C compiler.
echo.
start "" https://go.dev/dl/
pause
exit /b 1

:gofound
for /f "tokens=3" %%V in ('"%GOEXE%" version') do set "GOVER=%%V"
echo         Found Go %GOVER%
echo         at %GOEXE%

rem ---------------------------------------------------------------
rem  2. Wails build tool
rem ---------------------------------------------------------------
echo   [2/4] Installing the Wails build tool...

rem work out where `go install` puts binaries
set "GOBIN="
for /f "delims=" %%I in ('"%GOEXE%" env GOBIN 2^>nul') do set "GOBIN=%%I"
if not defined GOBIN (
    for /f "delims=" %%I in ('"%GOEXE%" env GOPATH 2^>nul') do set "GOBIN=%%I\bin"
)
set "PATH=%GOBIN%;%PATH%"

if exist "%GOBIN%\wails.exe" (
    echo         Already installed.
) else (
    "%GOEXE%" install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
)

if not exist "%GOBIN%\wails.exe" (
    echo.
    echo   [X] The Wails tool didn't install. The usual cause is no internet
    echo       connection, or a proxy blocking downloads.
    echo.
    pause
    exit /b 1
)
echo         Ready.

rem ---------------------------------------------------------------
rem  3. Dependencies
rem ---------------------------------------------------------------
echo   [3/4] Fetching Go packages (slow the first time)...
"%GOEXE%" mod tidy
if errorlevel 1 (
    echo.
    echo   [X] Could not download the Go packages. Check your internet
    echo       connection and try again.
    echo.
    pause
    exit /b 1
)
echo         Done.

rem ---------------------------------------------------------------
rem  4. Build
rem     -webview2 embed bundles the WebView2 bootstrapper, so the app
rem     also works on older Windows 10 machines that lack the runtime.
rem ---------------------------------------------------------------
echo   [4/4] Building the app...

rem     The version comes from wails.json rather than being repeated here, and
rem     the commit is stamped alongside it so a build can say what it came
rem     from. Both are optional - git in particular won't be installed on a
rem     machine that only ever downloads this repo as a zip - and without them
rem     the app simply calls itself a dev build.
set "VERSION="
for /f "delims=" %%V in ('powershell -NoProfile -Command "(Get-Content 'wails.json' -Raw ^| ConvertFrom-Json).info.productVersion" 2^>nul') do set "VERSION=%%V"
set "COMMIT="
for /f "delims=" %%C in ('git rev-parse --short HEAD 2^>nul') do set "COMMIT=%%C"

set "LDF=-w -s"
if defined VERSION set "LDF=%LDF% -X main.version=%VERSION%"
if defined COMMIT set "LDF=%LDF% -X main.commit=%COMMIT%"

echo.
"%GOBIN%\wails.exe" build -platform windows/amd64 -webview2 embed -ldflags "%LDF%"
echo.

set "OUT="
for %%F in ("%HERE%\build\bin\*.exe") do set "OUT=%%~fF"

if not defined OUT (
    echo   [X] The build finished but produced no .exe.
    echo       Scroll up - the compiler error will be just above.
    echo.
    pause
    exit /b 1
)

echo   Built: %OUT%

rem copy it somewhere obvious
copy /y "%OUT%" "%USERPROFILE%\Desktop\Sablewright.exe" >nul 2>&1
if errorlevel 1 (
    echo   Could not copy to the Desktop, but the .exe above works fine
    echo   where it is - drag it wherever you like.
) else (
    echo   Copied to your Desktop as "Sablewright.exe"
)

echo.
echo   ============================================================
echo    Done. It's a single self-contained .exe - no installer,
echo    nothing to set up. Move it anywhere; it'll still work.
echo   ============================================================
echo.
echo   Starting it now...
start "" "%USERPROFILE%\Desktop\Sablewright.exe"
timeout /t 4 >nul
exit /b 0

:trygo
rem a candidate only counts if it actually runs
if defined GOEXE exit /b 0
if not exist "%~1" exit /b 0
for %%A in ("%~1") do if %%~zA EQU 0 exit /b 0
"%~1" version >nul 2>&1
if errorlevel 1 exit /b 0
set "GOEXE=%~1"
exit /b 0
