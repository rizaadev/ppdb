package admissions_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/modules/admissions"
	"github.com/pbdb/platform/internal/modules/public"
	"github.com/pbdb/platform/internal/storage"
)

func setupAdmissionsTestWithStorage(t *testing.T) (*admissions.Service, *public.DemoStore, storage.ObjectStorage) {
	t.Helper()
	pubDemo := public.NewDemoStore()
	admDemo := admissions.NewDemoStore()
	localStore, err := storage.NewLocalStorage(t.TempDir(), "/api/v1/storage/object", "test-storage-secret")
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}
	svc := admissions.NewService(admDemo, localStore, admissions.NewIdempotencyCache(time.Hour))
	svc.SetDocumentSecret("test-doc-qr-secret")
	return svc, pubDemo, localStore
}

func setupAdmissionsTest(t *testing.T) (*admissions.Service, *public.DemoStore) {
	svc, pubDemo, _ := setupAdmissionsTestWithStorage(t)
	return svc, pubDemo
}

func uploadTestObject(t *testing.T, ctx context.Context, st storage.ObjectStorage, key, mime string, size int) {
	t.Helper()
	if err := st.PutObject(ctx, key, mime, bytes.Repeat([]byte("A"), size)); err != nil {
		t.Fatalf("PutObject(%s): %v", key, err)
	}
}

