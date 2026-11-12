package integrations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrGatewayNotConfigured indicates a production online payment request without configured gateway credentials.
	ErrGatewayNotConfigured = errors.New("payment gateway credentials are not configured")
)

// CheckoutRequest describes invoice details passed to a payment gateway adapter.
type CheckoutRequest struct {
	OrderID           string
	AmountIDR         int64
	Method            string
	PurposeLabel      string
	CustomerName      string
	CustomerEmail     string
	CustomerPhone     string
	ServerKeyOverride string
	SandboxOverride   *bool
}

// CheckoutResult holds provider checkout metadata persisted with the invoice.
type CheckoutResult struct {
	Provider    string
	PaymentURL  string
	ProviderRef string
}

// TransactionStatusResult captures normalized authoritative payment status from gateway API.
type TransactionStatusResult struct {
	OrderID           string
	TransactionStatus string
	FraudStatus       string
	NormalizedStatus  string
	GrossAmountIDR    int64
	PaymentType       string
	RawPayload        map[string]any
}

// PaymentGateway generates checkout URLs and queries authoritative status from the payment provider.
type PaymentGateway interface {
	CreateCheckout(ctx context.Context, req CheckoutRequest) (CheckoutResult, error)
	GetTransactionStatus(ctx context.Context, orderID string, serverKeyOverride string, sandboxOverride *bool) (TransactionStatusResult, error)
}

// NewPaymentGateway selects Midtrans Snap, manual-only, or dev mock checkout behavior based on config.
func NewPaymentGateway(provider, midtransServerKey string, sandbox bool) PaymentGateway {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "midtrans_snap", "midtrans":
		return &MidtransSnapGateway{
			ServerKey: strings.TrimSpace(midtransServerKey),
			Sandbox:   sandbox,
			Client:    &http.Client{Timeout: 10 * time.Second},
		}
	case "manual":
		return &ManualOnlyPaymentGateway{}
	default:
		if strings.TrimSpace(midtransServerKey) != "" && provider == "" {
			return &MidtransSnapGateway{
				ServerKey: strings.TrimSpace(midtransServerKey),
				Sandbox:   sandbox,
				Client:    &http.Client{Timeout: 10 * time.Second},
			}
		}
		return &MockPaymentGateway{}
	}
}

// MockPaymentGateway produces simulated checkout URLs strictly for local unit tests / offline dev.
type MockPaymentGateway struct{}

func (g *MockPaymentGateway) CreateCheckout(_ context.Context, req CheckoutRequest) (CheckoutResult, error) {
	return CheckoutResult{
		Provider:   "mock",
		PaymentURL: fmt.Sprintf("https://pay.spmb-platform.local/checkout/%s?method=%s", req.OrderID, req.Method),
	}, nil
}

func (g *MockPaymentGateway) GetTransactionStatus(_ context.Context, orderID string, _ string, _ *bool) (TransactionStatusResult, error) {
	return TransactionStatusResult{
		OrderID:          orderID,
		NormalizedStatus: "pending",
	}, nil
}

// ManualOnlyPaymentGateway disables automatic online channels when only manual transfer is configured.
type ManualOnlyPaymentGateway struct{}

func (g *ManualOnlyPaymentGateway) CreateCheckout(_ context.Context, _ CheckoutRequest) (CheckoutResult, error) {
	return CheckoutResult{}, ErrGatewayNotConfigured
}

func (g *ManualOnlyPaymentGateway) GetTransactionStatus(_ context.Context, _ string, _ string, _ *bool) (TransactionStatusResult, error) {
	return TransactionStatusResult{}, ErrGatewayNotConfigured
}

// MidtransSnapGateway creates live Midtrans Snap transactions and queries Midtrans Core Status API over Basic Auth.
type MidtransSnapGateway struct {
	EndpointURL       string
	StatusEndpointFmt string
	ServerKey         string
	Sandbox           bool
	Client            *http.Client
}

