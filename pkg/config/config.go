package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config 全局配置结构体
// 包含所有服务共用的配置项：MySQL、Redis、JWT、Etcd、Service、MinIO、Storage
// 每个服务通过各自的YAML配置文件加载，环境变量可覆盖敏感信息
type Config struct {
	MySQL   MySQLConfig   `yaml:"mysql"`
	Redis   RedisConfig   `yaml:"redis"`
	JWT     JWTConfig     `yaml:"jwt"`
	Etcd    EtcdConfig    `yaml:"etcd"`
	Service ServiceConfig `yaml:"service"`
	Minio   MinioConfig   `yaml:"minio"`
	Storage StorageConfig `yaml:"storage"`
	LLM     LLMConfig     `yaml:"llm"`
	Agent   AgentConfig   `yaml:"agent"`
}

// MySQLConfig MySQL数据库配置
type MySQLConfig struct {
	Host     string `yaml:"host"`     // 数据库主机地址
	Port     int    `yaml:"port"`     // 数据库端口
	User     string `yaml:"user"`     // 数据库用户名
	Password string `yaml:"password"` // 数据库密码
	Database string `yaml:"database"` // 数据库名称
	DSN      string `yaml:"dsn"`      // 完整数据源连接字符串（优先使用）
}

// RedisConfig Redis缓存配置
type RedisConfig struct {
	Host     string `yaml:"host"`     // Redis主机地址
	Port     int    `yaml:"port"`     // Redis端口
	Password string `yaml:"password"` // Redis密码
	DB       int    `yaml:"db"`       // Redis数据库编号
	Addr     string `yaml:"addr"`     // 完整Redis地址（自动构建：host:port）
}

// JWTConfig JWT认证配置
type JWTConfig struct {
	SecretKey  string `yaml:"secret_key"` // JWT签名密钥
	Expiration int64  `yaml:"expiration"` // Token过期时间（小时）
}

// EtcdConfig Etcd服务发现配置
type EtcdConfig struct {
	Endpoints []string `yaml:"endpoints"` // Etcd集群地址列表
}

// ServiceConfig 服务自身配置
type ServiceConfig struct {
	Name    string `yaml:"name"`    // 服务名称（用于Etcd注册）
	Address string `yaml:"address"` // 服务监听地址（如 127.0.0.1:9001）
}

// MinioConfig MinIO对象存储配置
type MinioConfig struct {
	Endpoint  string `yaml:"endpoint"`   // MinIO服务地址（如 localhost:9000）
	AccessKey string `yaml:"access_key"` // MinIO访问密钥
	SecretKey string `yaml:"secret_key"` // MinIO密钥
	Bucket    string `yaml:"bucket"`     // 存储桶名称
	UseMinio  bool   `yaml:"use_minio"`  // 是否使用MinIO（false则使用本地存储）
}

// StorageConfig 本地文件存储配置
type StorageConfig struct {
	Dir string `yaml:"dir"`
}

type LLMConfig struct {
	DefaultAPIKey  string `yaml:"default_api_key"`
	DefaultBaseURL string `yaml:"default_base_url"`
	DefaultModel   string `yaml:"default_model"`
}

type AgentConfig struct {
	SessionDir    string `yaml:"session_dir"`
	AgentRoot     string `yaml:"agent_root"`
	CozeloopToken string `yaml:"cozeloop_api_token"`
	CozeloopWSID  string `yaml:"cozeloop_workspace_id"`
	SkillsDir     string `yaml:"skills_dir"`
}

