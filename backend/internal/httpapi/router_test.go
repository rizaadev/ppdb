package httpapi_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/config"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi"
	"github.com/pbdb/platform/internal/httpapi/handlers"
	"github.com/pbdb/platform/internal/integrations"
	"github.com/pbdb/platform/internal/modules/admissions"
	"github.com/pbdb/platform/internal/modules/auth"
	"github.com/pbdb/platform/internal/modules/public"
	"github.com/pbdb/platform/internal/storage"
	"github.com/pbdb/platform/web"
)

type stubRouterDNS struct {
	txt   map[string][]string
	cname map[string]string
}

func (s *stubRouterDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	return s.txt[strings.ToLower(strings.TrimSuffix(name, "."))], nil
}

func (s *stubRouterDNS) LookupCNAME(_ context.Context, host string) (string, error) {
	return s.cname[strings.ToLower(strings.TrimSuffix(host, "."))], nil
}

func newTestRouterWithDeps(t *testing.T, dns handlers.DNSResolver, payGw integrations.PaymentGateway, midtransKey string) (http.Handler, *auth.JWTManager) {
	t.Helper()
	distFS, err := web.DistFS()
	if err != nil {
		t.Fatalf("web.DistFS: %v", err)
	}
	demo := public.NewDemoStore()
	authStore := auth.NewDemoStore()
	jwtMgr := auth.NewJWTManager("test-router-jwt-secret-key-32-bytes", 15*time.Minute)
	limiter := auth.NewOTPRateLimiter(time.Minute, 15*time.Minute, 5)
	authSvc := auth.NewService(authStore, jwtMgr, limiter, "test-router-otp-secret", 7*24*time.Hour, true, nil)

	localStore, err := storage.NewLocalStorage(t.TempDir(), "http://localhost:8080", "router-test-doc-secret")
	if err != nil {
		t.Fatalf("storage.NewLocalStorage: %v", err)
	}
	admDemo := admissions.NewDemoStore()
	admDemo.SetCatalogProvider(demo)
	admSvc := admissions.NewService(admDemo, localStore, admissions.NewIdempotencyCache(time.Hour))
	admSvc.SetDocumentSecret("router-test-doc-secret")
	admSvc.SetPaymentConfigProvider(demo)
	if payGw != nil {
		admSvc.SetPaymentGateway(payGw)
	}

	router := httpapi.NewRouter(httpapi.Dependencies{
		Config: config.Config{
			AppEnv:               "development",
			BaseDomain:           "spmb-platform.id",
			PublicBaseURL:        "http://localhost:8080",
			DevFallbackSlug:      "demo-spmb",
			DocumentSecret:       "router-test-doc-secret",
			LogRequests:          false,
			PaymentWebhookSecret: "router-test-webhook-secret",
			MidtransServerKey:    midtransKey,
		},
		TenantLookup:      demo,
		PublicStore:       demo,
		PlatformStore:     demo,
		SetupStore:        demo,
		ObjectStorage:     localStore,
		DNSResolver:       dns,
		AuthService:       authSvc,
		AdmissionsService: admSvc,
		JWTManager:        jwtMgr,
		FrontendFS:        distFS,
	})
	return router, jwtMgr
}

func newTestRouterWithJWT(t *testing.T) (http.Handler, *auth.JWTManager) {
	t.Helper()
	return newTestRouterWithDeps(t, &stubRouterDNS{txt: map[string][]string{}, cname: map[string]string{}}, nil, "")
}

func uploadSignedURL(t *testing.T, router http.Handler, uploadURL, contentType string, data []byte) {
	t.Helper()
	u, err := url.Parse(uploadURL)
	if err != nil {
		t.Fatalf("parse upload URL %q: %v", uploadURL, err)
	}
	req := httptest.NewRequest(http.MethodPut, u.RequestURI(), bytes.NewReader(data))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT presigned upload URL failed: %d (%s)", rec.Code, rec.Body.String())
	}
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	router, _ := newTestRouterWithJWT(t)
	return router
}

