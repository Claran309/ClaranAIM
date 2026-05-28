// Package health 提供服务启动阶段的依赖连通性检查和统一启动日志。
package health

import (
	"ClaranAIM/pkg/logger"
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CheckMySQL 检查 MySQL 是否可达，并尽量记录服务端版本，便于启动问题排查。
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

// CheckRedis 检查 Redis 是否可达。
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

// CheckEtcd 记录当前服务配置的 Etcd 地址。
// Kitex 注册流程会在服务启动时执行真正的连通性检查，这里主要用于日志可观测性。
func CheckEtcd(endpoints []string, serviceName string) bool {
	logger.Info("Etcd注册地址", "service", serviceName, "endpoints", fmt.Sprintf("%v", endpoints))
	return true
}

// ServiceInfo 描述一个服务的启动日志字段。
type ServiceInfo struct {
	Name    string
	Version string
	Port    string
}

// LogStartup 输出统一格式的服务启动完成日志。
func LogStartup(info ServiceInfo) {
	logger.Info("========================================")
	logger.Info("服务启动完成", "service", info.Name, "version", info.Version, "port", info.Port)
	logger.Info("========================================")
}
