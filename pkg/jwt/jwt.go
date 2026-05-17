package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt"
)

const (
	// RoleUser is the default role for normal application users.
	RoleUser = "user"
	// RoleAdmin grants access to future admin routes under /api/v1/admin.
	RoleAdmin = "admin"

	// TokenTypeAccess identifies short-lived tokens used for API/WebSocket auth.
	TokenTypeAccess = "access"
	// TokenTypeRefresh identifies long-lived tokens used only for renewal.
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
	TokenType string `json:"token_type"` // access/refresh
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

// GenerateAccessToken creates a short-lived JWT containing user ID, username and role.
func GenerateAccessToken(secretKey string, userID int64, username, role string, expirationHours int64) (string, error) {
	return generateTypedToken(secretKey, userID, username, normalizeRole(role), TokenTypeAccess, expirationHours, defaultAccessExpirationHours)
}

// GenerateRefreshToken creates a long-lived JWT that can only be used to renew access tokens.
func GenerateRefreshToken(secretKey string, userID int64, username, role string, expirationHours int64) (string, error) {
	return generateTypedToken(secretKey, userID, username, normalizeRole(role), TokenTypeRefresh, expirationHours, defaultRefreshExpirationHours)
}

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

// ParseAccessToken validates a token intended for normal request authorization.
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

// ParseRefreshToken validates a refresh token and rejects access tokens.
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

// SetTokenExpirations updates process-wide access/refresh expiration defaults.
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

// GetAccessExpirationHours returns the current access-token lifetime in hours.
func GetAccessExpirationHours() int64 {
	return accessExpirationHours
}

// GetRefreshExpirationHours returns the current refresh-token lifetime in hours.
func GetRefreshExpirationHours() int64 {
	return refreshExpirationHours
}

func normalizeRole(role string) string {
	if role == RoleAdmin {
		return RoleAdmin
	}
	return RoleUser
}
