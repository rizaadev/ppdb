package admissions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/integrations"
	"github.com/pbdb/platform/internal/storage"
)

var (
	ErrInvalidWebhookSignature = errors.New("invalid webhook signature")
	proofAllowedMimes          = []string{"image/jpeg", "image/png", "image/webp", "application/pdf"}
)

// CreatePayment creates an invoice for registration_fee (awaiting_payment) or re_enrollment_fee (re_enrollment).
func (s *Service) CreatePayment(
	ctx context.Context,
	tenantID, guardianUserID, applicationID uuid.UUID,
	input CreatePaymentInput,
	idempotencyKey string,
	actor ActorContext,
) (*PaymentTransaction, error) {
	detail, err := s.requireOwnedApp(ctx, tenantID, guardianUserID, applicationID)
	if err != nil {
		return nil, err
	}

	purpose := strings.ToLower(strings.TrimSpace(input.Purpose))
	method := strings.ToLower(strings.TrimSpace(input.Method))
	fields := map[string]string{}
	if purpose != PaymentPurposeRegistration && purpose != PaymentPurposeReEnrollment {
		fields["purpose"] = "Tujuan pembayaran harus registration_fee atau re_enrollment_fee."
	}
	switch method {
	case PaymentMethodQRIS, PaymentMethodVA, PaymentMethodEWallet, PaymentMethodManualTransfer:
	default:
		fields["method"] = "Metode pembayaran harus qris, va, ewallet, atau manual_transfer."
	}
	if len(fields) > 0 {
		return nil, &ValidationError{Message: "Parameter pembayaran tidak valid.", Fields: fields}
	}

	cacheScope := fmt.Sprintf("create_payment:%s:%s:%s", applicationID.String(), purpose, method)
	var cachedPay PaymentTransaction
	if hit, err := s.idempotency.Lookup(ctx, tenantID, guardianUserID, cacheScope, idempotencyKey, &cachedPay); err == nil && hit {
		cachedPay.TenantID = tenantID
		return &cachedPay, nil
	}

	var amountIDR int64
	switch purpose {
	case PaymentPurposeRegistration:
		if detail.Application.Status != domain.StatusAwaitingPayment {
			return nil, ErrApplicationLocked
		}
		amountIDR = detail.RegistrationFeeIDR
	case PaymentPurposeReEnrollment:
		if detail.Application.Status != domain.StatusReEnrollment {
			return nil, ErrApplicationLocked
		}
		amountIDR = detail.ReEnrollmentFeeIDR
	}
	if amountIDR <= 0 {
		return nil, ErrNoFeeConfigured
	}

	now := time.Now().UTC()
	paymentID := uuid.Must(uuid.NewV7())
	// Slice UUIDv7 random tail (chars 24..36) rather than timestamp prefix so sub-second invoices never collide.
	shortRef := strings.ToUpper(paymentID.String()[24:])
	extRef := fmt.Sprintf("INV-%s-%s", detail.Application.RegistrationNo, shortRef)
	expiresAt := now.Add(24 * time.Hour)

	var providerPtr, payURLPtr *string
	if method != PaymentMethodManualTransfer {
		purposeLabel := "Biaya Pendaftaran SPMB"
		if purpose == PaymentPurposeReEnrollment {
			purposeLabel = "Biaya Daftar Ulang SPMB"
		}
		customerName := detail.Application.RegistrationNo
		if detail.Application.StudentName != nil && strings.TrimSpace(*detail.Application.StudentName) != "" {
			customerName = strings.TrimSpace(*detail.Application.StudentName)
		}
		var serverKeyOverride string
		var sandboxOverride *bool
		if tcfg, cfgErr := s.resolveTenantPaymentConfig(ctx, tenantID); cfgErr == nil && tcfg != nil {
			serverKeyOverride = strings.TrimSpace(tcfg.MidtransServerKey)
			sandboxOverride = tcfg.MidtransSandbox
		}
		checkout, err := s.gateway().CreateCheckout(ctx, integrations.CheckoutRequest{
			OrderID:           extRef,
			AmountIDR:         amountIDR,
			Method:            method,
			PurposeLabel:      purposeLabel,
			CustomerName:      customerName,
			ServerKeyOverride: serverKeyOverride,
			SandboxOverride:   sandboxOverride,
		})
		if err != nil {
			return nil, fmt.Errorf("create payment gateway checkout: %w", err)
		}
		prov := checkout.Provider
		url := checkout.PaymentURL
		providerPtr = &prov
		payURLPtr = &url
	}

	created, err := s.store.CreatePayment(ctx, PaymentTransaction{
		TenantID:       tenantID,
		ID:             paymentID,
		ApplicationID:  applicationID,
		RegistrationNo: detail.Application.RegistrationNo,
		StudentName:    detail.Application.StudentName,
		UnitID:         detail.Application.UnitID,
		UnitName:       detail.Application.UnitName,
		TrackName:      detail.Application.TrackName,
		Purpose:        purpose,
		Method:         method,
		Provider:       providerPtr,
		ExternalRef:    &extRef,
		AmountIDR:      amountIDR,
		Status:         PaymentStatusPending,
		PaymentURL:     payURLPtr,
		ExpiresAt:      &expiresAt,
		CreatedAt:      now,
	})
	if err != nil {
		return nil, err
	}
	_ = s.idempotency.Save(ctx, tenantID, guardianUserID, cacheScope, idempotencyKey, created)
	return created, nil
}