func (g *MidtransSnapGateway) resolveConfig(serverKeyOverride string, sandboxOverride *bool) (serverKey string, snapURL string, statusBaseURL string, err error) {
	serverKey = strings.TrimSpace(serverKeyOverride)
	if serverKey == "" {
		serverKey = strings.TrimSpace(g.ServerKey)
	}
	if serverKey == "" {
		return "", "", "", ErrGatewayNotConfigured
	}

	sandbox := g.Sandbox
	if sandboxOverride != nil {
		sandbox = *sandboxOverride
	}

	snapURL = strings.TrimSpace(g.EndpointURL)
	if snapURL == "" {
		if sandbox {
			snapURL = "https://app.sandbox.midtrans.com/snap/v1/transactions"
		} else {
			snapURL = "https://app.midtrans.com/snap/v1/transactions"
		}
	}

	statusBaseURL = strings.TrimSpace(g.StatusEndpointFmt)
	if statusBaseURL == "" {
		if sandbox {
			statusBaseURL = "https://api.sandbox.midtrans.com/v2/%s/status"
		} else {
			statusBaseURL = "https://api.midtrans.com/v2/%s/status"
		}
	}
	return serverKey, snapURL, statusBaseURL, nil
}

func (g *MidtransSnapGateway) CreateCheckout(ctx context.Context, req CheckoutRequest) (CheckoutResult, error) {
	if req.AmountIDR <= 0 || strings.TrimSpace(req.OrderID) == "" {
		return CheckoutResult{}, fmt.Errorf("invalid checkout order or amount")
	}

	serverKey, snapURL, _, err := g.resolveConfig(req.ServerKeyOverride, req.SandboxOverride)
	if err != nil {
		return CheckoutResult{}, err
	}

	var enabledPayments []string
	switch strings.ToLower(req.Method) {
	case "qris":
		enabledPayments = []string{"other_qris", "gopay"}
	case "va":
		enabledPayments = []string{"bca_va", "bni_va", "bri_va", "permata_va", "echannel", "cimb_va"}
	case "ewallet":
		enabledPayments = []string{"gopay", "shopeepay"}
	}

	customer := map[string]any{
		"first_name": req.CustomerName,
	}
	if strings.TrimSpace(req.CustomerEmail) != "" {
		customer["email"] = strings.TrimSpace(req.CustomerEmail)
	}
	if strings.TrimSpace(req.CustomerPhone) != "" {
		customer["phone"] = strings.TrimSpace(req.CustomerPhone)
	}

	body := map[string]any{
		"transaction_details": map[string]any{
			"order_id":     req.OrderID,
			"gross_amount": req.AmountIDR,
		},
		"customer_details": customer,
		"item_details": []map[string]any{
			{
				"id":       req.OrderID,
				"price":    req.AmountIDR,
				"quantity": 1,
				"name":     truncateMidtransItemName(req.PurposeLabel),
			},
		},
		"expiry": map[string]any{
			"unit":     "hours",
			"duration": 24,
		},
	}
	if len(enabledPayments) > 0 {
		body["enabled_payments"] = enabledPayments
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return CheckoutResult{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, snapURL, bytes.NewReader(raw))
	if err != nil {
		return CheckoutResult{}, err
	}
	authVal := base64.StdEncoding.EncodeToString([]byte(serverKey + ":"))
	httpReq.Header.Set("Authorization", "Basic "+authVal)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("midtrans snap request: %w", err)
	}
	defer resp.Body.Close()

	respRaw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CheckoutResult{}, fmt.Errorf("midtrans snap HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respRaw)))
	}

	var parsed struct {
		Token       string `json:"token"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.Unmarshal(respRaw, &parsed); err != nil {
		return CheckoutResult{}, fmt.Errorf("decode midtrans snap response: %w", err)
	}
	if parsed.RedirectURL == "" {
		return CheckoutResult{}, fmt.Errorf("midtrans snap returned empty redirect_url")
	}

	return CheckoutResult{
		Provider:    "midtrans",
		PaymentURL:  parsed.RedirectURL,
		ProviderRef: parsed.Token,
	}, nil
}

// GetTransactionStatus calls Midtrans Core v2 Status API to actively verify transaction settlement state.
func (g *MidtransSnapGateway) GetTransactionStatus(ctx context.Context, orderID string, serverKeyOverride string, sandboxOverride *bool) (TransactionStatusResult, error) {
	if strings.TrimSpace(orderID) == "" {
		return TransactionStatusResult{}, fmt.Errorf("order_id is required")
	}
	serverKey, _, statusFmt, err := g.resolveConfig(serverKeyOverride, sandboxOverride)
	if err != nil {
		return TransactionStatusResult{}, err
	}

	targetURL := statusFmt
	if strings.Contains(statusFmt, "%s") {
		targetURL = fmt.Sprintf(statusFmt, orderID)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return TransactionStatusResult{}, err
	}
	authVal := base64.StdEncoding.EncodeToString([]byte(serverKey + ":"))
	httpReq.Header.Set("Authorization", "Basic "+authVal)
	httpReq.Header.Set("Accept", "application/json")

	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return TransactionStatusResult{}, fmt.Errorf("midtrans status request: %w", err)
	}
	defer resp.Body.Close()

	respRaw, _ := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if resp.StatusCode == http.StatusNotFound {
		return TransactionStatusResult{
			OrderID:          orderID,
			NormalizedStatus: "pending",
		}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TransactionStatusResult{}, fmt.Errorf("midtrans status HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respRaw)))
	}

	var parsed struct {
		OrderID           string `json:"order_id"`
		TransactionStatus string `json:"transaction_status"`
		FraudStatus       string `json:"fraud_status"`
		GrossAmount       string `json:"gross_amount"`
		PaymentType       string `json:"payment_type"`
		StatusCode        string `json:"status_code"`
	}
	if err := json.Unmarshal(respRaw, &parsed); err != nil {
		return TransactionStatusResult{}, fmt.Errorf("decode midtrans status: %w", err)
	}
	var rawMap map[string]any
	_ = json.Unmarshal(respRaw, &rawMap)

	amountIDR, _ := ParseMidtransGrossAmountIDR(parsed.GrossAmount)
	return TransactionStatusResult{
		OrderID:           firstNonEmpty(parsed.OrderID, orderID),
		TransactionStatus: parsed.TransactionStatus,
		FraudStatus:       parsed.FraudStatus,
		NormalizedStatus:  MapMidtransStatus(parsed.TransactionStatus, parsed.FraudStatus),
		GrossAmountIDR:    amountIDR,
		PaymentType:       parsed.PaymentType,
		RawPayload:        rawMap,
	}, nil
}

// VerifyMidtransSignature validates Midtrans official SHA512(order_id + status_code + gross_amount + ServerKey).
func VerifyMidtransSignature(orderID, statusCode, grossAmount, serverKey, signatureKey string) bool {
	if strings.TrimSpace(orderID) == "" ||
		strings.TrimSpace(statusCode) == "" ||
		strings.TrimSpace(grossAmount) == "" ||
		strings.TrimSpace(serverKey) == "" ||
		strings.TrimSpace(signatureKey) == "" {
		return false
	}
	sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + strings.TrimSpace(serverKey)))
	expected := hex.EncodeToString(sum[:])
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(signatureKey))), []byte(expected))
}

// MapMidtransStatus maps official Midtrans transaction_status and fraud_status values to internal payment_status.
func MapMidtransStatus(transactionStatus, fraudStatus string) string {
	tx := strings.ToLower(strings.TrimSpace(transactionStatus))
	fraud := strings.ToLower(strings.TrimSpace(fraudStatus))

	switch tx {
	case "settlement":
		return "paid"
	case "capture":
		if fraud == "" || fraud == "accept" {
			return "paid"
		}
		if fraud == "deny" {
			return "failed"
		}
		return "pending"
	case "expire":
		return "expired"
	case "deny", "cancel", "failure":
		return "failed"
	case "refund", "partial_refund":
		return "refunded"
	case "pending":
		return "pending"
	default:
		return ""
	}
}

// ParseMidtransGrossAmountIDR parses Midtrans decimal amount strings (e.g., "250000.00") into integer Rupiah.
func ParseMidtransGrossAmountIDR(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("empty gross_amount")
	}
	if dot := strings.IndexByte(trimmed, '.'); dot >= 0 {
		trimmed = trimmed[:dot]
	}
	val, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}

func truncateMidtransItemName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "Biaya Pendaftaran SPMB"
	}
	if len(trimmed) > 50 {
		return trimmed[:50]
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
