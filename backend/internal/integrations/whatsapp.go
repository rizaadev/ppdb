package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	TemplateOTP                  = "otp_verification"
	TemplatePasswordResetOTP     = "password_reset_otp"
	TemplateApplicationSubmitted = "application_submitted"
	TemplateVerificationApproved = "verification_approved"
	TemplateVerificationRevision = "verification_revision_required"
	TemplatePaymentConfirmed     = "payment_confirmed"
	TemplatePaymentVerified      = "payment_verified"
	TemplateSelectionAccepted    = "selection_result_accepted"
	TemplateSelectionWaitlisted  = "selection_result_waitlisted"
	TemplateSelectionRejected    = "selection_result_rejected"
	TemplateWaitlistPromoted     = "waitlist_promoted"
	TemplateReEnrollmentStarted  = "re_enrollment_started"
	TemplateEnrollmentConfirmed  = "enrollment_confirmed"
	TemplateAssessmentScheduled  = "assessment_scheduled"
)

// WhatsAppMessage describes an outbound transactional WhatsApp notification.
type WhatsAppMessage struct {
	RecipientPhone string
	TemplateCode   string
	Variables      []string
}

// WhatsAppSendResult holds delivery metadata returned by the provider.
type WhatsAppSendResult struct {
	ProviderMessageID string
}

// WhatsAppSender dispatches rendered messages via a WhatsApp gateway or local logger.
type WhatsAppSender interface {
	Send(ctx context.Context, msg WhatsAppMessage) (WhatsAppSendResult, error)
}

// FormatTemplateText turns template codes and variables into readable Indonesian WhatsApp messages.
func FormatTemplateText(templateCode string, vars []string) string {
	arg := func(i int, fallback string) string {
		if i < len(vars) && strings.TrimSpace(vars[i]) != "" {
			return strings.TrimSpace(vars[i])
		}
		return fallback
	}

	switch templateCode {
	case TemplateOTP:
		return fmt.Sprintf("Kode OTP verifikasi akun portal penerimaan murid baru Anda: *%s*. Berlaku 5 menit. Jangan bagikan kepada siapa pun.", arg(0, "------"))
	case TemplatePasswordResetOTP:
		return fmt.Sprintf("Kode OTP reset kata sandi portal penerimaan murid baru Anda: *%s*. Berlaku 5 menit. Abaikan jika Anda tidak meminta reset.", arg(0, "------"))
	case TemplateApplicationSubmitted:
		return fmt.Sprintf("Pendaftaran No. *%s* berhasil dikirim untuk verifikasi panitia. Pantau progresnya melalui portal pendaftar.", arg(0, "-"))
	case TemplateVerificationApproved:
		return fmt.Sprintf("Berkas pendaftaran No. *%s* telah dinyatakan LOLOS VERIFIKASI (%s).", arg(0, "-"), arg(1, "Lengkap"))
	case TemplateVerificationRevision:
		return fmt.Sprintf("Berkas pendaftaran No. *%s* memerlukan perbaikan: %s. Silakan unggah ulang melalui portal.", arg(0, "-"), arg(1, "Periksa catatan verifikator"))
	case TemplatePaymentConfirmed, TemplatePaymentVerified:
		return fmt.Sprintf("Pembayaran tagihan *%s* (%s) telah kami terima. Status: Lunas.", arg(0, "-"), arg(1, "Pendaftaran"))
	case TemplateSelectionAccepted, "selection_accepted":
		return fmt.Sprintf("Selamat! Pendaftaran No. *%s* dinyatakan DITERIMA. Silakan lakukan konfirmasi daftar ulang pada portal.", arg(0, "-"))
	case TemplateSelectionWaitlisted, "selection_waitlisted":
		return fmt.Sprintf("Pendaftaran No. *%s* masuk daftar CADANGAN. Kami akan menghubungi jika terdapat kuota yang terbuka.", arg(0, "-"))
	case TemplateSelectionRejected, "selection_rejected":
		return fmt.Sprintf("Hasil seleksi pendaftaran No. *%s* telah diterbitkan. Silakan cek detail pengumuman resmi di portal.", arg(0, "-"))
	case TemplateWaitlistPromoted:
		return fmt.Sprintf("Kabar baik! Pendaftaran No. *%s* dipromosikan dari daftar cadangan menjadi DITERIMA. Silakan lakukan konfirmasi daftar ulang di portal.", arg(0, "-"))
	case TemplateReEnrollmentStarted:
		return fmt.Sprintf("Konfirmasi daftar ulang pendaftaran No. *%s* telah diterima (%s). Selesaikan tagihan daftar ulang pada portal untuk finalisasi.", arg(0, "-"), arg(1, "Menunggu pembayaran daftar ulang"))
	case TemplateEnrollmentConfirmed, "re_enrollment_confirmed":
		return fmt.Sprintf("Selamat! Daftar ulang pendaftaran No. *%s* (%s) telah selesai dan calon peserta didik resmi TERDAFTAR.", arg(0, "-"), arg(1, "Selesai"))
	case TemplateAssessmentScheduled:
		return fmt.Sprintf("Jadwal *%s* untuk pendaftaran No. *%s* telah ditetapkan pada *%s* (Lokasi/Tautan: %s).", arg(1, "Tes Seleksi"), arg(0, "-"), arg(2, "-"), arg(3, "Lihat portal"))
	default:
		if len(vars) > 0 {
			return fmt.Sprintf("[%s] %s", templateCode, strings.Join(vars, " - "))
		}
		return fmt.Sprintf("Notifikasi sistem (%s)", templateCode)
	}
}