// ListApplicantPayments lists payment transactions belonging to an applicant's application.
func (s *Service) ListApplicantPayments(
	ctx context.Context,
	tenantID, guardianUserID, applicationID uuid.UUID,
) ([]PaymentTransaction, error) {
	if _, err := s.requireOwnedApp(ctx, tenantID, guardianUserID, applicationID); err != nil {
		return nil, err
	}
	return s.store.ListPaymentsByApplication(ctx, tenantID, applicationID)
}

// PresignManualProof generates a short-lived PUT URL scoped under the application's payment directory.
func (s *Service) PresignManualProof(
	ctx context.Context,
	tenantID, guardianUserID, applicationID, paymentID uuid.UUID,
	input PresignProofInput,
) (*PresignUploadResult, error) {
	if s.presigner == nil {
		return nil, storage.ErrStorageNotConfigured
	}
	if _, err := s.requireOwnedApp(ctx, tenantID, guardianUserID, applicationID); err != nil {
		return nil, err
	}
	payment, err := s.store.GetPaymentByID(ctx, tenantID, paymentID)
	if err != nil {
		return nil, err
	}
	if payment == nil || payment.ApplicationID != applicationID {
		return nil, ErrPaymentNotFound
	}
	if payment.Method != PaymentMethodManualTransfer || payment.Status != PaymentStatusPending {
		return nil, ErrApplicationLocked
	}
	if err := validateProofFile(input.Filename, input.MimeType, input.SizeBytes); err != nil {
		return nil, err
	}

	key := storage.BuildManualProofKey(tenantID, applicationID, paymentID, input.Filename)
	presigned, err := s.presigner.PresignPutObject(key, input.MimeType, 15*time.Minute)
	if err != nil {
		return nil, err
	}
	return &PresignUploadResult{
		UploadURL:       presigned.URL,
		StorageKey:      key,
		RequiredHeaders: presigned.RequiredHeaders,
		ExpiresAt:       presigned.ExpiresAt,
	}, nil
}

// AttachManualProof binds an uploaded proof object key after confirming object presence and size via HeadObject.
func (s *Service) AttachManualProof(
	ctx context.Context,
	tenantID, guardianUserID, applicationID, paymentID uuid.UUID,
	input AttachManualProofInput,
) (*PaymentTransaction, error) {
	if _, err := s.requireOwnedApp(ctx, tenantID, guardianUserID, applicationID); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(input.ProofStorageKey)
	if !storage.ValidateManualProofKey(key, tenantID, applicationID, paymentID) {
		return nil, ErrInvalidStorageKey
	}
	if _, err := s.verifyUploadedObject(ctx, key, 5*1024*1024, "proof_storage_key", "File bukti transfer belum ditemukan di penyimpanan. Silakan unggah ulang bukti terlebih dahulu."); err != nil {
		return nil, err
	}
	return s.store.AttachManualPaymentProof(ctx, tenantID, applicationID, paymentID, key)
}

// ListAdminPayments returns filtered payment transactions visible to authorized finance staff/admins.
func (s *Service) ListAdminPayments(ctx context.Context, tenantID uuid.UUID, filter AdminPaymentFilter) ([]PaymentTransaction, error) {
	return s.store.ListAdminPayments(ctx, tenantID, filter)
}

// GetAdminPayment loads a single payment transaction within tenantID for RBAC verification.
func (s *Service) GetAdminPayment(ctx context.Context, tenantID, paymentID uuid.UUID) (*PaymentTransaction, error) {
	p, err := s.store.GetPaymentByID(ctx, tenantID, paymentID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPaymentNotFound
	}
	return p, nil
}

