package auth

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

// DemoStore provides an in-memory auth store pre-seeded with sample accounts for local dev and unit tests.
type DemoStore struct {
	mu         sync.RWMutex
	users      map[uuid.UUID]*User
	roles      map[uuid.UUID][]domain.RoleScope
	challenges map[uuid.UUID]*OTPChallenge
	sessions   map[string]*SessionRecord
}

// SeedAccount bundles a pre-configured demo user and its multi-tenant RBAC role scopes.
type SeedAccount struct {
	User  User
	Roles []domain.RoleScope
}

// DemoSeedAccounts returns canonical demo accounts shared by the in-memory store and PostgreSQL seeder.
func DemoSeedAccounts() []SeedAccount {
	pwHash, _ := HashPassword("DemoPass123!")
	now := time.Now().UTC()

	spmbTenantID := uuid.MustParse("01960000-0000-7000-8000-000000000001")
	pmbmTenantID := uuid.MustParse("01960000-0000-7000-8000-000000000002")
	mandiriTenantID := uuid.MustParse("01960000-0000-7000-8000-000000000003")

	strPtr := func(s string) *string { return &s }

	return []SeedAccount{
		{
			User: User{
				ID:              uuid.MustParse("01960000-0000-7000-8001-000000000001"),
				WhatsAppPhone:   strPtr("+6281234567890"),
				Email:           strPtr("wali@demo.id"),
				PasswordHash:    pwHash,
				FullName:        "H. Ahmad Fauzan (Wali)",
				IsActive:        true,
				PhoneVerifiedAt: &now,
			},
		},
		{
			User: User{
				ID:           uuid.MustParse("01960000-0000-7000-8001-000000000002"),
				Email:        strPtr("verifikator@smaharapan.sch.id"),
				PasswordHash: pwHash,
				FullName:     "Siti Verifikator SPMB",
				IsActive:     true,
			},
			Roles: []domain.RoleScope{
				{TenantID: spmbTenantID, Role: domain.RoleVerifier},
			},
		},
		{
			User: User{
				ID:           uuid.MustParse("01960000-0000-7000-8001-000000000003"),
				Email:        strPtr("admin@alhamid.sch.id"),
				PasswordHash: pwHash,
				FullName:     "Admin Madrasah Al-Hamid",
				IsActive:     true,
			},
			Roles: []domain.RoleScope{
				{TenantID: pmbmTenantID, Role: domain.RoleTenantAdmin},
				{TenantID: spmbTenantID, Role: domain.RoleTenantAdmin},
				{TenantID: mandiriTenantID, Role: domain.RoleTenantAdmin},
			},
		},
		{
			User: User{
				ID:           uuid.MustParse("01960000-0000-7000-8001-000000000004"),
				Email:        strPtr("bendahara@alhamid.sch.id"),
				PasswordHash: pwHash,
				FullName:     "Hj. Maryam (Bendahara)",
				IsActive:     true,
			},
			Roles: []domain.RoleScope{
				{TenantID: pmbmTenantID, Role: domain.RoleTreasurer},
				{TenantID: spmbTenantID, Role: domain.RoleTreasurer},
				{TenantID: mandiriTenantID, Role: domain.RoleTreasurer},
			},
		},
		{
			User: User{
				ID:           uuid.MustParse("01960000-0000-7000-8001-000000000005"),
				Email:        strPtr("penguji@smaharapan.sch.id"),
				PasswordHash: pwHash,
				FullName:     "Ust. Hasan (Penguji Seleksi)",
				IsActive:     true,
			},
			Roles: []domain.RoleScope{
				{TenantID: spmbTenantID, Role: domain.RoleExaminer},
				{TenantID: pmbmTenantID, Role: domain.RoleExaminer},
				{TenantID: mandiriTenantID, Role: domain.RoleExaminer},
			},
		},
		{
			User: User{
				ID:           uuid.MustParse("01960000-0000-7000-8001-000000000006"),
				Email:        strPtr("kepsek@smaharapan.sch.id"),
				PasswordHash: pwHash,
				FullName:     "Drs. Hendra Wijaya (Kepala Sekolah)",
				IsActive:     true,
			},
			Roles: []domain.RoleScope{
				{TenantID: spmbTenantID, Role: domain.RolePrincipal},
				{TenantID: pmbmTenantID, Role: domain.RolePrincipal},
				{TenantID: mandiriTenantID, Role: domain.RolePrincipal},
			},
		},
		{
			User: User{
				ID:              uuid.MustParse("01960000-0000-7000-8001-000000000007"),
				Email:           strPtr("platform@spmb-platform.id"),
				PasswordHash:    pwHash,
				FullName:        "Platform Superadmin SaaS",
				IsPlatformAdmin: true,
				IsActive:        true,
			},
		},
	}
}

// NewDemoStore creates an initialized in-memory store with demo accounts for SPMB/PMBM/Mandiri.
func NewDemoStore() *DemoStore {
	s := &DemoStore{
		users:      make(map[uuid.UUID]*User),
		roles:      make(map[uuid.UUID][]domain.RoleScope),
		challenges: make(map[uuid.UUID]*OTPChallenge),
		sessions:   make(map[string]*SessionRecord),
	}

	for _, acc := range DemoSeedAccounts() {
		u := acc.User
		s.users[u.ID] = &u
		if len(acc.Roles) > 0 {
			s.roles[u.ID] = append([]domain.RoleScope(nil), acc.Roles...)
		}
	}

	return s
}

func (s *DemoStore) CreateOTPChallenge(_ context.Context, challenge OTPChallenge, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyVal := challenge
	s.challenges[challenge.ID] = &copyVal
	return nil
}

