package secrets

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncryptionAADTamperAndWrongKey(t *testing.T) {
	v, err := New(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	data, err := v.Seal("connection-1", map[string]string{"token": "sensitive-value"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("sensitive-value")) {
		t.Fatal("plaintext stored")
	}
	got, err := v.Open("connection-1", data)
	if err != nil || got["token"] != "sensitive-value" {
		t.Fatal(err)
	}
	if _, err = v.Open("connection-2", data); err == nil {
		t.Fatal("swapped records accepted")
	}
	data[len(data)-1] ^= 1
	if _, err = v.Open("connection-1", data); err == nil {
		t.Fatal("tamper accepted")
	}
	if _, err = New("CHANGE_ME"); err == nil {
		t.Fatal("invalid master key accepted")
	}
}
