package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	MySQL   MySQLConfig   `yaml:"mysql"`
	Redis   RedisConfig   `yaml:"redis"`
	JWT     JWTConfig     `yaml:"jwt"`
	Etcd    EtcdConfig    `yaml:"etcd"`
	Service ServiceConfig `yaml:"service"`
}

type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	DSN      string `yaml:"dsn"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	Addr     string `yaml:"addr"`
}

type JWTConfig struct {
	SecretKey  string `yaml:"secret_key"`
	Expiration int64  `yaml:"expiration"`
}

type EtcdConfig struct {
	Endpoints []string `yaml:"endpoints"`
}

type ServiceConfig struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
}

func Load(configPath string) (*Config, error) {
	// 加载 .env 文件（如果存在）
	_ = godotenv.Load()

	// 设置 viper 读取环境变量
	viper.AutomaticEnv()

	// 读取 YAML 配置文件
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 用环境变量覆盖敏感配置
	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// applyEnvOverrides 用环境变量覆盖配置
func applyEnvOverrides(cfg *Config) {
	// MySQL 配置
	if dsn := os.Getenv("MYSQL_DSN"); dsn != "" {
		cfg.MySQL.DSN = dsn
	}
	if host := os.Getenv("MYSQL_HOST"); host != "" {
		cfg.MySQL.Host = host
	}
	if port := os.Getenv("MYSQL_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.MySQL.Port = p
		}
	}
	if user := os.Getenv("MYSQL_USER"); user != "" {
		cfg.MySQL.User = user
	}
	if password := os.Getenv("MYSQL_PASSWORD"); password != "" {
		cfg.MySQL.Password = password
	}
	if database := os.Getenv("MYSQL_DATABASE"); database != "" {
		cfg.MySQL.Database = database
	}

	// 如果没有 DSN，自动构建
	if cfg.MySQL.DSN == "" && cfg.MySQL.Host != "" {
		cfg.MySQL.DSN = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.MySQL.User,
			cfg.MySQL.Password,
			cfg.MySQL.Host,
			cfg.MySQL.Port,
			cfg.MySQL.Database,
		)
	}

	// Redis 配置
	if host := os.Getenv("REDIS_HOST"); host != "" {
		cfg.Redis.Host = host
	}
	if port := os.Getenv("REDIS_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Redis.Port = p
		}
	}
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		cfg.Redis.Password = password
	}
	if db := os.Getenv("REDIS_DB"); db != "" {
		if d, err := strconv.Atoi(db); err == nil {
			cfg.Redis.DB = d
		}
	}
	// 构建 Redis 地址
	if cfg.Redis.Host != "" {
		cfg.Redis.Addr = fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	}

	// JWT 配置
	if secretKey := os.Getenv("JWT_SECRET_KEY"); secretKey != "" {
		cfg.JWT.SecretKey = secretKey
	}
	if expiration := os.Getenv("JWT_EXPIRATION"); expiration != "" {
		if e, err := strconv.ParseInt(expiration, 10, 64); err == nil {
			cfg.JWT.Expiration = e
		}
	}

	// Etcd 配置
	if endpoints := os.Getenv("ETCD_ENDPOINTS"); endpoints != "" {
		cfg.Etcd.Endpoints = strings.Split(endpoints, ",")
	}
}

// MustLoad 加载配置，失败则 panic
func MustLoad(configPath string) *Config {
	cfg, err := Load(configPath)
	if err != nil {
		panic(fmt.Sprintf("加载配置失败: %v", err))
	}
	return cfg
}
