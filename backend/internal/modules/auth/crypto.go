package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidPhone = errors.New("invalid whatsapp phone format")
	ErrInvalidToken = errors.New("invalid or expired token")
)

// AccessClaims defines the signed JWT payload carried on authenticated API requests.
type AccessClaims struct {
	TenantID        uuid.UUID          `json:"tid"`
	IsPlatformAdmin bool               `json:"adm"`
	Roles           []domain.RoleScope `json:"roles"`
	jwt.RegisteredClaims
}

// JWTManager issues and verifies HS256 access tokens bound to a tenant.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTManager initializes a JWTManager with the provided HMAC secret and access token lifetime.
func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// TTLSeconds returns access token lifetime in whole seconds.
func (m *JWTManager) TTLSeconds() int {
	return int(m.ttl.Seconds())
}

// Issue creates a signed JWT access token for a user within a tenant context.
func (m *JWTManager) Issue(userID, tenantID uuid.UUID, isPlatformAdmin bool, roles []domain.RoleScope) (string, error) {
	now := time.Now().UTC()
	claims := AccessClaims{
		TenantID:        tenantID,
		IsPlatformAdmin: isPlatformAdmin,
		Roles:           roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Verify parses and validates an access token string, returning its claims on success.
func (m *JWTManager) Verify(tokenStr string) (*AccessClaims, error) {
	parsed, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := parsed.Claims.(*AccessClaims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// NormalizeIndonesianPhone canonicalizes Indonesian WhatsApp numbers (08..., 628..., +628...) to E.164 (+62...).
func NormalizeIndonesianPhone(raw string) (string, error) {
	cleaned := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '+' {
			return r
		}
		return -1
	}, strings.TrimSpace(raw))

	cleaned = strings.TrimPrefix(cleaned, "+")
	if strings.HasPrefix(cleaned, "0") {
		cleaned = "62" + cleaned[1:]
	}
	if len(cleaned) < 10 || len(cleaned) > 15 || !strings.HasPrefix(cleaned, "62") {
		return "", ErrInvalidPhone
	}
	return "+" + cleaned, nil
}

// HashPassword hashes a plaintext password or PIN using bcrypt.
func HashPassword(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// CheckPassword compares a bcrypt password hash against a candidate plaintext password.
func CheckPassword(hash, candidate string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(candidate)) == nil
}

// GenerateOTPCode returns a cryptographically random 6-digit numeric string.
func GenerateOTPCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// HashOTP binds an OTP code to its phone, purpose, and server secret using HMAC-SHA256.
func HashOTP(secret, phone string, purpose domain.OTPPurpose, code string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(phone))
	mac.Write([]byte("|"))
	mac.Write([]byte(purpose))
	mac.Write([]byte("|"))
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyOTPHash performs constant-time HMAC comparison against a stored OTP hash.
func VerifyOTPHash(secret, phone string, purpose domain.OTPPurpose, candidateCode, expectedHash string) bool {
	computed := HashOTP(secret, phone, purpose, candidateCode)
	return hmac.Equal([]byte(computed), []byte(expectedHash))
}

// GenerateOpaqueToken generates a cryptographically random URL-safe refresh token and its SHA-256 hex digest.
func GenerateOpaqueToken() (raw string, digest string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// HashToken returns the SHA-256 hex digest of an opaque refresh token.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

