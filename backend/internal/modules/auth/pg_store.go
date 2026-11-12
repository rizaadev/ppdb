package auth

import (
	"context"
	"errors"
	"net/netip"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/integrations"
	"github.com/pbdb/platform/internal/worker"
	"github.com/riverqueue/river"
)

// PGStore persists authentication, OTP challenges, sessions, and RBAC roles in PostgreSQL.
type PGStore struct {
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
}

// NewPGStore creates a PostgreSQL-backed auth store with optional transactional River job enqueueing.
func NewPGStore(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *PGStore {
	return &PGStore{pool: pool, riverClient: riverClient}
}

func (s *PGStore) CreateOTPChallenge(ctx context.Context, challenge OTPChallenge, plainCodeForDispatch string) error {
	tmpl := integrations.TemplateOTP
	if challenge.Purpose == domain.OTPPurposeResetPin {
		tmpl = integrations.TemplatePasswordResetOTP
	}

	runTx := func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO otp_challenges (id, tenant_id, whatsapp_phone, purpose, code_hash, max_attempts, expires_at)
			VALUES ($1, $2, $3, $4::otp_purpose, $5, $6, $7)
		`, challenge.ID, challenge.TenantID, challenge.WhatsAppPhone, string(challenge.Purpose),
			challenge.CodeHash, challenge.MaxAttempts, challenge.ExpiresAt)
		if err != nil {
			return err
		}

		if s.riverClient != nil && challenge.TenantID != nil {
			notifID := uuid.Must(uuid.NewV7())
			_, err = tx.Exec(ctx, `
				INSERT INTO notification_logs (id, tenant_id, channel, recipient, template_code, status)
				VALUES ($1, $2, 'whatsapp', $3, $4, 'queued')
			`, notifID, *challenge.TenantID, challenge.WhatsAppPhone, tmpl)
			if err != nil {
				return err
			}

			_, err = s.riverClient.InsertTx(ctx, tx, worker.SendWhatsAppArgs{
				TenantID:        *challenge.TenantID,
				NotificationLog: notifID,
				RecipientPhone:  challenge.WhatsAppPhone,
				TemplateCode:    tmpl,
				Variables:       []string{plainCodeForDispatch},
			}, nil)
			if err != nil {
				return err
			}
		}
		return nil
	}

	if challenge.TenantID != nil {
		return database.WithTenantTx(ctx, s.pool, *challenge.TenantID, runTx)
	}
	return database.WithBypassRLSTx(ctx, s.pool, runTx)
}

func (s *PGStore) GetLatestActiveOTP(ctx context.Context, phone string, purpose domain.OTPPurpose) (*OTPChallenge, error) {
	var ch OTPChallenge
	var purposeStr string

	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, whatsapp_phone, purpose::text, code_hash, attempt_count, max_attempts, expires_at, consumed_at
		FROM otp_challenges
		WHERE whatsapp_phone = $1 AND purpose = $2::otp_purpose AND consumed_at IS NULL AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
	`, phone, string(purpose)).Scan(
		&ch.ID, &ch.TenantID, &ch.WhatsAppPhone, &purposeStr, &ch.CodeHash,
		&ch.AttemptCount, &ch.MaxAttempts, &ch.ExpiresAt, &ch.ConsumedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ch.Purpose = domain.OTPPurpose(purposeStr)
	return &ch, nil
}

func (s *PGStore) IncrementOTPAttempt(ctx context.Context, challengeID uuid.UUID) (int16, int16, error) {
	var attempts, maxAttempts int16
	err := s.pool.QueryRow(ctx, `
		UPDATE otp_challenges
		SET attempt_count = attempt_count + 1
		WHERE id = $1
		RETURNING attempt_count, max_attempts
	`, challengeID).Scan(&attempts, &maxAttempts)
	return attempts, maxAttempts, err
}

func (s *PGStore) ConsumeOTPChallenge(ctx context.Context, challengeID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE otp_challenges SET consumed_at = NOW() WHERE id = $1`, challengeID)
	return err
}

func (s *PGStore) GetUserByID(ctx context.Context, userID uuid.UUID) (*User, error) {
	return s.scanUser(ctx, `
		SELECT id, whatsapp_phone, email, password_hash, full_name, is_platform_admin, is_active, phone_verified_at
		FROM users WHERE id = $1 AND is_active = TRUE
	`, userID)
}

func (s *PGStore) GetUserByWhatsAppPhone(ctx context.Context, phone string) (*User, error) {
	return s.scanUser(ctx, `
		SELECT id, whatsapp_phone, email, password_hash, full_name, is_platform_admin, is_active, phone_verified_at
		FROM users WHERE whatsapp_phone = $1 AND is_active = TRUE
	`, phone)
}

func (s *PGStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.scanUser(ctx, `
		SELECT id, whatsapp_phone, email, password_hash, full_name, is_platform_admin, is_active, phone_verified_at
		FROM users WHERE LOWER(email) = LOWER($1) AND is_active = TRUE
	`, strings.TrimSpace(email))
}

func (s *PGStore) scanUser(ctx context.Context, query string, arg any) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, query, arg).Scan(
		&u.ID, &u.WhatsAppPhone, &u.Email, &u.PasswordHash, &u.FullName,
		&u.IsPlatformAdmin, &u.IsActive, &u.PhoneVerifiedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *PGStore) CreateUser(ctx context.Context, input CreateUserInput) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (id, whatsapp_phone, email, password_hash, full_name, phone_verified_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, whatsapp_phone, email, password_hash, full_name, is_platform_admin, is_active, phone_verified_at
	`, input.ID, input.WhatsAppPhone, input.Email, input.PasswordHash, input.FullName, input.PhoneVerifiedAt).Scan(
		&u.ID, &u.WhatsAppPhone, &u.Email, &u.PasswordHash, &u.FullName,
		&u.IsPlatformAdmin, &u.IsActive, &u.PhoneVerifiedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *PGStore) ProvisionTenantAdmin(ctx context.Context, tenantID uuid.UUID, email, fullName, passwordHash string) (*User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	var u User

	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, whatsapp_phone, email, password_hash, full_name, is_platform_admin, is_active, phone_verified_at
			FROM users
			WHERE LOWER(email) = LOWER($1)
		`, cleanEmail).Scan(
			&u.ID, &u.WhatsAppPhone, &u.Email, &u.PasswordHash, &u.FullName,
			&u.IsPlatformAdmin, &u.IsActive, &u.PhoneVerifiedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			newID := uuid.Must(uuid.NewV7())
			err = tx.QueryRow(ctx, `
				INSERT INTO users (id, email, password_hash, full_name, is_active)
				VALUES ($1, $2, $3, $4, TRUE)
				RETURNING id, whatsapp_phone, email, password_hash, full_name, is_platform_admin, is_active, phone_verified_at
			`, newID, cleanEmail, passwordHash, strings.TrimSpace(fullName)).Scan(
				&u.ID, &u.WhatsAppPhone, &u.Email, &u.PasswordHash, &u.FullName,
				&u.IsPlatformAdmin, &u.IsActive, &u.PhoneVerifiedAt,
			)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if !u.IsActive {
			if _, err := tx.Exec(ctx, `UPDATE users SET is_active = TRUE WHERE id = $1`, u.ID); err != nil {
				return err
			}
			u.IsActive = true
		}

		var roleID uuid.UUID
		roleErr := tx.QueryRow(ctx, `
			SELECT id
			FROM user_tenant_roles
			WHERE user_id = $1 AND tenant_id = $2 AND unit_id IS NULL AND role = 'tenant_admin'::tenant_role
		`, u.ID, tenantID).Scan(&roleID)
		if errors.Is(roleErr, pgx.ErrNoRows) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO user_tenant_roles (id, user_id, tenant_id, unit_id, role, is_active)
				VALUES ($1, $2, $3, NULL, 'tenant_admin'::tenant_role, TRUE)
			`, uuid.Must(uuid.NewV7()), u.ID, tenantID); err != nil {
				return err
			}
		} else if roleErr != nil {
			return roleErr
		} else {
			if _, err := tx.Exec(ctx, `UPDATE user_tenant_roles SET is_active = TRUE WHERE id = $1`, roleID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *PGStore) MarkUserPhoneVerified(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users
		SET phone_verified_at = COALESCE(phone_verified_at, NOW()), last_login_at = NOW()
		WHERE id = $1
	`, userID)
	return err
}

func (s *PGStore) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, passwordHash)
	return err
}

func (s *PGStore) ListUserRolesInTenant(ctx context.Context, userID, tenantID uuid.UUID) ([]domain.RoleScope, error) {
	var roles []domain.RoleScope
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT tenant_id, unit_id, role::text
			FROM user_tenant_roles
			WHERE user_id = $1 AND tenant_id = $2 AND is_active = TRUE
		`, userID, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var r domain.RoleScope
			var roleStr string
			if err := rows.Scan(&r.TenantID, &r.UnitID, &roleStr); err != nil {
				return err
			}
			r.Role = domain.TenantRole(roleStr)
			roles = append(roles, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return roles, nil
}

func (s *PGStore) CreateSession(ctx context.Context, session SessionRecord, userAgent string, ip *netip.Addr) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_sessions (id, user_id, tenant_id, refresh_token_hash, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7)
	`, session.ID, session.UserID, session.TenantID, session.RefreshTokenHash, userAgent, ip, session.ExpiresAt)
	return err
}

func (s *PGStore) RotateSession(
	ctx context.Context,
	oldTokenHash string,
	next SessionRecord,
	userAgent string,
	ip *netip.Addr,
) (*SessionRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existing SessionRecord
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, refresh_token_hash, expires_at, revoked_at
		FROM user_sessions
		WHERE refresh_token_hash = $1 AND revoked_at IS NULL AND expires_at > NOW()
		FOR UPDATE
	`, oldTokenHash).Scan(
		&existing.ID, &existing.UserID, &existing.TenantID,
		&existing.RefreshTokenHash, &existing.ExpiresAt, &existing.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if existing.TenantID != nil && next.TenantID != nil && *existing.TenantID != *next.TenantID {
		return nil, nil
	}

	if _, err := tx.Exec(ctx, `UPDATE user_sessions SET revoked_at = NOW() WHERE id = $1`, existing.ID); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_sessions (id, user_id, tenant_id, refresh_token_hash, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7)
	`, next.ID, existing.UserID, next.TenantID, next.RefreshTokenHash, userAgent, ip, next.ExpiresAt); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &existing, nil
}

func (s *PGStore) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE user_sessions SET revoked_at = NOW()
		WHERE refresh_token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	return err
}

func (s *PGStore) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE user_sessions SET revoked_at = NOW()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	return err
}
