package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/crypto/argon2"
	"strings"
	"unicode/utf8"
)

const PasswordMinLength = 6
const PasswordMaxLength = 20
const passwordMemory = 19 * 1024

// PHC carries the algorithm, bounded parameters and a random per-password salt.
func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || n < PasswordMinLength || n > PasswordMaxLength || len(password) > 512 || strings.ContainsRune(password, '\x00') {
		return errors.New("密码需6–20个字符，可使用空格和中文；不会自动去除首尾空格")
	}
	return nil
}
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 2, passwordMemory, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=2,p=1$%s$%s", passwordMemory, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func VerifyPassword(encoded, password string) bool {
	if len(password) > 512 || !utf8.ValidString(password) {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != fmt.Sprintf("m=%d,t=2,p=1", passwordMemory) {
		return false
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(expected) != 32 {
		return false
	}
	derived := argon2.IDKey([]byte(password), salt, 2, passwordMemory, 1, 32)
	return subtle.ConstantTimeCompare(derived, expected) == 1
}

// Unknown accounts perform the same password work, without producing real
// sample credentials or varying the response based on account existence.
var DummyPasswordHash = fmt.Sprintf("$argon2id$v=19$m=%d,t=2,p=1$%s$%s", passwordMemory, base64.RawStdEncoding.EncodeToString(make([]byte, 16)), base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