func TestApplicantWorkflow_CRUDPresignHaversineSubmitAndVerificationLoop(t *testing.T) {
	ctx := context.Background()
	svc, pubDemo, objStore := setupAdmissionsTestWithStorage(t)

	spmbTenant, err := pubDemo.FindBySlug(ctx, "demo-spmb")
	if err != nil || spmbTenant == nil {
		t.Fatalf("resolve demo-spmb tenant: %v", err)
	}
	portal, err := pubDemo.GetPortalData(ctx, spmbTenant)
	if err != nil {
		t.Fatalf("GetPortalData: %v", err)
	}

	unit := portal.Units[0]
	period := portal.Periods[0]
	domisiliTrack := period.Tracks[0] // RequiresCoordinates=true, Fee=0 -> draft_form

	guardianID := uuid.MustParse("01960000-0000-7000-8001-000000000001")
	verifierID := uuid.MustParse("01960000-0000-7000-8001-000000000002")

	// 1. Idempotent CreateApplication
	actorApplicant := admissions.ActorContext{UserID: guardianID, RoleName: "applicant"}
	created, err := svc.CreateApplication(ctx, spmbTenant.ID, guardianID, admissions.CreateApplicationInput{
		UnitID:          unit.ID,
		PeriodID:        period.ID,
		TrackID:         domisiliTrack.ID,
		StudentFullName: "Rizky Pratama",
	}, "create-key-1", actorApplicant)
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	if created.Application.Status != domain.StatusDraftForm {
		t.Fatalf("expected initial status draft_form, got %s", created.Application.Status)
	}

	replayed, err := svc.CreateApplication(ctx, spmbTenant.ID, guardianID, admissions.CreateApplicationInput{
		UnitID:          unit.ID,
		PeriodID:        period.ID,
		TrackID:         domisiliTrack.ID,
		StudentFullName: "Rizky Pratama",
	}, "create-key-1", actorApplicant)
	if err != nil || replayed.Application.ID != created.Application.ID {
		t.Fatalf("idempotent create mismatch: err=%v, id1=%s, id2=%s", err, created.Application.ID, replayed.Application.ID)
	}

	// Paid track starts in awaiting_payment
	pmbmTenant, _ := pubDemo.FindBySlug(ctx, "demo-pmbm")
	pmbmPortal, _ := pubDemo.GetPortalData(ctx, pmbmTenant)
	pmbmApp, err := svc.CreateApplication(ctx, pmbmTenant.ID, guardianID, admissions.CreateApplicationInput{
		UnitID:          pmbmPortal.Units[0].ID,
		PeriodID:        pmbmPortal.Periods[0].ID,
		TrackID:         pmbmPortal.Periods[0].Tracks[0].ID,
		StudentFullName: "Zaid Ahmad",
	}, "", actorApplicant)
	if err != nil {
		t.Fatalf("Create paid app failed: %v", err)
	}
	if pmbmApp.Application.Status != domain.StatusAwaitingPayment {
		t.Fatalf("expected awaiting_payment on paid track, got %s", pmbmApp.Application.Status)
	}

	// Cross-guardian ownership check
	otherGuardian := uuid.New()
	if _, err := svc.GetApplicantApplication(ctx, spmbTenant.ID, otherGuardian, created.Application.ID); !errors.Is(err, admissions.ErrApplicationNotFound) {
		t.Fatalf("expected ErrApplicationNotFound for other guardian, got %v", err)
	}

	// 2. Profile validation + optimistic lock + Haversine calculation
	badNIK := "12345"
	if _, err := svc.UpdateProfile(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.UpdateProfileInput{
		ApplicantProfile: admissions.ApplicantProfile{
			FullName: "Rizky Pratama",
			NIK:      &badNIK,
		},
	}); err == nil {
		t.Fatal("expected ValidationError for invalid 5-digit NIK")
	}

	// Stale version fails
	staleVer := 99
	if _, err := svc.UpdateProfile(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.UpdateProfileInput{
		ExpectedVersion: &staleVer,
		ApplicantProfile: admissions.ApplicantProfile{
			FullName: "Rizky Pratama",
		},
	}); !errors.Is(err, admissions.ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict, got %v", err)
	}

	// Update coordinates (~540m east of unit)
	lat := -6.2415
	lon := 106.8880
	validNIK := "3175091234560001"
	ver1 := created.Application.Version
	updated, err := svc.UpdateProfile(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.UpdateProfileInput{
		ExpectedVersion: &ver1,
		ApplicantProfile: admissions.ApplicantProfile{
			FullName:  "Rizky Pratama",
			NIK:       &validNIK,
			Latitude:  &lat,
			Longitude: &lon,
		},
	})
	if err != nil {
		t.Fatalf("UpdateProfile with coordinates: %v", err)
	}
	if updated.Application.DistanceMeters == nil || *updated.Application.DistanceMeters < 400 || *updated.Application.DistanceMeters > 700 {
		t.Fatalf("unexpected Haversine distance_meters: %v", updated.Application.DistanceMeters)
	}

	// Submit fails before uploading required documents (kk & akta)
	if _, err := svc.SubmitApplication(ctx, spmbTenant.ID, guardianID, created.Application.ID, updated.Application.Version, "", actorApplicant); err == nil {
		t.Fatal("expected MissingDocumentsError before upload")
	} else {
		var missErr *admissions.MissingDocumentsError
		if !errors.As(err, &missErr) || len(missErr.MissingDocCodes) != 2 {
			t.Fatalf("expected 2 missing docs (kk, akta), got %v", err)
		}
	}

	// 3. Presign upload validation (MIME & size & anti-IDOR confirm)
	kkReq := domisiliTrack.Requirements[0]
	aktaReq := domisiliTrack.Requirements[1]

	if _, err := svc.PresignUpload(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.PresignUploadInput{
		RequirementID: kkReq.ID,
		Filename:      "evil.exe",
		MimeType:      "application/x-msdownload",
		SizeBytes:     1024,
	}); !errors.Is(err, admissions.ErrUnsupportedMime) {
		t.Fatalf("expected ErrUnsupportedMime, got %v", err)
	}

	if _, err := svc.PresignUpload(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.PresignUploadInput{
		RequirementID: kkReq.ID,
		Filename:      "huge.pdf",
		MimeType:      "application/pdf",
		SizeBytes:     10 * 1024 * 1024,
	}); !errors.Is(err, admissions.ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}

	if _, err := svc.ConfirmDocument(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.ConfirmDocumentInput{
		RequirementID:    kkReq.ID,
		StorageKey:       "other-tenant/other-app/kk.pdf",
		OriginalFilename: "kk.pdf",
		MimeType:         "application/pdf",
		SizeBytes:        50000,
	}); !errors.Is(err, admissions.ErrInvalidStorageKey) {
		t.Fatalf("expected ErrInvalidStorageKey on foreign storage key, got %v", err)
	}

	// Upload KK + Akta cleanly (HeadObject rejects missing objects first!)
	kkPresign, err := svc.PresignUpload(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.PresignUploadInput{
		RequirementID: kkReq.ID,
		Filename:      "Kartu Keluarga.pdf",
		MimeType:      "application/pdf",
		SizeBytes:     120_000,
	})
	if err != nil {
		t.Fatalf("PresignUpload KK: %v", err)
	}
	if _, err := svc.ConfirmDocument(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.ConfirmDocumentInput{
		RequirementID:    kkReq.ID,
		StorageKey:       kkPresign.StorageKey,
		OriginalFilename: "Kartu Keluarga.pdf",
		MimeType:         "application/pdf",
		SizeBytes:        120_000,
	}); err == nil {
		t.Fatal("expected ValidationError when object has not been uploaded yet")
	}
	uploadTestObject(t, ctx, objStore, kkPresign.StorageKey, "application/pdf", 120_000)
	kkDoc, err := svc.ConfirmDocument(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.ConfirmDocumentInput{
		RequirementID:    kkReq.ID,
		StorageKey:       kkPresign.StorageKey,
		OriginalFilename: "Kartu Keluarga.pdf",
		MimeType:         "application/pdf",
		SizeBytes:        120_000,
	})
	if err != nil {
		t.Fatalf("ConfirmDocument KK: %v", err)
	}

	aktaPresign, _ := svc.PresignUpload(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.PresignUploadInput{
		RequirementID: aktaReq.ID,
		Filename:      "akta.jpg",
		MimeType:      "image/jpeg",
		SizeBytes:     240_000,
	})
	uploadTestObject(t, ctx, objStore, aktaPresign.StorageKey, "image/jpeg", 240_000)
	aktaDoc, err := svc.ConfirmDocument(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.ConfirmDocumentInput{
		RequirementID:    aktaReq.ID,
		StorageKey:       aktaPresign.StorageKey,
		OriginalFilename: "akta.jpg",
		MimeType:         "image/jpeg",
		SizeBytes:        240_000,
	})
	if err != nil {
		t.Fatalf("ConfirmDocument Akta: %v", err)
	}

	// Download presign works for owner
	if dl, err := svc.PresignApplicantDownload(ctx, spmbTenant.ID, guardianID, created.Application.ID, kkDoc.ID); err != nil || dl.URL == "" {
		t.Fatalf("PresignApplicantDownload: err=%v", err)
	}

	// 4. Submit succeeds & Idempotency-Key replays cleanly
	submitted, err := svc.SubmitApplication(ctx, spmbTenant.ID, guardianID, created.Application.ID, updated.Application.Version, "sub-1", actorApplicant)
	if err != nil {
		t.Fatalf("SubmitApplication: %v", err)
	}
	if submitted.Application.Status != domain.StatusSubmitted {
		t.Fatalf("expected submitted, got %s", submitted.Application.Status)
	}
	replaySubmit, err := svc.SubmitApplication(ctx, spmbTenant.ID, guardianID, created.Application.ID, updated.Application.Version, "sub-1", actorApplicant)
	if err != nil || replaySubmit.Application.Version != submitted.Application.Version {
		t.Fatalf("idempotent submit failed: %v", err)
	}

	// Locked from editing once submitted
	if _, err := svc.UpdateProfile(ctx, spmbTenant.ID, guardianID, created.Application.ID, admissions.UpdateProfileInput{
		ApplicantProfile: admissions.ApplicantProfile{FullName: "Mutated After Submit"},
	}); !errors.Is(err, admissions.ErrApplicationLocked) {
		t.Fatalf("expected ErrApplicationLocked while submitted, got %v", err)
	}

	// 5. Verifier claims queue -> rejects KK -> completes verification as revision_required
	actorVerifier := admissions.ActorContext{UserID: verifierID, RoleName: "verifier"}
	claimed, err := svc.ClaimNextForVerification(ctx, spmbTenant.ID, unit.ID, actorVerifier)
	if err != nil {
		t.Fatalf("ClaimNextForVerification: %v", err)
	}
	if claimed.Application.Status != domain.StatusUnderVerification {
		t.Fatalf("expected under_verification, got %s", claimed.Application.Status)
	}
	if _, err := svc.ClaimNextForVerification(ctx, spmbTenant.ID, unit.ID, actorVerifier); !errors.Is(err, admissions.ErrVerificationQueueEmpty) {
		t.Fatalf("expected empty queue on 2nd claim, got %v", err)
	}

	if _, err := svc.ReviewDocument(ctx, spmbTenant.ID, claimed.Application.ID, kkDoc.ID, domain.DocStatusRejectedForRevision, "", actorVerifier); !errors.Is(err, admissions.ErrReviewNoteRequired) {
		t.Fatalf("expected ErrReviewNoteRequired on reject without note, got %v", err)
	}
	if _, err := svc.ReviewDocument(ctx, spmbTenant.ID, claimed.Application.ID, kkDoc.ID, domain.DocStatusRejectedForRevision, "Scan KK terpotong di bagian bawah", actorVerifier); err != nil {
		t.Fatalf("ReviewDocument reject KK: %v", err)
	}
	if _, err := svc.ReviewDocument(ctx, spmbTenant.ID, claimed.Application.ID, aktaDoc.ID, domain.DocStatusApproved, "Sesuai", actorVerifier); err != nil {
		t.Fatalf("ReviewDocument approve Akta: %v", err)
	}

	revisionApp, err := svc.CompleteVerification(ctx, spmbTenant.ID, claimed.Application.ID, claimed.Application.Version, "Mohon unggah ulang KK lengkap", actorVerifier)
	if err != nil {
		t.Fatalf("CompleteVerification revision_required: %v", err)
	}
	if revisionApp.Application.Status != domain.StatusRevisionRequired {
		t.Fatalf("expected revision_required, got %s", revisionApp.Application.Status)
	}

	// Applicant cannot resubmit until rejected KK is replaced
	if _, err := svc.SubmitApplication(ctx, spmbTenant.ID, guardianID, revisionApp.Application.ID, revisionApp.Application.Version, "sub-fail", actorApplicant); err == nil {
		t.Fatal("expected MissingDocumentsError while KK still rejected_for_revision")
	}

	// Re-upload KK -> Resubmit -> Verifier approves -> CompleteVerification = verified!
	kkPresign2, _ := svc.PresignUpload(ctx, spmbTenant.ID, guardianID, revisionApp.Application.ID, admissions.PresignUploadInput{
		RequirementID: kkReq.ID,
		Filename:      "kk-jelas.pdf",
		MimeType:      "application/pdf",
		SizeBytes:     150_000,
	})
	uploadTestObject(t, ctx, objStore, kkPresign2.StorageKey, "application/pdf", 150_000)
	if _, err := svc.ConfirmDocument(ctx, spmbTenant.ID, guardianID, revisionApp.Application.ID, admissions.ConfirmDocumentInput{
		RequirementID:    kkReq.ID,
		StorageKey:       kkPresign2.StorageKey,
		OriginalFilename: "kk-jelas.pdf",
		MimeType:         "application/pdf",
		SizeBytes:        150_000,
	}); err != nil {
		t.Fatalf("ConfirmDocument KK revision: %v", err)
	}

	resubmitted, err := svc.SubmitApplication(ctx, spmbTenant.ID, guardianID, revisionApp.Application.ID, revisionApp.Application.Version, "sub-2", actorApplicant)
	if err != nil {
		t.Fatalf("resubmit failed: %v", err)
	}
	if resubmitted.Application.Status != domain.StatusSubmitted {
		t.Fatalf("expected submitted on resubmit, got %s", resubmitted.Application.Status)
	}

	if _, err := svc.ReviewDocument(ctx, spmbTenant.ID, resubmitted.Application.ID, kkDoc.ID, domain.DocStatusApproved, "Jelas dan lengkap", actorVerifier); err != nil {
		t.Fatalf("ReviewDocument approve revised KK: %v", err)
	}
	// Reload latest version after ReviewDocument moved submitted -> under_verification
	reloaded, _ := svc.GetApplicationForStaff(ctx, spmbTenant.ID, resubmitted.Application.ID)
	verified, err := svc.CompleteVerification(ctx, spmbTenant.ID, reloaded.Application.ID, reloaded.Application.Version, "Seluruh berkas terverifikasi", actorVerifier)
	if err != nil {
		t.Fatalf("CompleteVerification verified: %v", err)
	}
	if verified.Application.Status != domain.StatusVerified || verified.Application.VerifiedAt == nil {
		t.Fatalf("expected verified with verified_at timestamp, got status=%s verified_at=%v", verified.Application.Status, verified.Application.VerifiedAt)
	}
	if len(verified.AuditLogs) < 5 {
		t.Fatalf("expected rich audit trail entries, got %d", len(verified.AuditLogs))
	}
}
