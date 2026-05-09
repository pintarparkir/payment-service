// Package midtrans is a thin HTTP client for the Midtrans Snap/Core API,
// scoped to the QRIS use-case the mini app needs.
//
// Two implementations of `Client`:
//   - httpClient — calls the real sandbox/production endpoint
//   - stubClient — returns deterministic synthetic responses for local dev
//
// The cmd/payment wiring picks one based on cfg.MidtransStubMode so we never
// hit the network in tests, and so a developer without sandbox credentials
// can still drive the full flow end-to-end.
package midtrans

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the slice of Midtrans we depend on. Adding new operations
// (cancel, status-query, refund) extends this interface — keep it minimal.
type Client interface {
	// Charge creates a QRIS transaction. `orderID` must be unique per intent
	// (idempotent on Midtrans's side too). Returns the QRIS payload string the
	// mini app will render as a QR code, plus Midtrans's transaction_id which
	// we persist as `pg_reference` for webhook reconciliation.
	Charge(ctx context.Context, orderID string, amountIDR int64) (*ChargeResult, error)
}

// ChargeResult mirrors the bits of Midtrans's /charge response we use.
type ChargeResult struct {
	TransactionID  string    // pg_reference
	QrisPayload    string    // EMV-QRIS string (starts with 00020101...)
	ExpiresAt      time.Time // 15 min from issue by default
}

// ── HTTP client ──────────────────────────────────────────────────────────────

type httpClient struct {
	baseURL   string
	serverKey string
	hc        *http.Client
}

// NewHTTPClient returns a Client that hits the real Midtrans endpoint.
func NewHTTPClient(baseURL, serverKey string) Client {
	return &httpClient{
		baseURL:   baseURL,
		serverKey: serverKey,
		hc:        &http.Client{Timeout: 10 * time.Second},
	}
}

// Request shape per Midtrans Core API /charge for QRIS.
type chargeReq struct {
	PaymentType        string             `json:"payment_type"`
	TransactionDetails transactionDetails `json:"transaction_details"`
	CustomExpiry       *customExpiry      `json:"custom_expiry,omitempty"`
	Qris               *qrisOpts          `json:"qris,omitempty"`
}
type transactionDetails struct {
	OrderID     string `json:"order_id"`
	GrossAmount int64  `json:"gross_amount"`
}
type customExpiry struct {
	Unit            string `json:"unit"`
	ExpiryDuration  int    `json:"expiry_duration"`
}
type qrisOpts struct {
	Acquirer string `json:"acquirer"`
}

// Response shape (subset — Midtrans returns ~25 fields).
type chargeResp struct {
	StatusCode    string   `json:"status_code"`
	StatusMessage string   `json:"status_message"`
	TransactionID string   `json:"transaction_id"`
	OrderID       string   `json:"order_id"`
	GrossAmount   string   `json:"gross_amount"`
	QRString      string   `json:"qr_string"`
	ExpiryTime    string   `json:"expiry_time"`
	Actions       []action `json:"actions"`
}
type action struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Method string `json:"method"`
}

func (c *httpClient) Charge(ctx context.Context, orderID string, amountIDR int64) (*ChargeResult, error) {
	body, _ := json.Marshal(chargeReq{
		PaymentType: "qris",
		TransactionDetails: transactionDetails{
			OrderID:     orderID,
			GrossAmount: amountIDR,
		},
		CustomExpiry: &customExpiry{Unit: "minute", ExpiryDuration: 15},
		Qris:         &qrisOpts{Acquirer: "gopay"},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/charge", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("midtrans: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// Midtrans Server Key auth: HTTP Basic with key as username, blank password.
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.serverKey+":")))

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("midtrans: charge: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("midtrans: read body: %w", err)
	}

	var r chargeResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("midtrans: parse: %w (body: %s)", err, raw)
	}
	// Midtrans uses string status codes — "201" for success on /charge.
	if resp.StatusCode >= 400 || (r.StatusCode != "201" && r.StatusCode != "200") {
		return nil, fmt.Errorf("midtrans: charge failed status=%s message=%s", r.StatusCode, r.StatusMessage)
	}

	exp, _ := time.Parse("2006-01-02 15:04:05", r.ExpiryTime)
	return &ChargeResult{
		TransactionID: r.TransactionID,
		QrisPayload:   r.QRString,
		ExpiresAt:     exp,
	}, nil
}

// ── Stub client (local dev) ──────────────────────────────────────────────────

type stubClient struct{}

// NewStubClient returns a Client that never calls the network. Returns
// deterministic synthetic data shaped like the real response.
func NewStubClient() Client { return &stubClient{} }

func (s *stubClient) Charge(_ context.Context, orderID string, amountIDR int64) (*ChargeResult, error) {
	return &ChargeResult{
		TransactionID: "STUB-" + orderID,
		QrisPayload:   fmt.Sprintf("00020101021226680022ID.STUB.WWW011893600914%d51440014ID.CO.QRIS.WWW0215ID20232896800280303UMI51440014ID.CO.QRIS.WWW0215ID20232896800280303UMI52044812530336054%010d5802ID5912PARKIRPINTAR6013JAKARTA SELATAN61051234062180514STUB-%s", amountIDR, amountIDR, orderID),
		ExpiresAt:     time.Now().Add(15 * time.Minute),
	}, nil
}
