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
	if err := json.Unmarshal(headerBytes, &h); err != nil {
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
	// RFC 7518 §3.3 mandates RSASSA-PKCS1-v1_5 for `alg: RS256`. The
	// Bleichenbacher attack only applies to PKCS1v15 *encryption* — signature
	// verification is unaffected. Algorithm is locked to RS256 above so
	// alg-confusion attacks (e.g. `alg: none`, `alg: HS256`) are rejected
	// before we reach this point. Algorithm choice is dictated by the upstream
	// super-app JWT issuer and cannot be changed unilaterally. NOSONAR
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, h[:], sig); err != nil { //nolint:gosec // NOSONAR
		return ErrSignature
	}

	return nil
}
