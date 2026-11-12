package domain

import "github.com/google/uuid"

// TenantRole mirrors the PostgreSQL tenant_role enum for RBAC authorization.
type TenantRole string

const (
	RoleTenantAdmin TenantRole = "tenant_admin"
	RoleUnitAdmin   TenantRole = "unit_admin"
	RoleVerifier    TenantRole = "verifier"
	RoleExaminer    TenantRole = "examiner"
	RoleTreasurer   TenantRole = "treasurer"
	RolePrincipal   TenantRole = "principal"
)

// OTPPurpose mirrors the PostgreSQL otp_purpose enum.
type OTPPurpose string

const (
	OTPPurposeRegister    OTPPurpose = "register"
	OTPPurposeLogin       OTPPurpose = "login"
	OTPPurposeResetPin    OTPPurpose = "reset_pin"
	OTPPurposeVerifyPhone OTPPurpose = "verify_phone"
)

// IsValidOTPPurpose validates whether raw matches a supported OTP purpose.
func IsValidOTPPurpose(raw string) bool {
	switch OTPPurpose(raw) {
	case OTPPurposeRegister, OTPPurposeLogin, OTPPurposeResetPin, OTPPurposeVerifyPhone:
		return true
	default:
		return false
	}
}

// RoleScope represents a single RBAC role assignment inside a tenant (and optionally a specific unit).
type RoleScope struct {
	TenantID uuid.UUID  `json:"tenant_id"`
	UnitID   *uuid.UUID `json:"unit_id,omitempty"`
	Role     TenantRole `json:"role"`
}

