@echo off
setlocal

if not exist "aigle.exe" (
    echo [INFO] aigle.exe n'existe pas encore, lancement de build.cmd...
    call build.cmd
    if errorlevel 1 exit /b 1
)

echo Demarrage d'Aigle sur http://localhost:8080 ...
.\aigle.exe -open %*

endlocal