// PresignAdminPaymentProof returns a short-lived GET URL for a treasurer to inspect a manual transfer receipt.
func (s *Service) PresignAdminPaymentProof(ctx context.Context, tenantID, paymentID uuid.UUID) (storage.PresignedResult, error) {
	if s.presigner == nil {
		return storage.PresignedResult{}, storage.ErrStorageNotConfigured
	}
	p, err := s.GetAdminPayment(ctx, tenantID, paymentID)
	if err != nil {
		return storage.PresignedResult{}, err
	}
	if p.ProofStorageKey == nil || strings.TrimSpace(*p.ProofStorageKey) == "" {
		return storage.PresignedResult{}, ErrDocumentNotFound
	}
	return s.presigner.PresignGetObject(*p.ProofStorageKey, 15*time.Minute)
}

// VerifyManualPayment commits treasurer approval/rejection and atomically transitions application status when paid.
func (s *Service) VerifyManualPayment(
	ctx context.Context,
	tenantID, paymentID uuid.UUID,
	input VerifyManualPaymentInput,
	actor ActorContext,
) (*PaymentVerificationResult, error) {
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	note := strings.TrimSpace(input.Note)
	if decision != PaymentStatusPaid && decision != PaymentStatusFailed {
		return nil, &ValidationError{
			Message: "Keputusan verifikasi pembayaran tidak valid.",
			Fields:  map[string]string{"decision": "Gunakan paid atau failed."},
		}
	}
	if decision == PaymentStatusFailed && note == "" {
		return nil, &ValidationError{
			Message: "Catatan alasan wajib diisi saat menolak bukti pembayaran.",
			Fields:  map[string]string{"note": "Tuliskan alasan penolakan bukti transfer."},
		}
	}

	existing, err := s.GetAdminPayment(ctx, tenantID, paymentID)
	if err != nil {
		return nil, err
	}
	if existing.Status != PaymentStatusPending {
		return nil, ErrPaymentAlreadyFinal
	}
	if existing.Method == PaymentMethodManualTransfer && decision == PaymentStatusPaid &&
		(existing.ProofStorageKey == nil || strings.TrimSpace(*existing.ProofStorageKey) == "") {
		return nil, ErrManualProofRequired
	}

	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	uid := actor.UserID
	pay, app, err := s.store.VerifyPayment(ctx, tenantID, paymentID, decision, &uid, notePtr, time.Now().UTC(), nil, actor)
	if err != nil {
		return nil, err
	}
	return &PaymentVerificationResult{Payment: pay, Application: s.decorateDetail(tenantID, app)}, nil
}