func doJSONRequest(t *testing.T, router http.Handler, method, path, tenantSlug, bearerToken string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *bytes.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		bodyReader = bytes.NewReader(raw)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, bodyReader)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tenantSlug != "" {
		req.Header.Set("X-Tenant-Slug", tenantSlug)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRouterEndpoints(t *testing.T) {
	router := newTestRouter(t)

	t.Run("healthz returns ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"demo_in_memory"`) {
			t.Fatalf("unexpected healthz response: %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("caddy tls verification accepts known domain and rejects unknown", func(t *testing.T) {
		okReq := httptest.NewRequest(http.MethodGet, "/internal/tls/verify?domain=pmbm.maalhamid.sch.id", nil)
		okRec := httptest.NewRecorder()
		router.ServeHTTP(okRec, okReq)
		if okRec.Code != http.StatusOK {
			t.Fatalf("expected 200 for registered domain, got %d", okRec.Code)
		}

		badReq := httptest.NewRequest(http.MethodGet, "/internal/tls/verify?domain=unknown-attacker.com", nil)
		badRec := httptest.NewRecorder()
		router.ServeHTTP(badRec, badReq)
		if badRec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for unknown domain, got %d", badRec.Code)
		}
	})

	t.Run("public portal switches tenant via X-Tenant-Slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/public/portal", nil)
		req.Header.Set("X-Tenant-Slug", "demo-pmbm")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if !strings.Contains(rec.Body.String(), `"terminology_preset":"pmbm"`) {
			t.Fatalf("expected pmbm preset in payload, got %s", rec.Body.String())
		}
	})

	t.Run("check status validates empty input and masks valid result", func(t *testing.T) {
		emptyReq := httptest.NewRequest(http.MethodPost, "/api/v1/public/check-status", bytes.NewBufferString(`{"registration_no":""}`))
		emptyRec := httptest.NewRecorder()
		router.ServeHTTP(emptyRec, emptyReq)
		if emptyRec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", emptyRec.Code)
		}

		validReq := httptest.NewRequest(http.MethodPost, "/api/v1/public/check-status", bytes.NewBufferString(`{"registration_no":"SPMB-2601-0001"}`))
		validRec := httptest.NewRecorder()
		router.ServeHTTP(validRec, validReq)
		if validRec.Code != http.StatusOK || !strings.Contains(validRec.Body.String(), `"A*** F***"`) {
			t.Fatalf("unexpected check-status output: %d %s", validRec.Code, validRec.Body.String())
		}
	})

	t.Run("spa fallback injects tenant meta tags dynamically", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = "daftar.alikhlas.or.id"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<title>Yayasan Pendidikan &amp; Pesantren Al-Ikhlas</title>`) {
			t.Fatalf("expected dynamic tenant title in SPA HTML, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("auth otp register -> /me -> guardian blocked from admin route", func(t *testing.T) {
		otpRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/otp/request", "demo-spmb", "", map[string]string{
			"whatsapp_phone": "081355667788",
			"purpose":        "register",
		})
		if otpRec.Code != http.StatusAccepted {
			t.Fatalf("otp/request: got %d (%s)", otpRec.Code, otpRec.Body.String())
		}

		var otpEnv struct {
			Data auth.RequestOTPResult `json:"data"`
		}
		if err := json.Unmarshal(otpRec.Body.Bytes(), &otpEnv); err != nil {
			t.Fatalf("decode otp response: %v", err)
		}
		if otpEnv.Data.DevOTP == "" {
			t.Fatal("expected dev_otp in development mode")
		}

		verifyRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/otp/verify", "demo-spmb", "", map[string]string{
			"whatsapp_phone": "081355667788",
			"purpose":        "register",
			"code":           otpEnv.Data.DevOTP,
			"full_name":      "Ibu Dewi",
			"password":       "Secret123!",
		})
		if verifyRec.Code != http.StatusOK {
			t.Fatalf("otp/verify: got %d (%s)", verifyRec.Code, verifyRec.Body.String())
		}

		var verifyEnv struct {
			Data auth.TokenBundle `json:"data"`
		}
		if err := json.Unmarshal(verifyRec.Body.Bytes(), &verifyEnv); err != nil {
			t.Fatalf("decode verify response: %v", err)
		}

		meRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/me", "demo-spmb", verifyEnv.Data.AccessToken, nil)
		if meRec.Code != http.StatusOK || !strings.Contains(meRec.Body.String(), `"Ibu Dewi"`) {
			t.Fatalf("GET /me: got %d (%s)", meRec.Code, meRec.Body.String())
		}

		adminRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/tenant/settings", "demo-spmb", verifyEnv.Data.AccessToken, nil)
		if adminRec.Code != http.StatusForbidden {
			t.Fatalf("expected guardian on admin route to get 403, got %d (%s)", adminRec.Code, adminRec.Body.String())
		}
	})

	t.Run("rbac enforces role and tenant binding on admin tenant settings", func(t *testing.T) {
		unauthRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/tenant/settings", "demo-pmbm", "", nil)
		if unauthRec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 without token, got %d", unauthRec.Code)
		}

		verifierLogin := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", "demo-spmb", "", map[string]string{
			"identifier": "verifikator@smaharapan.sch.id",
			"password":   "DemoPass123!",
		})
		if verifierLogin.Code != http.StatusOK {
			t.Fatalf("verifier login failed: %d (%s)", verifierLogin.Code, verifierLogin.Body.String())
		}
		var verifierEnv struct {
			Data auth.TokenBundle `json:"data"`
		}
		_ = json.Unmarshal(verifierLogin.Body.Bytes(), &verifierEnv)

		verifierAdminRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/tenant/settings", "demo-spmb", verifierEnv.Data.AccessToken, nil)
		if verifierAdminRec.Code != http.StatusForbidden {
			t.Fatalf("expected verifier on tenant/settings to get 403, got %d (%s)", verifierAdminRec.Code, verifierAdminRec.Body.String())
		}

		adminLogin := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", "demo-pmbm", "", map[string]string{
			"identifier": "admin@alhamid.sch.id",
			"password":   "DemoPass123!",
		})
		if adminLogin.Code != http.StatusOK {
			t.Fatalf("admin login failed: %d (%s)", adminLogin.Code, adminLogin.Body.String())
		}
		var adminEnv struct {
			Data auth.TokenBundle `json:"data"`
		}
		_ = json.Unmarshal(adminLogin.Body.Bytes(), &adminEnv)

		okRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/tenant/settings", "demo-pmbm", adminEnv.Data.AccessToken, nil)
		if okRec.Code != http.StatusOK || !strings.Contains(okRec.Body.String(), `"demo-pmbm"`) {
			t.Fatalf("expected tenant_admin to read demo-pmbm settings, got %d (%s)", okRec.Code, okRec.Body.String())
		}

		crossTenantRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/tenant/settings", "demo-spmb", adminEnv.Data.AccessToken, nil)
		if crossTenantRec.Code != http.StatusUnauthorized {
			t.Fatalf("expected cross-tenant token to get 401, got %d (%s)", crossTenantRec.Code, crossTenantRec.Body.String())
		}
	})

	t.Run("refresh rotates token and logout revokes active session", func(t *testing.T) {
		loginRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", "demo-spmb", "", map[string]string{
			"identifier": "+6281234567890",
			"password":   "DemoPass123!",
		})
		if loginRec.Code != http.StatusOK {
			t.Fatalf("guardian login failed: %d (%s)", loginRec.Code, loginRec.Body.String())
		}
		var loginEnv struct {
			Data auth.TokenBundle `json:"data"`
		}
		_ = json.Unmarshal(loginRec.Body.Bytes(), &loginEnv)

		refreshRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/refresh", "demo-spmb", "", map[string]string{
			"refresh_token": loginEnv.Data.RefreshToken,
		})
		if refreshRec.Code != http.StatusOK {
			t.Fatalf("refresh failed: %d (%s)", refreshRec.Code, refreshRec.Body.String())
		}
		var rotatedEnv struct {
			Data auth.TokenBundle `json:"data"`
		}
		_ = json.Unmarshal(refreshRec.Body.Bytes(), &rotatedEnv)

		replayRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/refresh", "demo-spmb", "", map[string]string{
			"refresh_token": loginEnv.Data.RefreshToken,
		})
		if replayRec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on spent refresh token replay, got %d (%s)", replayRec.Code, replayRec.Body.String())
		}

		logoutRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/logout", "demo-spmb", "", map[string]string{
			"refresh_token": rotatedEnv.Data.RefreshToken,
		})
		if logoutRec.Code != http.StatusOK {
			t.Fatalf("logout failed: %d (%s)", logoutRec.Code, logoutRec.Body.String())
		}

		afterLogoutRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/refresh", "demo-spmb", "", map[string]string{
			"refresh_token": rotatedEnv.Data.RefreshToken,
		})
		if afterLogoutRec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 after logout revocation, got %d (%s)", afterLogoutRec.Code, afterLogoutRec.Body.String())
		}
	})
}

func TestApplicantAndVerifierEndToEndHTTP(t *testing.T) {
	router, jwtMgr := newTestRouterWithJWT(t)

	// 1. Discover units, period, track & required docs from public portal
	portalRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/portal", "demo-spmb", "", nil)
	if portalRec.Code != http.StatusOK {
		t.Fatalf("portal: got %d (%s)", portalRec.Code, portalRec.Body.String())
	}
	var portalEnv struct {
		Data handlers.PublicPortalPayload `json:"data"`
	}
	if err := json.Unmarshal(portalRec.Body.Bytes(), &portalEnv); err != nil {
		t.Fatalf("decode portal: %v", err)
	}
	unit := portalEnv.Data.Units[0]
	period := portalEnv.Data.Periods[0]
	domisili := period.Tracks[0]
	if len(domisili.Requirements) < 2 {
		t.Fatalf("expected seeded track requirements in portal, got %d", len(domisili.Requirements))
	}

	// 2. Guardian logs in via password
	guardianLogin := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", "demo-spmb", "", map[string]string{
		"identifier": "081234567890",
		"password":   "DemoPass123!",
	})
	if guardianLogin.Code != http.StatusOK {
		t.Fatalf("guardian login: got %d (%s)", guardianLogin.Code, guardianLogin.Body.String())
	}
	var gEnv struct {
		Data auth.TokenBundle `json:"data"`
	}
	_ = json.Unmarshal(guardianLogin.Body.Bytes(), &gEnv)
	guardianToken := gEnv.Data.AccessToken

	// 3. Create draft application with Idempotency-Key
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/applicant/applications", strings.NewReader(`{
		"unit_id": "`+unit.ID.String()+`",
		"period_id": "`+period.ID.String()+`",
		"track_id": "`+domisili.ID.String()+`",
		"student_full_name": "Nabila Zahra"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Tenant-Slug", "demo-spmb")
	createReq.Header.Set("Authorization", "Bearer "+guardianToken)
	createReq.Header.Set("Idempotency-Key", "http-create-1")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create app: got %d (%s)", createRec.Code, createRec.Body.String())
	}
	var appEnv struct {
		Data admissions.ApplicationDetail `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &appEnv); err != nil {
		t.Fatalf("decode created app: %v", err)
	}
	appID := appEnv.Data.Application.ID

	// 4. Update profile with coordinates & NIK, save guardians
	patchRec := doJSONRequest(t, router, http.MethodPatch, "/api/v1/applicant/applications/"+appID.String()+"/profile", "demo-spmb", guardianToken, map[string]any{
		"version":    appEnv.Data.Application.Version,
		"full_name":  "Nabila Zahra",
		"nik":        "3175096105100001",
		"nisn":       "0101234567",
		"gender":     "P",
		"birth_date": "2011-05-21",
		"latitude":   -6.2450,
		"longitude":  106.8850,
	})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch profile: got %d (%s)", patchRec.Code, patchRec.Body.String())
	}
	var patchedEnv struct {
		Data admissions.ApplicationDetail `json:"data"`
	}
	_ = json.Unmarshal(patchRec.Body.Bytes(), &patchedEnv)
	if patchedEnv.Data.Application.DistanceMeters == nil || *patchedEnv.Data.Application.DistanceMeters <= 0 {
		t.Fatalf("expected Haversine distance_meters > 0, got %v", patchedEnv.Data.Application.DistanceMeters)
	}

	guardiansRec := doJSONRequest(t, router, http.MethodPut, "/api/v1/applicant/applications/"+appID.String()+"/guardians", "demo-spmb", guardianToken, map[string]any{
		"guardians": []map[string]any{
			{"relation": "father", "full_name": "H. Ahmad Fauzan", "phone_whatsapp": "+6281234567890", "is_primary_contact": true},
		},
	})
	if guardiansRec.Code != http.StatusOK {
		t.Fatalf("put guardians: got %d (%s)", guardiansRec.Code, guardiansRec.Body.String())
	}

	// 5. Presign upload validation + upload KK & Akta
	badMimeRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/documents/presign-upload", "demo-spmb", guardianToken, map[string]any{
		"requirement_id": domisili.Requirements[0].ID,
		"filename":       "script.html",
		"mime_type":      "text/html",
		"size_bytes":     1024,
	})
	if badMimeRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on invalid MIME, got %d (%s)", badMimeRec.Code, badMimeRec.Body.String())
	}

	var confirmedDocs []admissions.Document
	for _, reqItem := range domisili.Requirements {
		if !reqItem.IsRequired {
			continue
		}
		preRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/documents/presign-upload", "demo-spmb", guardianToken, map[string]any{
			"requirement_id": reqItem.ID,
			"filename":       reqItem.DocCode + ".pdf",
			"mime_type":      "application/pdf",
			"size_bytes":     65536,
		})
		if preRec.Code != http.StatusOK {
			t.Fatalf("presign-upload %s: got %d (%s)", reqItem.DocCode, preRec.Code, preRec.Body.String())
		}
		var preEnv struct {
			Data admissions.PresignUploadResult `json:"data"`
		}
		_ = json.Unmarshal(preRec.Body.Bytes(), &preEnv)
		if !strings.Contains(preEnv.Data.UploadURL, "sig=") {
			t.Fatalf("expected signed upload_url, got %s", preEnv.Data.UploadURL)
		}
		uploadSignedURL(t, router, preEnv.Data.UploadURL, "application/pdf", bytes.Repeat([]byte("A"), 65536))

		confRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/documents/confirm", "demo-spmb", guardianToken, map[string]any{
			"requirement_id":    reqItem.ID,
			"storage_key":       preEnv.Data.StorageKey,
			"original_filename": reqItem.DocCode + ".pdf",
			"mime_type":         "application/pdf",
			"size_bytes":        65536,
		})
		if confRec.Code != http.StatusOK {
			t.Fatalf("confirm %s: got %d (%s)", reqItem.DocCode, confRec.Code, confRec.Body.String())
		}
		var docEnv struct {
			Data admissions.Document `json:"data"`
		}
		_ = json.Unmarshal(confRec.Body.Bytes(), &docEnv)
		confirmedDocs = append(confirmedDocs, docEnv.Data)
	}

	// Reload to capture latest version after PUT guardians before submit
	detailRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/applicant/applications/"+appID.String(), "demo-spmb", guardianToken, nil)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("GET detail: got %d", detailRec.Code)
	}
	var latestEnv struct {
		Data admissions.ApplicationDetail `json:"data"`
	}
	_ = json.Unmarshal(detailRec.Body.Bytes(), &latestEnv)

	// 6. Submit application
	submitRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/submit", "demo-spmb", guardianToken, map[string]any{
		"version": latestEnv.Data.Application.Version,
	})
	if submitRec.Code != http.StatusOK || !strings.Contains(submitRec.Body.String(), `"submitted"`) {
		t.Fatalf("submit: got %d (%s)", submitRec.Code, submitRec.Body.String())
	}

	// 7. RBAC checks: guardian & wrong-unit verifier blocked with 403
	if gStaffRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/applications", "demo-spmb", guardianToken, nil); gStaffRec.Code != http.StatusForbidden {
		t.Fatalf("expected guardian blocked from /admin/applications with 403, got %d", gStaffRec.Code)
	}

	otherUnitID := uuid.New()
	unitScopedOtherToken, err := jwtMgr.Issue(uuid.New(), portalEnv.Data.Tenant.ID, false, []domain.RoleScope{
		{TenantID: portalEnv.Data.Tenant.ID, UnitID: &otherUnitID, Role: domain.RoleVerifier},
	})
	if err != nil {
		t.Fatalf("issue wrong-unit token: %v", err)
	}
	if wrongUnitList := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/applications", "demo-spmb", unitScopedOtherToken, nil); wrongUnitList.Code == http.StatusOK && strings.Contains(wrongUnitList.Body.String(), appID.String()) {
		t.Fatalf("wrong-unit verifier must not see target unit's application in list: %s", wrongUnitList.Body.String())
	}
	if wrongUnitClaim := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/applications/claim-next", "demo-spmb", unitScopedOtherToken, map[string]any{
		"unit_id": unit.ID,
	}); wrongUnitClaim.Code != http.StatusForbidden {
		t.Fatalf("expected wrong-unit claim-next to get 403, got %d (%s)", wrongUnitClaim.Code, wrongUnitClaim.Body.String())
	}
	if wrongUnitDetail := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/applications/"+appID.String(), "demo-spmb", unitScopedOtherToken, nil); wrongUnitDetail.Code != http.StatusForbidden {
		t.Fatalf("expected wrong-unit detail to get 403, got %d", wrongUnitDetail.Code)
	}

	// 8. Authorized SPMB verifier claims next, previews docs, approves all, completes verification
	verifierLogin := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", "demo-spmb", "", map[string]string{
		"identifier": "verifikator@smaharapan.sch.id",
		"password":   "DemoPass123!",
	})
	var vEnv struct {
		Data auth.TokenBundle `json:"data"`
	}
	_ = json.Unmarshal(verifierLogin.Body.Bytes(), &vEnv)
	verifierToken := vEnv.Data.AccessToken

	claimRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/applications/claim-next", "demo-spmb", verifierToken, map[string]any{
		"unit_id": unit.ID,
	})
	if claimRec.Code != http.StatusOK || !strings.Contains(claimRec.Body.String(), `"under_verification"`) {
		t.Fatalf("claim-next: got %d (%s)", claimRec.Code, claimRec.Body.String())
	}
	var claimedEnv struct {
		Data admissions.ApplicationDetail `json:"data"`
	}
	_ = json.Unmarshal(claimRec.Body.Bytes(), &claimedEnv)

	staffDLRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/applications/"+appID.String()+"/documents/"+confirmedDocs[0].ID.String()+"/presign-download", "demo-spmb", verifierToken, nil)
	if staffDLRec.Code != http.StatusOK || !strings.Contains(staffDLRec.Body.String(), "sig=") {
		t.Fatalf("staff presign-download: got %d (%s)", staffDLRec.Code, staffDLRec.Body.String())
	}

	for _, doc := range confirmedDocs {
		revRec := doJSONRequest(t, router, http.MethodPatch, "/api/v1/admin/applications/"+appID.String()+"/documents/"+doc.ID.String()+"/review", "demo-spmb", verifierToken, map[string]string{
			"status":      "approved",
			"review_note": "Berkas asli jelas dan terbaca",
		})
		if revRec.Code != http.StatusOK {
			t.Fatalf("review doc %s: got %d (%s)", doc.DocCode, revRec.Code, revRec.Body.String())
		}
	}

	completeRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/applications/"+appID.String()+"/complete-verification", "demo-spmb", verifierToken, map[string]any{
		"version":      claimedEnv.Data.Application.Version,
		"summary_note": "Seluruh persyaratan jalur domisili sah",
	})
	if completeRec.Code != http.StatusOK || !strings.Contains(completeRec.Body.String(), `"verified"`) {
		t.Fatalf("complete-verification: got %d (%s)", completeRec.Code, completeRec.Body.String())
	}
}

func doRawSignedWebhook(t *testing.T, router http.Handler, path, tenantSlug, secret, signatureOverride string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	sig := signatureOverride
	if sig == "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		sig = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-Slug", tenantSlug)
	req.Header.Set("X-Webhook-Signature", sig)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRouterPaymentsReEnrollmentAndSelectionWorkflow(t *testing.T) {
	router := newTestRouter(t)

	loginToken := func(tenantSlug, identifier string) string {
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", tenantSlug, "", map[string]string{
			"identifier": identifier,
			"password":   "DemoPass123!",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("login %s: got %d (%s)", identifier, rec.Code, rec.Body.String())
		}
		var env struct {
			Data auth.TokenBundle `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return env.Data.AccessToken
	}

	guardianToken := loginToken("demo-pmbm", "+6281234567890")
	verifierToken := loginToken("demo-spmb", "verifikator@smaharapan.sch.id")
	treasurerToken := loginToken("demo-pmbm", "bendahara@alhamid.sch.id")
	examinerToken := loginToken("demo-pmbm", "penguji@smaharapan.sch.id")
	principalToken := loginToken("demo-pmbm", "kepsek@smaharapan.sch.id")

	portalRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/portal", "demo-pmbm", "", nil)
	if portalRec.Code != http.StatusOK {
		t.Fatalf("portal: got %d", portalRec.Code)
	}
	var portalEnv struct {
		Data handlers.PublicPortalPayload `json:"data"`
	}
	_ = json.Unmarshal(portalRec.Body.Bytes(), &portalEnv)
	unit := portalEnv.Data.Units[0]
	period := portalEnv.Data.Periods[0]
	regulerTrack := period.Tracks[0] // RegistrationFeeIDR=150000, ReEnrollmentFeeIDR=1250000

	// 1. Create paid PMBM application -> awaiting_payment
	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications", "demo-pmbm", guardianToken, map[string]any{
		"unit_id":           unit.ID,
		"period_id":         period.ID,
		"track_id":          regulerTrack.ID,
		"student_full_name": "Zainab Al-Hamid",
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create app: got %d (%s)", createRec.Code, createRec.Body.String())
	}
	var appEnv struct {
		Data admissions.ApplicationDetail `json:"data"`
	}
	_ = json.Unmarshal(createRec.Body.Bytes(), &appEnv)
	appID := appEnv.Data.Application.ID

	// 2. Create manual transfer invoice -> presign proof -> attach proof
	payCreateRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/payments", "demo-pmbm", guardianToken, map[string]string{
		"purpose": "registration_fee",
		"method":  "manual_transfer",
	})
	if payCreateRec.Code != http.StatusCreated {
		t.Fatalf("create payment: got %d (%s)", payCreateRec.Code, payCreateRec.Body.String())
	}
	var payEnv struct {
		Data admissions.PaymentTransaction `json:"data"`
	}
	_ = json.Unmarshal(payCreateRec.Body.Bytes(), &payEnv)
	paymentID := payEnv.Data.ID

	presignProof := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/payments/"+paymentID.String()+"/presign-proof", "demo-pmbm", guardianToken, map[string]any{
		"filename":   "bukti-transfer.jpg",
		"mime_type":  "image/jpeg",
		"size_bytes": 150000,
	})
	if presignProof.Code != http.StatusOK {
		t.Fatalf("presign proof: got %d (%s)", presignProof.Code, presignProof.Body.String())
	}
	var proofEnv struct {
		Data admissions.PresignUploadResult `json:"data"`
	}
	_ = json.Unmarshal(presignProof.Body.Bytes(), &proofEnv)
	uploadSignedURL(t, router, proofEnv.Data.UploadURL, "image/jpeg", bytes.Repeat([]byte("J"), 150000))

	attachRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/payments/"+paymentID.String()+"/manual-proof", "demo-pmbm", guardianToken, map[string]string{
		"proof_storage_key": proofEnv.Data.StorageKey,
	})
	if attachRec.Code != http.StatusOK {
		t.Fatalf("attach proof: got %d (%s)", attachRec.Code, attachRec.Body.String())
	}

	// Cross-tenant SPMB verifier token returns 401; in-tenant Examiner role blocked from payment verify with 403; Treasurer allowed (200) -> draft_form
	if crossTenantVerify := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+paymentID.String()+"/verify-manual", "demo-pmbm", verifierToken, map[string]string{"decision": "paid"}); crossTenantVerify.Code != http.StatusUnauthorized {
		t.Fatalf("expected cross-tenant verifier blocked with 401, got %d", crossTenantVerify.Code)
	}
	if wrongRoleVerify := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+paymentID.String()+"/verify-manual", "demo-pmbm", examinerToken, map[string]string{"decision": "paid"}); wrongRoleVerify.Code != http.StatusForbidden {
		t.Fatalf("expected examiner blocked from payment verify with 403, got %d", wrongRoleVerify.Code)
	}
	listPayRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/payments?status=pending", "demo-pmbm", treasurerToken, nil)
	if listPayRec.Code != http.StatusOK || !strings.Contains(listPayRec.Body.String(), paymentID.String()) {
		t.Fatalf("admin list payments: got %d (%s)", listPayRec.Code, listPayRec.Body.String())
	}
	previewProofRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/payments/"+paymentID.String()+"/presign-proof", "demo-pmbm", treasurerToken, nil)
	if previewProofRec.Code != http.StatusOK || !strings.Contains(previewProofRec.Body.String(), "sig=") {
		t.Fatalf("admin presign proof: got %d (%s)", previewProofRec.Code, previewProofRec.Body.String())
	}
	verifyManualRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/payments/"+paymentID.String()+"/verify-manual", "demo-pmbm", treasurerToken, map[string]string{
		"decision": "paid",
		"note":     "Valid sesuai mutasi rekening",
	})
	if verifyManualRec.Code != http.StatusOK || !strings.Contains(verifyManualRec.Body.String(), `"draft_form"`) {
		t.Fatalf("verify-manual: got %d (%s)", verifyManualRec.Code, verifyManualRec.Body.String())
	}

	// 3. Complete profile, upload KK/Akta, submit, admin verify -> verified
	var unlockedEnv struct {
		Data admissions.PaymentVerificationResult `json:"data"`
	}
	_ = json.Unmarshal(verifyManualRec.Body.Bytes(), &unlockedEnv)
	curVer := unlockedEnv.Data.Application.Application.Version
	for _, req := range unlockedEnv.Data.Application.Requirements {
		if !req.IsRequired {
			continue
		}
		pre := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/documents/presign-upload", "demo-pmbm", guardianToken, map[string]any{
			"requirement_id": req.ID,
			"filename":       req.DocCode + ".pdf",
			"mime_type":      "application/pdf",
			"size_bytes":     20480,
		})
		var uEnv struct {
			Data admissions.PresignUploadResult `json:"data"`
		}
		_ = json.Unmarshal(pre.Body.Bytes(), &uEnv)
		uploadSignedURL(t, router, uEnv.Data.UploadURL, "application/pdf", bytes.Repeat([]byte("P"), 20480))
		doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/documents/confirm", "demo-pmbm", guardianToken, map[string]any{
			"requirement_id":    req.ID,
			"storage_key":       uEnv.Data.StorageKey,
			"original_filename": req.DocCode + ".pdf",
			"mime_type":         "application/pdf",
			"size_bytes":        20480,
		})
	}
	refetched := doJSONRequest(t, router, http.MethodGet, "/api/v1/applicant/applications/"+appID.String(), "demo-pmbm", guardianToken, nil)
	_ = json.Unmarshal(refetched.Body.Bytes(), &appEnv)
	curVer = appEnv.Data.Application.Version

	subRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/submit", "demo-pmbm", guardianToken, map[string]any{"version": curVer})
	if subRec.Code != http.StatusOK {
		t.Fatalf("submit pmbm: got %d (%s)", subRec.Code, subRec.Body.String())
	}
	_ = json.Unmarshal(subRec.Body.Bytes(), &appEnv)
	adminToken := loginToken("demo-pmbm", "admin@alhamid.sch.id")
	for _, doc := range appEnv.Data.Documents {
		revRec := doJSONRequest(t, router, http.MethodPatch, "/api/v1/admin/applications/"+appID.String()+"/documents/"+doc.ID.String()+"/review", "demo-pmbm", adminToken, map[string]string{
			"status": "approved",
		})
		if revRec.Code != http.StatusOK {
			t.Fatalf("admin review doc: got %d", revRec.Code)
		}
	}
	refetched2 := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/applications/"+appID.String(), "demo-pmbm", adminToken, nil)
	_ = json.Unmarshal(refetched2.Body.Bytes(), &appEnv)
	compRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/applications/"+appID.String()+"/complete-verification", "demo-pmbm", adminToken, map[string]any{
		"version": appEnv.Data.Application.Version,
	})
	if compRec.Code != http.StatusOK {
		t.Fatalf("admin complete verification: got %d (%s)", compRec.Code, compRec.Body.String())
	}

	// 4. Examiner scores, Principal simulates & publishes, Applicant re-enrolls + pays fee via webhook -> enrolled!
	if forbiddenScore := doJSONRequest(t, router, http.MethodPut, "/api/v1/admin/applications/"+appID.String()+"/scores", "demo-pmbm", treasurerToken, map[string]any{
		"scores": []map[string]any{{"component_code": "TPA", "raw_score": 85, "weight_pct": 100}},
	}); forbiddenScore.Code != http.StatusForbidden {
		t.Fatalf("expected treasurer blocked from scoring with 403, got %d", forbiddenScore.Code)
	}
	scoreRec := doJSONRequest(t, router, http.MethodPut, "/api/v1/admin/applications/"+appID.String()+"/scores", "demo-pmbm", examinerToken, map[string]any{
		"scores": []map[string]any{
			{"component_code": "TPA", "raw_score": 85, "weight_pct": 60},
			{"component_code": "QURAN", "raw_score": 90, "weight_pct": 40},
		},
	})
	if scoreRec.Code != http.StatusOK || !strings.Contains(scoreRec.Body.String(), `"final_score":87`) {
		t.Fatalf("upsert scores: got %d (%s)", scoreRec.Code, scoreRec.Body.String())
	}

	simRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/tracks/"+regulerTrack.ID.String()+"/simulate-ranking", "demo-pmbm", principalToken, nil)
	if simRec.Code != http.StatusOK || !strings.Contains(simRec.Body.String(), appID.String()) {
		t.Fatalf("simulate-ranking: got %d (%s)", simRec.Code, simRec.Body.String())
	}

	if forbiddenPub := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/periods/"+period.ID.String()+"/publish-results", "demo-pmbm", examinerToken, nil); forbiddenPub.Code != http.StatusForbidden {
		t.Fatalf("expected examiner blocked from publish with 403, got %d", forbiddenPub.Code)
	}
	pubRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/periods/"+period.ID.String()+"/publish-results", "demo-pmbm", principalToken, nil)
	if pubRec.Code != http.StatusOK || !strings.Contains(pubRec.Body.String(), `"accepted_count":1`) {
		t.Fatalf("publish-results: got %d (%s)", pubRec.Code, pubRec.Body.String())
	}

	// Applicant checks detail -> accepted -> ReEnroll -> re_enrollment -> creates VA fee -> valid/invalid HMAC webhook -> enrolled
	accGet := doJSONRequest(t, router, http.MethodGet, "/api/v1/applicant/applications/"+appID.String(), "demo-pmbm", guardianToken, nil)
	_ = json.Unmarshal(accGet.Body.Bytes(), &appEnv)
	if appEnv.Data.Application.Status != domain.StatusAccepted {
		t.Fatalf("expected accepted, got %s", appEnv.Data.Application.Status)
	}

	reEnrollRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/re-enroll", "demo-pmbm", guardianToken, map[string]any{
		"version":            appEnv.Data.Application.Version,
		"agreement_accepted": true,
	})
	if reEnrollRec.Code != http.StatusOK || !strings.Contains(reEnrollRec.Body.String(), `"re_enrollment"`) {
		t.Fatalf("re-enroll: got %d (%s)", reEnrollRec.Code, reEnrollRec.Body.String())
	}

	rePayRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+appID.String()+"/payments", "demo-pmbm", guardianToken, map[string]string{
		"purpose": "re_enrollment_fee",
		"method":  "va",
	})
	if rePayRec.Code != http.StatusCreated {
		t.Fatalf("re-enroll payment: got %d (%s)", rePayRec.Code, rePayRec.Body.String())
	}
	_ = json.Unmarshal(rePayRec.Body.Bytes(), &payEnv)

	hookBody := []byte(`{"external_ref":"` + *payEnv.Data.ExternalRef + `","status":"paid"}`)
	badHook := doRawSignedWebhook(t, router, "/api/v1/webhooks/payments/midtrans", "demo-pmbm", "router-test-webhook-secret", "bad-sig", hookBody)
	if badHook.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad webhook sig, got %d", badHook.Code)
	}
	goodHook := doRawSignedWebhook(t, router, "/api/v1/webhooks/payments/midtrans", "demo-pmbm", "router-test-webhook-secret", "", hookBody)
	if goodHook.Code != http.StatusOK || !strings.Contains(goodHook.Body.String(), `"enrolled"`) {
		t.Fatalf("valid webhook failed: got %d (%s)", goodHook.Code, goodHook.Body.String())
	}

	// Admin exports (CSV & XLSX) + Audit Logs for the newly enrolled student
	csvExportRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/exports", "demo-pmbm", principalToken, map[string]any{
		"unit_id":   unit.ID,
		"period_id": period.ID,
		"format":    "custom_csv",
	})
	if csvExportRec.Code != http.StatusAccepted {
		t.Fatalf("create csv export: got %d (%s)", csvExportRec.Code, csvExportRec.Body.String())
	}
	var exportEnv struct {
		Data struct {
			ID          uuid.UUID `json:"id"`
			Status      string    `json:"status"`
			RowCount    *int      `json:"row_count"`
			DownloadURL *string   `json:"download_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(csvExportRec.Body.Bytes(), &exportEnv); err != nil {
		t.Fatalf("decode export env: %v", err)
	}
	if exportEnv.Data.Status != "completed" || exportEnv.Data.RowCount == nil || *exportEnv.Data.RowCount != 1 || exportEnv.Data.DownloadURL == nil {
		t.Fatalf("unexpected export payload: %+v", exportEnv.Data)
	}
	csvDownload := doJSONRequest(t, router, http.MethodGet, *exportEnv.Data.DownloadURL, "demo-pmbm", principalToken, nil)
	if csvDownload.Code != http.StatusOK || !strings.Contains(csvDownload.Body.String(), "Zainab Al-Hamid") {
		t.Fatalf("csv download unexpected: %d (%s)", csvDownload.Code, csvDownload.Body.String())
	}

	xlsxExportRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/exports", "demo-pmbm", principalToken, map[string]any{
		"unit_id":   unit.ID,
		"period_id": period.ID,
		"format":    "emis_xlsx",
	})
	if xlsxExportRec.Code != http.StatusAccepted {
		t.Fatalf("create xlsx export: got %d (%s)", xlsxExportRec.Code, xlsxExportRec.Body.String())
	}
	_ = json.Unmarshal(xlsxExportRec.Body.Bytes(), &exportEnv)
	xlsxDownload := doJSONRequest(t, router, http.MethodGet, *exportEnv.Data.DownloadURL, "demo-pmbm", principalToken, nil)
	if xlsxDownload.Code != http.StatusOK {
		t.Fatalf("xlsx download failed: %d", xlsxDownload.Code)
	}
	zr, err := zip.NewReader(bytes.NewReader(xlsxDownload.Body.Bytes()), int64(xlsxDownload.Body.Len()))
	if err != nil || len(zr.File) < 4 {
		t.Fatalf("expected valid OOXML zip workbook, err=%v files=%d", err, len(zr.File))
	}

	auditRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/audit-logs?application_id="+appID.String(), "demo-pmbm", principalToken, nil)
	if auditRec.Code != http.StatusOK || !strings.Contains(auditRec.Body.String(), `"re_enrollment_confirmed"`) {
		t.Fatalf("audit logs unexpected: %d (%s)", auditRec.Code, auditRec.Body.String())
	}
}

func TestRouterAdminSetupCRUDUpdatesLivePortalAndAdmissions(t *testing.T) {
	router := newTestRouter(t)
	portalRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/portal", "demo-spmb", "", nil)
	if portalRec.Code != http.StatusOK {
		t.Fatalf("public portal failed: %d", portalRec.Code)
	}
	if portalRec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("expected security header X-Content-Type-Options=nosniff")
	}

	login := func(identifier string) string {
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", "demo-spmb", "", map[string]string{
			"identifier": identifier,
			"password":   "DemoPass123!",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("login %s failed: %d (%s)", identifier, rec.Code, rec.Body.String())
		}
		var env struct {
			Data auth.TokenBundle `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return env.Data.AccessToken
	}

	adminToken := login("admin@alhamid.sch.id")
	principalToken := login("kepsek@smaharapan.sch.id")
	guardianToken := login("+6281234567890")

	// Only tenant_admin can patch white-label settings or create units
	if forbidden := doJSONRequest(t, router, http.MethodPatch, "/api/v1/admin/tenant/settings", "demo-spmb", principalToken, map[string]any{
		"name": "Unauthorized Change",
	}); forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected principal blocked from PATCH settings with 403, got %d", forbidden.Code)
	}

	patchSettings := doJSONRequest(t, router, http.MethodPatch, "/api/v1/admin/tenant/settings", "demo-spmb", adminToken, map[string]any{
		"name": "SMA Harapan Nusantara Terpadu",
		"theme_config": map[string]any{
			"primary_color": "#0F766E",
			"tagline":       "Portal PPDB Modern Terpadu",
		},
	})
	if patchSettings.Code != http.StatusOK || !strings.Contains(patchSettings.Body.String(), "#0F766E") {
		t.Fatalf("patch tenant settings failed: %d (%s)", patchSettings.Code, patchSettings.Body.String())
	}

	// Create Unit -> Create Period -> Create Track -> Replace Requirements -> Create Application on new track!
	createUnitRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/units", "demo-spmb", adminToken, map[string]any{
		"code":            "SMP-CABANG",
		"name":            "SMP Harapan Nusantara Cabang Timur",
		"education_level": "smp_mts",
	})
	if createUnitRec.Code != http.StatusCreated {
		t.Fatalf("create unit failed: %d (%s)", createUnitRec.Code, createUnitRec.Body.String())
	}
	var unitEnv struct {
		Data handlers.UnitView `json:"data"`
	}
	_ = json.Unmarshal(createUnitRec.Body.Bytes(), &unitEnv)

	createPeriodRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/periods", "demo-spmb", principalToken, map[string]any{
		"unit_id":       unitEnv.Data.ID,
		"academic_year": "2026/2027",
		"name":          "Gelombang Khusus Cabang",
		"opens_at":      time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339),
		"closes_at":     time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
	})
	if createPeriodRec.Code != http.StatusCreated {
		t.Fatalf("create period failed: %d (%s)", createPeriodRec.Code, createPeriodRec.Body.String())
	}
	var periodEnv struct {
		Data handlers.PeriodView `json:"data"`
	}
	_ = json.Unmarshal(createPeriodRec.Body.Bytes(), &periodEnv)

	createTrackRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/tracks", "demo-spmb", adminToken, map[string]any{
		"period_id":             periodEnv.Data.ID,
		"code":                  "olimpiade",
		"display_name":          "Jalur Olimpiade Sains",
		"selection_mode":        "hybrid",
		"quota":                 25,
		"registration_fee_idr":  100000,
		"re_enrollment_fee_idr": 500000,
		"requires_achievements": true,
	})
	if createTrackRec.Code != http.StatusCreated {
		t.Fatalf("create track failed: %d (%s)", createTrackRec.Code, createTrackRec.Body.String())
	}
	var trackEnv struct {
		Data handlers.TrackView `json:"data"`
	}
	_ = json.Unmarshal(createTrackRec.Body.Bytes(), &trackEnv)

	reqPut := doJSONRequest(t, router, http.MethodPut, "/api/v1/admin/tracks/"+trackEnv.Data.ID.String()+"/requirements", "demo-spmb", adminToken, map[string]any{
		"requirements": []map[string]any{
			{
				"doc_code":    "piagam_nasional",
				"label":       "Piagam Juara Olimpiade",
				"is_required": true,
				"max_size_kb": 4096,
			},
		},
	})
	if reqPut.Code != http.StatusOK || !strings.Contains(reqPut.Body.String(), "piagam_nasional") {
		t.Fatalf("put track requirements failed: %d (%s)", reqPut.Code, reqPut.Body.String())
	}

	// Verify /public/portal exposes updated branding & newly added unit/period/track/requirements
	updatedPortalRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/portal", "demo-spmb", "", nil)
	body := updatedPortalRec.Body.String()
	if !strings.Contains(body, "SMA Harapan Nusantara Terpadu") || !strings.Contains(body, "Jalur Olimpiade Sains") || !strings.Contains(body, "piagam_nasional") {
		t.Fatalf("updated portal missing live setup changes: %s", body)
	}

	// Verify admissions CreateApplication accepts newly created track!
	createAppRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications", "demo-spmb", guardianToken, map[string]any{
		"unit_id":           unitEnv.Data.ID,
		"period_id":         periodEnv.Data.ID,
		"track_id":          trackEnv.Data.ID,
		"student_full_name": "Bagas Adriansyah",
	})
	if createAppRec.Code != http.StatusCreated || !strings.Contains(createAppRec.Body.String(), `"awaiting_payment"`) {
		t.Fatalf("applicant create on newly configured track failed: %d (%s)", createAppRec.Code, createAppRec.Body.String())
	}
}