// Load 加载配置文件
// 流程：加载.env文件 → 读取YAML配置 → 环境变量覆盖敏感信息
// configPath: YAML配置文件路径（如 config/user-service.yaml）
func Load(configPath string) (*Config, error) {
	// 加载 .env 文件（如果存在），将环境变量加载到进程
	_ = godotenv.Load()

	// 设置 viper 自动读取环境变量
	viper.AutomaticEnv()

	// 读取 YAML 配置文件
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 将YAML配置解析到Config结构体
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 用环境变量覆盖敏感配置（优先级：环境变量 > YAML配置）
	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// applyEnvOverrides 用环境变量覆盖配置
// 敏感信息（数据库密码、JWT密钥等）不应硬编码在YAML中
// 而是通过 .env 文件或系统环境变量注入
func applyEnvOverrides(cfg *Config) {
	// MySQL 配置覆盖
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

	// 如果没有提供完整DSN，根据各字段自动构建
	if cfg.MySQL.DSN == "" && cfg.MySQL.Host != "" {
		cfg.MySQL.DSN = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.MySQL.User,
			cfg.MySQL.Password,
			cfg.MySQL.Host,
			cfg.MySQL.Port,
			cfg.MySQL.Database,
		)
	}

	// Redis 配置覆盖
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
	// 自动构建Redis地址
	if cfg.Redis.Host != "" {
		cfg.Redis.Addr = fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	}

	// JWT 配置覆盖
	if secretKey := os.Getenv("JWT_SECRET_KEY"); secretKey != "" {
		cfg.JWT.SecretKey = secretKey
	}
	if expiration := os.Getenv("JWT_EXPIRATION"); expiration != "" {
		if e, err := strconv.ParseInt(expiration, 10, 64); err == nil {
			cfg.JWT.Expiration = e
		}
	}

	// Etcd 配置覆盖（多个地址用逗号分隔）
	if endpoints := os.Getenv("ETCD_ENDPOINTS"); endpoints != "" {
		cfg.Etcd.Endpoints = strings.Split(endpoints, ",")
	}

	// MinIO 配置覆盖（使用 Docker MinIO 标准环境变量名）
	if endpoint := os.Getenv("MINIO_ENDPOINT"); endpoint != "" {
		cfg.Minio.Endpoint = endpoint
	}
	if rootUser := os.Getenv("MINIO_ROOT_USER"); rootUser != "" {
		cfg.Minio.AccessKey = rootUser
	}
	if rootPassword := os.Getenv("MINIO_ROOT_PASSWORD"); rootPassword != "" {
		cfg.Minio.SecretKey = rootPassword
	}
	if bucket := os.Getenv("MINIO_BUCKET_NAME"); bucket != "" {
		cfg.Minio.Bucket = bucket
	}
	if useMinio := os.Getenv("MINIO_USE_MINIO"); useMinio != "" {
		if v, err := strconv.ParseBool(useMinio); err == nil {
			cfg.Minio.UseMinio = v
		}
	}

	if apiKey := os.Getenv("LLM_DEFAULT_API_KEY"); apiKey != "" {
		cfg.LLM.DefaultAPIKey = apiKey
	}
	if baseURL := os.Getenv("LLM_DEFAULT_BASE_URL"); baseURL != "" {
		cfg.LLM.DefaultBaseURL = baseURL
	}
	if model := os.Getenv("LLM_DEFAULT_MODEL"); model != "" {
		cfg.LLM.DefaultModel = model
	}

	if sessionDir := os.Getenv("AGENT_SESSION_DIR"); sessionDir != "" {
		cfg.Agent.SessionDir = sessionDir
	}
	if agentRoot := os.Getenv("AGENT_ROOT"); agentRoot != "" {
		cfg.Agent.AgentRoot = agentRoot
	}
	if token := os.Getenv("COZELOOP_API_TOKEN"); token != "" {
		cfg.Agent.CozeloopToken = token
	}
	if wsID := os.Getenv("COZELOOP_WORKSPACE_ID"); wsID != "" {
		cfg.Agent.CozeloopWSID = wsID
	}
	if skillsDir := os.Getenv("SKILLS_DIR"); skillsDir != "" {
		cfg.Agent.SkillsDir = skillsDir
	}
}

// MustLoad 加载配置，失败则panic
// 用于服务启动阶段，配置加载失败应直接终止程序
func MustLoad(configPath string) *Config {
	cfg, err := Load(configPath)
	if err != nil {
		panic(fmt.Sprintf("加载配置失败: %v", err))
	}
	return cfg
}
