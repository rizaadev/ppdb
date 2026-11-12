package httpapi_test

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pbdb/platform/internal/config"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/httpapi"
	"github.com/pbdb/platform/internal/httpapi/handlers"
	"github.com/pbdb/platform/internal/integrations"
	"github.com/pbdb/platform/internal/modules/admissions"
	"github.com/pbdb/platform/internal/modules/auth"
	"github.com/pbdb/platform/internal/modules/public"
	"github.com/pbdb/platform/internal/seed"
	"github.com/pbdb/platform/internal/storage"
	"github.com/pbdb/platform/internal/worker"
	"github.com/pbdb/platform/web"
)

func TestPostgresEndToEndHTTPWithRLSAndWorkers(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping live PostgreSQL E2E HTTP integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, dbURL, 8, 1)
	if err != nil {
		t.Fatalf("connect postgresql: %v", err)
	}
	defer pool.Close()

	if err := database.RunMigrations(ctx, pool, nil); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := worker.EnsureRiverSchema(ctx, pool); err != nil {
		t.Fatalf("EnsureRiverSchema: %v", err)
	}
	if err := database.GrantRuntimePermissions(ctx, pool); err != nil {
		t.Fatalf("GrantRuntimePermissions: %v", err)
	}
	if err := seed.SeedDemoData(ctx, pool, nil); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}

	const midtransSecret = "SB-Mid-server-PGIntegration"
	const docSecret = "pg-e2e-document-signing-secret-32"

	var waCalls atomic.Int32
	fonnteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		waCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":true,"id":["fn-pg-1"]}`))
	}))
	defer fonnteSrv.Close()

	midtransSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/snap/v1/transactions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"snap-pg-1","redirect_url":"https://app.sandbox.midtrans.com/snap/v2/vtweb/snap-pg-1"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/") && strings.HasSuffix(r.URL.Path, "/status"):
			trimmed := strings.TrimPrefix(r.URL.Path, "/v2/")
			orderID := strings.TrimSuffix(trimmed, "/status")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"order_id":%q,"transaction_status":"settlement","gross_amount":"150000.00"}`, orderID)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer midtransSrv.Close()

	localStore, err := storage.NewLocalStorage(t.TempDir(), "http://localhost:8080", docSecret)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}
	waSender := integrations.NewWhatsAppSender("fonnte", fonnteSrv.URL+"/send", "fonnte-test-token", nil)
	payGw := &integrations.MidtransSnapGateway{
		EndpointURL:       midtransSrv.URL + "/snap/v1/transactions",
		StatusEndpointFmt: midtransSrv.URL + "/v2/%s/status",
		ServerKey:         midtransSecret,
		Sandbox:           true,
	}

	riverClient, err := worker.NewClient(pool, worker.ClientOptions{
		Storage:  localStore,
		WhatsApp: waSender,
	}, true)
	if err != nil {
		t.Fatalf("worker.NewClient: %v", err)
	}
	if err := riverClient.Start(ctx); err != nil {
		t.Fatalf("riverClient.Start: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = riverClient.Stop(stopCtx)
	}()

	pgPublic := public.NewPGStore(pool)
	authStore := auth.NewPGStore(pool, riverClient)
	admStore := admissions.NewPGStore(pool, riverClient)
	jwtMgr := auth.NewJWTManager("pg-e2e-jwt-secret-key-32-bytes-ok", 15*time.Minute)
	otpLimiter := auth.NewPGOTPRateLimiter(pool, 0, time.Hour, 50)
	authSvc := auth.NewService(authStore, jwtMgr, otpLimiter, "pg-e2e-otp-secret", 7*24*time.Hour, true, nil)

	admSvc := admissions.NewService(admStore, localStore, admissions.NewPGIdempotencyCache(pool, time.Hour))
	admSvc.SetPaymentGateway(payGw)
	admSvc.SetPaymentConfigProvider(pgPublic)
	admSvc.SetDocumentSecret(docSecret)

	distFS, err := web.DistFS()
	if err != nil {
		t.Fatalf("web.DistFS: %v", err)
	}

	dns := &stubRouterDNS{txt: map[string][]string{}, cname: map[string]string{}}
	router := httpapi.NewRouter(httpapi.Dependencies{
		Config: config.Config{
			AppEnv:               "development",
			BaseDomain:           "spmb-platform.id",
			PublicBaseURL:        "http://localhost:8080",
			DevFallbackSlug:      "demo-spmb",
			DocumentSecret:       docSecret,
			PaymentWebhookSecret: "pg-webhook-secret",
			MidtransServerKey:    midtransSecret,
		},
		Pool:              pool,
		TenantLookup:      pgPublic,
		PublicStore:       pgPublic,
		PlatformStore:     pgPublic,
		SetupStore:        pgPublic,
		ObjectStorage:     localStore,
		DNSResolver:       dns,
		AuthService:       authSvc,
		AdmissionsService: admSvc,
		JWTManager:        jwtMgr,
		FrontendFS:        distFS,
	})

	suffix := time.Now().UnixNano() % 1000000
	testSlug := fmt.Sprintf("pgtest-%d", suffix)
	adminEmail := fmt.Sprintf("admin-%d@pgtest.sch.id", suffix)
	examinerEmail := fmt.Sprintf("examiner-%d@pgtest.sch.id", suffix)
	guardianPhone := fmt.Sprintf("0819%08d", suffix)
	var createdTenantID uuid.UUID

	defer func() {
		if createdTenantID == uuid.Nil {
			return
		}
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanCancel()
		normGuardian, _ := auth.NormalizeIndonesianPhone(guardianPhone)
		_ = database.WithBypassRLSTx(cleanCtx, pool, func(tx pgx.Tx) error {
			for _, tbl := range []string{
				"export_jobs", "notification_logs", "status_audit_logs", "assessment_scores",
				"assessment_schedules", "payment_transactions", "applicant_documents",
				"applicant_achievements", "applicant_guardians", "applicant_profiles",
				"applications", "track_requirements", "admission_tracks",
				"admission_periods", "user_tenant_roles", "units", "idempotency_keys",
				"otp_challenges", "user_sessions",
			} {
				_, _ = tx.Exec(cleanCtx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, createdTenantID)
			}
			_, _ = tx.Exec(cleanCtx, `DELETE FROM tenants WHERE id = $1`, createdTenantID)
			_, _ = tx.Exec(cleanCtx, `DELETE FROM users WHERE email IN ($1, $2) OR whatsapp_phone = $3`, adminEmail, examinerEmail, normGuardian)
			return nil
		})
	}()

	// 1. Platform onboarding + admin login
	pfLogin := doJSONRequest(t, router, http.MethodPost, "/api/v1/platform/auth/login", "", "", map[string]string{
		"email":    "platform@spmb-platform.id",
		"password": "DemoPass123!",
	})
	if pfLogin.Code != http.StatusOK {
		t.Fatalf("platform login failed: %d (%s)", pfLogin.Code, pfLogin.Body.String())
	}
	var pfEnv struct {
		Data auth.TokenBundle `json:"data"`
	}
	_ = json.Unmarshal(pfLogin.Body.Bytes(), &pfEnv)
	pfToken := pfEnv.Data.AccessToken

	customDom := fmt.Sprintf("ppdb-%d.contoh.sch.id", suffix)
	createTenant := doJSONRequest(t, router, http.MethodPost, "/api/v1/platform/tenants", "", pfToken, map[string]any{
		"slug":               testSlug,
		"name":               "Sekolah Uji PostgreSQL",
		"institution_type":   "school",
		"terminology_preset": "spmb",
		"primary_color":      "#0F766E",
		"tagline":            "RLS Verified",
		"custom_domain":      customDom,
		"subscription_plan":  "pro",
		"starter_unit_code":  "SMA-PG",
		"starter_unit_name":  "SMA Uji PG",
		"starter_unit_level": "sma_ma",
		"admin_email":        adminEmail,
		"admin_full_name":    "Admin PG Test",
		"admin_password":     "AdminPG123!",
	})
	if createTenant.Code != http.StatusCreated {
		t.Fatalf("create tenant on PG failed: %d (%s)", createTenant.Code, createTenant.Body.String())
	}
	var tEnv struct {
		Data handlers.PlatformCreateTenantResult `json:"data"`
	}
	_ = json.Unmarshal(createTenant.Body.Bytes(), &tEnv)
	createdTenantID = tEnv.Data.Tenant.ID

	adminLogin := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", testSlug, "", map[string]string{
		"identifier": adminEmail,
		"password":   "AdminPG123!",
	})
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("admin login on PG failed: %d (%s)", adminLogin.Code, adminLogin.Body.String())
	}
	var adminAuthEnv struct {
		Data auth.TokenBundle `json:"data"`
	}
	_ = json.Unmarshal(adminLogin.Body.Bytes(), &adminAuthEnv)
	adminToken := adminAuthEnv.Data.AccessToken

	// 2. Staff CRUD + DNS verify + Logo upload against PG
	if tEnv.Data.Tenant.DomainVerificationToken != nil {
		dns.txt["_spmb-verify."+customDom] = []string{*tEnv.Data.Tenant.DomainVerificationToken}
	}
	verifyDomain := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/tenant/domain/verify", testSlug, adminToken, nil)
	if verifyDomain.Code != http.StatusOK || !strings.Contains(verifyDomain.Body.String(), `"verified":true`) {
		t.Fatalf("PG domain verify failed: %d (%s)", verifyDomain.Code, verifyDomain.Body.String())
	}

	unitsRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/units", testSlug, adminToken, nil)
	var unitsEnv struct {
		Data []handlers.UnitView `json:"data"`
	}
	_ = json.Unmarshal(unitsRec.Body.Bytes(), &unitsEnv)
	unitID := unitsEnv.Data[0].ID

	staffCreate := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/staff", testSlug, adminToken, map[string]any{
		"email":     examinerEmail,
		"full_name": "Penguji PG",
		"role":      "examiner",
		"unit_id":   unitID,
		"password":  "ExaminerPG1!",
	})
	if staffCreate.Code != http.StatusCreated {
		t.Fatalf("PG staff create failed: %d (%s)", staffCreate.Code, staffCreate.Body.String())
	}
	var exLogin struct {
		Data auth.TokenBundle `json:"data"`
	}
	exRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", testSlug, "", map[string]string{
		"identifier": examinerEmail,
		"password":   "ExaminerPG1!",
	})
	if exRec.Code != http.StatusOK {
		t.Fatalf("new staff login on PG failed: %d (%s)", exRec.Code, exRec.Body.String())
	}
	_ = json.Unmarshal(exRec.Body.Bytes(), &exLogin)

	logoPre := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/tenant/logo/presign", testSlug, adminToken, map[string]any{
		"filename":   "pglogo.png",
		"mime_type":  "image/png",
		"size_bytes": 64,
	})
	var logoPreEnv struct {
		Data admissions.PresignUploadResult `json:"data"`
	}
	_ = json.Unmarshal(logoPre.Body.Bytes(), &logoPreEnv)
	logoBytes := []byte("\x89PNG\r\n\x1a\nPGLOGO")
	uploadSignedURL(t, router, logoPreEnv.Data.UploadURL, "image/png", logoBytes)
	if conf := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/tenant/logo/confirm", testSlug, adminToken, map[string]string{
		"storage_key": logoPreEnv.Data.StorageKey,
	}); conf.Code != http.StatusOK {
		t.Fatalf("PG logo confirm failed: %d (%s)", conf.Code, conf.Body.String())
	}

	// 3. Configure period, weighted track + custom schema, and requirements
	periodRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/periods", testSlug, adminToken, map[string]any{
		"unit_id":       unitID,
		"academic_year": "2026/2027",
		"name":          "Gelombang 1 PG",
		"opens_at":      time.Now().UTC().Add(-24 * time.Hour),
		"closes_at":     time.Now().UTC().Add(30 * 24 * time.Hour),
		"is_published":  true,
		"is_active":     true,
	})
	if periodRec.Code != http.StatusCreated {
		t.Fatalf("PG create period failed: %d (%s)", periodRec.Code, periodRec.Body.String())
	}
	var periodEnv struct {
		Data handlers.PeriodView `json:"data"`
	}
	_ = json.Unmarshal(periodRec.Body.Bytes(), &periodEnv)
	periodID := periodEnv.Data.ID

	trackRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/tracks", testSlug, adminToken, map[string]any{
		"period_id":            periodID,
		"code":                 "PRESTASI-PG",
		"display_name":         "Jalur Prestasi PG",
		"selection_mode":       "test_based",
		"quota":                10,
		"registration_fee_idr": 150000,
		"scoring_weights":      map[string]float64{"academic": 60, "interview": 40},
		"custom_form_schema": []map[string]any{
			{"key": "hobby", "label": "Minat & Bakat", "type": "text", "required": true},
		},
		"is_active": true,
	})
	if trackRec.Code != http.StatusCreated {
		t.Fatalf("PG create track failed: %d (%s)", trackRec.Code, trackRec.Body.String())
	}
	var trackEnv struct {
		Data handlers.TrackView `json:"data"`
	}
	_ = json.Unmarshal(trackRec.Body.Bytes(), &trackEnv)
	trackID := trackEnv.Data.ID

	reqPut := doJSONRequest(t, router, http.MethodPut, "/api/v1/admin/tracks/"+trackID.String()+"/requirements", testSlug, adminToken, map[string]any{
		"requirements": []map[string]any{
			{
				"doc_code":           "KK",
				"label":              "Kartu Keluarga",
				"is_required":        true,
				"max_size_kb":        2048,
				"allowed_mime_types": []string{"application/pdf"},
			},
		},
	})
	if reqPut.Code != http.StatusOK {
		t.Fatalf("PG put track requirements failed: %d (%s)", reqPut.Code, reqPut.Body.String())
	}
	var reqEnv struct {
		Data []handlers.RequirementView `json:"data"`
	}
	_ = json.Unmarshal(reqPut.Body.Bytes(), &reqEnv)
	reqID := reqEnv.Data[0].ID

	// 4. Guardian OTP register + PGOTPRateLimiter cooldown verification + password reset
	regOTP := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/otp/request", testSlug, "", map[string]string{
		"whatsapp_phone": guardianPhone,
		"purpose":        "register",
	})
	if regOTP.Code != http.StatusAccepted {
		t.Fatalf("PG guardian OTP request failed: %d (%s)", regOTP.Code, regOTP.Body.String())
	}
	var otpEnv struct {
		Data struct {
			NormalizedPhone string `json:"whatsapp_phone"`
			DevOTP          string `json:"dev_otp"`
		} `json:"data"`
	}
	_ = json.Unmarshal(regOTP.Body.Bytes(), &otpEnv)

	strictLimiter := auth.NewPGOTPRateLimiter(pool, 60*time.Second, time.Hour, 5)
	if allowed, retry := strictLimiter.AllowCtx(ctx, otpEnv.Data.NormalizedPhone, time.Now().UTC().Add(5*time.Second)); allowed || retry <= 0 {
		t.Fatalf("expected PGOTPRateLimiter to block immediate repeat OTP on PostgreSQL, got allowed=%v retry=%v", allowed, retry)
	}
	verRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/otp/verify", testSlug, "", map[string]string{
		"whatsapp_phone": guardianPhone,
		"purpose":        "register",
		"code":           otpEnv.Data.DevOTP,
		"full_name":      "Wali PG Test",
		"password":       "InitialPass1!",
	})
	if verRec.Code != http.StatusOK {
		t.Fatalf("PG guardian verify failed: %d (%s)", verRec.Code, verRec.Body.String())
	}
	var gEnv struct {
		Data auth.TokenBundle `json:"data"`
	}
	_ = json.Unmarshal(verRec.Body.Bytes(), &gEnv)
	oldRefresh := gEnv.Data.RefreshToken

	resetOTP := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/otp/request", testSlug, "", map[string]string{
		"whatsapp_phone": guardianPhone,
		"purpose":        "reset_pin",
	})
	_ = json.Unmarshal(resetOTP.Body.Bytes(), &otpEnv)
	resetRes := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/password/reset", testSlug, "", map[string]string{
		"whatsapp_phone": guardianPhone,
		"code":           otpEnv.Data.DevOTP,
		"new_password":   "ResetPGPass2!",
	})
	if resetRes.Code != http.StatusOK {
		t.Fatalf("PG reset password failed: %d (%s)", resetRes.Code, resetRes.Body.String())
	}
	_ = json.Unmarshal(resetRes.Body.Bytes(), &gEnv)
	guardianToken := gEnv.Data.AccessToken

	if staleRefresh := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/refresh", testSlug, "", map[string]string{
		"refresh_token": oldRefresh,
	}); staleRefresh.Code != http.StatusUnauthorized {
		t.Fatalf("expected old PG refresh token revoked (401), got %d", staleRefresh.Code)
	}

	// 5. Idempotency + Midtrans Snap payment + central webhook without X-Tenant-Slug + custom schema + upload + verify
	createReq1 := httptest.NewRequest(http.MethodPost, "/api/v1/applicant/applications", strings.NewReader(fmt.Sprintf(`{
		"unit_id": %q, "period_id": %q, "track_id": %q, "student_full_name": "Zaidan PostgreSQL"
	}`, unitID, periodID, trackID)))
	createReq1.Header.Set("Content-Type", "application/json")
	createReq1.Header.Set("X-Tenant-Slug", testSlug)
	createReq1.Header.Set("Authorization", "Bearer "+guardianToken)
	createReq1.Header.Set("Idempotency-Key", "pg-idem-create-1")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, createReq1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("PG create app 1 failed: %d (%s)", rec1.Code, rec1.Body.String())
	}
	var appEnv1, appEnv2 struct {
		Data admissions.ApplicationDetail `json:"data"`
	}
	_ = json.Unmarshal(rec1.Body.Bytes(), &appEnv1)

	createReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/applicant/applications", strings.NewReader(fmt.Sprintf(`{
		"unit_id": %q, "period_id": %q, "track_id": %q, "student_full_name": "Zaidan PostgreSQL"
	}`, unitID, periodID, trackID)))
	createReq2.Header.Set("Content-Type", "application/json")
	createReq2.Header.Set("X-Tenant-Slug", testSlug)
	createReq2.Header.Set("Authorization", "Bearer "+guardianToken)
	createReq2.Header.Set("Idempotency-Key", "pg-idem-create-1")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, createReq2)
	_ = json.Unmarshal(rec2.Body.Bytes(), &appEnv2)
	if appEnv1.Data.Application.ID != appEnv2.Data.Application.ID {
		t.Fatalf("expected PGIdempotencyCache replay to return same app ID %s, got %s", appEnv1.Data.Application.ID, appEnv2.Data.Application.ID)
	}
	appID := appEnv1.Data.Application.ID

	payRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/payments", testSlug, guardianToken, map[string]string{
		"purpose": "registration_fee",
		"method":  "va",
	})
	if payRec.Code != http.StatusCreated {
		t.Fatalf("PG create payment failed: %d (%s)", payRec.Code, payRec.Body.String())
	}
	var payEnv struct {
		Data admissions.PaymentTransaction `json:"data"`
	}
	_ = json.Unmarshal(payRec.Body.Bytes(), &payEnv)
	orderID := *payEnv.Data.ExternalRef
	sum := sha512.Sum512([]byte(orderID + "200150000.00" + midtransSecret))
	centralHook := doJSONRequest(t, router, http.MethodPost, "/api/v1/webhooks/payments/midtrans", "", "", map[string]any{
		"order_id":           orderID,
		"status_code":        "200",
		"gross_amount":       "150000.00",
		"transaction_status": "settlement",
		"signature_key":      hex.EncodeToString(sum[:]),
	})
	if centralHook.Code != http.StatusOK || !strings.Contains(centralHook.Body.String(), `"draft_form"`) {
		t.Fatalf("PG central Midtrans webhook failed: %d (%s)", centralHook.Code, centralHook.Body.String())
	}

	// Reload latest version after payment unlocked draft_form
	appGet := doJSONRequest(t, router, http.MethodGet, "/api/v1/applicant/applications/"+appID.String(), testSlug, guardianToken, nil)
	_ = json.Unmarshal(appGet.Body.Bytes(), &appEnv1)

	profRec := doJSONRequest(t, router, http.MethodPatch, "/api/v1/applicant/applications/"+appID.String()+"/profile", testSlug, guardianToken, map[string]any{
		"version":       appEnv1.Data.Application.Version,
		"full_name":     "Zaidan PostgreSQL",
		"nik":           "3174011205140099",
		"nisn":          "0145678901",
		"birth_place":   "Jakarta",
		"birth_date":    "2010-06-15",
		"gender":        "L",
		"extra_answers": map[string]any{"hobby": "Robotika"},
	})
	if profRec.Code != http.StatusOK {
		t.Fatalf("PG update profile failed: %d (%s)", profRec.Code, profRec.Body.String())
	}
	_ = json.Unmarshal(profRec.Body.Bytes(), &appEnv1)

	guardRec := doJSONRequest(t, router, http.MethodPut, "/api/v1/applicant/applications/"+appID.String()+"/guardians", testSlug, guardianToken, map[string]any{
		"version": appEnv1.Data.Application.Version,
		"guardians": []map[string]any{
			{"relation": "father", "full_name": "Ayah PG", "phone_whatsapp": guardianPhone, "is_primary_contact": true},
		},
	})
	if guardRec.Code != http.StatusOK {
		t.Fatalf("PG put guardians failed: %d (%s)", guardRec.Code, guardRec.Body.String())
	}
	_ = json.Unmarshal(guardRec.Body.Bytes(), &appEnv1)

	docPre := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/documents/presign-upload", testSlug, guardianToken, map[string]any{
		"requirement_id": reqID,
		"filename":       "kk.pdf",
		"mime_type":      "application/pdf",
		"size_bytes":     1024,
	})
	var docPreEnv struct {
		Data admissions.PresignUploadResult `json:"data"`
	}
	_ = json.Unmarshal(docPre.Body.Bytes(), &docPreEnv)
	uploadSignedURL(t, router, docPreEnv.Data.UploadURL, "application/pdf", []byte("%PDF-1.4\nPG-DOC"))
	docConf := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/documents/confirm", testSlug, guardianToken, map[string]any{
		"requirement_id":    reqID,
		"storage_key":       docPreEnv.Data.StorageKey,
		"original_filename": "kk.pdf",
		"mime_type":         "application/pdf",
		"size_bytes":        15,
	})
	if docConf.Code != http.StatusOK {
		t.Fatalf("PG doc confirm failed: %d (%s)", docConf.Code, docConf.Body.String())
	}
	var docItem struct {
		Data admissions.Document `json:"data"`
	}
	_ = json.Unmarshal(docConf.Body.Bytes(), &docItem)

	appRefresh := doJSONRequest(t, router, http.MethodGet, "/api/v1/applicant/applications/"+appID.String(), testSlug, guardianToken, nil)
	_ = json.Unmarshal(appRefresh.Body.Bytes(), &appEnv1)

	subRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/submit", testSlug, guardianToken, map[string]any{
		"version": appEnv1.Data.Application.Version,
	})
	if subRec.Code != http.StatusOK {
		t.Fatalf("PG submit app failed: %d (%s)", subRec.Code, subRec.Body.String())
	}

	// Claim, approve doc, complete verification
	claimRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/applications/claim-next", testSlug, adminToken, map[string]any{
		"unit_id": unitID,
	})
	if claimRec.Code != http.StatusOK {
		t.Fatalf("PG claim-next failed: %d (%s)", claimRec.Code, claimRec.Body.String())
	}
	_ = json.Unmarshal(claimRec.Body.Bytes(), &appEnv1)

	revRec := doJSONRequest(t, router, http.MethodPatch, "/api/v1/admin/applications/"+appID.String()+"/documents/"+docItem.Data.ID.String()+"/review", testSlug, adminToken, map[string]string{
		"status": "approved",
	})
	if revRec.Code != http.StatusOK {
		t.Fatalf("PG review doc failed: %d (%s)", revRec.Code, revRec.Body.String())
	}

	compVer := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/applications/"+appID.String()+"/complete-verification", testSlug, adminToken, map[string]any{
		"version":      appEnv1.Data.Application.Version,
		"summary_note": "Berkas lengkap via PG E2E",
	})
	if compVer.Code != http.StatusOK {
		t.Fatalf("PG complete verification failed: %d (%s)", compVer.Code, compVer.Body.String())
	}

	// Assessment Schedule + QR verify + Score + Publish + River Export Job
	schedRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/schedules", testSlug, adminToken, map[string]any{
		"application_id": appID,
		"session_name":   "Tes Akademik PG",
		"room_or_link":   "Ruang 202",
		"scheduled_at":   time.Now().UTC().Add(24 * time.Hour),
	})
	if schedRec.Code != http.StatusCreated {
		t.Fatalf("PG schedule create failed: %d (%s)", schedRec.Code, schedRec.Body.String())
	}

	appFinalGet := doJSONRequest(t, router, http.MethodGet, "/api/v1/applicant/applications/"+appID.String(), testSlug, guardianToken, nil)
	_ = json.Unmarshal(appFinalGet.Body.Bytes(), &appEnv1)
	if len(appEnv1.Data.Schedules) != 1 || appEnv1.Data.VerificationToken == "" {
		t.Fatalf("expected 1 schedule and non-empty verification token on PG detail, got %+v", appEnv1.Data)
	}
	qrRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/verify-document?token="+url.QueryEscape(appEnv1.Data.VerificationToken), testSlug, "", nil)
	if qrRec.Code != http.StatusOK || !strings.Contains(qrRec.Body.String(), "Z***** P*********") {
		t.Fatalf("PG public QR verify failed: %d (%s)", qrRec.Code, qrRec.Body.String())
	}

	scoreRec := doJSONRequest(t, router, http.MethodPut, "/api/v1/admin/applications/"+appID.String()+"/scores", testSlug, exLogin.Data.AccessToken, map[string]any{
		"scores": []map[string]any{
			{"component_code": "academic", "raw_score": 90, "weight_pct": 60},
			{"component_code": "interview", "raw_score": 85, "weight_pct": 40},
		},
	})
	if scoreRec.Code != http.StatusOK {
		t.Fatalf("PG upsert scores failed: %d (%s)", scoreRec.Code, scoreRec.Body.String())
	}

	pubRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/periods/"+periodID.String()+"/publish-results", testSlug, adminToken, nil)
	if pubRec.Code != http.StatusOK {
		t.Fatalf("PG publish results failed: %d (%s)", pubRec.Code, pubRec.Body.String())
	}

	expRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/exports", testSlug, adminToken, map[string]any{
		"unit_id":   unitID,
		"period_id": periodID,
		"format":    "dapodik_xlsx",
	})
	if expRec.Code != http.StatusAccepted {
		t.Fatalf("PG create export failed: %d (%s)", expRec.Code, expRec.Body.String())
	}
	var expEnv struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(expRec.Body.Bytes(), &expEnv)

	var dlReady bool
	var lastCode int
	var lastBody string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pollRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/exports/"+expEnv.Data.ID.String(), testSlug, adminToken, nil)
		lastCode = pollRec.Code
		lastBody = pollRec.Body.String()
		if pollRec.Code == http.StatusOK && strings.Contains(lastBody, `"completed"`) {
			dlReady = true
			break
		}
		if pollRec.Code == http.StatusOK && strings.Contains(lastBody, `"failed"`) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !dlReady {
		var rState, rQueue string
		var rAttempt int
		_ = pool.QueryRow(ctx, `SELECT state::text, queue, attempt FROM river_job WHERE kind = 'generate_national_export' ORDER BY id DESC LIMIT 1`).Scan(&rState, &rQueue, &rAttempt)
		t.Fatalf("River export job did not finish: http=%d body=%s river_state=%s queue=%s attempt=%d", lastCode, lastBody, rState, rQueue, rAttempt)
	}
	dlRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/exports/"+expEnv.Data.ID.String()+"/download", testSlug, adminToken, nil)
	if dlRec.Code != http.StatusFound || !strings.Contains(dlRec.Header().Get("Location"), "sig=") {
		t.Fatalf("PG export download failed: %d (%s)", dlRec.Code, dlRec.Body.String())
	}
	dlObjReq := httptest.NewRequest(http.MethodGet, dlRec.Header().Get("Location"), nil)
	dlObjRec := httptest.NewRecorder()
	router.ServeHTTP(dlObjRec, dlObjReq)
	if dlObjRec.Code != http.StatusOK || !strings.HasPrefix(dlObjRec.Body.String(), "PK") {
		t.Fatalf("expected valid XLSX ZIP payload from LocalStorage download, got %d len=%d", dlObjRec.Code, dlObjRec.Body.Len())
	}
	if waCalls.Load() == 0 {
		t.Fatalf("expected Fonnte WhatsApp worker to deliver queued notification jobs on PostgreSQL")
	}
}
