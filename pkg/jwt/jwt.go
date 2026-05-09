// Package jwt provides minimal RS256 JWT parsing using stdlib only.
// Signature verification is skipped when pubKeyPEM is empty (dev mode).
package jwt

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Claims holds the super-app JWT payload fields used by ParkirPintar.
type Claims struct {
	Sub   string `json:"sub"`   // external_user_id
	Phone string `json:"phone"` // E.164 MSISDN
	Exp   int64  `json:"exp"`
}

var (
	ErrMalformed = errors.New("malformed token")
	ErrExpired   = errors.New("token expired")
	ErrSignature = errors.New("invalid signature")
)

// Parse parses a raw JWT string and optionally verifies the RS256 signature.
// If pubKeyPEM is empty, signature verification is skipped (useful for local dev).
func Parse(token, pubKeyPEM string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrMalformed
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
		signingInput := parts[0] + "." + parts[1]
		if err := verifyRS256(signingInput, parts[2], pubKeyPEM); err != nil {
			return nil, err
		}
	}

	return &claims, nil
}

func verifyRS256(signingInput, sigB64, pubKeyPEM string) error {
	block, _ := pem.Decode([]byte(pubKeyPEM))
	if block == nil {
		return fmt.Errorf("%w: invalid PEM", ErrSignature)
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("%w: parse key: %v", ErrSignature, err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("%w: not RSA", ErrSignature)
	}

	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("%w: decode sig", ErrSignature)
	}

	h := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, h[:], sig); err != nil {
		return ErrSignature
	}

	return nil
}
