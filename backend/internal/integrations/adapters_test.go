package integrations

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPWhatsAppSender(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message_id":"wa-msg-991"}`))
	}))
	defer srv.Close()

	sender := NewWhatsAppSender("webhook", srv.URL, "secret-token", nil)
	res, err := sender.Send(context.Background(), WhatsAppMessage{
		RecipientPhone: "+6281234567890",
		TemplateCode:   TemplateOTP,
		Variables:      []string{"482910"},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if gotAuth != "Bearer secret-token" || res.ProviderMessageID != "wa-msg-991" {
		t.Fatalf("unexpected header/result: auth=%q res=%+v", gotAuth, res)
	}
	if msg, _ := gotBody["message"].(string); !strings.Contains(msg, "482910") {
		t.Fatalf("expected formatted OTP message, got %q", msg)
	}
}

func TestFonnteWhatsAppSenderAndTemplateAlignment(t *testing.T) {
	var gotAuth string
	var gotPayload map[string]any
	failMode := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		if failMode {
			_, _ = w.Write([]byte(`{"status":false,"reason":"device disconnected"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":true,"id":["fonnte-id-778"]}`))
	}))
	defer srv.Close()

	sender := NewWhatsAppSender("fonnte", srv.URL, "fonnte-device-token", nil)
	res, err := sender.Send(context.Background(), WhatsAppMessage{
		RecipientPhone: "+6281355667788",
		TemplateCode:   TemplatePaymentConfirmed,
		Variables:      []string{"REG-2026-001", "Rp 250.000"},
	})
	if err != nil {
		t.Fatalf("Fonnte send failed: %v", err)
	}
	if gotAuth != "fonnte-device-token" {
		t.Fatalf("expected raw Authorization header without Bearer prefix, got %q", gotAuth)
	}
	if gotPayload["target"] != "6281355667788" {
		t.Fatalf("expected normalized phone target 6281355667788, got %v", gotPayload["target"])
	}
	if res.ProviderMessageID != "fonnte-id-778" {
		t.Fatalf("unexpected message id: %q", res.ProviderMessageID)
	}
	msgText, _ := gotPayload["message"].(string)
	if strings.HasPrefix(msgText, "[payment_confirmed]") || !strings.Contains(msgText, "REG-2026-001") {
		t.Fatalf("template formatting broken for payment_confirmed: %q", msgText)
	}

	// Verify previously mismatched templates now format cleanly without bracket fallback.
	for _, tc := range []string{
		TemplatePasswordResetOTP,
		TemplateEnrollmentConfirmed,
		TemplateReEnrollmentStarted,
		TemplateSelectionAccepted,
		TemplateSelectionWaitlisted,
		TemplateSelectionRejected,
		TemplateWaitlistPromoted,
		TemplateAssessmentScheduled,
	} {
		out := FormatTemplateText(tc, []string{"REG-01", "Tahap 1", "Besok 09:00", "Ruang A"})
		if strings.HasPrefix(out, "[") {
			t.Fatalf("template %q fell back to raw bracket format: %q", tc, out)
		}
	}

	failMode = true
	if _, err := sender.Send(context.Background(), WhatsAppMessage{
		RecipientPhone: "081234567890",
		TemplateCode:   TemplateOTP,
		Variables:      []string{"123456"},
	}); err == nil || !strings.Contains(err.Error(), "device disconnected") {
		t.Fatalf("expected fonnte status=false error, got %v", err)
	}
}

func TestMidtransSnapStatusAndSignature(t *testing.T) {
	var gotAuth string
	var gotOrder string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"order_id":"INV-SPMB-0001","status_code":"200","transaction_status":"settlement","fraud_status":"accept","gross_amount":"250000.00","payment_type":"qris"}`))
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if td, ok := req["transaction_details"].(map[string]any); ok {
			gotOrder, _ = td["order_id"].(string)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"snap-tok-123","redirect_url":"https://app.sandbox.midtrans.com/snap/v2/vtweb/snap-tok-123"}`))
	}))
	defer srv.Close()

	gw := &MidtransSnapGateway{
		EndpointURL:       srv.URL,
		StatusEndpointFmt: srv.URL + "/v2/%s/status",
		ServerKey:         "SB-Mid-server-TEST",
	}
	res, err := gw.CreateCheckout(context.Background(), CheckoutRequest{
		OrderID:      "INV-SPMB-0001",
		AmountIDR:    250000,
		Method:       "qris",
		PurposeLabel: "Biaya Pendaftaran",
		CustomerName: "Zaidan",
	})
	if err != nil {
		t.Fatalf("CreateCheckout failed: %v", err)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") || gotOrder != "INV-SPMB-0001" {
		t.Fatalf("unexpected auth/order: auth=%q order=%q", gotAuth, gotOrder)
	}
	if !strings.Contains(res.PaymentURL, "snap-tok-123") {
		t.Fatalf("expected snap redirect url, got %+v", res)
	}

	status, err := gw.GetTransactionStatus(context.Background(), "INV-SPMB-0001", "", nil)
	if err != nil {
		t.Fatalf("GetTransactionStatus failed: %v", err)
	}
	if status.NormalizedStatus != "paid" || status.GrossAmountIDR != 250000 {
		t.Fatalf("unexpected status result: %+v", status)
	}

	rawSig := sha512.Sum512([]byte("INV-SPMB-0001" + "200" + "250000.00" + "SB-Mid-server-TEST"))
	validSig := hex.EncodeToString(rawSig[:])
	if !VerifyMidtransSignature("INV-SPMB-0001", "200", "250000.00", "SB-Mid-server-TEST", validSig) {
		t.Fatal("expected valid Midtrans SHA512 signature")
	}
	if VerifyMidtransSignature("INV-SPMB-0001", "200", "1000.00", "SB-Mid-server-TEST", validSig) {
		t.Fatal("tampered gross_amount must fail signature check")
	}
}
