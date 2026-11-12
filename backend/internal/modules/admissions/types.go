package admissions

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

var (
	ErrApplicationNotFound    = errors.New("application not found")
	ErrTrackNotFound          = errors.New("unit, period, or admission track not found or closed")
	ErrApplicationLocked      = errors.New("application status does not allow this operation")
	ErrVersionConflict        = errors.New("application version conflict; please reload latest data")
	ErrRequirementNotFound    = errors.New("track document requirement not found")
	ErrUnsupportedMime        = errors.New("file mime type is not allowed for this requirement")
	ErrFileTooLarge           = errors.New("file size exceeds maximum allowed limit")
	ErrInvalidStorageKey      = errors.New("storage key does not match application scope")
	ErrDocumentNotFound       = errors.New("uploaded document not found")
	ErrMissingCoordinates     = errors.New("domicile coordinates are required for this track")
	ErrMissingAchievements    = errors.New("at least one achievement is required for this track")
	ErrPendingDocsRemain      = errors.New("all uploaded documents must be reviewed before completing verification")
	ErrReviewNoteRequired     = errors.New("review note is required when requesting revision")
	ErrVerificationQueueEmpty = errors.New("no submitted applications waiting in verification queue")
)

// ValidationError captures field-level validation failures for 422 HTTP responses.
type ValidationError struct {
	Message string
	Fields  map[string]string
}

func (e *ValidationError) Error() string { return e.Message }

// MissingDocumentsError reports required track requirements missing on submission.
type MissingDocumentsError struct {
	MissingDocCodes []string
}

func (e *MissingDocumentsError) Error() string {
	return fmt.Sprintf("required documents missing or rejected: %s", strings.Join(e.MissingDocCodes, ", "))
}

// ActorContext captures the authenticated actor and audit metadata performing an operation.
type ActorContext struct {
	UserID   uuid.UUID
	RoleName string
	IP       *netip.Addr
}

// TrackRequirement describes an uploadable requirement attached to an admission track.
type TrackRequirement struct {
	ID            uuid.UUID `json:"id"`
	TrackID       uuid.UUID `json:"track_id,omitempty"`
	DocCode       string    `json:"doc_code"`
	Label         string    `json:"label"`
	Description   *string   `json:"description,omitempty"`
	IsRequired    bool      `json:"is_required"`
	AcceptedMimes []string  `json:"accepted_mimes"`
	MaxSizeKB     int       `json:"max_size_kb"`
	SortOrder     int16     `json:"sort_order"`
}

// CatalogTrack joins active unit, period, track, and requirements metadata for admissions validation.
type CatalogTrack struct {
	TenantID             uuid.UUID
	UnitID               uuid.UUID
	UnitName             string
	UnitLat              *float64
	UnitLon              *float64
	PeriodID             uuid.UUID
	PeriodName           string
	AcademicYear         string
	TrackID              uuid.UUID
	TrackCode            string
	TrackName            string
	SelectionMode        string
	Quota                int
	SpilloverTargetTrack *uuid.UUID
	RegistrationFeeIDR   int64
	ReEnrollmentFeeIDR   int64
	MinAgeMonths         *int
	MaxAgeMonths         *int
	AgeReferenceDate     *string
	RequiresCoordinates  bool
	RequiresAchievements bool
	ScoringWeights       map[string]float64
	CustomFormSchema     []domain.CustomFormField
	Requirements         []TrackRequirement
}

