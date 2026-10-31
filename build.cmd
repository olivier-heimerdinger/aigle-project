@echo off
setlocal

echo ================================================================
echo   AIGLE - Build Complet (Frontend Svelte + Tests + Binaire Go)
echo ================================================================

REM 1. Detection de Bun (pour compiler le frontend Svelte)
set "BUN_BIN=bun"
if exist "%USERPROFILE%\.bun\bin\bun.exe" (
    set "BUN_BIN=%USERPROFILE%\.bun\bin\bun.exe"
)

REM 2. Detection de la Toolchain Go propre
set "GO_BIN=go"
if exist "%USERPROFILE%\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64\bin\go.exe" (
    set "GOROOT=%USERPROFILE%\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64"
    set "GO_BIN=%USERPROFILE%\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64\bin\go.exe"
)

echo.
echo [1/4] Verification des dependances Frontend (Bun)...
if not exist "web\node_modules" (
    "%BUN_BIN%" install --cwd web
    if errorlevel 1 (
        echo [ERREUR] Echec de l'installation des dependances frontend.
        exit /b 1
    )
)

echo.
echo [2/4] Compilation de l'interface Svelte 5 + TailwindCSS (web/dist)...
"%BUN_BIN%" run --cwd web build
if errorlevel 1 (
    echo [ERREUR] Echec de la compilation du frontend Svelte.
    exit /b 1
)

echo.
echo [3/4] Execution des tests unitaires et d'integration Go...
"%GO_BIN%" test ./...
if errorlevel 1 (
    echo [ERREUR] Les tests Go ont echoue.
    exit /b 1
)

echo.
echo [4/4] Compilation du binaire unique autonome (aigle.exe)...
"%GO_BIN%" build -ldflags="-s -w" -o aigle.exe ./cmd/aigle
if errorlevel 1 (
    echo [ERREUR] Echec de la compilation de aigle.exe.
    exit /b 1
)

echo.
echo ================================================================
echo   BUILD REUSSI : aigle.exe est pret !
echo   Lancez "run.cmd" ou ".\aigle.exe -open" pour demarrer.
echo ================================================================
endlocal
