package password

import "golang.org/x/crypto/bcrypt"

// HashPassword 对密码进行bcrypt哈希加密
// 使用bcrypt算法（自动加盐），cost等级为DefaultCost(10)
// 用于用户注册时存储加密后的密码
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword 校验明文密码与哈希密码是否匹配
// 用于用户登录时验证密码
// 返回true表示密码正确，false表示密码错误
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
