package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// CustomFormField describes one configurable extra applicant question defined per admission track.
type CustomFormField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // text | textarea | number | select | date | checkbox
	Required    bool     `json:"required"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// TenantPaymentConfig stores optional per-tenant Midtrans gateway credentials in tenants.payment_config JSONB.
type TenantPaymentConfig struct {
	MidtransMerchantID string `json:"midtrans_merchant_id,omitempty"`
	MidtransClientKey  string `json:"midtrans_client_key,omitempty"`
	MidtransServerKey  string `json:"midtrans_server_key,omitempty"`
	MidtransSandbox    *bool  `json:"midtrans_sandbox,omitempty"`
}

// GenerateDomainVerificationToken creates a random TXT record value for proving custom domain ownership.
func GenerateDomainVerificationToken() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return "spmb-verify-" + hex.EncodeToString(raw[:])
}

// MaskSecret displays only the last 4 characters of a secret key while keeping its prefix/presence clear.
func MaskSecret(secret string) string {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 6 {
		return strings.Repeat("*", len(trimmed))
	}
	return "****" + trimmed[len(trimmed)-4:]
}

// SignDocumentToken generates a tenant-bound HMAC-SHA256 verification token for Registration Cards and Acceptance Letters.
func SignDocumentToken(secret string, tenantID, applicationID uuid.UUID) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "doc-verify:%s:%s", tenantID.String(), applicationID.String())
	sig := hex.EncodeToString(mac.Sum(nil)[:16])
	return applicationID.String() + "." + sig
}

// VerifyDocumentToken parses and validates a QR verification token against the tenant and server secret.
func VerifyDocumentToken(secret, token string, tenantID uuid.UUID) (uuid.UUID, bool) {
	if strings.TrimSpace(secret) == "" {
		return uuid.Nil, false
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return uuid.Nil, false
	}
	appID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, false
	}
	expected := SignDocumentToken(secret, tenantID, appID)
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(token))) {
		return uuid.Nil, false
	}
	return appID, true
}

