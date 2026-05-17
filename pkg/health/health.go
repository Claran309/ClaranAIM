// Package health contains simple startup health checks and service banner logs.
package health

import (
	"ClaranAIM/pkg/logger"
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CheckMySQL verifies that MySQL is reachable and logs server version when available.
func CheckMySQL(db *sql.DB, serviceName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		logger.Error("MySQL连接检查失败", "service", serviceName, "error", err)
		return false
	}

	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		logger.Warn("MySQL版本查询失败", "service", serviceName, "error", err)
	} else {
		logger.Info("MySQL连接正常", "service", serviceName, "version", version)
	}
	return true
}

// CheckRedis verifies that Redis is reachable.
func CheckRedis(rdb *redis.Client, serviceName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Error("Redis连接检查失败", "service", serviceName, "error", err)
		return false
	}

	info, err := rdb.Info(ctx, "server").Result()
	if err != nil {
		logger.Warn("Redis信息查询失败", "service", serviceName, "error", err)
	} else {
		logger.Info("Redis连接正常", "service", serviceName)
		_ = info
	}
	return true
}

// CheckEtcd logs configured Etcd endpoints. Kitex service registration performs
// the real connectivity check during service startup.
func CheckEtcd(endpoints []string, serviceName string) bool {
	logger.Info("Etcd注册地址", "service", serviceName, "endpoints", fmt.Sprintf("%v", endpoints))
	return true
}

// ServiceInfo describes one service for startup logging.
type ServiceInfo struct {
	Name    string
	Version string
	Port    string
}

// LogStartup writes a consistent service-start banner.
func LogStartup(info ServiceInfo) {
	logger.Info("========================================")
	logger.Info("服务启动完成", "service", info.Name, "version", info.Version, "port", info.Port)
	logger.Info("========================================")
}
