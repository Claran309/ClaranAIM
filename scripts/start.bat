@echo off
chcp 65001 >nul 2>&1

echo ========================================
echo   ClaranAIM Services Starting
echo ========================================
echo.

echo [1/18] Checking Docker containers...
docker ps --format "table {{.Names}}\t{{.Status}}" | findstr /i "MySQL Redis Etcd" >nul 2>&1
if errorlevel 1 (
    echo [WARN] Docker containers not detected, please run: docker-compose up -d
    echo.
    choice /C YN /M "Start Docker containers now"
    if errorlevel 2 goto skip_docker
    docker-compose up -d
    echo Waiting for containers...
    powershell -NoProfile -Command "Start-Sleep -Seconds 10" >nul 2>&1
)
:skip_docker

echo.
goto :start_services

:start_service
set "STEP=%~1"
set "NAME=%~2"
set "PORT=%~3"
set "CMD_PATH=%~4"
echo [%STEP%/18] Starting %NAME% (port %PORT%)...
powershell -NoProfile -ExecutionPolicy Bypass -Command "$port=%PORT%; $name='%NAME%'; $cmd='%CMD_PATH%'.ToLower().Replace('\','/'); $listeners=@(Get-NetTCPConnection -LocalAddress 127.0.0.1 -LocalPort $port -State Listen -ErrorAction SilentlyContinue); if ($listeners.Count -eq 0) { exit 0 }; foreach ($listener in $listeners) { $pidValue=$listener.OwningProcess; $proc=Get-CimInstance Win32_Process -Filter \"ProcessId=$pidValue\" -ErrorAction SilentlyContinue; $parent=$null; if ($proc -and $proc.ParentProcessId) { $parent=Get-CimInstance Win32_Process -Filter \"ProcessId=$($proc.ParentProcessId)\" -ErrorAction SilentlyContinue }; $grand=$null; if ($parent -and $parent.ParentProcessId) { $grand=Get-CimInstance Win32_Process -Filter \"ProcessId=$($parent.ParentProcessId)\" -ErrorAction SilentlyContinue }; $hay=(($proc.CommandLine,$parent.CommandLine,$grand.CommandLine) -join ' ').ToLower().Replace('\','/'); if ($hay.Contains($cmd)) { Write-Host ('[SKIP] ' + $name + ' already running on 127.0.0.1:' + $port + ' pid=' + $pidValue); exit 10 } }; Write-Host ('[BLOCKED] ' + $name + ' cannot start because 127.0.0.1:' + $port + ' is occupied by:'); foreach ($listener in $listeners) { $pidValue=$listener.OwningProcess; $proc=Get-CimInstance Win32_Process -Filter \"ProcessId=$pidValue\" -ErrorAction SilentlyContinue; $line=''; if ($proc) { $line=$proc.CommandLine; if ([string]::IsNullOrWhiteSpace($line)) { $line=$proc.Name } }; Write-Host ('  pid=' + $pidValue + ' ' + $line) }; exit 20"
set "STATUS=%ERRORLEVEL%"
if "%STATUS%"=="10" (
    powershell -NoProfile -Command "Start-Sleep -Seconds 1" >nul 2>&1
    exit /b 0
)
if "%STATUS%"=="20" (
    powershell -NoProfile -Command "Start-Sleep -Seconds 1" >nul 2>&1
    exit /b 1
)
start "%NAME%" cmd /k "cd /d %~dp0.. && go run %CMD_PATH%"
powershell -NoProfile -Command "Start-Sleep -Seconds 3" >nul 2>&1
exit /b 0

:start_services
set "FAILED_SERVICES="
call :start_service 2 user-service 9101 cmd/user-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% user-service"

call :start_service 3 group-service 9002 cmd/group-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% group-service"

call :start_service 4 msg-core-service 9003 cmd/msg-core-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% msg-core-service"

call :start_service 5 msg-history-service 9004 cmd/msg-history-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% msg-history-service"

call :start_service 6 file-service 9005 cmd/file-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% file-service"

call :start_service 7 memory-service 9008 cmd/memory-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% memory-service"

call :start_service 8 settings-service 9009 cmd/settings-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% settings-service"

call :start_service 9 web-search-service 9114 cmd/web-search-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% web-search-service"

call :start_service 10 rag-service 9112 cmd/rag-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% rag-service"

call :start_service 11 knowledge-service 9113 cmd/knowledge-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% knowledge-service"

call :start_service 12 conversation-intelligence-service 9015 cmd/conversation-intelligence-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% conversation-intelligence-service"

call :start_service 13 mcp-gateway-service 9016 cmd/mcp-gateway-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% mcp-gateway-service"

call :start_service 14 admin-service 9017 cmd/admin-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% admin-service"

call :start_service 15 agent-runtime-service 9007 cmd/agent-runtime-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% agent-runtime-service"

call :start_service 16 agent-manager-service 9006 cmd/agent-manager-service/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% agent-manager-service"

call :start_service 17 api-gateway 8080 cmd/api-gateway/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% api-gateway"

call :start_service 18 websocket-gateway 8081 cmd/websocket-gateway/main.go
if errorlevel 1 set "FAILED_SERVICES=%FAILED_SERVICES% websocket-gateway"

echo.
echo ========================================
if "%FAILED_SERVICES%"=="" (
echo   All services are running or were started.
) else (
echo   Some services were blocked:%FAILED_SERVICES%
)
echo ========================================
echo.
echo   API Gateway:          http://localhost:8080
echo   WebSocket Gateway:    ws://localhost:8081
echo   MinIO Console:        http://localhost:9001
echo   Frontend:             Open dist/index.html in browser
echo.
echo   Press any key to exit (services keep running)
pause >nul


