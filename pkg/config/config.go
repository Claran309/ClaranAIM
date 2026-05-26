package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Config 全局配置结构体
// 包含所有服务共用的配置项：MySQL、Redis、JWT、Etcd、Service、MinIO、Storage
// 每个服务通过各自的YAML配置文件加载，环境变量可覆盖敏感信息
type Config struct {
	MySQL      MySQLConfig      `yaml:"mysql"`
	Redis      RedisConfig      `yaml:"redis"`
	JWT        JWTConfig        `yaml:"jwt"`
	Etcd       EtcdConfig       `yaml:"etcd"`
	Service    ServiceConfig    `yaml:"service"`
	Minio      MinioConfig      `yaml:"minio"`
	Storage    StorageConfig    `yaml:"storage"`
	LLM        LLMConfig        `yaml:"llm"`
	Agent      AgentConfig      `yaml:"agent"`
	Kafka      KafkaConfig      `yaml:"kafka"`
	DTM        DTMConfig        `yaml:"dtm"`
	Governance GovernanceConfig `yaml:"governance"`
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
	SecretKey         string `yaml:"secret_key"`         // JWT签名密钥
	Expiration        int64  `yaml:"expiration"`         // Access Token过期时间（小时），兼容旧配置
	AccessExpiration  int64  `yaml:"access_expiration"`  // Access Token过期时间（小时）
	RefreshExpiration int64  `yaml:"refresh_expiration"` // Refresh Token过期时间（小时）
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

// LLMConfig holds default provider settings for internal bots.
type LLMConfig struct {
	DefaultAPIKey  string `yaml:"default_api_key"`
	DefaultBaseURL string `yaml:"default_base_url"`
	DefaultModel   string `yaml:"default_model"`
}

// AgentConfig controls bot memory, tool and agent filesystem locations.
type AgentConfig struct {
	SessionDir    string `yaml:"session_dir"`
	AgentRoot     string `yaml:"agent_root"`
	CozeloopToken string `yaml:"cozeloop_api_token"`
	CozeloopWSID  string `yaml:"cozeloop_workspace_id"`
	SkillsDir     string `yaml:"skills_dir"`
}

// KafkaConfig 控制服务是否启用 Kafka 事件总线。
// Enabled=false 时服务仍按同步 RPC/HTTP fallback 工作，便于本地开发渐进接入。
type KafkaConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Brokers  []string `yaml:"brokers"`
	ClientID string   `yaml:"client_id"`
}

// DTMConfig controls optional distributed transaction integration.
type DTMConfig struct {
	Enabled              bool   `yaml:"enabled"`
	Server               string `yaml:"server"`
	GroupServiceURL      string `yaml:"group_service_url"`
	MsgCoreServiceURL    string `yaml:"msg_core_service_url"`
	GroupBranchAddress   string `yaml:"group_branch_address"`
	MsgCoreBranchAddress string `yaml:"msg_core_branch_address"`
}

// GovernanceConfig groups rate-limit and RPC governance settings.
type GovernanceConfig struct {
	RateLimit RateLimitConfig     `yaml:"rate_limit"`
	RPC       RPCGovernanceConfig `yaml:"rpc"`
	AgentRPC  RPCGovernanceConfig `yaml:"agent_rpc"`
}

// RateLimitConfig configures the api-gateway token-bucket limiter.
type RateLimitConfig struct {
	Enabled       bool `yaml:"enabled"`
	Burst         int  `yaml:"burst"`
	WindowSeconds int  `yaml:"window_seconds"`
}

// RPCGovernanceConfig configures Kitex client/server timeout and protection knobs.
type RPCGovernanceConfig struct {
	TimeoutMS      int  `yaml:"timeout_ms"`
	CircuitBreaker bool `yaml:"circuit_breaker"`
	MaxConnections int  `yaml:"max_connections"`
	MaxQPS         int  `yaml:"max_qps"`
}

