package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt"
)

// 下面这组常量定义当前包使用的固定取值，集中声明可以避免业务代码中散落魔法字符串或魔法数字。
const (
	// RoleUser 是普通应用用户的默认系统角色。
	RoleUser = "user"
	// RoleAdmin 预留给未来 /api/v1/admin 管理接口做角色鉴权。
	RoleAdmin = "admin"

	// TokenTypeAccess 表示短期访问 token，用于 API 和 WebSocket 鉴权。
	TokenTypeAccess = "access"
	// TokenTypeRefresh 表示长期刷新 token，只允许用于换取新的 access token。
	TokenTypeRefresh = "refresh"

	defaultAccessExpirationHours  int64 = 2
	defaultRefreshExpirationHours int64 = 168
)

// Claims JWT Token的载荷（Payload）
// 包含用户身份信息和标准声明
type Claims struct {
	UserID    int64  `json:"user_id"`    // 用户ID
	Username  string `json:"username"`   // 用户名
	Role      string `json:"role"`       // 系统角色：user/admin
	TokenType string `json:"token_type"` // token 类型：access 表示访问令牌，refresh 表示刷新令牌
	jwt.StandardClaims
}

// GenerateToken 生成JWT Token
// secretKey: 签名密钥
// userID: 用户ID
// username: 用户名
// expirationHours: Token有效期（小时）
// 返回签名的Token字符串
func GenerateToken(secretKey string, userID int64, username string, expirationHours int64) (string, error) {
	return GenerateAccessToken(secretKey, userID, username, RoleUser, expirationHours)
}

// GenerateAccessToken 生成短期访问 JWT，载荷包含用户 ID、用户名和系统角色。
func GenerateAccessToken(secretKey string, userID int64, username, role string, expirationHours int64) (string, error) {
	return generateTypedToken(secretKey, userID, username, normalizeRole(role), TokenTypeAccess, expirationHours, defaultAccessExpirationHours)
}

// GenerateRefreshToken 生成长期刷新 JWT，只能用于续签 access token。
func GenerateRefreshToken(secretKey string, userID int64, username, role string, expirationHours int64) (string, error) {
	return generateTypedToken(secretKey, userID, username, normalizeRole(role), TokenTypeRefresh, expirationHours, defaultRefreshExpirationHours)
}

// generateTypedToken 按 token 类型生成带过期时间的 JWT，并在未配置有效期时使用默认值。
func generateTypedToken(secretKey string, userID int64, username, role, tokenType string, expirationHours, defaultHours int64) (string, error) {
	if expirationHours <= 0 {
		expirationHours = defaultHours
	}
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Username:  username,
		Role:      role,
		TokenType: tokenType,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: now.Add(time.Duration(expirationHours) * time.Hour).Unix(), // 过期时间
			IssuedAt:  now.Unix(),                                                 // 签发时间
			Issuer:    "ClaranAIM",                                                // 签发者标识
		},
	}

	// 使用HS256算法签名
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

// ParseToken 解析并验证JWT Token
// tokenString: 待解析的Token字符串
// 返回解析后的Claims和验证结果
// 验证失败的情况：Token过期、签名不匹配、格式错误等
func ParseToken(tokenString string) (*Claims, error) {
	return ParseAccessToken(tokenString)
}

// ParseAccessToken 校验普通请求鉴权使用的 access token。
func ParseAccessToken(tokenString string) (*Claims, error) {
	claims, err := parseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != "" && claims.TokenType != TokenTypeAccess {
		return nil, fmt.Errorf("invalid token type: %s", claims.TokenType)
	}
	claims.Role = normalizeRole(claims.Role)
	claims.TokenType = TokenTypeAccess
	return claims, nil
}

// ParseRefreshToken 校验 refresh token，并拒绝误传的 access token。
func ParseRefreshToken(tokenString string) (*Claims, error) {
	claims, err := parseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeRefresh {
		return nil, fmt.Errorf("invalid token type: %s", claims.TokenType)
	}
	claims.Role = normalizeRole(claims.Role)
	return claims, nil
}

// parseToken 使用进程内签名密钥解析并校验 JWT 签名。
func parseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(GetSecretKey()), nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// jwtSecretKey 全局JWT密钥
// 在服务启动时通过 SetSecretKey 设置
var jwtSecretKey string

// 下面这组变量保存当前包需要复用的运行时状态或配置入口，调用方应通过公开函数间接使用。
var (
	accessExpirationHours  = defaultAccessExpirationHours
	refreshExpirationHours = defaultRefreshExpirationHours
)

// SetSecretKey 设置JWT签名密钥
// 在服务启动的main函数中调用，从配置中读取密钥
func SetSecretKey(key string) {
	jwtSecretKey = key
}

// GetSecretKey 获取JWT签名密钥
// ParseToken解析Token时使用此密钥验证签名
func GetSecretKey() string {
	return jwtSecretKey
}

// SetTokenExpirations 更新当前进程内 access/refresh token 的默认有效期。
func SetTokenExpirations(accessHours, refreshHours int64) {
	if accessHours <= 0 {
		accessHours = defaultAccessExpirationHours
	}
	if refreshHours <= 0 {
		refreshHours = defaultRefreshExpirationHours
	}
	accessExpirationHours = accessHours
	refreshExpirationHours = refreshHours
}

// GetAccessExpirationHours 返回当前 access token 默认有效期，单位小时。
func GetAccessExpirationHours() int64 {
	return accessExpirationHours
}

// GetRefreshExpirationHours 返回当前 refresh token 默认有效期，单位小时。
func GetRefreshExpirationHours() int64 {
	return refreshExpirationHours
}

// normalizeRole 将未知角色降级为普通用户，避免 token 中出现未授权角色字符串。
func normalizeRole(role string) string {
	if role == RoleAdmin {
		return RoleAdmin
	}
	return RoleUser
}
