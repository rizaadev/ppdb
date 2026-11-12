package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/modules/auth"
)

var (
	testSPMBTenantID = uuid.MustParse("01960000-0000-7000-8000-000000000001")
	testPMBMTenantID = uuid.MustParse("01960000-0000-7000-8000-000000000002")
)

func newTestAuthService(t *testing.T, isDev bool) (*auth.Service, *auth.JWTManager) {
	t.Helper()
	store := auth.NewDemoStore()
	jwtMgr := auth.NewJWTManager("test-jwt-secret-key-at-least-32-bytes", 15*time.Minute)
	limiter := auth.NewOTPRateLimiter(time.Minute, 15*time.Minute, 5)
	svc := auth.NewService(store, jwtMgr, limiter, "test-otp-secret", 7*24*time.Hour, isDev, nil)
	return svc, jwtMgr
}

func TestService_OTPRegisterAndPasswordLogin(t *testing.T) {
	ctx := context.Background()
	svc, jwtMgr := newTestAuthService(t, true)

	otpRes, err := svc.RequestOTP(ctx, testSPMBTenantID, "081998877665", domain.OTPPurposeRegister)
	if err != nil {
		t.Fatalf("RequestOTP failed: %v", err)
	}
	if otpRes.NormalizedPhone != "+6281998877665" {
		t.Fatalf("expected normalized phone +6281998877665, got %q", otpRes.NormalizedPhone)
	}
	if len(otpRes.DevOTP) != 6 {
		t.Fatalf("expected 6-digit DevOTP in dev mode, got %q", otpRes.DevOTP)
	}

	bundle, err := svc.VerifyOTP(
		ctx,
		testSPMBTenantID,
		"+6281998877665",
		domain.OTPPurposeRegister,
		otpRes.DevOTP,
		"Budi Santoso",
		"Rahasia123!",
		auth.ClientMeta{UserAgent: "go-test"},
	)
	if err != nil {
		t.Fatalf("VerifyOTP failed: %v", err)
	}
	if bundle.AccessToken == "" || bundle.RefreshToken == "" {
		t.Fatal("expected non-empty access and refresh tokens")
	}
	if bundle.User.FullName != "Budi Santoso" {
		t.Fatalf("unexpected FullName: %q", bundle.User.FullName)
	}
	if len(bundle.User.Roles) != 0 {
		t.Fatalf("expected applicant to have 0 staff roles, got %v", bundle.User.Roles)
	}

	claims, err := jwtMgr.Verify(bundle.AccessToken)
	if err != nil {
		t.Fatalf("JWT Verify failed: %v", err)
	}
	if claims.TenantID != testSPMBTenantID {
		t.Fatalf("expected token tenant %s, got %s", testSPMBTenantID, claims.TenantID)
	}

	// Newly registered guardian should be able to sign in with local 08 prefix + password.
	loginBundle, err := svc.Login(ctx, testSPMBTenantID, "081998877665", "Rahasia123!", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("Login after register failed: %v", err)
	}
	if loginBundle.User.ID != bundle.User.ID {
		t.Fatalf("expected same user ID %s, got %s", bundle.User.ID, loginBundle.User.ID)
	}

	if _, err := svc.Login(ctx, testSPMBTenantID, "081998877665", "WrongPass", auth.ClientMeta{}); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestService_OTPRateLimitAndDevOTPHiddenInProd(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestAuthService(t, false)

	res, err := svc.RequestOTP(ctx, testSPMBTenantID, "081112223334", domain.OTPPurposeLogin)
	if err != nil {
		t.Fatalf("first RequestOTP failed: %v", err)
	}
	if res.DevOTP != "" {
		t.Fatalf("expected DevOTP omitted in non-dev mode, got %q", res.DevOTP)
	}

	limitedRes, err := svc.RequestOTP(ctx, testSPMBTenantID, "+6281112223334", domain.OTPPurposeLogin)
	if !errors.Is(err, auth.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if limitedRes.RetryAfterSecond <= 0 {
		t.Fatalf("expected positive RetryAfterSecond, got %d", limitedRes.RetryAfterSecond)
	}
}

func TestService_OTPMaxAttemptsLockout(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestAuthService(t, true)

	otpRes, err := svc.RequestOTP(ctx, testSPMBTenantID, "+6281234567890", domain.OTPPurposeLogin)
	if err != nil {
		t.Fatalf("RequestOTP failed: %v", err)
	}

	wrongCode := "000000"
	if otpRes.DevOTP == wrongCode {
		wrongCode = "999999"
	}

	for attempt := 1; attempt <= 4; attempt++ {
		_, err := svc.VerifyOTP(ctx, testSPMBTenantID, "+6281234567890", domain.OTPPurposeLogin, wrongCode, "", "", auth.ClientMeta{})
		if !errors.Is(err, auth.ErrOTPInvalidOrExpired) {
			t.Fatalf("attempt %d: expected ErrOTPInvalidOrExpired, got %v", attempt, err)
		}
	}

	// 5th invalid attempt reaches MaxAttempts=5.
	_, err = svc.VerifyOTP(ctx, testSPMBTenantID, "+6281234567890", domain.OTPPurposeLogin, wrongCode, "", "", auth.ClientMeta{})
	if !errors.Is(err, auth.ErrOTPMaxAttempts) {
		t.Fatalf("5th attempt: expected ErrOTPMaxAttempts, got %v", err)
	}

	// Even with the right OTP afterwards, the locked challenge must reject verification.
	_, err = svc.VerifyOTP(ctx, testSPMBTenantID, "+6281234567890", domain.OTPPurposeLogin, otpRes.DevOTP, "", "", auth.ClientMeta{})
	if !errors.Is(err, auth.ErrOTPMaxAttempts) {
		t.Fatalf("post-lockout attempt: expected ErrOTPMaxAttempts, got %v", err)
	}
}

func TestService_StaffLoginAndTenantScopedRoles(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestAuthService(t, true)

	spmbBundle, err := svc.Login(ctx, testSPMBTenantID, "verifikator@smaharapan.sch.id", "DemoPass123!", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("staff Login failed: %v", err)
	}
	if len(spmbBundle.User.Roles) != 1 || spmbBundle.User.Roles[0].Role != domain.RoleVerifier {
		t.Fatalf("expected verifier role in SPMB tenant, got %+v", spmbBundle.User.Roles)
	}

	// Same user has no role assigned inside PMBM tenant.
	pmbmBundle, err := svc.Login(ctx, testPMBMTenantID, "verifikator@smaharapan.sch.id", "DemoPass123!", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("staff Login in PMBM failed: %v", err)
	}
	if len(pmbmBundle.User.Roles) != 0 {
		t.Fatalf("expected empty roles in PMBM tenant, got %+v", pmbmBundle.User.Roles)
	}

	me, err := svc.GetMe(ctx, spmbBundle.User.ID, testSPMBTenantID)
	if err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}
	if len(me.Roles) != 1 || me.Roles[0].Role != domain.RoleVerifier {
		t.Fatalf("expected GetMe to include verifier role, got %+v", me.Roles)
	}
}

func TestService_RefreshRotationReplayAndLogout(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestAuthService(t, true)

	initial, err := svc.Login(ctx, testPMBMTenantID, "admin@alhamid.sch.id", "DemoPass123!", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Refresh bound to wrong tenant must be rejected without revoking legitimate session.
	if _, err := svc.Refresh(ctx, testSPMBTenantID, initial.RefreshToken, auth.ClientMeta{}); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Fatalf("cross-tenant refresh: expected ErrSessionInvalid, got %v", err)
	}

	rotated, err := svc.Refresh(ctx, testPMBMTenantID, initial.RefreshToken, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if rotated.RefreshToken == initial.RefreshToken {
		t.Fatal("expected rotated refresh token to differ from initial token")
	}
	if rotated.User.ID != initial.User.ID || len(rotated.User.Roles) != 1 {
		t.Fatalf("unexpected rotated user payload: %+v", rotated.User)
	}

	// Replaying the spent initial refresh token must fail.
	if _, err := svc.Refresh(ctx, testPMBMTenantID, initial.RefreshToken, auth.ClientMeta{}); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Fatalf("replay refresh: expected ErrSessionInvalid, got %v", err)
	}

	// Logout revokes the active refresh token.
	if err := svc.Logout(ctx, rotated.RefreshToken); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}
	if _, err := svc.Refresh(ctx, testPMBMTenantID, rotated.RefreshToken, auth.ClientMeta{}); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Fatalf("post-logout refresh: expected ErrSessionInvalid, got %v", err)
	}
}