// HandlePaymentWebhook verifies Midtrans SHA512 or HMAC signatures, persists raw webhook payloads, and reconciles payment status.
func (s *Service) HandlePaymentWebhook(
	ctx context.Context,
	tenantID uuid.UUID,
	rawBody []byte,
	signatureHeader string,
	webhookSecret string,
	input PaymentWebhookInput,
) (*PaymentVerificationResult, bool, error) {
	isMidtransNative := strings.TrimSpace(input.SignatureKey) != "" ||
		(strings.EqualFold(strings.TrimSpace(input.Provider), "midtrans") && strings.TrimSpace(input.OrderID) != "")

	extRef := strings.TrimSpace(input.OrderID)
	if extRef == "" {
		extRef = strings.TrimSpace(input.ExternalRef)
	}
	if extRef == "" && (input.PaymentID == nil || *input.PaymentID == uuid.Nil) {
		return nil, false, &ValidationError{
			Message: "Payload webhook harus menyertakan external_ref, order_id, atau payment_id.",
			Fields:  map[string]string{"external_ref": "Referensi tagihan wajib ada."},
		}
	}

	if !isMidtransNative {
		if !VerifyWebhookSignature(webhookSecret, rawBody, signatureHeader) {
			return nil, false, ErrInvalidWebhookSignature
		}
	}

	var payment *PaymentTransaction
	var err error
	if extRef != "" {
		if tenantID != uuid.Nil {
			payment, err = s.store.GetPaymentByExternalRef(ctx, tenantID, extRef)
		} else {
			payment, err = s.store.ResolvePaymentByExternalRef(ctx, extRef)
		}
	}
	if err == nil && payment == nil && tenantID != uuid.Nil && input.PaymentID != nil && *input.PaymentID != uuid.Nil {
		payment, err = s.store.GetPaymentByID(ctx, tenantID, *input.PaymentID)
	}
	if err != nil {
		return nil, false, err
	}
	if payment == nil {
		return nil, false, ErrPaymentNotFound
	}

	resolvedTenantID := tenantID
	if resolvedTenantID == uuid.Nil {
		resolvedTenantID = payment.TenantID
	}

	var statusNorm string
	if isMidtransNative {
		effectiveServerKey := strings.TrimSpace(webhookSecret)
		if tcfg, cfgErr := s.resolveTenantPaymentConfig(ctx, resolvedTenantID); cfgErr == nil && tcfg != nil && strings.TrimSpace(tcfg.MidtransServerKey) != "" {
			effectiveServerKey = strings.TrimSpace(tcfg.MidtransServerKey)
		}
		if !integrations.VerifyMidtransSignature(extRef, input.StatusCode, input.GrossAmount, effectiveServerKey, input.SignatureKey) {
			return nil, false, ErrInvalidWebhookSignature
		}
		if strings.TrimSpace(input.GrossAmount) != "" {
			grossIDR, parseErr := integrations.ParseMidtransGrossAmountIDR(input.GrossAmount)
			if parseErr != nil || grossIDR != payment.AmountIDR {
				return nil, false, &ValidationError{
					Message: "Nominal gross_amount webhook Midtrans tidak cocok dengan tagihan.",
					Fields:  map[string]string{"gross_amount": "Nominal tidak sesuai dengan tagihan terdaftar."},
				}
			}
		}
		txStatus := strings.TrimSpace(input.TransactionStatus)
		if txStatus == "" {
			txStatus = input.Status
		}
		statusNorm = integrations.MapMidtransStatus(txStatus, input.FraudStatus)
	} else {
		statusNorm = normalizeGatewayStatus(input.Status)
		if input.AmountIDR != nil && *input.AmountIDR != payment.AmountIDR {
			return nil, false, &ValidationError{
				Message: "Nominal webhook tidak cocok dengan tagihan.",
				Fields:  map[string]string{"amount_idr": "Nominal pembayaran tidak sesuai."},
			}
		}
	}

	if statusNorm == PaymentStatusPending {
		app, _ := s.store.GetApplicationDetail(ctx, resolvedTenantID, payment.ApplicationID)
		return &PaymentVerificationResult{Payment: payment, Application: s.decorateDetail(resolvedTenantID, app)}, true, nil
	}

	// Idempotent handling if already finalized.
	if payment.Status != PaymentStatusPending {
		app, _ := s.store.GetApplicationDetail(ctx, resolvedTenantID, payment.ApplicationID)
		return &PaymentVerificationResult{Payment: payment, Application: s.decorateDetail(resolvedTenantID, app)}, true, nil
	}

	decision := PaymentStatusFailed
	switch statusNorm {
	case PaymentStatusPaid:
		decision = PaymentStatusPaid
	case PaymentStatusExpired:
		decision = PaymentStatusExpired
	case PaymentStatusFailed:
		decision = PaymentStatusFailed
	default:
		return nil, false, &ValidationError{
			Message: "Status webhook gateway tidak dikenali.",
			Fields:  map[string]string{"status": "Gunakan paid/settlement, failed, atau expired."},
		}
	}

	note := fmt.Sprintf("Webhook %s: %s", strings.TrimSpace(input.Provider), statusNorm)
	actor := ActorContext{RoleName: "webhook:" + strings.TrimSpace(input.Provider)}
	updatedPay, updatedApp, err := s.store.VerifyPayment(ctx, resolvedTenantID, payment.ID, decision, nil, &note, time.Now().UTC(), rawBody, actor)
	if err != nil {
		return nil, false, err
	}
	return &PaymentVerificationResult{Payment: updatedPay, Application: s.decorateDetail(resolvedTenantID, updatedApp)}, false, nil
}