// ApplicationSummary mirrors the OpenAPI ApplicationSummary schema plus unit/track display labels.
type ApplicationSummary struct {
	ID             uuid.UUID                `json:"id"`
	RegistrationNo string                   `json:"registration_no"`
	Status         domain.ApplicationStatus `json:"status"`
	UnitID         uuid.UUID                `json:"unit_id"`
	UnitName       string                   `json:"unit_name,omitempty"`
	PeriodID       uuid.UUID                `json:"period_id"`
	PeriodName     string                   `json:"period_name,omitempty"`
	AcademicYear   string                   `json:"academic_year,omitempty"`
	TrackID        uuid.UUID                `json:"track_id"`
	TrackName      string                   `json:"track_name,omitempty"`
	StudentName    *string                  `json:"student_name"`
	NISN           *string                  `json:"nisn,omitempty"`
	DistanceMeters *int                     `json:"distance_meters"`
	FinalScore     *string                  `json:"final_score"`
	RankPosition   *int                     `json:"rank_position"`
	Version        int                      `json:"version"`
	SubmittedAt        *time.Time               `json:"submitted_at"`
	VerifiedAt         *time.Time               `json:"verified_at,omitempty"`
	DecidedAt          *time.Time               `json:"decided_at"`
	ResultsPublishedAt *time.Time               `json:"results_published_at,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
}

// ApplicantProfile holds national Dapodik/EMIS compliant student biodata and geolocation.
type ApplicantProfile struct {
	FullName         string   `json:"full_name"`
	NIK              *string  `json:"nik"`
	KKNumber         *string  `json:"kk_number"`
	KKIssuedDate     *string  `json:"kk_issued_date"`
	NISN             *string  `json:"nisn"`
	BirthPlace       *string  `json:"birth_place"`
	BirthDate        *string  `json:"birth_date"`
	Gender           *string  `json:"gender"`
	Religion         *string  `json:"religion"`
	Citizenship      string   `json:"citizenship"`
	ChildOrder       *int16   `json:"child_order"`
	SiblingsCount    *int16   `json:"siblings_count"`
	SpecialNeedsCode *string  `json:"special_needs_code"`
	AddressStreet    *string  `json:"address_street"`
	RT               *string  `json:"rt"`
	RW               *string  `json:"rw"`
	Hamlet           *string  `json:"hamlet"`
	VillageCode      *string  `json:"village_code"`
	VillageName      *string  `json:"village_name"`
	DistrictCode     *string  `json:"district_code"`
	DistrictName     *string  `json:"district_name"`
	RegencyCode      *string  `json:"regency_code"`
	RegencyName      *string  `json:"regency_name"`
	ProvinceCode     *string  `json:"province_code"`
	ProvinceName     *string  `json:"province_name"`
	PostalCode       *string  `json:"postal_code"`
	Latitude         *float64 `json:"latitude"`
	Longitude        *float64 `json:"longitude"`
	PrevSchoolName   *string  `json:"prev_school_name"`
	PrevSchoolNPSN   *string  `json:"prev_school_npsn"`
	GraduationYear   *int16   `json:"graduation_year"`
	IjazahNumber     *string  `json:"ijazah_number"`
}

// Guardian captures parent or legal guardian information for an application.
type Guardian struct {
	ID               uuid.UUID `json:"id"`
	Relation         string    `json:"relation"`
	FullName         string    `json:"full_name"`
	NIK              *string   `json:"nik"`
	BirthYear        *int16    `json:"birth_year"`
	EducationLevel   *string   `json:"education_level"`
	Occupation       *string   `json:"occupation"`
	IncomeBracket    *string   `json:"income_bracket"`
	PhoneWhatsApp    *string   `json:"phone_whatsapp"`
	IsPrimaryContact bool      `json:"is_primary_contact"`
}

// Achievement represents an academic/non-academic certificate claimed on an application.
type Achievement struct {
	ID               uuid.UUID `json:"id"`
	Category         string    `json:"category"`
	Title            string    `json:"title"`
	Level            string    `json:"level"`
	RankAchieved     *string   `json:"rank_achieved"`
	AchievementYear  *int16    `json:"achievement_year"`
	Organizer        *string   `json:"organizer"`
	RawPoints        float64   `json:"raw_points"`
	VerifiedPoints   *float64  `json:"verified_points,omitempty"`
	VerifiedByUserID *uuid.UUID `json:"verified_by_user_id,omitempty"`
}

// Document represents a private uploaded requirement file and its verifier review decision.
type Document struct {
	ID               uuid.UUID                   `json:"id"`
	RequirementID    uuid.UUID                   `json:"requirement_id"`
	DocCode          string                      `json:"doc_code"`
	RequirementLabel string                      `json:"requirement_label"`
	IsRequired       bool                        `json:"is_required"`
	StorageKey       string                      `json:"storage_key"`
	OriginalFilename string                      `json:"original_filename"`
	MimeType         string                      `json:"mime_type"`
	SizeBytes        int64                       `json:"size_bytes"`
	Status           domain.DocumentReviewStatus `json:"status"`
	ReviewerUserID   *uuid.UUID                  `json:"reviewer_user_id,omitempty"`
	ReviewNote       *string                     `json:"review_note"`
	ReviewedAt       *time.Time                  `json:"reviewed_at"`
	Version          int                         `json:"version"`
}

// AuditEntry exposes immutable status and verification audit history rows for an application.
type AuditEntry struct {
	ID          uuid.UUID  `json:"id"`
	ActorUserID *uuid.UUID `json:"actor_user_id,omitempty"`
	ActorName   *string    `json:"actor_name,omitempty"`
	ActorRole   string     `json:"actor_role"`
	ActionType  string     `json:"action_type"`
	EntityType  string     `json:"entity_type"`
	EntityID    uuid.UUID  `json:"entity_id"`
	FromStatus  *string    `json:"from_status,omitempty"`
	ToStatus    *string    `json:"to_status,omitempty"`
	Reason      *string    `json:"reason,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ApplicationDetail is the full dossier payload returned to applicants and authorized staff.
type ApplicationDetail struct {
	Application          ApplicationSummary `json:"application"`
	GuardianUserID       uuid.UUID          `json:"guardian_user_id"`
	ClaimedByVerifierID  *uuid.UUID         `json:"claimed_by_verifier_id,omitempty"`
	ClaimedAt            *time.Time         `json:"claimed_at,omitempty"`
	UnitLatitude         *float64           `json:"unit_latitude,omitempty"`
	UnitLongitude        *float64           `json:"unit_longitude,omitempty"`
	RequiresCoordinates  bool                 `json:"requires_coordinates"`
	RequiresAchievements bool                 `json:"requires_achievements"`
	RegistrationFeeIDR   int64                `json:"registration_fee_idr"`
	ReEnrollmentFeeIDR   int64                `json:"re_enrollment_fee_idr"`
	ExtraAnswers         map[string]any       `json:"extra_answers"`
	Profile              *ApplicantProfile    `json:"profile"`
	Guardians            []Guardian           `json:"guardians"`
	Achievements         []Achievement        `json:"achievements"`
	Requirements         []TrackRequirement   `json:"requirements"`
	Documents            []Document           `json:"documents"`
	Payments             []PaymentTransaction `json:"payments"`
	Scores               []AssessmentScore    `json:"scores"`
	Schedules            []AssessmentSchedule `json:"schedules"`
	AuditLogs            []AuditEntry         `json:"audit_logs"`
	VerificationToken    string               `json:"verification_token,omitempty"`
	TrackScoringWeights  map[string]float64   `json:"track_scoring_weights,omitempty"`
	CustomFormSchema     []domain.CustomFormField `json:"custom_form_schema,omitempty"`
}

// CreateApplicationInput requests creation of a new child application in a unit/period/track.
type CreateApplicationInput struct {
	UnitID          uuid.UUID      `json:"unit_id"`
	PeriodID        uuid.UUID      `json:"period_id"`
	TrackID         uuid.UUID      `json:"track_id"`
	StudentFullName string         `json:"student_full_name"`
	ExtraAnswers    map[string]any `json:"extra_answers,omitempty"`
}

// UpdateProfileInput holds biodata, coordinates, and optional optimistic version/extra answers.
type UpdateProfileInput struct {
	ApplicantProfile
	ExpectedVersion *int           `json:"version,omitempty"`
	ExtraAnswers    map[string]any `json:"extra_answers,omitempty"`
}

// PresignUploadInput requests a short-lived upload URL for a track requirement.
type PresignUploadInput struct {
	RequirementID uuid.UUID `json:"requirement_id"`
	Filename      string    `json:"filename"`
	MimeType      string    `json:"mime_type"`
	SizeBytes     int64     `json:"size_bytes"`
}

// PresignUploadResult matches the OpenAPI PresignedUploadResponse schema.
type PresignUploadResult struct {
	UploadURL       string            `json:"upload_url"`
	StorageKey      string            `json:"storage_key"`
	RequiredHeaders map[string]string `json:"required_headers,omitempty"`
	ExpiresAt       time.Time         `json:"expires_at"`
}

// ConfirmDocumentInput records completed direct-to-S3/MinIO object upload metadata.
type ConfirmDocumentInput struct {
	RequirementID    uuid.UUID `json:"requirement_id"`
	StorageKey       string    `json:"storage_key"`
	OriginalFilename string    `json:"original_filename"`
	MimeType         string    `json:"mime_type"`
	SizeBytes        int64     `json:"size_bytes"`
}

// StaffListFilter constrains staff/verifier application queries, including RBAC unit restrictions.
type StaffListFilter struct {
	UnitID          *uuid.UUID
	AllowedUnitIDs  []uuid.UUID // Empty when user holds tenant-wide access
	PeriodID        *uuid.UUID
	TrackID         *uuid.UUID
	Status          *domain.ApplicationStatus
	Search          string
	CursorCreatedAt *time.Time
	CursorID        *uuid.UUID
	Limit           int
}

// AuditWrite captures a single audit trail insert executed atomically within store mutations.
type AuditWrite struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Application *uuid.UUID
	Actor       ActorContext
	ActionType  string
	EntityType  string
	EntityID    uuid.UUID
	FromStatus  *string
	ToStatus    *string
	PayloadDiff any
	Reason      *string
}

// NotificationWrite records a WhatsApp notification queued atomically on state transitions.
type NotificationWrite struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	ApplicationID uuid.UUID
	TemplateCode  string
	Variables     []string
}

