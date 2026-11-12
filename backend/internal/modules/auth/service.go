package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

var (
	ErrRateLimited           = errors.New("otp request rate limited")
	ErrOTPInvalidOrExpired   = errors.New("otp code invalid or expired")
	ErrOTPMaxAttempts        = errors.New("otp maximum attempts exceeded")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrPhoneAlreadyExists    = errors.New("whatsapp phone already registered")
	ErrUserNotFound          = errors.New("user not found")
	ErrSessionInvalid        = errors.New("refresh session invalid or expired")
	ErrPasswordTooShort      = errors.New("password must be at least 6 characters")
	ErrFullNameRequired      = errors.New("full name is required")
	ErrPlatformAdminRequired = errors.New("platform superadmin access required")
	ErrInvalidEmail          = errors.New("valid email address is required")
)

// ClientMeta captures audit context for issued sessions.
type ClientMeta struct {
	UserAgent string
	IP        *netip.Addr
}

// CurrentUser matches the OpenAPI CurrentUser schema.
type CurrentUser struct {
	ID              uuid.UUID          `json:"id"`
	FullName        string             `json:"full_name"`
	WhatsAppPhone   *string            `json:"whatsapp_phone,omitempty"`
	Email           *string            `json:"email,omitempty"`
	IsPlatformAdmin bool               `json:"is_platform_admin"`
	Roles           []domain.RoleScope `json:"roles"`
}

// TokenBundle matches the OpenAPI AuthTokenBundle schema.
type TokenBundle struct {
	AccessToken      string      `json:"access_token"`
	RefreshToken     string      `json:"refresh_token"`
	ExpiresInSeconds int         `json:"expires_in_seconds"`
	User             CurrentUser `json:"user"`
}

// RequestOTPResult summarizes an OTP request. DevOTP is populated only in development mode.
type RequestOTPResult struct {
	NormalizedPhone  string    `json:"whatsapp_phone"`
	ExpiresAt        time.Time `json:"expires_at"`
	RetryAfterSecond int       `json:"retry_after_seconds"`
	DevOTP           string    `json:"dev_otp,omitempty"`
}

// Service coordinates OTP flows, password authentication, session rotation, and RBAC lookup.
type Service struct {
	store      Store
	jwt        *JWTManager
	limiter    RateLimiter
	otpSecret  string
	refreshTTL time.Duration
	isDev      bool
	logger     *slog.Logger
}

// NewService constructs an auth Service with dependencies.
func NewService(
	store Store,
	jwt *JWTManager,
	limiter RateLimiter,
	otpSecret string,
	refreshTTL time.Duration,
	isDev bool,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:      store,
		jwt:        jwt,
		limiter:    limiter,
		otpSecret:  otpSecret,
		refreshTTL: refreshTTL,
		isDev:      isDev,
		logger:     logger,
	}
}

// RequestOTP generates and queues a 6-digit WhatsApp OTP with cooldown protection.
func (s *Service) RequestOTP(ctx context.Context, tenantID uuid.UUID, rawPhone string, purpose domain.OTPPurpose) (RequestOTPResult, error) {
	phone, err := NormalizeIndonesianPhone(rawPhone)
	if err != nil {
		return RequestOTPResult{}, err
	}

	now := time.Now().UTC()
	if s.limiter != nil {
		allowed, retryAfter := s.limiter.AllowCtx(ctx, phone, now)
		if !allowed {
			return RequestOTPResult{
				NormalizedPhone:  phone,
				RetryAfterSecond: int(retryAfter.Seconds()) + 1,
			}, ErrRateLimited
		}
	}

	code, err := GenerateOTPCode()
	if err != nil {
		return RequestOTPResult{}, err
	}

	expiresAt := now.Add(5 * time.Minute)
	challenge := OTPChallenge{
		ID:            uuid.Must(uuid.NewV7()),
		TenantID:      &tenantID,
		WhatsAppPhone: phone,
		Purpose:       purpose,
		CodeHash:      HashOTP(s.otpSecret, phone, purpose, code),
		MaxAttempts:   5,
		ExpiresAt:     expiresAt,
	}

	if err := s.store.CreateOTPChallenge(ctx, challenge, code); err != nil {
		return RequestOTPResult{}, err
	}

	res := RequestOTPResult{
		NormalizedPhone:  phone,
		ExpiresAt:        expiresAt,
		RetryAfterSecond: 60,
	}
	if s.isDev {
		res.DevOTP = code
		s.logger.Info("development whatsapp otp generated", "phone", phone, "purpose", purpose, "otp", code)
	}
	return res, nil
}

