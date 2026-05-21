// Package jwt provides minimal RS256 JWT parsing.
// Signature verification is skipped when pubKeyPEM is empty (dev mode).
package jwt

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Claims holds the super-app JWT payload fields used by ParkirPintar.
type Claims struct {
	Sub   string `json:"sub"`   // external_user_id
	Phone string `json:"phone"` // E.164 MSISDN
	Exp   int64  `json:"exp"`
}

// header carries the protected JOSE header. We only care about `alg` so we can
// reject algorithm-confusion attacks (e.g. `alg: none`, `alg: HS256`).
type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

var (
	ErrMalformed   = errors.New("malformed token")
	ErrExpired     = errors.New("token expired")
	ErrSignature   = errors.New("invalid signature")
	ErrUnsupported = errors.New("unsupported algorithm")
)

// expectedAlg is the only algorithm we accept. RS256 is mandated by the
// super-app upstream JWT issuer; rejecting anything else closes the
// `alg: none` and `alg: HS256` confusion classes.
const expectedAlg = "RS256"

// Parse parses a raw JWT string and optionally verifies the RS256 signature.
// If pubKeyPEM is empty, signature verification is skipped (useful for local dev).
func Parse(token, pubKeyPEM string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrMalformed
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: decode header", ErrMalformed)
	}
	var h header
	if unmarshalErr := json.Unmarshal(headerBytes, &h); unmarshalErr != nil {
		return nil, fmt.Errorf("%w: header json", ErrMalformed)
	}
	if h.Alg != expectedAlg {
		return nil, fmt.Errorf("%w: got %q, want %q", ErrUnsupported, h.Alg, expectedAlg)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: decode payload", ErrMalformed)
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("%w: unmarshal", ErrMalformed)
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, ErrExpired
	}

	if pubKeyPEM != "" {
		if err := verifyRS256(token, pubKeyPEM); err != nil {
			return nil, err
		}
	}

	return &claims, nil
}

func verifyRS256(tokenString, pubKeyPEM string) error {
	rsaPub, err := jwtlib.ParseRSAPublicKeyFromPEM([]byte(pubKeyPEM))
	if err != nil {
		return fmt.Errorf("%w: parse key: %w", ErrSignature, err)
	}

	parsed, err := jwtlib.Parse(tokenString, func(token *jwtlib.Token) (any, error) {
		if token.Method.Alg() != expectedAlg {
			return nil, fmt.Errorf("%w: got %q, want %q", ErrUnsupported, token.Method.Alg(), expectedAlg)
		}
		return rsaPub, nil
	}, jwtlib.WithValidMethods([]string{expectedAlg}), jwtlib.WithoutClaimsValidation())
	if err != nil {
		return ErrSignature
	}
	if parsed == nil || !parsed.Valid {
		return ErrSignature
	}

	return nil
}
