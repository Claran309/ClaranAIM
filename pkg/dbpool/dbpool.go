// Package dbpool 统一配置 database/sql 连接池。
// 未设置环境变量时保持原有默认行为，避免影响现有部署；
// 压测环境通过 DB_MAX_OPEN_CONNS 等变量固定池大小，防止连接churn耗尽临时端口。
package dbpool

import (
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// Configure 按环境变量调整 GORM 底层的 sql.DB 池参数。
func Configure(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		return
	}
	if v := envInt("DB_MAX_OPEN_CONNS"); v > 0 {
		sqlDB.SetMaxOpenConns(v)
	}
	if v := envInt("DB_MAX_IDLE_CONNS"); v > 0 {
		sqlDB.SetMaxIdleConns(v)
	}
	if v := envInt("DB_CONN_MAX_LIFETIME_MINUTES"); v > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(v) * time.Minute)
	}
	if v := envInt("DB_CONN_MAX_IDLE_MINUTES"); v > 0 {
		sqlDB.SetConnMaxIdleTime(time.Duration(v) * time.Minute)
	}
}

func envInt(key string) int {
	value := os.Getenv(key)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}