func (s *DemoStore) GetLatestActiveOTP(_ context.Context, phone string, purpose domain.OTPPurpose) (*OTPChallenge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now().UTC()
	var latest *OTPChallenge
	for _, ch := range s.challenges {
		if ch.WhatsAppPhone == phone && ch.Purpose == purpose && ch.ConsumedAt == nil && ch.ExpiresAt.After(now) {
			if latest == nil || ch.ExpiresAt.After(latest.ExpiresAt) {
				copyVal := *ch
				latest = &copyVal
			}
		}
	}
	return latest, nil
}

func (s *DemoStore) IncrementOTPAttempt(_ context.Context, challengeID uuid.UUID) (int16, int16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.challenges[challengeID]
	if !ok {
		return 0, 0, nil
	}
	ch.AttemptCount++
	return ch.AttemptCount, ch.MaxAttempts, nil
}

func (s *DemoStore) ConsumeOTPChallenge(_ context.Context, challengeID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.challenges[challengeID]; ok {
		now := time.Now().UTC()
		ch.ConsumedAt = &now
	}
	return nil
}

func (s *DemoStore) GetUserByID(_ context.Context, userID uuid.UUID) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[userID]
	if !ok || !u.IsActive {
		return nil, nil
	}
	copyVal := *u
	return &copyVal, nil
}

func (s *DemoStore) GetUserByWhatsAppPhone(_ context.Context, phone string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.IsActive && u.WhatsAppPhone != nil && *u.WhatsAppPhone == phone {
			copyVal := *u
			return &copyVal, nil
		}
	}
	return nil, nil
}

func (s *DemoStore) GetUserByEmail(_ context.Context, email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.IsActive && u.Email != nil && strings.EqualFold(*u.Email, email) {
			copyVal := *u
			return &copyVal, nil
		}
	}
	return nil, nil
}

func (s *DemoStore) CreateUser(_ context.Context, input CreateUserInput) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := &User{
		ID:              input.ID,
		WhatsAppPhone:   input.WhatsAppPhone,
		Email:           input.Email,
		PasswordHash:    input.PasswordHash,
		FullName:        input.FullName,
		IsActive:        true,
		PhoneVerifiedAt: input.PhoneVerifiedAt,
	}
	s.users[u.ID] = u
	copyVal := *u
	return &copyVal, nil
}

func (s *DemoStore) ProvisionTenantAdmin(_ context.Context, tenantID uuid.UUID, email, fullName, passwordHash string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	var target *User
	for _, existing := range s.users {
		if existing.Email != nil && strings.EqualFold(*existing.Email, cleanEmail) {
			target = existing
			target.IsActive = true
			if passwordHash != "" {
				target.PasswordHash = passwordHash
			}
			break
		}
	}
	if target == nil {
		target = &User{
			ID:           uuid.Must(uuid.NewV7()),
			Email:        &cleanEmail,
			PasswordHash: passwordHash,
			FullName:     strings.TrimSpace(fullName),
			IsActive:     true,
		}
		s.users[target.ID] = target
	}

	hasRole := false
	for _, r := range s.roles[target.ID] {
		if r.TenantID == tenantID && r.UnitID == nil && r.Role == domain.RoleTenantAdmin {
			hasRole = true
			break
		}
	}
	if !hasRole {
		s.roles[target.ID] = append(s.roles[target.ID], domain.RoleScope{
			TenantID: tenantID,
			Role:     domain.RoleTenantAdmin,
		})
	}

	copyVal := *target
	return &copyVal, nil
}

func (s *DemoStore) MarkUserPhoneVerified(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.users[userID]; ok && u.PhoneVerifiedAt == nil {
		now := time.Now().UTC()
		u.PhoneVerifiedAt = &now
	}
	return nil
}

func (s *DemoStore) UpdateUserPassword(_ context.Context, userID uuid.UUID, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.users[userID]; ok {
		u.PasswordHash = passwordHash
	}
	return nil
}

func (s *DemoStore) ListUserRolesInTenant(_ context.Context, userID, tenantID uuid.UUID) ([]domain.RoleScope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.RoleScope
	for _, r := range s.roles[userID] {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *DemoStore) CreateSession(_ context.Context, session SessionRecord, _ string, _ *netip.Addr) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyVal := session
	s.sessions[session.RefreshTokenHash] = &copyVal
	return nil
}

func (s *DemoStore) RotateSession(
	_ context.Context,
	oldTokenHash string,
	next SessionRecord,
	_ string,
	_ *netip.Addr,
) (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.sessions[oldTokenHash]
	now := time.Now().UTC()
	if !ok || existing.RevokedAt != nil || !existing.ExpiresAt.After(now) {
		return nil, nil
	}
	if existing.TenantID != nil && next.TenantID != nil && *existing.TenantID != *next.TenantID {
		return nil, nil
	}

	existing.RevokedAt = &now
	copyNext := next
	copyNext.UserID = existing.UserID
	s.sessions[next.RefreshTokenHash] = &copyNext
	copyOld := *existing
	return &copyOld, nil
}

func (s *DemoStore) RevokeSessionByTokenHash(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[tokenHash]; ok && existing.RevokedAt == nil {
		now := time.Now().UTC()
		existing.RevokedAt = &now
	}
	return nil
}

func (s *DemoStore) RevokeAllUserSessions(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for _, existing := range s.sessions {
		if existing.UserID == userID && existing.RevokedAt == nil {
			existing.RevokedAt = &now
		}
	}
	return nil
}
