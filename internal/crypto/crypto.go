package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// HashPassword hashes a password using scrypt
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	dk, err := scrypt.Key([]byte(password), salt, 32768, 8, 1, 64)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("scrypt:%s:%s",
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(dk)), nil
}

// VerifyPassword verifies a password against a stored hash
func VerifyPassword(password, stored string) bool {
	parts := strings.Split(stored, ":")
	if len(parts) != 3 || parts[0] != "scrypt" {
		return false
	}

	salt, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	storedHash, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	dk, err := scrypt.Key([]byte(password), salt, 32768, 8, 1, 64)
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare(dk, storedHash) == 1
}

// RandomToken generates a random token
func RandomToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// getAESKey derives a 32-byte key from the secret
func getAESKey(secret string) []byte {
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

// EncryptText encrypts plaintext using AES-256-GCM
func EncryptText(plaintext, secret string) (string, error) {
	key := getAESKey(secret)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	return fmt.Sprintf("aes256gcm:%s:%s",
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(ciphertext)), nil
}

// DecryptText decrypts ciphertext using AES-256-GCM
func DecryptText(encrypted, secret string) (string, error) {
	parts := strings.Split(encrypted, ":")
	if len(parts) != 3 || parts[0] != "aes256gcm" {
		return "", fmt.Errorf("不支持的加密算法")
	}

	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}

	key := getAESKey(secret)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
