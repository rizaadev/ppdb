package admissions

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

var (
	ErrPaymentNotFound     = errors.New("payment transaction not found")
	ErrPaymentAlreadyFinal = errors.New("payment transaction is already finalized")
	ErrNoFeeConfigured     = errors.New("track does not charge a fee for this payment purpose")
	ErrManualProofRequired = errors.New("manual transfer proof must be uploaded before marking paid")
	ErrPeriodNotFound      = errors.New("admission period or tracks not found")
	ErrAgreementRequired   = errors.New("re-enrollment agreement must be accepted")
	ErrScheduleNotFound    = errors.New("assessment schedule not found")
)

const (
	PaymentPurposeRegistration = "registration_fee"
	PaymentPurposeReEnrollment = "re_enrollment_fee"

	PaymentMethodQRIS           = "qris"
	PaymentMethodVA             = "va"
	PaymentMethodEWallet        = "ewallet"
	PaymentMethodManualTransfer = "manual_transfer"

	PaymentStatusPending  = "pending"
	PaymentStatusPaid     = "paid"
	PaymentStatusFailed   = "failed"
	PaymentStatusExpired  = "expired"
	PaymentStatusRefunded = "refunded"
)

// PaymentTransaction represents a billing invoice or verified payment record for registration or re-enrollment.
type PaymentTransaction struct {
	TenantID         uuid.UUID  `json:"-"`
	ID               uuid.UUID  `json:"id"`
	ApplicationID    uuid.UUID  `json:"application_id"`
	RegistrationNo   string     `json:"registration_no,omitempty"`
	StudentName      *string    `json:"student_name,omitempty"`
	UnitID           uuid.UUID  `json:"unit_id,omitempty"`
	UnitName         string     `json:"unit_name,omitempty"`
	TrackName        string     `json:"track_name,omitempty"`
	Purpose          string     `json:"purpose"`
	Method           string     `json:"method"`
	Provider         *string    `json:"provider,omitempty"`
	ExternalRef      *string    `json:"external_ref,omitempty"`
	AmountIDR        int64      `json:"amount_idr"`
	Status           string     `json:"status"`
	PaymentURL       *string    `json:"payment_url,omitempty"`
	ProofStorageKey  *string    `json:"proof_storage_key,omitempty"`
	VerifiedByUserID *uuid.UUID `json:"verified_by_user_id,omitempty"`
	VerificationNote *string    `json:"verification_note,omitempty"`
	PaidAt           *time.Time `json:"paid_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// CreatePaymentInput requests creation of an invoice for an application.
type CreatePaymentInput struct {
	Purpose string `json:"purpose"`
	Method  string `json:"method"`
}

// PresignProofInput requests a presigned upload URL for manual transfer receipt proof.
type PresignProofInput struct {
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

// AttachManualProofInput binds an uploaded storage key to a manual transfer transaction.
type AttachManualProofInput struct {
	ProofStorageKey string `json:"proof_storage_key"`
}

// VerifyManualPaymentInput records a treasurer approval or rejection decision.
type VerifyManualPaymentInput struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

// PaymentVerificationResult bundles the updated payment and application state returned to the treasurer.
type PaymentVerificationResult struct {
	Payment     *PaymentTransaction `json:"payment"`
	Application *ApplicationDetail  `json:"application"`
}

// AdminPaymentFilter narrows finance payment queries by RBAC unit scope and transaction attributes.
type AdminPaymentFilter struct {
	UnitID         *uuid.UUID
	AllowedUnitIDs []uuid.UUID
	Status         *string
	Method         *string
	Purpose        *string
	Limit          int
}

// PaymentWebhookInput represents normalized gateway callback fields verified via HMAC.
// PaymentWebhookInput represents normalized or Midtrans native gateway callback fields.
type PaymentWebhookInput struct {
	Provider          string         `json:"-"`
	ExternalRef       string         `json:"external_ref"`
	PaymentID         *uuid.UUID     `json:"payment_id,omitempty"`
	Status            string         `json:"status"`
	AmountIDR         *int64         `json:"amount_idr,omitempty"`
	OrderID           string         `json:"order_id,omitempty"`
	StatusCode        string         `json:"status_code,omitempty"`
	GrossAmount       string         `json:"gross_amount,omitempty"`
	SignatureKey      string         `json:"signature_key,omitempty"`
	TransactionStatus string         `json:"transaction_status,omitempty"`
	FraudStatus       string         `json:"fraud_status,omitempty"`
	RawPayload        map[string]any `json:"raw_payload,omitempty"`
}

// TenantPaymentConfigProvider resolves per-tenant payment credentials (with platform fallback handled by caller).
type TenantPaymentConfigProvider interface {
	GetTenantPaymentConfig(ctx context.Context, tenantID uuid.UUID) (*domain.TenantPaymentConfig, error)
}

// AssessmentSchedule represents an applicant's scheduled test/interview session.
type AssessmentSchedule struct {
	ID             uuid.UUID  `json:"id"`
	ApplicationID  uuid.UUID  `json:"application_id"`
	RegistrationNo string     `json:"registration_no,omitempty"`
	StudentName    *string    `json:"student_name,omitempty"`
	UnitID         uuid.UUID  `json:"unit_id,omitempty"`
	UnitName       string     `json:"unit_name,omitempty"`
	TrackName      string     `json:"track_name,omitempty"`
	SessionName    string     `json:"session_name"`
	RoomOrLink     *string    `json:"room_or_link,omitempty"`
	ScheduledAt    time.Time  `json:"scheduled_at"`
	ExaminerUserID *uuid.UUID `json:"examiner_user_id,omitempty"`
	ExaminerName   *string    `json:"examiner_name,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// AssessmentScheduleFilter narrows assessment schedule listings by applicant, unit, track, and RBAC units.
type AssessmentScheduleFilter struct {
	ApplicationID  *uuid.UUID
	UnitID         *uuid.UUID
	TrackID        *uuid.UUID
	AllowedUnitIDs []uuid.UUID
	Limit          int
}

// CreateAssessmentScheduleInput requests scheduling one or more verified applicants for assessment/interview.
type CreateAssessmentScheduleInput struct {
	ApplicationID  *uuid.UUID  `json:"application_id,omitempty"`
	ApplicationIDs []uuid.UUID `json:"application_ids,omitempty"`
	SessionName    string      `json:"session_name"`
	RoomOrLink     *string     `json:"room_or_link,omitempty"`
	ScheduledAt    time.Time   `json:"scheduled_at"`
	ExaminerUserID *uuid.UUID  `json:"examiner_user_id,omitempty"`
}

// ReEnrollInput captures applicant confirmation of acceptance and optional uniform/boarding answers.
type ReEnrollInput struct {
	ExpectedVersion   int            `json:"version"`
	AgreementAccepted bool           `json:"agreement_accepted"`
	ExtraAnswers      map[string]any `json:"extra_answers,omitempty"`
}

// AssessmentScore records one scored component on an applicant dossier.
type AssessmentScore struct {
	ID             uuid.UUID  `json:"id"`
	ApplicationID  uuid.UUID  `json:"application_id"`
	ComponentCode  string     `json:"component_code"`
	RawScore       float64    `json:"raw_score"`
	WeightPct      float64    `json:"weight_pct"`
	WeightedScore  float64    `json:"weighted_score"`
	Notes          *string    `json:"notes,omitempty"`
	ExaminerUserID *uuid.UUID `json:"examiner_user_id,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ScoreComponentInput captures an examiner score component submission.
type ScoreComponentInput struct {
	ComponentCode string  `json:"component_code"`
	RawScore      float64 `json:"raw_score"`
	WeightPct     float64 `json:"weight_pct"`
	Notes         string  `json:"notes"`
}

// UpsertScoresInput replaces or updates an application's score components.
type UpsertScoresInput struct {
	Scores []ScoreComponentInput `json:"scores"`
}

// UpsertScoresResult returns recalculated final_score and updated components.
type UpsertScoresResult struct {
	Application *ApplicationDetail `json:"application"`
	Scores      []AssessmentScore  `json:"scores"`
	FinalScore  float64            `json:"final_score"`
}

// RankedCandidate displays deterministic tie-breaker sorting and simulated/published outcome.
type RankedCandidate struct {
	ApplicationID     uuid.UUID                `json:"application_id"`
	RegistrationNo    string                   `json:"registration_no"`
	StudentName       *string                  `json:"student_name"`
	CurrentStatus     domain.ApplicationStatus `json:"current_status"`
	SimulatedDecision domain.ApplicationStatus `json:"simulated_decision"`
	RankPosition      int                      `json:"rank_position"`
	FinalScore        float64                  `json:"final_score"`
	DistanceMeters    *int                     `json:"distance_meters,omitempty"`
	BirthDate         *string                  `json:"birth_date,omitempty"`
	SubmittedAt       *time.Time               `json:"submitted_at,omitempty"`
	KKIssuedDate      *string                  `json:"kk_issued_date,omitempty"`
}

// TrackRankingSimulation summarizes projected quota allocation, waitlist slots, and remaining spillover capacity.
type TrackRankingSimulation struct {
	TrackID                uuid.UUID         `json:"track_id"`
	TrackName              string            `json:"track_name"`
	UnitID                 uuid.UUID         `json:"unit_id"`
	Quota                  int               `json:"quota"`
	WaitlistSlots          int               `json:"waitlist_slots"`
	SpilloverTargetTrackID *uuid.UUID        `json:"spillover_target_track_id,omitempty"`
	AcceptedCount          int               `json:"accepted_count"`
	WaitlistedCount        int               `json:"waitlisted_count"`
	RejectedCount          int               `json:"rejected_count"`
	RemainingQuota         int               `json:"remaining_quota"`
	Candidates             []RankedCandidate `json:"candidates"`
}

// PublishResultsResult summarizes period-wide selection decisions locked and announced to applicants.
type PublishResultsResult struct {
	PeriodID        uuid.UUID                `json:"period_id"`
	PublishedAt     time.Time                `json:"published_at"`
	TotalAccepted   int                      `json:"total_accepted"`
	TotalWaitlisted int                      `json:"total_waitlisted"`
	TotalRejected   int                      `json:"total_rejected"`
	Tracks          []TrackRankingSimulation `json:"tracks"`
}
