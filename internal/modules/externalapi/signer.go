package externalapi

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/golang-jwt/jwt/v5"
	"slices"
	"time"
)

const MaxTokenBytes = 2048
const signingInfo = "media-tool:external-api:jwt:hs256:v1"
const issuer = "media-tool:external-api"
const audience = "media-tool:external:v1"

// Field order is the v1 wire contract. Never embed map claims or a changing timestamp.
type Claims struct {
	ID       string   `json:"jti"`
	Scopes   []string `json:"scopes"`
	IssuedAt int64    `json:"iat"`
	Issuer   string   `json:"iss"`
	Audience string   `json:"aud"`
	Version  int      `json:"ver"`
}

func (c Claims) GetExpirationTime() (*jwt.NumericDate, error) { return nil, nil }
func (c Claims) GetNotBefore() (*jwt.NumericDate, error)      { return nil, nil }
func (c Claims) GetIssuedAt() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(c.IssuedAt, 0)), nil
}
func (c Claims) GetIssuer() (string, error)             { return c.Issuer, nil }
func (c Claims) GetSubject() (string, error)            { return "", nil }
func (c Claims) GetAudience() (jwt.ClaimStrings, error) { return jwt.ClaimStrings{c.Audience}, nil }

type Signer struct {
	key []byte
	Now func() time.Time
}

func NewSigner(master string) (*Signer, error) {
	root, e := hex.DecodeString(master)
	if e != nil || len(root) != 32 {
		return nil, ErrUnavailable
	}
	defer clear(root)
	key, e := hkdf.Key(sha256.New, root, nil, signingInfo, 32)
	if e != nil {
		return nil, ErrUnavailable
	}
	return &Signer{key: key, Now: time.Now}, nil
}
func (s *Signer) Sign(id string, scopes []string, at time.Time) (string, error) {
	if s == nil || len(s.key) != 32 {
		return "", ErrUnavailable
	}
	values := slices.Clone(scopes)
	slices.Sort(values)
	c := Claims{id, values, at.Unix(), issuer, audience, 1}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	t.Header["kid"] = "external-api-v1"
	return t.SignedString(s.key)
}
func (s *Signer) Verify(raw string) (Claims, error) {
	var c Claims
	if s == nil || len(s.key) != 32 {
		return c, ErrUnavailable
	}
	if len(raw) == 0 || len(raw) > MaxTokenBytes {
		return c, ErrUnauthorized
	}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuedAt(), jwt.WithLeeway(time.Minute), jwt.WithIssuer(issuer), jwt.WithAudience(audience), jwt.WithStrictDecoding(), jwt.WithTimeFunc(s.Now))
	token, e := parser.ParseWithClaims(raw, &c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrUnauthorized
		}
		return s.key, nil
	})
	if e != nil || !token.Valid || c.Version != 1 || c.IssuedAt <= 0 || len(c.Scopes) == 0 || len(c.Scopes) > len(Scopes) || !validID(c.ID) {
		return Claims{}, ErrUnauthorized
	}
	// Only canonical v1 tokens issued here are accepted. This also rejects unknown/
	// duplicate claims (including exp), extra headers and alternate serialization.
	canonical, e := s.Sign(c.ID, c.Scopes, time.Unix(c.IssuedAt, 0))
	if e != nil || !hmac.Equal([]byte(canonical), []byte(raw)) {
		return Claims{}, ErrUnauthorized
	}
	for i, v := range c.Scopes {
		if !Scopes[v] || (i > 0 && c.Scopes[i-1] >= v) {
			return Claims{}, ErrUnauthorized
		}
	}
	return c, nil
}