// SyncPaymentStatus actively queries Midtrans Status API and reconciles pending invoices immediately.
func (s *Service) SyncPaymentStatus(
	ctx context.Context,
	tenantID uuid.UUID,
	callerUserID *uuid.UUID,
	paymentID uuid.UUID,
	actor ActorContext,
) (*PaymentVerificationResult, error) {
	payment, err := s.GetAdminPayment(ctx, tenantID, paymentID)
	if err != nil {
		return nil, err
	}
	if callerUserID != nil {
		if _, err := s.requireOwnedApp(ctx, tenantID, *callerUserID, payment.ApplicationID); err != nil {
			return nil, err
		}
	}
	if payment.Status != PaymentStatusPending || payment.Method == PaymentMethodManualTransfer || payment.ExternalRef == nil || strings.TrimSpace(*payment.ExternalRef) == "" {
		app, _ := s.store.GetApplicationDetail(ctx, tenantID, payment.ApplicationID)
		return &PaymentVerificationResult{Payment: payment, Application: s.decorateDetail(tenantID, app)}, nil
	}

	var serverKeyOverride string
	var sandboxOverride *bool
	if tcfg, cfgErr := s.resolveTenantPaymentConfig(ctx, tenantID); cfgErr == nil && tcfg != nil {
		serverKeyOverride = strings.TrimSpace(tcfg.MidtransServerKey)
		sandboxOverride = tcfg.MidtransSandbox
	}
	statusRes, err := s.gateway().GetTransactionStatus(ctx, *payment.ExternalRef, serverKeyOverride, sandboxOverride)
	if err != nil {
		return nil, fmt.Errorf("sync status pembayaran gateway: %w", err)
	}
	if statusRes.GrossAmountIDR > 0 && statusRes.GrossAmountIDR != payment.AmountIDR {
		return nil, &ValidationError{
			Message: "Nominal transaksi dari gateway tidak sesuai dengan invoice terdaftar.",
			Fields:  map[string]string{"amount_idr": "Nominal gateway tidak cocok."},
		}
	}
	if statusRes.NormalizedStatus == "" || statusRes.NormalizedStatus == PaymentStatusPending || statusRes.NormalizedStatus == "unknown" {
		app, _ := s.store.GetApplicationDetail(ctx, tenantID, payment.ApplicationID)
		return &PaymentVerificationResult{Payment: payment, Application: s.decorateDetail(tenantID, app)}, nil
	}

	var rawBytes []byte
	if statusRes.RawPayload != nil {
		rawBytes, _ = json.Marshal(statusRes.RawPayload)
	}
	provName := "midtrans"
	if payment.Provider != nil && strings.TrimSpace(*payment.Provider) != "" {
		provName = strings.TrimSpace(*payment.Provider)
	}
	note := fmt.Sprintf("Sinkronisasi status gateway (%s): %s", provName, statusRes.TransactionStatus)
	updatedPay, updatedApp, err := s.store.VerifyPayment(ctx, tenantID, payment.ID, statusRes.NormalizedStatus, nil, &note, time.Now().UTC(), rawBytes, actor)
	if err != nil {
		if errors.Is(err, ErrPaymentAlreadyFinal) {
			latestPay, _ := s.store.GetPaymentByID(ctx, tenantID, paymentID)
			app, _ := s.store.GetApplicationDetail(ctx, tenantID, payment.ApplicationID)
			return &PaymentVerificationResult{Payment: latestPay, Application: s.decorateDetail(tenantID, app)}, nil
		}
		return nil, err
	}
	return &PaymentVerificationResult{Payment: updatedPay, Application: s.decorateDetail(tenantID, updatedApp)}, nil
}

func (s *Service) resolveTenantPaymentConfig(ctx context.Context, tenantID uuid.UUID) (*domain.TenantPaymentConfig, error) {
	if s.paymentConfigProvider == nil || tenantID == uuid.Nil {
		return nil, nil
	}
	return s.paymentConfigProvider.GetTenantPaymentConfig(ctx, tenantID)
}

// ReEnroll transitions an accepted applicant into re_enrollment (paid) or enrolled (free re-enrollment).
func (s *Service) ReEnroll(
	ctx context.Context,
	tenantID, guardianUserID, applicationID uuid.UUID,
	input ReEnrollInput,
	actor ActorContext,
) (*ApplicationDetail, error) {
	if !input.AgreementAccepted {
		return nil, ErrAgreementRequired
	}
	detail, err := s.requireOwnedApp(ctx, tenantID, guardianUserID, applicationID)
	if err != nil {
		return nil, err
	}
	if detail.Application.Version != input.ExpectedVersion {
		return nil, ErrVersionConflict
	}
	if detail.Application.Status != domain.StatusAccepted {
		return nil, ErrApplicationLocked
	}

	toStatus := domain.StatusEnrolled
	templateCode := integrations.TemplateEnrollmentConfirmed
	if detail.ReEnrollmentFeeIDR > 0 {
		toStatus = domain.StatusReEnrollment
		templateCode = integrations.TemplateReEnrollmentStarted
	}

	fromStr := string(detail.Application.Status)
	toStr := string(toStatus)
	studentName := "Calon Murid"
	if detail.Application.StudentName != nil && *detail.Application.StudentName != "" {
		studentName = *detail.Application.StudentName
	}

	notify := NotificationWrite{
		ID:            uuid.Must(uuid.NewV7()),
		TenantID:      tenantID,
		ApplicationID: applicationID,
		TemplateCode:  templateCode,
		Variables:     []string{detail.Application.RegistrationNo, studentName},
	}

	res, err := s.store.ConfirmReEnrollment(ctx, tenantID, applicationID, input.ExpectedVersion, detail.Application.Status, toStatus, input.ExtraAnswers, AuditWrite{
		ID:          uuid.Must(uuid.NewV7()),
		TenantID:    tenantID,
		Application: &applicationID,
		Actor:       actor,
		ActionType:  "re_enrollment_confirmed",
		EntityType:  "application",
		EntityID:    applicationID,
		FromStatus:  &fromStr,
		ToStatus:    &toStr,
	}, &notify)
	return s.decorateDetail(tenantID, res), err
}

