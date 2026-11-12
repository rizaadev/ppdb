package auth

import (
	"context"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

// User represents an account for either an applicant/guardian or an institution staff member.
type User struct {
	ID              uuid.UUID
	WhatsAppPhone   *string
	Email           *string
	PasswordHash    string
	FullName        string
	IsPlatformAdmin bool
	IsActive        bool
	PhoneVerifiedAt *time.Time
}

// OTPChallenge represents a persisted WhatsApp verification code challenge.
type OTPChallenge struct {
	ID            uuid.UUID
	TenantID      *uuid.UUID
	WhatsAppPhone string
	Purpose       domain.OTPPurpose
	CodeHash      string
	AttemptCount  int16
	MaxAttempts   int16
	ExpiresAt     time.Time
	ConsumedAt    *time.Time
}

// SessionRecord stores a hashed refresh token session bound to a user and tenant.
type SessionRecord struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	TenantID         *uuid.UUID
	RefreshTokenHash string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
}

// CreateUserInput holds validated attributes for inserting a new user account.
type CreateUserInput struct {
	ID              uuid.UUID
	WhatsAppPhone   *string
	Email           *string
	PasswordHash    string
	FullName        string
	PhoneVerifiedAt *time.Time
}

// Store abstracts persistence operations for authentication, OTP challenges, sessions, and RBAC roles.
type Store interface {
	CreateOTPChallenge(ctx context.Context, challenge OTPChallenge, plainCodeForDispatch string) error
	GetLatestActiveOTP(ctx context.Context, phone string, purpose domain.OTPPurpose) (*OTPChallenge, error)
	IncrementOTPAttempt(ctx context.Context, challengeID uuid.UUID) (attemptCount int16, maxAttempts int16, err error)
	ConsumeOTPChallenge(ctx context.Context, challengeID uuid.UUID) error

	GetUserByID(ctx context.Context, userID uuid.UUID) (*User, error)
	GetUserByWhatsAppPhone(ctx context.Context, phone string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	CreateUser(ctx context.Context, input CreateUserInput) (*User, error)
	ProvisionTenantAdmin(ctx context.Context, tenantID uuid.UUID, email, fullName, passwordHash string) (*User, error)
	MarkUserPhoneVerified(ctx context.Context, userID uuid.UUID) error
	UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
	ListUserRolesInTenant(ctx context.Context, userID, tenantID uuid.UUID) ([]domain.RoleScope, error)

	CreateSession(ctx context.Context, session SessionRecord, userAgent string, ip *netip.Addr) error
	RotateSession(ctx context.Context, oldTokenHash string, next SessionRecord, userAgent string, ip *netip.Addr) (*SessionRecord, error)
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error
	RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error
}

