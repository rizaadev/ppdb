package admissions_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/integrations"
	"github.com/pbdb/platform/internal/modules/admissions"
)

func signWebhookPayload(secret string, raw []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestBillingAndReEnrollmentWorkflow(t *testing.T) {
	ctx := context.Background()
	svc, pubDemo, objStore := setupAdmissionsTestWithStorage(t)

	pmbmTenant, err := pubDemo.FindBySlug(ctx, "demo-pmbm")
	if err != nil || pmbmTenant == nil {
		t.Fatalf("resolve demo-pmbm tenant: %v", err)
	}
	pmbmPortal, err := pubDemo.GetPortalData(ctx, pmbmTenant)
	if err != nil {
		t.Fatalf("GetPortalData: %v", err)
	}
	unit := pmbmPortal.Units[0]
	period := pmbmPortal.Periods[0]
	regulerTrack := period.Tracks[0] // RegistrationFeeIDR = 150000, ReEnrollmentFeeIDR = 1250000

	guardianID := uuid.MustParse("01960000-0000-7000-8001-000000000001")
	treasurerID := uuid.MustParse("01960000-0000-7000-8001-000000000004")
	actorApplicant := admissions.ActorContext{UserID: guardianID, RoleName: "applicant"}
	actorTreasurer := admissions.ActorContext{UserID: treasurerID, RoleName: "treasurer"}

	appA, err := svc.CreateApplication(ctx, pmbmTenant.ID, guardianID, admissions.CreateApplicationInput{
		UnitID: unit.ID, PeriodID: period.ID, TrackID: regulerTrack.ID, StudentFullName: "Hasan Manual",
	}, "pay-app-a", actorApplicant)
	if err != nil {
		t.Fatalf("create paid appA: %v", err)
	}
	if appA.Application.Status != domain.StatusAwaitingPayment {
		t.Fatalf("expected awaiting_payment, got %s", appA.Application.Status)
	}

	// 1. Idempotent manual_transfer creation
	pay1, err := svc.CreatePayment(ctx, pmbmTenant.ID, guardianID, appA.Application.ID, admissions.CreatePaymentInput{
		Purpose: admissions.PaymentPurposeRegistration,
		Method:  admissions.PaymentMethodManualTransfer,
	}, "inv-a-1", actorApplicant)
	if err != nil {
		t.Fatalf("CreatePayment manual: %v", err)
	}
	pay1Replay, err := svc.CreatePayment(ctx, pmbmTenant.ID, guardianID, appA.Application.ID, admissions.CreatePaymentInput{
		Purpose: admissions.PaymentPurposeRegistration,
		Method:  admissions.PaymentMethodManualTransfer,
	}, "inv-a-1", actorApplicant)
	if err != nil || pay1Replay.ID != pay1.ID {
		t.Fatalf("idempotent payment mismatch: err=%v, id1=%s, id2=%s", err, pay1.ID, pay1Replay.ID)
	}
	if pay1.AmountIDR != 150000 || pay1.Status != admissions.PaymentStatusPending {
		t.Fatalf("unexpected invoice fields: %+v", pay1)
	}

	// Treasurer cannot verify manual transfer paid without proof attachment
	if _, err := svc.VerifyManualPayment(ctx, pmbmTenant.ID, pay1.ID, admissions.VerifyManualPaymentInput{
		Decision: admissions.PaymentStatusPaid,
	}, actorTreasurer); !errors.Is(err, admissions.ErrManualProofRequired) {
		t.Fatalf("expected ErrManualProofRequired, got %v", err)
	}

	// Applicant presigns and attaches receipt proof (must upload to storage first!)
	proofPresign, err := svc.PresignManualProof(ctx, pmbmTenant.ID, guardianID, appA.Application.ID, pay1.ID, admissions.PresignProofInput{
		Filename: "struk-bri.jpg", MimeType: "image/jpeg", SizeBytes: 120000,
	})
	if err != nil || proofPresign.UploadURL == "" {
		t.Fatalf("PresignManualProof: %v", err)
	}
	if _, err := svc.AttachManualProof(ctx, pmbmTenant.ID, guardianID, appA.Application.ID, pay1.ID, admissions.AttachManualProofInput{
		ProofStorageKey: proofPresign.StorageKey,
	}); err == nil {
		t.Fatal("expected ValidationError when transfer receipt object not uploaded yet")
	}
	uploadTestObject(t, ctx, objStore, proofPresign.StorageKey, "image/jpeg", 120000)
	attached, err := svc.AttachManualProof(ctx, pmbmTenant.ID, guardianID, appA.Application.ID, pay1.ID, admissions.AttachManualProofInput{
		ProofStorageKey: proofPresign.StorageKey,
	})
	if err != nil || attached.ProofStorageKey == nil || *attached.ProofStorageKey != proofPresign.StorageKey {
		t.Fatalf("AttachManualProof: %v (%+v)", err, attached)
	}

	// Treasurer rejects without note -> ValidationError; approves with note -> transitions awaiting_payment -> draft_form
	var valErr *admissions.ValidationError
	if _, err := svc.VerifyManualPayment(ctx, pmbmTenant.ID, pay1.ID, admissions.VerifyManualPaymentInput{
		Decision: admissions.PaymentStatusFailed,
	}, actorTreasurer); !errors.As(err, &valErr) {
		t.Fatalf("expected ValidationError for empty reject note, got %v", err)
	}

	verifiedRes, err := svc.VerifyManualPayment(ctx, pmbmTenant.ID, pay1.ID, admissions.VerifyManualPaymentInput{
		Decision: admissions.PaymentStatusPaid,
		Note:     "Mutasi rekening BCA terkonfirmasi",
	}, actorTreasurer)
	if err != nil {
		t.Fatalf("VerifyManualPayment paid: %v", err)
	}
	if verifiedRes.Payment.Status != admissions.PaymentStatusPaid || verifiedRes.Application.Application.Status != domain.StatusDraftForm {
		t.Fatalf("expected paid + draft_form, got pay=%s app=%s", verifiedRes.Payment.Status, verifiedRes.Application.Application.Status)
	}

	// 2. Gateway QRIS payment + webhook HMAC verification + idempotent replay
	appB, err := svc.CreateApplication(ctx, pmbmTenant.ID, guardianID, admissions.CreateApplicationInput{
		UnitID: unit.ID, PeriodID: period.ID, TrackID: regulerTrack.ID, StudentFullName: "Fatimah QRIS",
	}, "pay-app-b", actorApplicant)
	if err != nil {
		t.Fatalf("create appB: %v", err)
	}
	qrisPay, err := svc.CreatePayment(ctx, pmbmTenant.ID, guardianID, appB.Application.ID, admissions.CreatePaymentInput{
		Purpose: admissions.PaymentPurposeRegistration,
		Method:  admissions.PaymentMethodQRIS,
	}, "inv-b-1", actorApplicant)
	if err != nil || qrisPay.PaymentURL == nil || qrisPay.ExternalRef == nil {
		t.Fatalf("CreatePayment QRIS failed: err=%v, pay=%+v", err, qrisPay)
	}

	rawBody := []byte(`{"external_ref":"` + *qrisPay.ExternalRef + `","status":"settlement"}`)
	secret := "test-webhook-secret"
	if _, _, err := svc.HandlePaymentWebhook(ctx, pmbmTenant.ID, rawBody, "sha256=deadbeef", secret, admissions.PaymentWebhookInput{
		Provider:    "midtrans",
		ExternalRef: *qrisPay.ExternalRef,
		Status:      "settlement",
	}); !errors.Is(err, admissions.ErrInvalidWebhookSignature) {
		t.Fatalf("expected ErrInvalidWebhookSignature, got %v", err)
	}

	sig := signWebhookPayload(secret, rawBody)
	hookRes, already, err := svc.HandlePaymentWebhook(ctx, pmbmTenant.ID, rawBody, sig, secret, admissions.PaymentWebhookInput{
		Provider:    "midtrans",
		ExternalRef: *qrisPay.ExternalRef,
		Status:      "settlement",
	})
	if err != nil || already || hookRes.Application.Application.Status != domain.StatusDraftForm {
		t.Fatalf("HandlePaymentWebhook first pass: err=%v, already=%v, res=%+v", err, already, hookRes)
	}

	hookReplay, already2, err := svc.HandlePaymentWebhook(ctx, pmbmTenant.ID, rawBody, sig, secret, admissions.PaymentWebhookInput{
		Provider:    "midtrans",
		ExternalRef: *qrisPay.ExternalRef,
		Status:      "settlement",
	})
	if err != nil || !already2 || hookReplay.Payment.Status != admissions.PaymentStatusPaid {
		t.Fatalf("HandlePaymentWebhook replay failed: err=%v, already2=%v", err, already2)
	}
}

func TestScoringRankingSimulationPublishPromoteAndPaidReEnroll(t *testing.T) {
	ctx := context.Background()
	svc, pubDemo, objStore := setupAdmissionsTestWithStorage(t)

	spmbTenant, _ := pubDemo.FindBySlug(ctx, "demo-spmb")
	spmbPortal, _ := pubDemo.GetPortalData(ctx, spmbTenant)
	unit := spmbPortal.Units[0]
	period := spmbPortal.Periods[0]
	domisiliTrack := period.Tracks[0]

	examinerID := uuid.MustParse("01960000-0000-7000-8001-000000000005")
	principalID := uuid.MustParse("01960000-0000-7000-8001-000000000006")
	verifierID := uuid.MustParse("01960000-0000-7000-8001-000000000002")

	createVerifiedCandidate := func(guardianID uuid.UUID, name string, latOffset float64, birthDate string) *admissions.ApplicationDetail {
		actorApp := admissions.ActorContext{UserID: guardianID, RoleName: "applicant"}
		created, err := svc.CreateApplication(ctx, spmbTenant.ID, guardianID, admissions.CreateApplicationInput{
			UnitID: unit.ID, PeriodID: period.ID, TrackID: domisiliTrack.ID, StudentFullName: name,
		}, "", actorApp)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		lat := *created.UnitLatitude + latOffset
		lon := *created.UnitLongitude
		bd := birthDate
		ver := created.Application.Version
		updated, err := svc.UpdateProfile(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.UpdateProfileInput{
			ApplicantProfile: admissions.ApplicantProfile{
				FullName:  name,
				BirthDate: &bd,
				Latitude:  &lat,
				Longitude: &lon,
			},
			ExpectedVersion: &ver,
		})
		if err != nil {
			t.Fatalf("update %s: %v", name, err)
		}

		for _, req := range updated.Requirements {
			if !req.IsRequired {
				continue
			}
			pre, err := svc.PresignUpload(ctx, spmbTenant.ID, guardianID, updated.Application.ID, admissions.PresignUploadInput{
				RequirementID: req.ID,
				Filename:      "doc.pdf",
				MimeType:      "application/pdf",
				SizeBytes:     1024,
			})
			if err != nil {
				t.Fatalf("presign %s doc: %v", name, err)
			}
			uploadTestObject(t, ctx, objStore, pre.StorageKey, "application/pdf", 1024)
			if _, err := svc.ConfirmDocument(ctx, spmbTenant.ID, guardianID, updated.Application.ID, admissions.ConfirmDocumentInput{
				RequirementID: req.ID, StorageKey: pre.StorageKey, OriginalFilename: "doc.pdf", MimeType: "application/pdf", SizeBytes: 1024,
			}); err != nil {
				t.Fatalf("confirm %s doc: %v", name, err)
			}
		}
		refreshed, _ := svc.GetApplicantApplication(ctx, spmbTenant.ID, guardianID, updated.Application.ID)
		submitted, err := svc.SubmitApplication(ctx, spmbTenant.ID, guardianID, updated.Application.ID, refreshed.Application.Version, "", actorApp)
		if err != nil {
			t.Fatalf("submit %s: %v", name, err)
		}

		verActor := admissions.ActorContext{UserID: verifierID, RoleName: "verifier"}
		for _, doc := range submitted.Documents {
			if _, err := svc.ReviewDocument(ctx, spmbTenant.ID, submitted.Application.ID, doc.ID, domain.DocStatusApproved, "OK", verActor); err != nil {
				t.Fatalf("review doc: %v", err)
			}
		}
		afterRev, _ := svc.GetApplicationForStaff(ctx, spmbTenant.ID, submitted.Application.ID)
		verified, err := svc.CompleteVerification(ctx, spmbTenant.ID, submitted.Application.ID, afterRev.Application.Version, "Sah", verActor)
		if err != nil {
			t.Fatalf("complete verify %s: %v", name, err)
		}
		return verified
	}

	g1 := uuid.New()
	g2 := uuid.New()
	candFarHighScore := createVerifiedCandidate(g1, "Juara Ujian", 0.02, "2010-09-01")
	candEqualScoreNear := createVerifiedCandidate(g2, "Dekat Rumah", 0.002, "2010-10-01")

	exActor := admissions.ActorContext{UserID: examinerID, RoleName: "examiner"}
	scoreRes1, err := svc.UpsertScores(ctx, spmbTenant.ID, candFarHighScore.Application.ID, admissions.UpsertScoresInput{
		Scores: []admissions.ScoreComponentInput{
			{ComponentCode: "TPA", RawScore: 90, WeightPct: 60},
			{ComponentCode: "WAWANCARA", RawScore: 85, WeightPct: 40},
		},
	}, exActor)
	if err != nil {
		t.Fatalf("UpsertScores 1: %v", err)
	}
	if scoreRes1.FinalScore != 88.0 || scoreRes1.Application.Application.Status != domain.StatusSelectionInProgress {
		t.Fatalf("unexpected score 1: final=%.2f status=%s", scoreRes1.FinalScore, scoreRes1.Application.Application.Status)
	}

	scoreRes2, err := svc.UpsertScores(ctx, spmbTenant.ID, candEqualScoreNear.Application.ID, admissions.UpsertScoresInput{
		Scores: []admissions.ScoreComponentInput{
			{ComponentCode: "TPA", RawScore: 88, WeightPct: 100},
		},
	}, exActor)
	if err != nil {
		t.Fatalf("UpsertScores 2: %v", err)
	}
	if scoreRes2.FinalScore != 88.0 {
		t.Fatalf("expected 88.0, got %.2f", scoreRes2.FinalScore)
	}

	// Tie-breaker with equal final_score (88.0): candEqualScoreNear has shorter distance -> rank #1
	sim, err := svc.SimulateTrackRanking(ctx, spmbTenant.ID, domisiliTrack.ID)
	if err != nil {
		t.Fatalf("SimulateTrackRanking: %v", err)
	}
	if len(sim.Candidates) != 2 || sim.Candidates[0].ApplicationID != candEqualScoreNear.Application.ID {
		t.Fatalf("expected closer domicile candidate rank #1, got %+v", sim.Candidates)
	}

	// Publish Period Results -> sets results_published_at & accepted status
	prActor := admissions.ActorContext{UserID: principalID, RoleName: "principal"}
	pub, err := svc.PublishPeriodResults(ctx, spmbTenant.ID, period.ID, "pub-spmb-1", prActor)
	if err != nil {
		t.Fatalf("PublishPeriodResults: %v", err)
	}
	if pub.TotalAccepted != 2 {
		t.Fatalf("expected 2 accepted, got %+v", pub)
	}

	// Free track ReEnroll requires agreement_accepted, then directly becomes enrolled
	appNear, _ := svc.GetApplicantApplication(ctx, spmbTenant.ID, g2, candEqualScoreNear.Application.ID)
	if appNear.Application.Status != domain.StatusAccepted || appNear.Application.ResultsPublishedAt == nil {
		t.Fatalf("expected accepted with results_published_at, got %+v", appNear.Application)
	}
	if _, err := svc.ReEnroll(ctx, spmbTenant.ID, g2, appNear.Application.ID, admissions.ReEnrollInput{
		ExpectedVersion:   appNear.Application.Version,
		AgreementAccepted: false,
	}, admissions.ActorContext{UserID: g2, RoleName: "applicant"}); !errors.Is(err, admissions.ErrAgreementRequired) {
		t.Fatalf("expected ErrAgreementRequired, got %v", err)
	}

	enrolledFree, err := svc.ReEnroll(ctx, spmbTenant.ID, g2, appNear.Application.ID, admissions.ReEnrollInput{
		ExpectedVersion:   appNear.Application.Version,
		AgreementAccepted: true,
		ExtraAnswers:      map[string]any{"ukuran_seragam": "L"},
	}, admissions.ActorContext{UserID: g2, RoleName: "applicant"})
	if err != nil || enrolledFree.Application.Status != domain.StatusEnrolled {
		t.Fatalf("free ReEnroll failed: err=%v status=%v", err, enrolledFree)
	}
	if enrolledFree.ExtraAnswers["ukuran_seragam"] != "L" {
		t.Fatalf("expected extra_answers merged, got %+v", enrolledFree.ExtraAnswers)
	}
}

type staticPaymentConfigProvider map[uuid.UUID]*domain.TenantPaymentConfig

func (p staticPaymentConfigProvider) GetTenantPaymentConfig(_ context.Context, tenantID uuid.UUID) (*domain.TenantPaymentConfig, error) {
	return p[tenantID], nil
}

func TestMidtransWebhookSyncAndAssessmentSchedules(t *testing.T) {
	ctx := context.Background()
	svc, pubDemo, objStore := setupAdmissionsTestWithStorage(t)

	pmbmTenant, err := pubDemo.FindBySlug(ctx, "demo-pmbm")
	if err != nil || pmbmTenant == nil {
		t.Fatalf("resolve demo-pmbm tenant: %v", err)
	}
	pmbmPortal, _ := pubDemo.GetPortalData(ctx, pmbmTenant)
	unit := pmbmPortal.Units[0]
	period := pmbmPortal.Periods[0]
	track := period.Tracks[0]

	tenantServerKey := "SB-Mid-server-tenant-secret"
	svc.SetPaymentConfigProvider(staticPaymentConfigProvider{
		pmbmTenant.ID: {
			MidtransServerKey: tenantServerKey,
		},
	})

	midtransSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/snap/v1/transactions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":        "snap-tok-123",
				"redirect_url": "https://app.sandbox.midtrans.com/snap/v4/redirection/snap-tok-123",
			})
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"order_id":           "INV-SYNC",
				"transaction_status": "settlement",
				"fraud_status":       "accept",
				"status_code":        "200",
				"gross_amount":       "150000.00",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer midtransSrv.Close()

	svc.SetPaymentGateway(&integrations.MidtransSnapGateway{
		ServerKey:         "platform-fallback",
		Sandbox:           true,
		EndpointURL:       midtransSrv.URL + "/snap/v1/transactions",
		StatusEndpointFmt: midtransSrv.URL + "/v2/%s/status",
	})

	guardianID := uuid.MustParse("01960000-0000-7000-8001-000000000001")
	actorApp := admissions.ActorContext{UserID: guardianID, RoleName: "applicant"}
	appC, err := svc.CreateApplication(ctx, pmbmTenant.ID, guardianID, admissions.CreateApplicationInput{
		UnitID: unit.ID, PeriodID: period.ID, TrackID: track.ID, StudentFullName: "Bilal Midtrans",
	}, "", actorApp)
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	if appC.VerificationToken == "" {
		t.Fatal("expected VerificationToken when DocumentSecret configured")
	}

	pay, err := svc.CreatePayment(ctx, pmbmTenant.ID, guardianID, appC.Application.ID, admissions.CreatePaymentInput{
		Purpose: admissions.PaymentPurposeRegistration,
		Method:  admissions.PaymentMethodQRIS,
	}, "", actorApp)
	if err != nil || pay.PaymentURL == nil || *pay.PaymentURL == "" {
		t.Fatalf("CreatePayment Midtrans: err=%v, pay=%+v", err, pay)
	}

	// Verify central Midtrans webhook without tenantID (uuid.Nil) using SHA512 signature_key
	sum := sha512.Sum512([]byte(*pay.ExternalRef + "200" + "150000.00" + tenantServerKey))
	sigKey := hex.EncodeToString(sum[:])
	hookInput := admissions.PaymentWebhookInput{
		Provider:          "midtrans",
		OrderID:           *pay.ExternalRef,
		StatusCode:        "200",
		GrossAmount:       "150000.00",
		SignatureKey:      sigKey,
		TransactionStatus: "settlement",
		FraudStatus:       "accept",
	}
	rawHook, _ := json.Marshal(hookInput)
	res, already, err := svc.HandlePaymentWebhook(ctx, uuid.Nil, rawHook, "", "wrong-fallback", hookInput)
	if err != nil || already || res.Payment.Status != admissions.PaymentStatusPaid || res.Application.Application.Status != domain.StatusDraftForm {
		t.Fatalf("central Midtrans webhook failed: err=%v already=%v res=%+v", err, already, res)
	}

	// Second invoice reconciled actively via SyncPaymentStatus -> Midtrans API GET status
	appD, err := svc.CreateApplication(ctx, pmbmTenant.ID, guardianID, admissions.CreateApplicationInput{
		UnitID: unit.ID, PeriodID: period.ID, TrackID: track.ID, StudentFullName: "Salman Sync",
	}, "", actorApp)
	if err != nil {
		t.Fatalf("CreateApplication D: %v", err)
	}
	payD, err := svc.CreatePayment(ctx, pmbmTenant.ID, guardianID, appD.Application.ID, admissions.CreatePaymentInput{
		Purpose: admissions.PaymentPurposeRegistration,
		Method:  admissions.PaymentMethodVA,
	}, "", actorApp)
	if err != nil {
		t.Fatalf("CreatePayment D: %v", err)
	}
	synced, err := svc.SyncPaymentStatus(ctx, pmbmTenant.ID, &guardianID, payD.ID, actorApp)
	if err != nil || synced.Payment.Status != admissions.PaymentStatusPaid || synced.Application.Application.Status != domain.StatusDraftForm {
		t.Fatalf("SyncPaymentStatus failed: err=%v synced=%+v", err, synced)
	}

	// Upload docs & verify assessment schedule CRUD
	for _, req := range appC.Requirements {
		if !req.IsRequired {
			continue
		}
		pre, err := svc.PresignUpload(ctx, pmbmTenant.ID, guardianID, appC.Application.ID, admissions.PresignUploadInput{
			RequirementID: req.ID, Filename: "doc.pdf", MimeType: "application/pdf", SizeBytes: 1024,
		})
		if err != nil {
			t.Fatalf("PresignUpload: %v", err)
		}
		uploadTestObject(t, ctx, objStore, pre.StorageKey, "application/pdf", 1024)
		if _, err := svc.ConfirmDocument(ctx, pmbmTenant.ID, guardianID, appC.Application.ID, admissions.ConfirmDocumentInput{
			RequirementID: req.ID, StorageKey: pre.StorageKey, OriginalFilename: "doc.pdf", MimeType: "application/pdf", SizeBytes: 1024,
		}); err != nil {
			t.Fatalf("ConfirmDocument: %v", err)
		}
	}

	room := "Ruang CBT-1"
	staffActor := admissions.ActorContext{UserID: uuid.New(), RoleName: "unit_admin"}
	createdScheds, err := svc.CreateAssessmentSchedule(ctx, pmbmTenant.ID, admissions.CreateAssessmentScheduleInput{
		ApplicationIDs: []uuid.UUID{appC.Application.ID},
		SessionName:    "Tes Potensi Akademik Gelombang 1",
		RoomOrLink:     &room,
		ScheduledAt:    time.Now().UTC().Add(48 * time.Hour),
	}, staffActor)
	if err != nil || len(createdScheds) != 1 {
		t.Fatalf("CreateAssessmentSchedule failed: err=%v scheds=%+v", err, createdScheds)
	}

	loadedApp, err := svc.GetApplicantApplication(ctx, pmbmTenant.ID, guardianID, appC.Application.ID)
	if err != nil || len(loadedApp.Schedules) != 1 {
		t.Fatalf("expected schedule embedded on ApplicationDetail: err=%v len=%d", err, len(loadedApp.Schedules))
	}
	if err := svc.DeleteAssessmentSchedule(ctx, pmbmTenant.ID, createdScheds[0].ID, staffActor); err != nil {
		t.Fatalf("DeleteAssessmentSchedule: %v", err)
	}
}