// UpsertScores records examiner assessment component scores and recalculates weighted final_score.
func (s *Service) UpsertScores(
	ctx context.Context,
	tenantID, applicationID uuid.UUID,
	input UpsertScoresInput,
	actor ActorContext,
) (*UpsertScoresResult, error) {
	if len(input.Scores) == 0 {
		return nil, &ValidationError{
			Message: "Daftar komponen skor penilaian wajib diisi.",
			Fields:  map[string]string{"scores": "Minimal satu komponen penilaian diperlukan."},
		}
	}

	cleaned := make([]ScoreComponentInput, 0, len(input.Scores))
	fields := map[string]string{}
	for i, sc := range input.Scores {
		code := strings.ToUpper(strings.TrimSpace(sc.ComponentCode))
		if code == "" {
			fields[fmt.Sprintf("scores[%d].component_code", i)] = "Kode komponen nilai wajib diisi."
		}
		if sc.RawScore < 0 || sc.RawScore > 100 {
			fields[fmt.Sprintf("scores[%d].raw_score", i)] = "Nilai mentah harus bernilai 0 hingga 100."
		}
		if sc.WeightPct < 0 || sc.WeightPct > 100 {
			fields[fmt.Sprintf("scores[%d].weight_pct", i)] = "Bobot komponen harus bernilai 0 hingga 100%."
		}
		cleaned = append(cleaned, ScoreComponentInput{
			ComponentCode: code,
			RawScore:      sc.RawScore,
			WeightPct:     sc.WeightPct,
			Notes:         strings.TrimSpace(sc.Notes),
		})
	}
	if len(fields) > 0 {
		return nil, &ValidationError{Message: "Format skor penilaian tidak valid.", Fields: fields}
	}

	detail, err := s.GetApplicationForStaff(ctx, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	switch detail.Application.Status {
	case domain.StatusVerified, domain.StatusSelectionInProgress, domain.StatusRanked:
	default:
		return nil, ErrApplicationLocked
	}

	return s.store.UpsertApplicationScores(ctx, tenantID, applicationID, actor.UserID, cleaned, actor)
}

// GetTrackForStaff resolves active track metadata for RBAC checks prior to simulation.
func (s *Service) GetTrackForStaff(ctx context.Context, tenantID, trackID uuid.UUID) (*CatalogTrack, error) {
	tr, err := s.store.GetTrackByID(ctx, tenantID, trackID)
	if err != nil {
		return nil, err
	}
	if tr == nil {
		return nil, ErrTrackNotFound
	}
	return tr, nil
}

// GetPeriodTracksForStaff resolves active tracks within periodID for RBAC unit verification before publishing.
func (s *Service) GetPeriodTracksForStaff(ctx context.Context, tenantID, periodID uuid.UUID) ([]CatalogTrack, error) {
	tracks, err := s.store.ListPeriodTracks(ctx, tenantID, periodID)
	if err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		return nil, ErrPeriodNotFound
	}
	return tracks, nil
}

// SimulateTrackRanking previews deterministic tie-broken candidate ranks against track quota without locking status.
func (s *Service) SimulateTrackRanking(ctx context.Context, tenantID, trackID uuid.UUID) (*TrackRankingSimulation, error) {
	return s.store.SimulateTrackRanking(ctx, tenantID, trackID)
}

