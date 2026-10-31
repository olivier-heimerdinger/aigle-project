@echo off
setlocal
cd /d "%~dp0"

echo ============================================================================
echo   AIGLE SAAS SERVER - LANCEMENT LOCAL EN MODE DEV (SQLITE + SUPERADMIN)
echo ============================================================================

if not exist "aigle-server.exe" (
    call build-server.cmd
    if errorlevel 1 exit /b 1
)

echo.
echo Demarrage d'Aigle SaaS Server sur http://localhost:8090 ...
aigle-server.exe -port 8090 -db-driver sqlite -db aigle_saas_dev.db -auth-mode dev
endlocal