// VerifyOTP validates a WhatsApp OTP challenge, executes the purpose action, and issues JWT + refresh tokens.
func (s *Service) VerifyOTP(
	ctx context.Context,
	tenantID uuid.UUID,
	rawPhone string,
	purpose domain.OTPPurpose,
	code, fullName, password string,
	meta ClientMeta,
) (*TokenBundle, error) {
	phone, err := NormalizeIndonesianPhone(rawPhone)
	if err != nil {
		return nil, err
	}

	ch, err := s.store.GetLatestActiveOTP(ctx, phone, purpose)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, ErrOTPInvalidOrExpired
	}

	if ch.AttemptCount >= ch.MaxAttempts {
		return nil, ErrOTPMaxAttempts
	}

	if !VerifyOTPHash(s.otpSecret, phone, purpose, strings.TrimSpace(code), ch.CodeHash) {
		attempts, maxAttempts, incErr := s.store.IncrementOTPAttempt(ctx, ch.ID)
		if incErr == nil && attempts >= maxAttempts {
			return nil, ErrOTPMaxAttempts
		}
		return nil, ErrOTPInvalidOrExpired
	}

	user, err := s.applyVerifiedOTPPurpose(ctx, phone, purpose, fullName, password)
	if err != nil {
		return nil, err
	}

	if err := s.store.ConsumeOTPChallenge(ctx, ch.ID); err != nil {
		return nil, err
	}

	return s.issueTokenBundle(ctx, user, tenantID, meta)
}

func (s *Service) applyVerifiedOTPPurpose(
	ctx context.Context,
	phone string,
	purpose domain.OTPPurpose,
	fullName, password string,
) (*User, error) {
	existing, err := s.store.GetUserByWhatsAppPhone(ctx, phone)
	if err != nil {
		return nil, err
	}

	switch purpose {
	case domain.OTPPurposeRegister:
		if existing != nil {
			return nil, ErrPhoneAlreadyExists
		}
		trimmedName := strings.TrimSpace(fullName)
		if trimmedName == "" {
			return nil, ErrFullNameRequired
		}
		if len(password) < 6 {
			return nil, ErrPasswordTooShort
		}
		pwHash, err := HashPassword(password)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		return s.store.CreateUser(ctx, CreateUserInput{
			ID:              uuid.Must(uuid.NewV7()),
			WhatsAppPhone:   &phone,
			PasswordHash:    pwHash,
			FullName:        trimmedName,
			PhoneVerifiedAt: &now,
		})

	case domain.OTPPurposeLogin, domain.OTPPurposeVerifyPhone:
		if existing == nil {
			return nil, ErrUserNotFound
		}
		if err := s.store.MarkUserPhoneVerified(ctx, existing.ID); err != nil {
			return nil, err
		}
		return existing, nil

	case domain.OTPPurposeResetPin:
		if existing == nil {
			return nil, ErrUserNotFound
		}
		if len(password) < 6 {
			return nil, ErrPasswordTooShort
		}
		pwHash, err := HashPassword(password)
		if err != nil {
			return nil, err
		}
		if err := s.store.UpdateUserPassword(ctx, existing.ID, pwHash); err != nil {
			return nil, err
		}
		if err := s.store.RevokeAllUserSessions(ctx, existing.ID); err != nil {
			return nil, err
		}
		return existing, nil

	default:
		return nil, ErrOTPInvalidOrExpired
	}
}

// ResetPasswordViaOTP verifies a reset_pin OTP code, replaces the user's password, revokes prior sessions, and returns fresh tokens.
func (s *Service) ResetPasswordViaOTP(
	ctx context.Context,
	tenantID uuid.UUID,
	rawPhone, code, newPassword string,
	meta ClientMeta,
) (*TokenBundle, error) {
	return s.VerifyOTP(ctx, tenantID, rawPhone, domain.OTPPurposeResetPin, code, "", newPassword, meta)
}

// Login authenticates an applicant (WhatsApp phone) or staff member (Email) with password.
func (s *Service) Login(
	ctx context.Context,
	tenantID uuid.UUID,
	identifier, password string,
	meta ClientMeta,
) (*TokenBundle, error) {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	var user *User
	var err error
	if strings.Contains(trimmed, "@") {
		user, err = s.store.GetUserByEmail(ctx, trimmed)
	} else {
		phone, normErr := NormalizeIndonesianPhone(trimmed)
		if normErr != nil {
			return nil, ErrInvalidCredentials
		}
		user, err = s.store.GetUserByWhatsAppPhone(ctx, phone)
	}
	if err != nil {
		return nil, err
	}
	if user == nil || !CheckPassword(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}

	return s.issueTokenBundle(ctx, user, tenantID, meta)
}

// LoginPlatformAdmin authenticates a SaaS platform superadmin without requiring tenant resolution.
func (s *Service) LoginPlatformAdmin(
	ctx context.Context,
	email, password string,
	meta ClientMeta,
) (*TokenBundle, error) {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" || !strings.Contains(trimmed, "@") || password == "" {
		return nil, ErrInvalidCredentials
	}
	user, err := s.store.GetUserByEmail(ctx, trimmed)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive || !CheckPassword(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if !user.IsPlatformAdmin {
		return nil, ErrPlatformAdminRequired
	}
	return s.issueTokenBundle(ctx, user, uuid.Nil, meta)
}

