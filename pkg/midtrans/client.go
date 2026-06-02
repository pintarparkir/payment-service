// Package midtrans is a thin HTTP client for the Midtrans SNAP API,
// scoped to the QRIS checkout use-case the mini app needs.
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

	"github.com/sony/gobreaker"
)

// Client is the slice of Midtrans we depend on. Adding new operations
// (cancel, status-query, refund) extends this interface — keep it minimal.
type Client interface {
	// Charge creates a SNAP transaction for QRIS checkout. `orderID` must be
	// unique per intent. Returns Midtrans transaction/reference data the mini app
	// can use to continue payment.
	Charge(ctx context.Context, orderID string, amountIDR int64) (*ChargeResult, error)
}

// ChargeResult mirrors the bits of Midtrans SNAP response we use.
type ChargeResult struct {
	TransactionID string    // pg_reference / order reference correlation
	QrisPayload   string    // backward-compatible field; populated with RedirectURL when using SNAP
	RedirectURL   string    // SNAP redirect URL for checkout
	SnapToken     string    // SNAP token for embedded/web checkout if needed later
	ExpiresAt     time.Time // 15 min from issue by default
}

// ── HTTP client ──────────────────────────────────────────────────────────────

type httpClient struct {
	baseURL   string
	serverKey string
	hc        *http.Client
	breaker   *gobreaker.CircuitBreaker
}

// NewHTTPClient returns a Client that hits the real Midtrans endpoint.
func NewHTTPClient(baseURL, serverKey string) Client {
	return &httpClient{
		baseURL:   baseURL,
		serverKey: serverKey,
		hc:        &http.Client{Timeout: 10 * time.Second},
		breaker:   newBreaker("midtrans-charge"),
	}
}

// SNAP request shape per Midtrans SNAP API /snap/v1/transactions.
type snapChargeReq struct {
	TransactionDetails transactionDetails `json:"transaction_details"`
	CustomerInfo       *customerInfo      `json:"customer_info,omitempty"`
	EnablePayments     []string           `json:"enable_payments,omitempty"`
	ItemDetails        []itemDetail       `json:"item_details,omitempty"`
}

type transactionDetails struct {
	OrderID     string `json:"order_id"`
	GrossAmount int64  `json:"gross_amount"`
}

type customerInfo struct {
	FirstName string `json:"first_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
}

type itemDetail struct {
	Name        string `json:"name"`
	Price       int64  `json:"price"`
	Quantity    int    `json:"quantity"`
	PaymentType string `json:"payment_type,omitempty"` // "qris" hint
}

// Response shape (subset — Midtrans SNAP response).
type snapResp struct {
	StatusCode    string `json:"status_code"`
	StatusMessage string `json:"status_message"`
	OrderID       string `json:"order_id"`
	GrossAmount   string `json:"gross_amount"`
	RedirectURL   string `json:"redirect_url"`
	Token         string `json:"token"`
}

func (c *httpClient) Charge(ctx context.Context, orderID string, amountIDR int64) (*ChargeResult, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.charge(ctx, orderID, amountIDR)
	})
	if err != nil {
		return nil, err
	}
	return result.(*ChargeResult), nil
}

func (c *httpClient) charge(ctx context.Context, orderID string, amountIDR int64) (*ChargeResult, error) {
	body, err := json.Marshal(snapChargeReq{
		TransactionDetails: transactionDetails{
			OrderID:     orderID,
			GrossAmount: amountIDR,
		},
		CustomerInfo: &customerInfo{
			FirstName: "Parking User",
			Email:     "user@example.com",
			Phone:     "+628000000000",
		},
		ItemDetails: []itemDetail{{
			Name:        "Booking Fee",
			Price:       5000,
			Quantity:    1,
			PaymentType: "qris",
		}},
		EnablePayments: []string{"qris"}, // QRIS only
	})
	if err != nil {
		return nil, fmt.Errorf("midtrans: marshal request: %w", err)
	}

	// SNAP endpoint is /snap/v1/transactions, not Core /charge
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/snap/v1/transactions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("midtrans: build SNAP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.serverKey+":")))

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("midtrans: charge: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("midtrans: read body: %w", err)
	}

	var r snapResp
	if unmarshalErr := json.Unmarshal(raw, &r); unmarshalErr != nil {
		return nil, fmt.Errorf("midtrans: parse SNAP: %w (body: %s)", unmarshalErr, raw)
	}
	// SNAP status "200" = success on /snap/v1/transactions
	if resp.StatusCode >= 400 || r.StatusCode != "200" {
		return nil, fmt.Errorf("midtrans: SNAP failed status=%s message=%s order_id=%s", r.StatusCode, r.StatusMessage, r.OrderID)
	}

	return &ChargeResult{
		SnapToken:     r.Token,                          // Can be used for embedded checkout
		RedirectURL:   r.RedirectURL,                    // Mini app redirects user here to complete payment
		TransactionID: r.OrderID,                        // Use as pg_reference
		ExpiresAt:     time.Now().Add(15 * time.Minute), // Default timeout
	}, nil
}

// ── Stub client (local dev) ──────────────────────────────────────────────────

type stubClient struct{}

// NewStubClient returns a Client that never calls the network. Returns
// deterministic synthetic data shaped like the real response.
func NewStubClient() Client { return &stubClient{} }

func (s *stubClient) Charge(_ context.Context, orderID string, amountIDR int64) (*ChargeResult, error) {
	return &ChargeResult{
		SnapToken:     "STUB-TOKEN-" + orderID,
		RedirectURL:   "https://app.midtrans.com/snap/v1/transactions/" + orderID + "/pay",
		TransactionID: "STUB-" + orderID,
		QrisPayload:   fmt.Sprintf("00020101021234567890%010d5802ID5912PARKIRPINTAR6013JAKARTA62150x%010d6304", amountIDR, amountIDR),
		ExpiresAt:     time.Now().Add(15 * time.Minute),
	}, nil
}
