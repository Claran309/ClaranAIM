package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient Redis客户端封装
// 对 go-redis/v9 客户端的二次封装，提供更简洁的API
// 支持字符串操作、JSON序列化存储、集合操作、哈希操作、发布订阅等
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient 创建Redis客户端连接
// addr: Redis地址（如 localhost:6379）
// password: Redis密码（无密码传空字符串）
// db: Redis数据库编号（默认0）
// 连接参数：拨号超时5秒、读写超时3秒、连接池大小10、最小空闲连接5
func NewRedisClient(addr, password string, db int) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
	})

	// 测试连接是否可用
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis连接失败: %w", err)
	}

	return &RedisClient{client: client}, nil
}

func (r *RedisClient) GetInnerClient() *redis.Client {
	return r.client
}

func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	result, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return result, err
}

// Set 设置字符串值（带过期时间）
func (r *RedisClient) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

// GetJSON 获取JSON值并反序列化
// 返回值：原始JSON字符串（用于判断缓存是否存在）和反序列化错误
// key不存在时返回空字符串和nil
func (r *RedisClient) GetJSON(ctx context.Context, key string, dest interface{}) (string, error) {
	val, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	if val == "" {
		return "", nil
	}
	return val, json.Unmarshal([]byte(val), dest)
}

// SetJSON 将对象序列化为JSON并存储
// 自动进行JSON序列化，设置过期时间
func (r *RedisClient) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.Set(ctx, key, string(data), expiration)
}

// Del 删除一个或多个键
func (r *RedisClient) Del(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

// Exists 检查键是否存在
func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// Expire 设置键的过期时间
func (r *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.client.Expire(ctx, key, expiration).Err()
}

// Incr 自增计数器
// 常用于限流、计数等场景
func (r *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}

// SAdd 向集合添加成员
// 常用于存储好友列表、群组成员等
func (r *RedisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return r.client.SAdd(ctx, key, members...).Err()
}

// SRem 从集合移除成员
func (r *RedisClient) SRem(ctx context.Context, key string, members ...interface{}) error {
	return r.client.SRem(ctx, key, members...).Err()
}

// SMembers 获取集合的所有成员
func (r *RedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return r.client.SMembers(ctx, key).Result()
}

// SIsMember 检查成员是否在集合中
func (r *RedisClient) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return r.client.SIsMember(ctx, key, member).Result()
}

// HSet 设置哈希字段值
// 常用于存储用户信息等结构化数据
func (r *RedisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	return r.client.HSet(ctx, key, values...).Err()
}

// HGet 获取哈希字段值
// 字段不存在时返回空字符串和nil
func (r *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	result, err := r.client.HGet(ctx, key, field).Result()
	if err == redis.Nil {
		return "", nil
	}
	return result, err
}

// HGetAll 获取哈希的所有字段和值
func (r *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, key).Result()
}

// HDel 删除哈希的一个或多个字段
func (r *RedisClient) HDel(ctx context.Context, key string, fields ...string) error {
	return r.client.HDel(ctx, key, fields...).Err()
}

// Publish 发布消息到频道
// 用于Redis Pub/Sub模式的消息发布
func (r *RedisClient) Publish(ctx context.Context, channel string, message interface{}) error {
	return r.client.Publish(ctx, channel, message).Err()
}

// Subscribe 订阅一个或多个频道
// 返回PubSub对象，需要调用方持续接收消息
func (r *RedisClient) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return r.client.Subscribe(ctx, channels...)
}

// Close 关闭Redis连接
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// GetClient 获取底层go-redis客户端
// 用于需要使用原生Redis命令的场景
func (r *RedisClient) GetClient() *redis.Client {
	return r.client
}
