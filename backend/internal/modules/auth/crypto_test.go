package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

func TestNormalizeIndonesianPhone(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"0812-3456-7890", "+6281234567890", true},
		{"+62 812 3456 7890", "+6281234567890", true},
		{"6281234567890", "+6281234567890", true},
		{"12345", "", false},
		{"+14155552671", "", false},
	}

	for _, tc := range cases {
		got, err := NormalizeIndonesianPhone(tc.raw)
		if (err == nil) != tc.ok || got != tc.want {
			t.Fatalf("NormalizeIndonesianPhone(%q) = (%q, %v), want (%q, ok=%v)", tc.raw, got, err, tc.want, tc.ok)
		}
	}
}

func TestJWTAndOTPHelpers(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-with-sufficient-length", 15*time.Minute)
	uid := uuid.New()
	tid := uuid.New()
	roles := []domain.RoleScope{{TenantID: tid, Role: domain.RoleVerifier}}

	token, err := mgr.Issue(uid, tid, false, roles)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	claims, err := mgr.Verify(token)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if claims.Subject != uid.String() || claims.TenantID != tid || len(claims.Roles) != 1 {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	hash := HashOTP("pepper", "+6281234567890", domain.OTPPurposeLogin, "123456")
	if !VerifyOTPHash("pepper", "+6281234567890", domain.OTPPurposeLogin, "123456", hash) {
		t.Fatal("expected OTP hash verification to succeed")
	}
	if VerifyOTPHash("pepper", "+6281234567890", domain.OTPPurposeRegister, "123456", hash) {
		t.Fatal("expected OTP hash tied to different purpose to fail")
	}
}

