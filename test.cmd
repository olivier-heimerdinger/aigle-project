@echo off
setlocal

echo ================================================================
echo   AIGLE - Execution des Tests Unitaires et d'Integration Go
echo ================================================================

set "GO_BIN=go"
if exist "%USERPROFILE%\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64\bin\go.exe" (
    set "GOROOT=%USERPROFILE%\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64"
    set "GO_BIN=%USERPROFILE%\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.7.windows-amd64\bin\go.exe"
)

"%GO_BIN%" test -count=1 ./... -v
endlocal