func TestPlatformTenantOnboardingAndLifecycle(t *testing.T) {
	router := newTestRouter(t)
	loginAccount := func(slug, identifier, password string) string {
		rec := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", slug, "", map[string]string{
			"identifier": identifier,
			"password":   password,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("login %s on %s failed: %d (%s)", identifier, slug, rec.Code, rec.Body.String())
		}
		var env struct {
			Data auth.TokenBundle `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return env.Data.AccessToken
	}

	// 1. Tenant admin cannot login to platform console (must have is_platform_admin=true)
	nonPlatformRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/platform/auth/login", "", "", map[string]string{
		"email":    "admin@alhamid.sch.id",
		"password": "DemoPass123!",
	})
	if nonPlatformRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-platform user, got %d (%s)", nonPlatformRec.Code, nonPlatformRec.Body.String())
	}

	// 2. Seeded platform superadmin logs in without any X-Tenant-Slug header
	loginRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/platform/auth/login", "", "", map[string]string{
		"email":    "platform@spmb-platform.id",
		"password": "DemoPass123!",
	})
	if loginRec.Code != http.StatusOK {
		t.Fatalf("platform login failed: %d (%s)", loginRec.Code, loginRec.Body.String())
	}
	var loginEnv struct {
		Data auth.TokenBundle `json:"data"`
	}
	_ = json.Unmarshal(loginRec.Body.Bytes(), &loginEnv)
	platformToken := loginEnv.Data.AccessToken
	if platformToken == "" || !loginEnv.Data.User.IsPlatformAdmin {
		t.Fatalf("expected valid platform admin token, got %+v", loginEnv.Data.User)
	}

	// 3. Tenant user token is rejected on /api/v1/platform/tenants
	regularAdminToken := loginAccount("demo-spmb", "admin@alhamid.sch.id", "DemoPass123!")
	forbiddenList := doJSONRequest(t, router, http.MethodGet, "/api/v1/platform/tenants", "", regularAdminToken, nil)
	if forbiddenList.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for tenant_admin on platform endpoint, got %d", forbiddenList.Code)
	}

	// 4. Platform admin lists seeded tenants
	listRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/platform/tenants", "", platformToken, nil)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "demo-spmb") {
		t.Fatalf("list platform tenants failed: %d (%s)", listRec.Code, listRec.Body.String())
	}

	// 5. Platform admin onboards a new foundation tenant + starter unit + initial tenant_admin account
	createRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/platform/tenants", "", platformToken, map[string]any{
		"slug":               "yayasan-alfalah",
		"name":               "Yayasan Pendidikan Al-Falah",
		"institution_type":   "foundation",
		"terminology_preset": "mandiri",
		"primary_color":      "#0F766E",
		"tagline":            "Penerimaan Terpadu Al-Falah",
		"custom_domain":      "ppdb.alfalah.sch.id",
		"subscription_plan":  "pro",
		"starter_unit_code":  "SMP-AF",
		"starter_unit_name":  "SMP Islam Al-Falah",
		"starter_unit_level": "smp_mts",
		"admin_email":        "ketua@alfalah.sch.id",
		"admin_full_name":    "Ustadz Yusuf (Ketua Panitia)",
		"admin_password":     "AlFalah2026!",
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create platform tenant failed: %d (%s)", createRec.Code, createRec.Body.String())
	}
	var createEnv struct {
		Data handlers.PlatformCreateTenantResult `json:"data"`
	}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createEnv)
	newTenantID := createEnv.Data.Tenant.ID
	if createEnv.Data.InitialAdmin == nil || createEnv.Data.InitialAdmin.Email != "ketua@alfalah.sch.id" {
		t.Fatalf("expected provisioned initial admin, got %+v", createEnv.Data)
	}

	// 6. Newly provisioned admin can log into the new tenant and see its portal & starter unit
	portalRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/portal", "yayasan-alfalah", "", nil)
	if portalRec.Code != http.StatusOK || !strings.Contains(portalRec.Body.String(), "SMP Islam Al-Falah") {
		t.Fatalf("new tenant portal lookup failed: %d (%s)", portalRec.Code, portalRec.Body.String())
	}
	newAdminToken := loginAccount("yayasan-alfalah", "ketua@alfalah.sch.id", "AlFalah2026!")
	settingsRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/tenant/settings", "yayasan-alfalah", newAdminToken, nil)
	if settingsRec.Code != http.StatusOK || !strings.Contains(settingsRec.Body.String(), "yayasan-alfalah") {
		t.Fatalf("new tenant admin settings fetch failed: %d (%s)", settingsRec.Code, settingsRec.Body.String())
	}

	// 7. Custom domain Caddy TLS verification follows domain_verified + is_active
	unverifiedTLS := doJSONRequest(t, router, http.MethodGet, "/internal/tls/verify?domain=ppdb.alfalah.sch.id", "", "", nil)
	if unverifiedTLS.Code != http.StatusForbidden {
		t.Fatalf("expected 403 before domain_verified=true, got %d", unverifiedTLS.Code)
	}
	verifyPatch := doJSONRequest(t, router, http.MethodPatch, "/api/v1/platform/tenants/"+newTenantID.String(), "", platformToken, map[string]any{
		"domain_verified": true,
	})
	if verifyPatch.Code != http.StatusOK {
		t.Fatalf("patch domain_verified failed: %d (%s)", verifyPatch.Code, verifyPatch.Body.String())
	}
	verifiedTLS := doJSONRequest(t, router, http.MethodGet, "/internal/tls/verify?domain=ppdb.alfalah.sch.id", "", "", nil)
	if verifiedTLS.Code != http.StatusOK {
		t.Fatalf("expected 200 after domain_verified=true, got %d", verifiedTLS.Code)
	}

	// 8. Suspending tenant hides public portal and revokes Caddy TLS approval
	suspendPatch := doJSONRequest(t, router, http.MethodPatch, "/api/v1/platform/tenants/"+newTenantID.String(), "", platformToken, map[string]any{
		"is_active": false,
	})
	if suspendPatch.Code != http.StatusOK {
		t.Fatalf("suspend tenant failed: %d (%s)", suspendPatch.Code, suspendPatch.Body.String())
	}
	suspendedPortal := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/portal", "yayasan-alfalah", "", nil)
	if suspendedPortal.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on suspended tenant portal, got %d", suspendedPortal.Code)
	}
	suspendedTLS := doJSONRequest(t, router, http.MethodGet, "/internal/tls/verify?domain=ppdb.alfalah.sch.id", "", "", nil)
	if suspendedTLS.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for Caddy TLS on suspended tenant, got %d", suspendedTLS.Code)
	}
}

func TestOperationalEndpointsAndProductionHardening(t *testing.T) {
	const midtransSecret = "SB-Mid-server-RouterSecret"
	midtransSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/snap/v1/transactions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"snap-router-tok","redirect_url":"https://app.sandbox.midtrans.com/snap/v2/vtweb/snap-router-tok"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/") && strings.HasSuffix(r.URL.Path, "/status"):
			trimmed := strings.TrimPrefix(r.URL.Path, "/v2/")
			orderID := strings.TrimSuffix(trimmed, "/status")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"order_id":%q,"transaction_status":"settlement","gross_amount":"250000.00"}`, orderID)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer midtransSrv.Close()

	dns := &stubRouterDNS{
		txt: map[string][]string{},
		cname: map[string]string{
			"ppdb.customsekolah.id": "demo-spmb.spmb-platform.id",
		},
	}
	payGw := &integrations.MidtransSnapGateway{
		EndpointURL:       midtransSrv.URL + "/snap/v1/transactions",
		StatusEndpointFmt: midtransSrv.URL + "/v2/%s/status",
		ServerKey:         midtransSecret,
		Sandbox:           true,
	}
	router, _ := newTestRouterWithDeps(t, dns, payGw, midtransSecret)

	// 1. Auth password reset via OTP revokes old password and enables login with new password
	resetOtpReq := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/otp/request", "demo-spmb", "", map[string]string{
		"whatsapp_phone": "081234567890",
		"purpose":        "reset_pin",
	})
	if resetOtpReq.Code != http.StatusAccepted {
		t.Fatalf("reset otp request failed: %d (%s)", resetOtpReq.Code, resetOtpReq.Body.String())
	}
	var resetOTP struct {
		Data struct {
			DevOTP string `json:"dev_otp"`
		} `json:"data"`
	}
	_ = json.Unmarshal(resetOtpReq.Body.Bytes(), &resetOTP)
	resetRes := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/password/reset", "demo-spmb", "", map[string]string{
		"whatsapp_phone": "081234567890",
		"code":           resetOTP.Data.DevOTP,
		"new_password":   "NewSecret999",
	})
	if resetRes.Code != http.StatusOK {
		t.Fatalf("password reset failed: %d (%s)", resetRes.Code, resetRes.Body.String())
	}
	if oldLogin := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", "demo-spmb", "", map[string]string{
		"identifier": "081234567890",
		"password":   "DemoPass123!",
	}); oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password to be rejected (401), got %d", oldLogin.Code)
	}
	newLogin := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", "demo-spmb", "", map[string]string{
		"identifier": "081234567890",
		"password":   "NewSecret999",
	})
	if newLogin.Code != http.StatusOK {
		t.Fatalf("login with reset password failed: %d (%s)", newLogin.Code, newLogin.Body.String())
	}
	var guardianEnv struct {
		Data auth.TokenBundle `json:"data"`
	}
	_ = json.Unmarshal(newLogin.Body.Bytes(), &guardianEnv)
	guardianToken := guardianEnv.Data.AccessToken

	// 2. Staff & role management CRUD by tenant_admin
	adminLogin := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/login", "demo-spmb", "", map[string]string{
		"identifier": "admin@alhamid.sch.id",
		"password":   "DemoPass123!",
	})
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("admin login failed: %d (%s)", adminLogin.Code, adminLogin.Body.String())
	}
	var adminEnv struct {
		Data auth.TokenBundle `json:"data"`
	}
	_ = json.Unmarshal(adminLogin.Body.Bytes(), &adminEnv)
	adminToken := adminEnv.Data.AccessToken

	staffList := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/staff", "demo-spmb", adminToken, nil)
	if staffList.Code != http.StatusOK || !strings.Contains(staffList.Body.String(), "admin@alhamid.sch.id") {
		t.Fatalf("list staff failed: %d (%s)", staffList.Code, staffList.Body.String())
	}

	// 3. Logo presign -> PUT /api/v1/storage/object -> confirm -> GET /api/v1/public/logo
	logoPresign := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/tenant/logo/presign", "demo-spmb", adminToken, map[string]any{
		"filename":   "logo.png",
		"mime_type":  "image/png",
		"size_bytes": 128,
	})
	if logoPresign.Code != http.StatusOK {
		t.Fatalf("logo presign failed: %d (%s)", logoPresign.Code, logoPresign.Body.String())
	}
	var logoPreEnv struct {
		Data admissions.PresignUploadResult `json:"data"`
	}
	_ = json.Unmarshal(logoPresign.Body.Bytes(), &logoPreEnv)
	logoBytes := []byte("\x89PNG\r\n\x1a\nMOCKLOGO")
	uploadSignedURL(t, router, logoPreEnv.Data.UploadURL, "image/png", logoBytes)

	logoConfirm := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/tenant/logo/confirm", "demo-spmb", adminToken, map[string]string{
		"storage_key": logoPreEnv.Data.StorageKey,
	})
	if logoConfirm.Code != http.StatusOK {
		t.Fatalf("logo confirm failed: %d (%s)", logoConfirm.Code, logoConfirm.Body.String())
	}
	logoGetReq := httptest.NewRequest(http.MethodGet, "/api/v1/public/logo", nil)
	logoGetReq.Header.Set("X-Tenant-Slug", "demo-spmb")
	logoGetRec := httptest.NewRecorder()
	router.ServeHTTP(logoGetRec, logoGetReq)
	if logoGetRec.Code != http.StatusOK || !bytes.Equal(logoGetRec.Body.Bytes(), logoBytes) {
		t.Fatalf("GET /api/v1/public/logo failed: %d body=%q", logoGetRec.Code, logoGetRec.Body.String())
	}

	// 4. Custom domain update -> DNS verification
	patchDomain := doJSONRequest(t, router, http.MethodPatch, "/api/v1/admin/tenant/settings", "demo-spmb", adminToken, map[string]any{
		"custom_domain": "ppdb.customsekolah.id",
	})
	if patchDomain.Code != http.StatusOK {
		t.Fatalf("patch custom_domain failed: %d (%s)", patchDomain.Code, patchDomain.Body.String())
	}
	verifyDNSRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/tenant/domain/verify", "demo-spmb", adminToken, nil)
	if verifyDNSRec.Code != http.StatusOK || !strings.Contains(verifyDNSRec.Body.String(), `"verified":true`) {
		t.Fatalf("DNS verify failed: %d (%s)", verifyDNSRec.Code, verifyDNSRec.Body.String())
	}

	// 5. Public QR document verification & Assessment Schedule CRUD
	portalRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/portal", "demo-spmb", "", nil)
	var portalEnv struct {
		Data handlers.PublicPortalPayload `json:"data"`
	}
	_ = json.Unmarshal(portalRec.Body.Bytes(), &portalEnv)
	spmbPeriod := portalEnv.Data.Periods[0]
	trackID := spmbPeriod.Tracks[0].ID

	createApp := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications", "demo-spmb", guardianToken, map[string]any{
		"unit_id":           spmbPeriod.UnitID,
		"period_id":         spmbPeriod.ID,
		"track_id":          trackID,
		"student_full_name": "Bintang QR & Midtrans",
	})
	if createApp.Code != http.StatusCreated {
		t.Fatalf("create app failed: %d (%s)", createApp.Code, createApp.Body.String())
	}
	var appEnv struct {
		Data admissions.ApplicationDetail `json:"data"`
	}
	_ = json.Unmarshal(createApp.Body.Bytes(), &appEnv)
	if appEnv.Data.VerificationToken == "" {
		t.Fatalf("expected non-empty verification_token on application detail")
	}

	qrVerify := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/verify-document?token="+url.QueryEscape(appEnv.Data.VerificationToken), "demo-spmb", "", nil)
	if qrVerify.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/public/verify-document failed: %d (%s)", qrVerify.Code, qrVerify.Body.String())
	}
	var qrEnv struct {
		Data handlers.PublicDocumentVerifyResult `json:"data"`
	}
	_ = json.Unmarshal(qrVerify.Body.Bytes(), &qrEnv)
	if !qrEnv.Data.Valid || qrEnv.Data.RegistrationNo != appEnv.Data.Application.RegistrationNo || qrEnv.Data.MaskedName != "B****** Q* & M*******" {
		t.Fatalf("unexpected verify-document payload: %+v", qrEnv.Data)
	}

	schedCreate := doJSONRequest(t, router, http.MethodPost, "/api/v1/admin/schedules", "demo-spmb", adminToken, map[string]any{
		"application_id": appEnv.Data.Application.ID,
		"session_name":   "Wawancara BTQ Gelombang 1",
		"room_or_link":   "Aula Gedung B",
		"scheduled_at":   time.Now().UTC().Add(48 * time.Hour),
	})
	if schedCreate.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/schedules failed: %d (%s)", schedCreate.Code, schedCreate.Body.String())
	}
	var schedEnv struct {
		Data []admissions.AssessmentSchedule `json:"data"`
	}
	_ = json.Unmarshal(schedCreate.Body.Bytes(), &schedEnv)
	schedID := schedEnv.Data[0].ID

	schedList := doJSONRequest(t, router, http.MethodGet, "/api/v1/admin/schedules?application_id="+appEnv.Data.Application.ID.String(), "demo-spmb", adminToken, nil)
	if schedList.Code != http.StatusOK || !strings.Contains(schedList.Body.String(), "Wawancara BTQ Gelombang 1") {
		t.Fatalf("GET /api/v1/admin/schedules failed: %d (%s)", schedList.Code, schedList.Body.String())
	}
	schedDel := doJSONRequest(t, router, http.MethodDelete, "/api/v1/admin/schedules/"+schedID.String(), "demo-spmb", adminToken, nil)
	if schedDel.Code != http.StatusOK {
		t.Fatalf("DELETE /api/v1/admin/schedules failed: %d (%s)", schedDel.Code, schedDel.Body.String())
	}

	// 6. Central Midtrans webhook (WITHOUT X-Tenant-Slug) & Sync endpoint on demo-pmbm paid track
	pmbmPortalRec := doJSONRequest(t, router, http.MethodGet, "/api/v1/public/portal", "demo-pmbm", "", nil)
	var pmbmPortal handlers.PublicPortalPayload
	var pmbmPortalEnv struct {
		Data handlers.PublicPortalPayload `json:"data"`
	}
	_ = json.Unmarshal(pmbmPortalRec.Body.Bytes(), &pmbmPortalEnv)
	pmbmPortal = pmbmPortalEnv.Data
	pmbmPeriod := pmbmPortal.Periods[0]
	var paidTrack uuid.UUID
	for _, tr := range pmbmPeriod.Tracks {
		if tr.RegistrationFeeIDR > 0 {
			paidTrack = tr.ID
			break
		}
	}
	pmbmRegOTP := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/otp/request", "demo-pmbm", "", map[string]string{
		"whatsapp_phone": "081998877666",
		"purpose":        "register",
	})
	var pmbmOTPEnv struct {
		Data struct {
			DevOTP string `json:"dev_otp"`
		} `json:"data"`
	}
	_ = json.Unmarshal(pmbmRegOTP.Body.Bytes(), &pmbmOTPEnv)
	pmbmVerify := doJSONRequest(t, router, http.MethodPost, "/api/v1/auth/otp/verify", "demo-pmbm", "", map[string]string{
		"whatsapp_phone": "081998877666",
		"purpose":        "register",
		"code":           pmbmOTPEnv.Data.DevOTP,
		"full_name":      "Wali PMBM Central",
		"password":       "SecretPMBM1",
	})
	var pmbmAuthEnv struct {
		Data auth.TokenBundle `json:"data"`
	}
	_ = json.Unmarshal(pmbmVerify.Body.Bytes(), &pmbmAuthEnv)
	pmbmToken := pmbmAuthEnv.Data.AccessToken

	pmbmAppRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications", "demo-pmbm", pmbmToken, map[string]any{
		"unit_id":           pmbmPeriod.UnitID,
		"period_id":         pmbmPeriod.ID,
		"track_id":          paidTrack,
		"student_full_name": "Farhan Midtrans Central",
	})
	if pmbmAppRec.Code != http.StatusCreated {
		t.Fatalf("create pmbm app failed: %d (%s)", pmbmAppRec.Code, pmbmAppRec.Body.String())
	}
	var pmbmAppEnv struct {
		Data admissions.ApplicationDetail `json:"data"`
	}
	_ = json.Unmarshal(pmbmAppRec.Body.Bytes(), &pmbmAppEnv)
	pmbmAppID := pmbmAppEnv.Data.Application.ID

	paySnapRec := doJSONRequest(t, router, http.MethodPost, "/api/v1/applicant/applications/"+pmbmAppID.String()+"/payments", "demo-pmbm", pmbmToken, map[string]string{
		"purpose": "registration_fee",
		"method":  "va",
	})
	if paySnapRec.Code != http.StatusCreated {
		t.Fatalf("create snap payment failed: %d (%s)", paySnapRec.Code, paySnapRec.Body.String())
	}
	var snapPayEnv struct {
		Data admissions.PaymentTransaction `json:"data"`
	}
	_ = json.Unmarshal(paySnapRec.Body.Bytes(), &snapPayEnv)
	if snapPayEnv.Data.PaymentURL == nil || !strings.Contains(*snapPayEnv.Data.PaymentURL, "snap-router-tok") {
		t.Fatalf("expected Midtrans Snap payment_url, got %+v", snapPayEnv.Data.PaymentURL)
	}

	// Central Midtrans callback WITHOUT X-Tenant-Slug header on root domain
	orderID := *snapPayEnv.Data.ExternalRef
	grossStr := fmt.Sprintf("%d.00", snapPayEnv.Data.AmountIDR)
	sum := sha512.Sum512([]byte(orderID + "200" + grossStr + midtransSecret))
	sigKey := hex.EncodeToString(sum[:])
	centralHook := doJSONRequest(t, router, http.MethodPost, "/api/v1/webhooks/payments/midtrans", "", "", map[string]any{
		"order_id":           orderID,
		"status_code":        "200",
		"gross_amount":       grossStr,
		"transaction_status": "settlement",
		"signature_key":      sigKey,
	})
	if centralHook.Code != http.StatusOK || !strings.Contains(centralHook.Body.String(), `"paid"`) {
		t.Fatalf("central midtrans webhook failed: %d (%s)", centralHook.Code, centralHook.Body.String())
	}
}

