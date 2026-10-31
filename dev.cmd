@echo off
setlocal

echo ================================================================
echo   AIGLE - Mode Developpement (Backend Go :8080 + Vite HMR :5173)
echo ================================================================

REM 1. Detection de Bun
set "BUN_BIN=bun"
if exist "%USERPROFILE%\.bun\bin\bun.exe" (
    set "BUN_BIN=%USERPROFILE%\.bun\bin\bun.exe"
)

REM 2. Detection de la Toolchain Go
set "GO_BIN=go"
if exist "%USERPROFILE%\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64\bin\go.exe" (
    set "GOROOT=%USERPROFILE%\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64"
    set "GO_BIN=%USERPROFILE%\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64\bin\go.exe"
)

if not exist "web\node_modules" (
    echo Installation des dependances frontend avec Bun...
    "%BUN_BIN%" install --cwd web
)

REM S'assurer que web/dist contient au moins un fichier pour go:embed
if not exist "web\dist\index.html" (
    "%BUN_BIN%" run --cwd web build
)

echo.
echo [1/2] Demarrage du serveur API Go en arriere-plan sur http://localhost:8080 ...
start "Aigle Backend API (Go :8080)" cmd /k ""%GO_BIN%" run ./cmd/aigle -port=8080"

echo [2/2] Demarrage du serveur Frontend Vite (Hot-Reload) sur http://localhost:5173 ...
echo       Ouvrez http://localhost:5173 dans votre navigateur pour le mode Dev.
echo.
"%BUN_BIN%" run --cwd web dev

endlocal