// NewWhatsAppSender selects Fonnte native adapter, custom HTTP webhook relay, or dev log sender.
func NewWhatsAppSender(provider, gatewayURL, token string, logger *slog.Logger) WhatsAppSender {
	if logger == nil {
		logger = slog.Default()
	}
	trimmedProvider := strings.ToLower(strings.TrimSpace(provider))
	trimmedURL := strings.TrimSpace(gatewayURL)
	trimmedToken := strings.TrimSpace(token)

	if trimmedProvider == "fonnte" {
		if trimmedURL == "" {
			trimmedURL = "https://api.fonnte.com/send"
		}
		if trimmedToken != "" {
			return &FonnteWhatsAppSender{
				EndpointURL: trimmedURL,
				Token:       trimmedToken,
				Client:      &http.Client{Timeout: 10 * time.Second},
			}
		}
	}

	if trimmedURL != "" && trimmedProvider != "" && trimmedProvider != "log" {
		return &HTTPWhatsAppSender{
			GatewayURL: trimmedURL,
			Token:      trimmedToken,
			Provider:   trimmedProvider,
			Client:     &http.Client{Timeout: 10 * time.Second},
		}
	}
	return &LogWhatsAppSender{Logger: logger}
}

// LogWhatsAppSender records outgoing WhatsApp notifications in structured logs for dev mode.
type LogWhatsAppSender struct {
	Logger *slog.Logger
}

func (s *LogWhatsAppSender) Send(ctx context.Context, msg WhatsAppMessage) (WhatsAppSendResult, error) {
	body := FormatTemplateText(msg.TemplateCode, msg.Variables)
	s.Logger.InfoContext(ctx, "whatsapp notification dispatched (log driver)",
		"recipient", msg.RecipientPhone,
		"template", msg.TemplateCode,
		"message_preview", body,
	)
	return WhatsAppSendResult{
		ProviderMessageID: fmt.Sprintf("log-%d", time.Now().UnixNano()),
	}, nil
}

// FonnteWhatsAppSender integrates natively with Fonnte API (https://api.fonnte.com/send).
// Fonnte expects raw token in Authorization header (no "Bearer " prefix) and JSON/form {target, message, countryCode}.
type FonnteWhatsAppSender struct {
	EndpointURL string
	Token       string
	Client      *http.Client
}

