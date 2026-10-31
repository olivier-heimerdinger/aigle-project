@echo off
setlocal
cd /d "%~dp0"

echo ============================================================================
echo   AIGLE SAAS SERVER - BUILD FRONTEND + TESTS + CROSS-BUILD WIN/MAC/LINUX
echo ============================================================================

set "BUN_EXE=C:\Users\olivi\.bun\bin\bun.exe"
if not exist "%BUN_EXE%" set "BUN_EXE=bun"

set "GO_EXE=C:\Users\olivi\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64\bin\go.exe"
if exist "%GO_EXE%" (
    set "GOROOT=C:\Users\olivi\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64"
) else (
    set "GO_EXE=go"
)

echo.
echo [1/4] Verification et compilation du Frontend SaaS (Vitrine Publique + SaaS + Console Admin)...
if not exist "web\node_modules" (
    if exist "..\web\node_modules" (
        robocopy "..\web\node_modules" "web\node_modules" /E /NFL /NDL /NJH /NJS /MT:8 >nul
    ) else (
        call "%BUN_EXE%" install --cwd web
        if errorlevel 1 exit /b 1
    )
)
call "%BUN_EXE%" run --cwd web build
if errorlevel 1 exit /b 1

echo.
echo [2/4] Preparation du dossier public downloads/ (Binaires Windows, macOS, Linux + Presentations)...
if not exist "downloads" mkdir "downloads"
if exist "..\presentation.pptx" copy /Y "..\presentation.pptx" "downloads\aigle-presentation.pptx" >nul
if exist "..\presentation.html" copy /Y "..\presentation.html" "downloads\aigle-presentation.html" >nul

set CGO_ENABLED=0
pushd ..
echo   - Cross-compilation Windows x64 (aigle-windows-amd64.exe)...
set GOOS=windows
set GOARCH=amd64
call "%GO_EXE%" build -trimpath -ldflags="-s -w" -o "server\downloads\aigle-windows-amd64.exe" ./cmd/aigle
if errorlevel 1 ( popd & exit /b 1 )

echo   - Cross-compilation macOS Apple Silicon ARM64 (aigle-darwin-arm64)...
set GOOS=darwin
set GOARCH=arm64
call "%GO_EXE%" build -trimpath -ldflags="-s -w" -o "server\downloads\aigle-darwin-arm64" ./cmd/aigle
if errorlevel 1 ( popd & exit /b 1 )

echo   - Cross-compilation macOS Intel x64 (aigle-darwin-amd64)...
set GOOS=darwin
set GOARCH=amd64
call "%GO_EXE%" build -trimpath -ldflags="-s -w" -o "server\downloads\aigle-darwin-amd64" ./cmd/aigle
if errorlevel 1 ( popd & exit /b 1 )

echo   - Cross-compilation Linux x64 Statique (aigle-linux-amd64)...
set GOOS=linux
set GOARCH=amd64
call "%GO_EXE%" build -trimpath -ldflags="-s -w" -o "server\downloads\aigle-linux-amd64" ./cmd/aigle
if errorlevel 1 ( popd & exit /b 1 )
popd

set GOOS=windows
set GOARCH=amd64

echo.
echo [3/4] Execution des tests unitaires et de securite (Zone Publique, Zero-Trust Authelia, RBAC)...
call "%GO_EXE%" test ./... -v
if errorlevel 1 exit /b 1

echo.
echo [4/4] Compilation du binaire serveur SaaS aigle-server.exe...
call "%GO_EXE%" build -trimpath -ldflags="-s -w" -o aigle-server.exe ./cmd/aigle-server
if errorlevel 1 exit /b 1

echo.
echo [OK] Serveur SaaS et les 4 binaires clients (Win/Mac/Linux) sont prets !
echo   - Binaire Serveur : %~dp0aigle-server.exe
echo   - Dossier Public  : %~dp0downloads\
endlocal