// PublishPeriodResults locks accepted/waitlisted/rejected outcomes across all period tracks idempotently.
func (s *Service) PublishPeriodResults(
	ctx context.Context,
	tenantID, periodID uuid.UUID,
	idempotencyKey string,
	actor ActorContext,
) (*PublishResultsResult, error) {
	scope := "publish_results:" + periodID.String()
	var cachedPub PublishResultsResult
	if hit, err := s.idempotency.Lookup(ctx, tenantID, actor.UserID, scope, idempotencyKey, &cachedPub); err == nil && hit {
		return &cachedPub, nil
	}
	res, err := s.store.PublishPeriodResults(ctx, tenantID, periodID, actor, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	_ = s.idempotency.Save(ctx, tenantID, actor.UserID, scope, idempotencyKey, res)
	return res, nil
}

// PromoteWaitlist advances a waitlisted candidate to accepted with immutable audit logging and WA queue.
func (s *Service) PromoteWaitlist(
	ctx context.Context,
	tenantID, applicationID uuid.UUID,
	actor ActorContext,
) (*ApplicationDetail, error) {
	res, err := s.store.PromoteWaitlist(ctx, tenantID, applicationID, actor)
	return s.decorateDetail(tenantID, res), err
}

// ListAssessmentSchedules returns filtered test/interview schedules visible within tenantID.
func (s *Service) ListAssessmentSchedules(
	ctx context.Context,
	tenantID uuid.UUID,
	filter AssessmentScheduleFilter,
) ([]AssessmentSchedule, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 100
	}
	return s.store.ListAssessmentSchedules(ctx, tenantID, filter)
}

// CreateAssessmentSchedule creates one or batch test/interview schedules and enqueues WA notifications.
func (s *Service) CreateAssessmentSchedule(
	ctx context.Context,
	tenantID uuid.UUID,
	input CreateAssessmentScheduleInput,
	actor ActorContext,
) ([]AssessmentSchedule, error) {
	sessionName := strings.TrimSpace(input.SessionName)
	fields := map[string]string{}
	if sessionName == "" {
		fields["session_name"] = "Nama sesi ujian/wawancara wajib diisi."
	}
	if input.ScheduledAt.IsZero() {
		fields["scheduled_at"] = "Waktu pelaksanaan asesmen wajib diisi."
	}

	appIDs := make([]uuid.UUID, 0, len(input.ApplicationIDs)+1)
	seen := map[uuid.UUID]bool{}
	if input.ApplicationID != nil && *input.ApplicationID != uuid.Nil {
		appIDs = append(appIDs, *input.ApplicationID)
		seen[*input.ApplicationID] = true
	}
	for _, id := range input.ApplicationIDs {
		if id != uuid.Nil && !seen[id] {
			appIDs = append(appIDs, id)
			seen[id] = true
		}
	}
	if len(appIDs) == 0 {
		fields["application_ids"] = "Pilih minimal satu pendaftar."
	}
	if len(fields) > 0 {
		return nil, &ValidationError{Message: "Data jadwal asesmen belum lengkap.", Fields: fields}
	}

	var roomPtr *string
	if input.RoomOrLink != nil {
		if trimmed := strings.TrimSpace(*input.RoomOrLink); trimmed != "" {
			roomPtr = &trimmed
		}
	}

	records := make([]AssessmentSchedule, 0, len(appIDs))
	for _, appID := range appIDs {
		records = append(records, AssessmentSchedule{
			ID:             uuid.Must(uuid.NewV7()),
			ApplicationID:  appID,
			SessionName:    sessionName,
			RoomOrLink:     roomPtr,
			ScheduledAt:    input.ScheduledAt.UTC(),
			ExaminerUserID: input.ExaminerUserID,
		})
	}
	return s.store.CreateAssessmentSchedules(ctx, tenantID, records, actor)
}

// DeleteAssessmentSchedule removes a scheduled assessment session inside tenantID.
func (s *Service) DeleteAssessmentSchedule(
	ctx context.Context,
	tenantID, scheduleID uuid.UUID,
	actor ActorContext,
) error {
	return s.store.DeleteAssessmentSchedule(ctx, tenantID, scheduleID, actor)
}

// VerifyWebhookSignature validates HMAC-SHA256 hex digest in signatureHeader against secret and rawBody.
func VerifyWebhookSignature(secret string, rawBody []byte, signatureHeader string) bool {
	if strings.TrimSpace(secret) == "" {
		return false
	}
	sig := strings.TrimSpace(signatureHeader)
	sig = strings.TrimPrefix(sig, "sha256=")
	sig = strings.TrimPrefix(sig, "SHA256=")
	gotBytes, err := hex.DecodeString(sig)
	if err != nil || len(gotBytes) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	expected := mac.Sum(nil)
	return hmac.Equal(gotBytes, expected)
}

