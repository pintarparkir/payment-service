package midtrans

import (
	"encoding/json"
	"errors"
	"testing"
)

const testServerKey = "SB-Mid-server-test-key"

// makeNotification builds a webhook payload with a correct signature for the
// given fields, mirroring what Midtrans sends.
func makeNotification(t *testing.T, status, transactionStatus, fraudStatus string) []byte {
	t.Helper()
	orderID := "ord-1"
	grossAmount := "10000.00"
	sig := computeSignature(orderID, status, grossAmount, testServerKey)
	body, _ := json.Marshal(Notification{
		OrderID:           orderID,
		StatusCode:        status,
		GrossAmount:       grossAmount,
		SignatureKey:      sig,
		TransactionID:     "txn-1",
		TransactionStatus: transactionStatus,
		FraudStatus:       fraudStatus,
		PaymentType:       "qris",
	})
	return body
}

func TestParseAndVerify_HappySettlement(t *testing.T) {
	body := makeNotification(t, "200", "settlement", "accept")
	n, err := ParseAndVerify(body, testServerKey)
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if !n.IsTerminalSuccess() {
		t.Errorf("settlement+accept must be terminal success")
	}
	if n.IsTerminalFailure() {
		t.Errorf("settlement is not failure")
	}
}

func TestParseAndVerify_TamperedSignatureRejected(t *testing.T) {
	body := makeNotification(t, "200", "settlement", "accept")
	// Flip one byte in the signature
	var n Notification
	_ = json.Unmarshal(body, &n)
	n.SignatureKey = "00" + n.SignatureKey[2:] // overwrite first byte
	tampered, _ := json.Marshal(n)

	_, err := ParseAndVerify(tampered, testServerKey)
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Errorf("expected ErrSignatureInvalid, got %v", err)
	}
}

func TestParseAndVerify_WrongServerKeyRejected(t *testing.T) {
	body := makeNotification(t, "200", "settlement", "accept")
	_, err := ParseAndVerify(body, "wrong-key")
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Errorf("expected ErrSignatureInvalid for wrong key, got %v", err)
	}
}

func TestParseAndVerify_EmptyKeySkipsCheck(t *testing.T) {
	// Even with a tampered signature, empty key bypasses verification (dev mode).
	var n Notification
	_ = json.Unmarshal(makeNotification(t, "200", "settlement", "accept"), &n)
	n.SignatureKey = "garbage"
	body, _ := json.Marshal(n)

	got, err := ParseAndVerify(body, "")
	if err != nil {
		t.Fatalf("dev-mode parse should not error: %v", err)
	}
	if got.OrderID != n.OrderID {
		t.Errorf("payload not parsed")
	}
}

func TestNotification_TerminalClassification(t *testing.T) {
	cases := []struct {
		txStatus  string
		fraud     string
		wantPaid  bool
		wantFail  bool
	}{
		{"settlement", "accept", true, false},
		{"capture", "accept", true, false},
		{"settlement", "deny", false, false}, // success blocked by fraud check; needs manual
		{"deny", "accept", false, true},
		{"expire", "accept", false, true},
		{"cancel", "accept", false, true},
		{"failure", "accept", false, true},
		{"pending", "accept", false, false}, // QRIS unscanned → keep PENDING
	}
	for _, tc := range cases {
		n := &Notification{TransactionStatus: tc.txStatus, FraudStatus: tc.fraud}
		if got := n.IsTerminalSuccess(); got != tc.wantPaid {
			t.Errorf("%s/%s IsTerminalSuccess: got %v want %v", tc.txStatus, tc.fraud, got, tc.wantPaid)
		}
		if got := n.IsTerminalFailure(); got != tc.wantFail {
			t.Errorf("%s/%s IsTerminalFailure: got %v want %v", tc.txStatus, tc.fraud, got, tc.wantFail)
		}
	}
}
