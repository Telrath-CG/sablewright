@echo off
rem ===========================================================
rem  Sablewright - build the Windows app
rem
rem  Double-click this. It checks for Go, installs the Wails
rem  tool, compiles the app, and puts the finished .exe on your
rem  Desktop.
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
for /f "tokens=3" %%V in ('"%GOEXE%" version') do set "GOVER=%%V"
echo         Found Go %GOVER%
echo         at %GOEXE%

rem Wails v3 needs Go 1.26 or newer. GOVER reads like go1.26.11, so the
rem second dot-separated piece is the minor version.
set "GOMINOR="
for /f "tokens=2 delims=." %%M in ("%GOVER%") do set "GOMINOR=%%M"
if not defined GOMINOR goto :oldgo
if %GOMINOR% LSS 26 goto :oldgo
goto :gook

:oldgo
echo.
echo   [X] Sablewright needs Go 1.26 or newer, and this is %GOVER%.
echo       Install the current Go from the page about to open, close this
echo       window, then run this script again.
echo.
start "" https://go.dev/dl/
pause
exit /b 1

:gook

rem ---------------------------------------------------------------
rem  2. Wails tool
rem     The app builds with plain `go build`. The Wails tool is used for
rem     one step: compiling the icon, the file properties and the
rem     manifest into the .exe. The manifest is what lets the confirm
rem     dialogs show their own button labels.
rem ---------------------------------------------------------------
echo   [2/4] Installing the Wails tool...

rem work out where `go install` puts binaries
set "GOBIN="
for /f "delims=" %%I in ('"%GOEXE%" env GOBIN 2^>nul') do set "GOBIN=%%I"
if not defined GOBIN (
    for /f "delims=" %%I in ('"%GOEXE%" env GOPATH 2^>nul') do set "GOBIN=%%I\bin"
)
set "PATH=%GOBIN%;%PATH%"

rem Always installed, even over an existing copy: `go install` of a pinned
rem version is quick when it's already there, and an older one left over
rem from a previous build would otherwise be used without a word - so a
rem failed install stops here rather than falling back on it.
"%GOEXE%" install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
if errorlevel 1 goto :wails_failed
if not exist "%GOBIN%\wails3.exe" goto :wails_failed
goto :wails_ready

:wails_failed
echo.
echo   [X] The Wails tool didn't install. The usual cause is no internet
echo       connection, or a proxy blocking downloads.
echo.
pause
exit /b 1

:wails_ready
echo         Ready.

rem ---------------------------------------------------------------
rem  3. Dependencies
rem     download, not tidy: go.mod and go.sum are committed and say
rem     exactly what to fetch, and a build shouldn't rewrite them.
rem ---------------------------------------------------------------
echo   [3/4] Fetching Go packages (slow the first time)...
"%GOEXE%" mod download
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
rem ---------------------------------------------------------------
echo   [4/4] Building the app...

rem     The version comes from build\config.yml rather than being repeated
rem     here, and the commit is stamped alongside it so a build can say what
rem     it came from. Both are optional - git in particular won't be
rem     installed on a machine that only ever downloads this repo as a zip -
rem     and without them the app simply calls itself a dev build.
rem     PowerShell writes the version to a file rather than being run inside
rem     a for /f, where cmd would have to parse its quotes and brackets too.
set "VERSION="
set "VERFILE=%TEMP%\sablewright-version.txt"
del /q "%VERFILE%" >nul 2>&1
powershell -NoProfile -Command "$l = Get-Content 'build\config.yml' | Where-Object { $_ -match '^\s+version:' } | Select-Object -First 1; if ($l) { ($l -split ':',2)[1].Split('#')[0].Trim().Trim([char]34) }" > "%VERFILE%" 2>nul
if exist "%VERFILE%" set /p VERSION=<"%VERFILE%"
set "COMMIT="
for /f "delims=" %%C in ('git rev-parse --short HEAD 2^>nul') do set "COMMIT=%%C"

set "LDF=-w -s -H windowsgui"
if defined VERSION set "LDF=%LDF% -X main.version=%VERSION%"
if defined COMMIT set "LDF=%LDF% -X main.commit=%COMMIT%"

rem The file properties carry the version too. They are written to a copy in
rem TEMP so the tracked build\windows\info.json stays as it is.
set "INFO=%TEMP%\sablewright-info.json"
set "STAMP=%VERSION%"
if not defined STAMP set "STAMP=0.0.0"
rem WriteAllText rather than Set-Content: it writes no byte-order mark, which
rem the JSON reader in the Wails tool would refuse.
powershell -NoProfile -Command "[IO.File]::WriteAllText('%INFO%', ((Get-Content 'build\windows\info.json' -Raw) -replace '0\.0\.0', '%STAMP%'))"

"%GOBIN%\wails3.exe" generate syso -arch amd64 -icon build\windows\icon.ico -manifest build\windows\wails.exe.manifest -info "%INFO%" -out wails_windows_amd64.syso
if errorlevel 1 (
    echo.
    echo   [X] Could not prepare the Windows resources - the error is above.
    echo.
    pause
    exit /b 1
)

echo.
if not exist "%HERE%\build\bin" mkdir "%HERE%\build\bin"
set "CGO_ENABLED=0"
"%GOEXE%" build -tags production -trimpath -buildvcs=false -ldflags "%LDF%" -o build\bin\Sablewright.exe .
set "BUILD_ERR=%ERRORLEVEL%"
del /q wails_windows_amd64.syso >nul 2>&1
echo.

if not "%BUILD_ERR%"=="0" (
    echo   [X] The build failed.
    echo       Scroll up - the compiler error will be just above.
    echo.
    pause
    exit /b 1
)

set "OUT=%HERE%\build\bin\Sablewright.exe"
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
echo    Done. It's a single .exe - no installer, nothing to set up.
echo    Move it anywhere; it'll still work. It uses Microsoft's
echo    WebView2, which Windows 11 always has; on a Windows 10 PC
echo    without it, use the installer from the releases page instead.
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