func validateProofFile(filename, mimeType string, sizeBytes int64) error {
	if strings.TrimSpace(filename) == "" {
		return &ValidationError{Message: "Nama file bukti transfer wajib diisi.", Fields: map[string]string{"filename": "Nama berkas wajib diisi."}}
	}
	allowed := false
	for _, m := range proofAllowedMimes {
		if m == mimeType {
			allowed = true
			break
		}
	}
	if !allowed {
		return ErrUnsupportedMime
	}
	if sizeBytes <= 0 || sizeBytes > 5*1024*1024 {
		return ErrFileTooLarge
	}
	return nil
}

func normalizeGatewayStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "paid", "settlement", "capture", "succeeded":
		return PaymentStatusPaid
	case "expired", "expire":
		return PaymentStatusExpired
	case "failed", "deny", "cancel", "cancelled":
		return PaymentStatusFailed
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func computeTrackSimulation(track CatalogTrack, pool []*ApplicationDetail) TrackRankingSimulation {
	type sortItem struct {
		detail     *ApplicationDetail
		finalScore float64
		dist       int
		birthDate  string
		subTime    time.Time
		kkIssued   *string
	}

	items := make([]sortItem, 0, len(pool))
	for _, app := range pool {
		scoreVal := 0.0
		if app.Application.FinalScore != nil {
			if parsed, err := strconv.ParseFloat(*app.Application.FinalScore, 64); err == nil {
				scoreVal = parsed
			}
		}
		dist := math.MaxInt
		if app.Application.DistanceMeters != nil {
			dist = *app.Application.DistanceMeters
		}
		bDate := "2100-01-01"
		var kkIssued *string
		if app.Profile != nil {
			if app.Profile.BirthDate != nil && *app.Profile.BirthDate != "" {
				bDate = *app.Profile.BirthDate
			}
			kkIssued = app.Profile.KKIssuedDate
		}
		sub := app.Application.CreatedAt
		if app.Application.SubmittedAt != nil {
			sub = *app.Application.SubmittedAt
		}
		items = append(items, sortItem{
			detail:     app,
			finalScore: scoreVal,
			dist:       dist,
			birthDate:  bDate,
			subTime:    sub,
			kkIssued:   kkIssued,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].finalScore != items[j].finalScore {
			return items[i].finalScore > items[j].finalScore
		}
		if items[i].dist != items[j].dist {
			return items[i].dist < items[j].dist
		}
		if items[i].birthDate != items[j].birthDate {
			return items[i].birthDate < items[j].birthDate
		}
		if !items[i].subTime.Equal(items[j].subTime) {
			return items[i].subTime.Before(items[j].subTime)
		}
		return items[i].detail.Application.ID.String() < items[j].detail.Application.ID.String()
	})

	waitlistSlots := 0
	if track.Quota > 0 {
		waitlistSlots = max(1, track.Quota/5)
	}

	sim := TrackRankingSimulation{
		TrackID:                track.TrackID,
		TrackName:              track.TrackName,
		UnitID:                 track.UnitID,
		Quota:                  track.Quota,
		WaitlistSlots:          waitlistSlots,
		SpilloverTargetTrackID: track.SpilloverTargetTrack,
		Candidates:             make([]RankedCandidate, 0, len(items)),
	}

	for idx, item := range items {
		rankPos := idx + 1
		decision := domain.StatusRejected
		switch {
		case rankPos <= track.Quota:
			decision = domain.StatusAccepted
			sim.AcceptedCount++
		case rankPos <= track.Quota+waitlistSlots:
			decision = domain.StatusWaitlisted
			sim.WaitlistedCount++
		default:
			sim.RejectedCount++
		}

		sim.Candidates = append(sim.Candidates, RankedCandidate{
			ApplicationID:     item.detail.Application.ID,
			RegistrationNo:    item.detail.Application.RegistrationNo,
			StudentName:       item.detail.Application.StudentName,
			CurrentStatus:     item.detail.Application.Status,
			SimulatedDecision: decision,
			RankPosition:      rankPos,
			FinalScore:        item.finalScore,
			DistanceMeters:    item.detail.Application.DistanceMeters,
			BirthDate:         profileBirthDate(item.detail.Profile),
			SubmittedAt:       item.detail.Application.SubmittedAt,
			KKIssuedDate:      item.kkIssued,
		})
	}

	if track.Quota > sim.AcceptedCount {
		sim.RemainingQuota = track.Quota - sim.AcceptedCount
	}
	return sim
}

func profileBirthDate(p *ApplicantProfile) *string {
	if p == nil {
		return nil
	}
	return p.BirthDate
}

func calculateWeightedTotal(scores []AssessmentScore) float64 {
	total := 0.0
	for _, s := range scores {
		total += s.WeightedScore
	}
	return math.Round(total*100) / 100
}
