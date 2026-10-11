package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

var ErrInvalidKey = errors.New("APP_SECRET_KEY must be a 32-byte hexadecimal key")
var ErrDecrypt = errors.New("encrypted credentials cannot be opened; check the master key")

type Vault struct{ aead cipher.AEAD }

func New(raw string) (*Vault, error) {
	key, err := hex.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidKey
	}
	return &Vault{aead: aead}, nil
}
func (v *Vault) Seal(id string, values map[string]string) ([]byte, error) {
	payload, err := json.Marshal(values)
	if err != nil {
		return nil, errors.New("credential encoding failed")
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.New("credential encryption failed")
	}
	return v.aead.Seal(nonce, nonce, payload, []byte("media-tool:integration:v1:"+id)), nil
}
func (v *Vault) Open(id string, ciphertext []byte) (map[string]string, error) {
	if len(ciphertext) < v.aead.NonceSize() {
		return nil, ErrDecrypt
	}
	n := v.aead.NonceSize()
	plain, err := v.aead.Open(nil, ciphertext[:n], ciphertext[n:], []byte("media-tool:integration:v1:"+id))
	if err != nil {
		return nil, ErrDecrypt
	}
	var values map[string]string
	if json.Unmarshal(plain, &values) != nil {
		return nil, ErrDecrypt
	}
	return values, nil
}
