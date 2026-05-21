package midtrans

import (
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
)

// Notification is the relevant slice of Midtrans's webhook payload.
// Full payload has ~30 fields; we only persist what we need.
type Notification struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionID     string `json:"transaction_id"`
	TransactionStatus string `json:"transaction_status"` // "settlement" | "capture" | "deny" | "expire" | "cancel" | "failure" | "pending"
	FraudStatus       string `json:"fraud_status"`       // "accept" | "deny" | "challenge"
	PaymentType       string `json:"payment_type"`
}

// IsTerminalSuccess reports whether the notification represents a final paid
// outcome we should act on (emit `payment.paid.v1`).
//
// Midtrans's docs: for QRIS, a successful payment moves to "settlement" via
// the bank net; "capture" is the credit-card path. Both with fraud_status
// "accept" mean the money has been confirmed.
func (n *Notification) IsTerminalSuccess() bool {
	if n.FraudStatus == "deny" || n.FraudStatus == "challenge" {
		return false
	}
	return n.TransactionStatus == "settlement" || n.TransactionStatus == "capture"
}

// IsTerminalFailure reports whether this is a final-but-failed outcome we
// should mark FAILED and emit `payment.failed.v1`.
func (n *Notification) IsTerminalFailure() bool {
	switch n.TransactionStatus {
	case "deny", "expire", "cancel", "failure":
		return true
	}
	return false
}

// Verify compares the signature_key in the payload against
//
//	SHA-512(order_id + status_code + gross_amount + server_key)
//
// in constant time. Returns nil on match.
func Verify(payload []byte, serverKey string) error {
	var n Notification
	if err := json.Unmarshal(payload, &n); err != nil {
		return err
	}
	expected := computeSignature(n.OrderID, n.StatusCode, n.GrossAmount, serverKey)
	got, err := hex.DecodeString(n.SignatureKey)
	if err != nil {
		return ErrSignatureInvalid
	}
	want, _ := hex.DecodeString(expected)
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrSignatureInvalid
	}
	return nil
}

// ParseAndVerify is the convenient front door for the webhook handler:
// returns the parsed Notification when the signature matches, or an error.
// When serverKey is empty, signature check is skipped (dev-only — never set
// it empty in production).
func ParseAndVerify(payload []byte, serverKey string) (*Notification, error) {
	var n Notification
	if err := json.Unmarshal(payload, &n); err != nil {
		return nil, err
	}
	if serverKey == "" {
		return &n, nil
	}
	expected := computeSignature(n.OrderID, n.StatusCode, n.GrossAmount, serverKey)
	got, err := hex.DecodeString(n.SignatureKey)
	if err != nil {
		return nil, ErrSignatureInvalid
	}
	want, _ := hex.DecodeString(expected)
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return nil, ErrSignatureInvalid
	}
	return &n, nil
}

// computeSignature returns the lowercase-hex SHA-512 of the canonical string.
func computeSignature(orderID, statusCode, grossAmount, serverKey string) string {
	h := sha512.New()
	h.Write([]byte(orderID + statusCode + grossAmount + serverKey))
	return hex.EncodeToString(h.Sum(nil))
}

// ErrSignatureInvalid surfaces a tampered or wrong-key signature.
type sigErr struct{}

func (sigErr) Error() string { return "midtrans: signature_key mismatch" }

var ErrSignatureInvalid = sigErr{}
