package public

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/handlers"
)

type storedThemeConfig struct {
	PrimaryColor   string  `json:"primary_color"`
	LogoURL        *string `json:"logo_url,omitempty"`
	LogoStorageKey *string `json:"logo_storage_key,omitempty"`
	Tagline        string  `json:"tagline"`
}

func parseStoredThemeConfig(raw []byte) storedThemeConfig {
	out := storedThemeConfig{PrimaryColor: "#1E3A5F"}
	if len(raw) == 0 {
		return out
	}
	var parsed storedThemeConfig
	if json.Unmarshal(raw, &parsed) == nil {
		if strings.TrimSpace(parsed.PrimaryColor) != "" {
			out.PrimaryColor = strings.TrimSpace(parsed.PrimaryColor)
		}
		if parsed.LogoURL != nil && strings.TrimSpace(*parsed.LogoURL) != "" {
			v := strings.TrimSpace(*parsed.LogoURL)
			out.LogoURL = &v
		}
		if parsed.LogoStorageKey != nil && strings.TrimSpace(*parsed.LogoStorageKey) != "" {
			v := strings.TrimSpace(*parsed.LogoStorageKey)
			out.LogoStorageKey = &v
		}
		out.Tagline = parsed.Tagline
	}
	return out
}

func publicLogoURLFromStorageKey(storageKey *string) *string {
	if storageKey == nil || strings.TrimSpace(*storageKey) == "" {
		return nil
	}
	v := "/api/v1/public/logo?v=" + url.QueryEscape(strings.TrimSpace(*storageKey))
	return &v
}

func toThemeConfigView(stored storedThemeConfig) handlers.ThemeConfigView {
	logoURL := stored.LogoURL
	if computed := publicLogoURLFromStorageKey(stored.LogoStorageKey); computed != nil {
		logoURL = computed
	}
	return handlers.ThemeConfigView{
		PrimaryColor: stored.PrimaryColor,
		LogoURL:      logoURL,
		Tagline:      stored.Tagline,
	}
}

func parseThemeConfigJSON(raw []byte) handlers.ThemeConfigView {
	return toThemeConfigView(parseStoredThemeConfig(raw))
}

func parsePaymentConfigJSON(raw []byte) domain.TenantPaymentConfig {
	var out domain.TenantPaymentConfig
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func toPaymentConfigView(cfg domain.TenantPaymentConfig) handlers.PaymentConfigView {
	masked := domain.MaskSecret(cfg.MidtransServerKey)
	return handlers.PaymentConfigView{
		MidtransMerchantID:          strings.TrimSpace(cfg.MidtransMerchantID),
		MidtransClientKey:           strings.TrimSpace(cfg.MidtransClientKey),
		MidtransSandbox:             cfg.MidtransSandbox,
		MidtransServerKeyConfigured: strings.TrimSpace(cfg.MidtransServerKey) != "",
		MidtransServerKeyMasked:     masked,
	}
}

func mergePaymentConfigUpdate(cur domain.TenantPaymentConfig, upd *handlers.PaymentConfigUpdate) domain.TenantPaymentConfig {
	if upd == nil {
		return cur
	}
	if upd.MidtransMerchantID != nil {
		cur.MidtransMerchantID = strings.TrimSpace(*upd.MidtransMerchantID)
	}
	if upd.MidtransClientKey != nil {
		cur.MidtransClientKey = strings.TrimSpace(*upd.MidtransClientKey)
	}
	if upd.MidtransServerKey != nil {
		// Validation already cleared masked echoes to nil; empty string intentionally wipes override.
		cur.MidtransServerKey = strings.TrimSpace(*upd.MidtransServerKey)
	}
	if upd.MidtransSandbox != nil {
		b := *upd.MidtransSandbox
		cur.MidtransSandbox = &b
	}
	return cur
}

func documentTypeForStatus(status domain.ApplicationStatus) string {
	switch status {
	case domain.StatusAccepted, domain.StatusReEnrollment, domain.StatusEnrolled, domain.StatusWaitlisted, domain.StatusRejected:
		return "acceptance_letter"
	default:
		return "registration_card"
	}
}

func maskFullName(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	for i, part := range parts {
		runes := []rune(part)
		if len(runes) <= 1 {
			continue
		}
		parts[i] = string(runes[0]) + strings.Repeat("*", len(runes)-1)
	}
	return strings.Join(parts, " ")
}