func TestService_PasswordResetViaOTPRevokesSessions(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestAuthService(t, true)

	// Existing guardian signs in first so an active refresh session exists.
	oldSession, err := svc.Login(ctx, testSPMBTenantID, "081234567890", "DemoPass123!", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("initial Login failed: %v", err)
	}

	otpRes, err := svc.RequestOTP(ctx, testSPMBTenantID, "+6281234567890", domain.OTPPurposeResetPin)
	if err != nil {
		t.Fatalf("RequestOTP reset_pin failed: %v", err)
	}

	resetBundle, err := svc.ResetPasswordViaOTP(ctx, testSPMBTenantID, "081234567890", otpRes.DevOTP, "NewStrong99!", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("ResetPasswordViaOTP failed: %v", err)
	}
	if resetBundle.AccessToken == "" {
		t.Fatal("expected fresh tokens after password reset")
	}

	// Prior refresh token must be revoked after password reset.
	if _, err := svc.Refresh(ctx, testSPMBTenantID, oldSession.RefreshToken, auth.ClientMeta{}); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Fatalf("expected old session to be revoked after password reset, got %v", err)
	}

	// Old password fails, new password succeeds.
	if _, err := svc.Login(ctx, testSPMBTenantID, "081234567890", "DemoPass123!", auth.ClientMeta{}); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected old password to be rejected, got %v", err)
	}
	if _, err := svc.Login(ctx, testSPMBTenantID, "081234567890", "NewStrong99!", auth.ClientMeta{}); err != nil {
		t.Fatalf("expected login with new password to succeed, got %v", err)
	}
}

