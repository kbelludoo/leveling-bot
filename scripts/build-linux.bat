@echo off
echo ============================================================
echo   COMPILANDO BOT GO PARA LINUX (AMD64 / VPS)
echo ============================================================

set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o bot-app-linux .

if %ERRORLEVEL% EQU 0 (
    echo.
    echo ✔ Sucesso! Binario gerado: bot-app-linux
    echo Envie para a VPS via SCP:
    echo   scp bot-app-linux usuario@ip:/caminho/do/bot/
) else (
    echo.
    echo ✖ Erro durante a compilacao.
)
pause
