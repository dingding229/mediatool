package externalapi

import (
	"encoding/base64"
	"github.com/golang-jwt/jwt/v5"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const fixtureID = "11111111-2222-4333-8444-555555555555"

func fixedSignature(t *testing.T) string {
	t.Helper()
	s, e := NewSigner(strings.Repeat("a", 64))
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.Sign(fixtureID, []string{"tasks.read", "status.read"}, time.Unix(1700000000, 0))
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestT15SignerRestartHelper(t *testing.T) {
	if os.Getenv("T15_SIGNER_HELPER") != "1" {
		return
	}
	os.Stdout.WriteString(fixedSignature(t) + "\n")
}
func TestT15CanonicalClaimsAndRestartByteEquality(t *testing.T) {
	a := fixedSignature(t)
	if a != fixedSignature(t) {
		t.Fatal("same claims changed bytes")
	}
	payload, e := base64.RawURLEncoding.DecodeString(strings.Split(a, ".")[1])
	if e != nil || string(payload) != `{"jti":"11111111-2222-4333-8444-555555555555","scopes":["status.read","tasks.read"],"iat":1700000000,"iss":"media-tool:external-api","aud":"media-tool:external:v1","ver":1}` {
		t.Fatal("canonical field order changed")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestT15SignerRestartHelper$")
	cmd.Env = append(os.Environ(), "T15_SIGNER_HELPER=1")
	raw, e := cmd.Output()
	if e != nil || strings.Split(string(raw), "\n")[0] != a {
		t.Fatal("process restart changed signature", e)
	}
}
func TestT15AlgorithmTamperLengthAndPurposeRejection(t *testing.T) {
	s, _ := NewSigner(strings.Repeat("a", 64))
	valid := fixedSignature(t)
	if _, e := s.Verify(valid); e != nil {
		t.Fatal(e)
	}
	c := Claims{fixtureID, []string{"status.read"}, 1700000000, issuer, audience, 1}
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodNone, jwt.SigningMethodHS384, jwt.SigningMethodHS512} {
		tok := jwt.NewWithClaims(method, c)
		tok.Header["kid"] = "external-api-v1"
		var key any = s.key
		if method == jwt.SigningMethodNone {
			key = jwt.UnsafeAllowNoneSignatureType
		}
		v, e := tok.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Verify(v); e == nil {
			t.Fatal("wrong alg accepted", method.Alg())
		}
	}
	for _, raw := range []string{strings.Repeat("a", 2049), "mt_api_" + strings.Repeat("a", 64), valid[:len(valid)-4] + "abcd"} {
		if _, e := s.Verify(raw); e == nil {
			t.Fatal("tampered credential accepted")
		}
	}
	wrong, _ := NewSigner(strings.Repeat("b", 64))
	if _, e := wrong.Verify(valid); e == nil {
		t.Fatal("different master accepted")
	}
	c.Audience = "media-user"
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	tok.Header["kid"] = "external-api-v1"
	raw, _ := tok.SignedString(s.key)
	if _, e := s.Verify(raw); e == nil {
		t.Fatal("cross-purpose accepted")
	}
}
