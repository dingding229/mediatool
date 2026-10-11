package auth

import (
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/argon2"
	"strings"
	"testing"
)

func TestPasswordHashRandomSaltAndExactPassword(t *testing.T) {
	password := "  中文 media 密码  "
	a, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword(password)
	if err != nil || a == b {
		t.Fatal("salt reused")
	}
	if strings.Contains(a, password) || !VerifyPassword(a, password) || VerifyPassword(a, strings.TrimSpace(password)) || VerifyPassword(a, "another-long-password") {
		t.Fatal("password storage or exact comparison failed")
	}
}
func TestPasswordLimitsAndMalformedHash(t *testing.T) {
	for _, p := range []string{"", strings.Repeat("a", 5), strings.Repeat("a", 21), strings.Repeat("a", 20) + "\x00"} {
		if ValidatePassword(p) == nil {
			t.Fatal("weak/unbounded password accepted")
		}
	}
	for _, encoded := range []string{"plaintext", "$argon2id$v=19$m=999999999,t=2,p=1$a$a", DummyPasswordHash + "$extra"} {
		if VerifyPassword(encoded, "long-password-fixture") {
			t.Fatal("malformed hash accepted")
		}
	}
}

func TestPasswordInclusiveLengthBoundaries(t *testing.T) {
	for _, password := range []string{"abc123", strings.Repeat("a", 20), strings.Repeat("中", 6), strings.Repeat("中", 20)} {
		hash, err := HashPassword(password)
		if err != nil || !VerifyPassword(hash, password) {
			t.Fatal("valid boundary rejected", len([]rune(password)), err)
		}
	}
	for _, password := range []string{strings.Repeat("中", 5), strings.Repeat("中", 21)} {
		if _, err := HashPassword(password); err == nil {
			t.Fatal("invalid Unicode boundary accepted")
		}
	}
}
func TestExistingLongPasswordCanStillAuthenticate(t *testing.T) {
	password := "previous-strong-password-2026"
	salt := []byte("legacy-test-salt")
	key := argon2.IDKey([]byte(password), salt, 2, passwordMemory, 1, 32)
	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=2,p=1$%s$%s", passwordMemory, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
	if !VerifyPassword(encoded, password) {
		t.Fatal("existing account locked out by new length policy")
	}
	if _, err := HashPassword(password); err == nil {
		t.Fatal("new long password accepted despite changed policy")
	}
}
