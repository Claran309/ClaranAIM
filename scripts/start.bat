@echo off
chcp 65001 >nul 2>&1
REM ClaranAIM Startup Script (Windows)
REM Make sure Docker containers (MySQL, Redis, Etcd) are running

echo ========================================
echo   ClaranAIM Services Starting
echo ========================================
echo.

REM Check Docker containers
echo [1/7] Checking Docker containers...
docker ps --format "table {{.Names}}\t{{.Status}}" | findstr /i "MySQL Redis Etcd" >nul 2>&1
if errorlevel 1 (
    echo [WARN] Docker containers not detected, please run: docker-compose up -d
    echo.
    choice /C YN /M "Start Docker containers now"
    if errorlevel 2 goto skip_docker
    docker-compose up -d
    echo Waiting for containers...
    timeout /t 10 /nobreak >nul
)
:skip_docker

echo.
echo [2/7] Starting user-service (port 9001)...
start "user-service" cmd /k "cd /d %~dp0.. && go run cmd/user-service/main.go"
timeout /t 3 /nobreak >nul

echo [3/7] Starting group-service (port 9002)...
start "group-service" cmd /k "cd /d %~dp0.. && go run cmd/group-service/main.go"
timeout /t 3 /nobreak >nul

echo [4/7] Starting msg-core-service (port 9003)...
start "msg-core-service" cmd /k "cd /d %~dp0.. && go run cmd/msg-core-service/main.go"
timeout /t 3 /nobreak >nul

echo [5/7] Starting msg-history-service (port 9004)...
start "msg-history-service" cmd /k "cd /d %~dp0.. && go run cmd/msg-history-service/main.go"
timeout /t 3 /nobreak >nul

echo [6/7] Starting api-gateway (port 8080)...
start "api-gateway" cmd /k "cd /d %~dp0.. && go run cmd/api-gateway/main.go"
timeout /t 3 /nobreak >nul

echo [7/7] Starting websocket-gateway (port 8081)...
start "websocket-gateway" cmd /k "cd /d %~dp0.. && go run cmd/websocket-gateway/main.go"
timeout /t 3 /nobreak >nul

echo.
echo ========================================
echo   All services started!
echo ========================================
echo.
echo   API Gateway:       http://localhost:8080
echo   WebSocket Gateway: ws://localhost:8081
echo   Frontend:          Open dist/index.html in browser
echo.
echo   Press any key to exit (services keep running)
echo ========================================
pause >nul