// Load 加载配置文件
// 流程：加载.env文件 → 读取YAML配置 → 环境变量覆盖敏感信息
// configPath: YAML配置文件路径（如 config/user-service.yaml）
func Load(configPath string) (*Config, error) {
	// 加载 .env 文件（如果存在），将环境变量加载到进程
	_ = godotenv.Load()

	// 每次 Load 使用独立 Viper 实例，避免测试或同进程多服务加载配置时串值。
	v := viper.New()
	v.AutomaticEnv()
	v.SetDefault("kafka.enabled", true)
	v.SetDefault("kafka.brokers", []string{"127.0.0.1:9092"})
	v.SetDefault("dtm.enabled", true)
	v.SetDefault("dtm.server", "http://localhost:36789")
	v.SetDefault("dtm.group_service_url", "http://127.0.0.1:9102")
	v.SetDefault("dtm.msg_core_service_url", "http://127.0.0.1:9103")
	v.SetDefault("dtm.group_branch_address", "127.0.0.1:9102")
	v.SetDefault("dtm.msg_core_branch_address", "127.0.0.1:9103")
	v.SetDefault("governance.rate_limit.enabled", true)
	v.SetDefault("governance.rate_limit.burst", 120)
	v.SetDefault("governance.rate_limit.window_seconds", 60)
	v.SetDefault("governance.rpc.timeout_ms", 60000)
	v.SetDefault("governance.rpc.circuit_breaker", true)
	v.SetDefault("governance.rpc.max_connections", 1000)
	v.SetDefault("governance.rpc.max_qps", 1000)
	v.SetDefault("governance.agent_rpc.timeout_ms", 0)
	v.SetDefault("governance.agent_rpc.circuit_breaker", true)
	v.SetDefault("governance.agent_rpc.max_connections", 1000)
	v.SetDefault("governance.agent_rpc.max_qps", 500)

	// 读取 YAML 配置文件
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 将YAML配置解析到Config结构体
	var cfg Config
	if err := v.Unmarshal(&cfg, func(decoderConfig *mapstructure.DecoderConfig) {
		decoderConfig.TagName = "yaml"
	}); err != nil {
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
	if expiration := os.Getenv("JWT_ACCESS_EXPIRATION"); expiration != "" {
		if e, err := strconv.ParseInt(expiration, 10, 64); err == nil {
			cfg.JWT.AccessExpiration = e
		}
	}
	if expiration := os.Getenv("JWT_REFRESH_EXPIRATION"); expiration != "" {
		if e, err := strconv.ParseInt(expiration, 10, 64); err == nil {
			cfg.JWT.RefreshExpiration = e
		}
	}
	if cfg.JWT.AccessExpiration <= 0 {
		cfg.JWT.AccessExpiration = cfg.JWT.Expiration
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

	if enabled := os.Getenv("KAFKA_ENABLED"); enabled != "" {
		if v, err := strconv.ParseBool(enabled); err == nil {
			cfg.Kafka.Enabled = v
		}
	}
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		cfg.Kafka.Brokers = splitAndTrim(brokers)
	}
	if clientID := os.Getenv("KAFKA_CLIENT_ID"); clientID != "" {
		cfg.Kafka.ClientID = clientID
	}
	if cfg.Kafka.ClientID == "" {
		cfg.Kafka.ClientID = cfg.Service.Name
	}

	if enabled := os.Getenv("DTM_ENABLED"); enabled != "" {
		if v, err := strconv.ParseBool(enabled); err == nil {
			cfg.DTM.Enabled = v
		}
	}
	if server := os.Getenv("DTM_SERVER"); server != "" {
		cfg.DTM.Server = server
	}
	if url := os.Getenv("DTM_GROUP_SERVICE_URL"); url != "" {
		cfg.DTM.GroupServiceURL = url
	}
	if url := os.Getenv("DTM_MSG_CORE_SERVICE_URL"); url != "" {
		cfg.DTM.MsgCoreServiceURL = url
	}
	if address := os.Getenv("DTM_GROUP_BRANCH_ADDRESS"); address != "" {
		cfg.DTM.GroupBranchAddress = address
	}
	if address := os.Getenv("DTM_MSG_CORE_BRANCH_ADDRESS"); address != "" {
		cfg.DTM.MsgCoreBranchAddress = address
	}

	if enabled := os.Getenv("RATE_LIMIT_ENABLED"); enabled != "" {
		if v, err := strconv.ParseBool(enabled); err == nil {
			cfg.Governance.RateLimit.Enabled = v
		}
	}
	if burst := os.Getenv("RATE_LIMIT_BURST"); burst != "" {
		if v, err := strconv.Atoi(burst); err == nil {
			cfg.Governance.RateLimit.Burst = v
		}
	}
	if window := os.Getenv("RATE_LIMIT_WINDOW_SECONDS"); window != "" {
		if v, err := strconv.Atoi(window); err == nil {
			cfg.Governance.RateLimit.WindowSeconds = v
		}
	}
	if timeout := os.Getenv("RPC_TIMEOUT_MS"); timeout != "" {
		if v, err := strconv.Atoi(timeout); err == nil {
			cfg.Governance.RPC.TimeoutMS = v
		}
	}
	if enabled := os.Getenv("RPC_CIRCUIT_BREAKER"); enabled != "" {
		if v, err := strconv.ParseBool(enabled); err == nil {
			cfg.Governance.RPC.CircuitBreaker = v
		}
	}
	if maxConnections := os.Getenv("RPC_MAX_CONNECTIONS"); maxConnections != "" {
		if v, err := strconv.Atoi(maxConnections); err == nil {
			cfg.Governance.RPC.MaxConnections = v
		}
	}
	if maxQPS := os.Getenv("RPC_MAX_QPS"); maxQPS != "" {
		if v, err := strconv.Atoi(maxQPS); err == nil {
			cfg.Governance.RPC.MaxQPS = v
		}
	}
	if timeout := os.Getenv("AGENT_RPC_TIMEOUT_MS"); timeout != "" {
		if v, err := strconv.Atoi(timeout); err == nil {
			cfg.Governance.AgentRPC.TimeoutMS = v
		}
	}
	if enabled := os.Getenv("AGENT_RPC_CIRCUIT_BREAKER"); enabled != "" {
		if v, err := strconv.ParseBool(enabled); err == nil {
			cfg.Governance.AgentRPC.CircuitBreaker = v
		}
	}
	if maxConnections := os.Getenv("AGENT_RPC_MAX_CONNECTIONS"); maxConnections != "" {
		if v, err := strconv.Atoi(maxConnections); err == nil {
			cfg.Governance.AgentRPC.MaxConnections = v
		}
	}
	if maxQPS := os.Getenv("AGENT_RPC_MAX_QPS"); maxQPS != "" {
		if v, err := strconv.Atoi(maxQPS); err == nil {
			cfg.Governance.AgentRPC.MaxQPS = v
		}
	}
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
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
