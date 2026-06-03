@echo off
setlocal
chcp 65001 >nul 2>&1
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0service-control.ps1" stop %*
exit /b %ERRORLEVEL%