func (s *FonnteWhatsAppSender) Send(ctx context.Context, msg WhatsAppMessage) (WhatsAppSendResult, error) {
	if strings.TrimSpace(s.Token) == "" {
		return WhatsAppSendResult{}, fmt.Errorf("fonnte api token is not configured")
	}
	endpoint := strings.TrimSpace(s.EndpointURL)
	if endpoint == "" {
		endpoint = "https://api.fonnte.com/send"
	}

	target := normalizeFonnteTarget(msg.RecipientPhone)
	payload := map[string]any{
		"target":      target,
		"message":     FormatTemplateText(msg.TemplateCode, msg.Variables),
		"countryCode": "62",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return WhatsAppSendResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return WhatsAppSendResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", strings.TrimSpace(s.Token))

	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return WhatsAppSendResult{}, fmt.Errorf("fonnte gateway request: %w", err)
	}
	defer resp.Body.Close()

	respRaw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WhatsAppSendResult{}, fmt.Errorf("fonnte gateway HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respRaw)))
	}

	var parsed struct {
		Status bool     `json:"status"`
		Detail string   `json:"detail"`
		Reason string   `json:"reason"`
		ID     []string `json:"id"`
	}
	if err := json.Unmarshal(respRaw, &parsed); err != nil {
		return WhatsAppSendResult{}, fmt.Errorf("decode fonnte response: %w", err)
	}
	if !parsed.Status {
		reason := firstNonEmpty(parsed.Reason, parsed.Detail, string(respRaw))
		return WhatsAppSendResult{}, fmt.Errorf("fonnte rejected message: %s", reason)
	}

	msgID := ""
	if len(parsed.ID) > 0 {
		msgID = strings.TrimSpace(parsed.ID[0])
	}
	if msgID == "" {
		msgID = fmt.Sprintf("fonnte-%d", time.Now().UnixNano())
	}
	return WhatsAppSendResult{ProviderMessageID: msgID}, nil
}

func normalizeFonnteTarget(phone string) string {
	cleaned := strings.TrimSpace(phone)
	cleaned = strings.TrimPrefix(cleaned, "+")
	if strings.HasPrefix(cleaned, "0") && len(cleaned) > 1 {
		return "62" + cleaned[1:]
	}
	return cleaned
}

// HTTPWhatsAppSender posts outbound notifications to a generic custom WhatsApp HTTP relay.
type HTTPWhatsAppSender struct {
	GatewayURL string
	Token      string
	Provider   string
	Client     *http.Client
}

func (s *HTTPWhatsAppSender) Send(ctx context.Context, msg WhatsAppMessage) (WhatsAppSendResult, error) {
	payload := map[string]any{
		"provider":      s.Provider,
		"target":        msg.RecipientPhone,
		"to":            msg.RecipientPhone,
		"template_code": msg.TemplateCode,
		"variables":     msg.Variables,
		"message":       FormatTemplateText(msg.TemplateCode, msg.Variables),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return WhatsAppSendResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.GatewayURL, bytes.NewReader(raw))
	if err != nil {
		return WhatsAppSendResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}

	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return WhatsAppSendResult{}, fmt.Errorf("whatsapp gateway request: %w", err)
	}
	defer resp.Body.Close()

	respRaw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WhatsAppSendResult{}, fmt.Errorf("whatsapp gateway HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respRaw)))
	}

	var parsed struct {
		ID        string `json:"id"`
		MessageID string `json:"message_id"`
	}
	_ = json.Unmarshal(respRaw, &parsed)
	msgID := parsed.MessageID
	if msgID == "" {
		msgID = parsed.ID
	}
	if msgID == "" {
		msgID = fmt.Sprintf("wa-%d", time.Now().UnixNano())
	}
	return WhatsAppSendResult{ProviderMessageID: msgID}, nil
}
