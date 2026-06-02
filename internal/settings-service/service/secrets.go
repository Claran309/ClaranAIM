package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const encryptedSecretPrefix = "enc:v1:"

// secretCodec 负责 settings-service 内部敏感字段的可逆保护。
// 数据库只保存密文；调用 runtime、mcp-gateway 等内部服务时再在 service 层解密。
type secretCodec interface {
	Encrypt(plain string) (string, error)
	Decrypt(stored string) (string, error)
}

type noopSecretCodec struct{}

func (noopSecretCodec) Encrypt(plain string) (string, error) {
	return plain, nil
}

func (noopSecretCodec) Decrypt(stored string) (string, error) {
	return stored, nil
}

type aesGCMSecretCodec struct {
	aead cipher.AEAD
}

// newAESGCMSecretCodec 使用 SHA-256 派生 32 字节 AES key。
// 这里不是 KMS，只是本地部署可用的应用层加密；生产环境后续可替换为 KMS/Secret Store。
func newAESGCMSecretCodec(secret string) (secretCodec, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return noopSecretCodec{}, nil
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aesGCMSecretCodec{aead: aead}, nil
}

func (c aesGCMSecretCodec) Encrypt(plain string) (string, error) {
	if strings.TrimSpace(plain) == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := c.aead.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return encryptedSecretPrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

func (c aesGCMSecretCodec) Decrypt(stored string) (string, error) {
	if strings.TrimSpace(stored) == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, encryptedSecretPrefix) {
		return stored, nil
	}
	raw := strings.TrimPrefix(stored, encryptedSecretPrefix)
	payload, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	nonceSize := c.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", errors.New("加密密钥内容损坏")
	}
	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plain, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("解密密钥失败，请检查SETTINGS_SECRET_KEY是否与保存时一致")
	}
	return string(plain), nil
}
