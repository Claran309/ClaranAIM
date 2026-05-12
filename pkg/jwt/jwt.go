package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt"
)

// Claims JWT Token的载荷（Payload）
// 包含用户身份信息和标准声明
type Claims struct {
	UserID   int64  `json:"user_id"`  // 用户ID
	Username string `json:"username"` // 用户名
	jwt.StandardClaims                  // JWT标准声明（过期时间、签发时间、签发者等）
}

// GenerateToken 生成JWT Token
// secretKey: 签名密钥
// userID: 用户ID
// username: 用户名
// expirationHours: Token有效期（小时）
// 返回签名的Token字符串
func GenerateToken(secretKey string, userID int64, username string, expirationHours int64) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: now.Add(time.Duration(expirationHours) * time.Hour).Unix(), // 过期时间
			IssuedAt:  now.Unix(),                                                 // 签发时间
			Issuer:    "ClaranAIM",                                                 // 签发者标识
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