// ProvisionTenantAdmin creates or binds an initial tenant_admin user for a newly onboarded tenant.
func (s *Service) ProvisionTenantAdmin(
	ctx context.Context,
	tenantID uuid.UUID,
	email, fullName, rawPassword string,
) (*User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	if cleanEmail == "" || !strings.Contains(cleanEmail, "@") {
		return nil, ErrInvalidEmail
	}
	cleanName := strings.TrimSpace(fullName)
	if cleanName == "" {
		return nil, ErrFullNameRequired
	}
	if len(rawPassword) < 6 {
		return nil, ErrPasswordTooShort
	}
	pwHash, err := HashPassword(rawPassword)
	if err != nil {
		return nil, err
	}
	return s.store.ProvisionTenantAdmin(ctx, tenantID, cleanEmail, cleanName, pwHash)
}

// Refresh validates and rotates an active refresh token into a fresh access and refresh token pair.
func (s *Service) Refresh(
	ctx context.Context,
	tenantID uuid.UUID,
	rawRefreshToken string,
	meta ClientMeta,
) (*TokenBundle, error) {
	trimmed := strings.TrimSpace(rawRefreshToken)
	if trimmed == "" {
		return nil, ErrSessionInvalid
	}

	oldHash := HashToken(trimmed)
	nextRaw, nextHash, err := GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var tenantPtr *uuid.UUID
	if tenantID != uuid.Nil {
		tenantPtr = &tenantID
	}
	nextSession := SessionRecord{
		ID:               uuid.Must(uuid.NewV7()),
		TenantID:         tenantPtr,
		RefreshTokenHash: nextHash,
		ExpiresAt:        now.Add(s.refreshTTL),
	}

	rotatedFrom, err := s.store.RotateSession(ctx, oldHash, nextSession, meta.UserAgent, meta.IP)
	if err != nil {
		return nil, err
	}
	if rotatedFrom == nil {
		return nil, ErrSessionInvalid
	}

	user, err := s.store.GetUserByID(ctx, rotatedFrom.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive {
		return nil, ErrSessionInvalid
	}

	return s.buildTokenBundle(ctx, user, tenantID, nextRaw)
}

// Logout revokes the refresh session matching rawRefreshToken if provided.
func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	trimmed := strings.TrimSpace(rawRefreshToken)
	if trimmed == "" {
		return nil
	}
	return s.store.RevokeSessionByTokenHash(ctx, HashToken(trimmed))
}

// GetMe loads the current active user profile and tenant-scoped RBAC roles.
func (s *Service) GetMe(ctx context.Context, userID, tenantID uuid.UUID) (*CurrentUser, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive {
		return nil, ErrUserNotFound
	}

	profile, err := s.buildCurrentUser(ctx, user, tenantID)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *Service) issueTokenBundle(
	ctx context.Context,
	user *User,
	tenantID uuid.UUID,
	meta ClientMeta,
) (*TokenBundle, error) {
	if !user.IsActive {
		return nil, ErrInvalidCredentials
	}

	rawRefresh, refreshHash, err := GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}

	var tenantPtr *uuid.UUID
	if tenantID != uuid.Nil {
		tenantPtr = &tenantID
	}
	session := SessionRecord{
		ID:               uuid.Must(uuid.NewV7()),
		UserID:           user.ID,
		TenantID:         tenantPtr,
		RefreshTokenHash: refreshHash,
		ExpiresAt:        time.Now().UTC().Add(s.refreshTTL),
	}
	if err := s.store.CreateSession(ctx, session, meta.UserAgent, meta.IP); err != nil {
		return nil, err
	}

	return s.buildTokenBundle(ctx, user, tenantID, rawRefresh)
}

func (s *Service) buildTokenBundle(
	ctx context.Context,
	user *User,
	tenantID uuid.UUID,
	rawRefresh string,
) (*TokenBundle, error) {
	profile, err := s.buildCurrentUser(ctx, user, tenantID)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.jwt.Issue(user.ID, tenantID, user.IsPlatformAdmin, profile.Roles)
	if err != nil {
		return nil, err
	}

	return &TokenBundle{
		AccessToken:      accessToken,
		RefreshToken:     rawRefresh,
		ExpiresInSeconds: s.jwt.TTLSeconds(),
		User:             profile,
	}, nil
}

func (s *Service) buildCurrentUser(ctx context.Context, user *User, tenantID uuid.UUID) (CurrentUser, error) {
	roles := []domain.RoleScope{}
	if tenantID != uuid.Nil {
		loaded, err := s.store.ListUserRolesInTenant(ctx, user.ID, tenantID)
		if err != nil {
			return CurrentUser{}, err
		}
		if loaded != nil {
			roles = loaded
		}
	}

	return CurrentUser{
		ID:              user.ID,
		FullName:        user.FullName,
		WhatsAppPhone:   user.WhatsAppPhone,
		Email:           user.Email,
		IsPlatformAdmin: user.IsPlatformAdmin,
		Roles:           roles,
	}, nil
}
