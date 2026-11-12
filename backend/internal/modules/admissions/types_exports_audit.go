package admissions

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	ExportFormatDapodikXLSX = "dapodik_xlsx"
	ExportFormatEMISXLSX    = "emis_xlsx"
	ExportFormatCustomCSV   = "custom_csv"

	ExportStatusPending    = "pending"
	ExportStatusProcessing = "processing"
	ExportStatusCompleted  = "completed"
	ExportStatusFailed     = "failed"
)

var (
	ErrExportJobNotFound = errors.New("export job not found")
	ErrExportNotReady    = errors.New("export file is not ready for download")
)

// ExportJob represents an asynchronous or demo-completed national export job (Dapodik, EMIS, CSV).
type ExportJob struct {
	ID                uuid.UUID  `json:"id"`
	UnitID            uuid.UUID  `json:"unit_id"`
	UnitName          string     `json:"unit_name,omitempty"`
	PeriodID          uuid.UUID  `json:"period_id"`
	PeriodName        string     `json:"period_name,omitempty"`
	RequestedByUserID uuid.UUID  `json:"requested_by_user_id"`
	Format            string     `json:"format"`
	Status            string     `json:"status"`
	ResultStorageKey  *string    `json:"result_storage_key,omitempty"`
	Filename          *string    `json:"filename,omitempty"`
	DownloadURL       *string    `json:"download_url,omitempty"`
	RowCount          *int       `json:"row_count,omitempty"`
	ErrorMessage      *string    `json:"error_message,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
}

// CreateExportJobInput validates unit, period, and export format selection from admin staff.
type CreateExportJobInput struct {
	UnitID   uuid.UUID `json:"unit_id"`
	PeriodID uuid.UUID `json:"period_id"`
	Format   string    `json:"format"`
}

// DemoExportFile caches generated export artifact bytes for direct download when S3/MinIO is absent.
type DemoExportFile struct {
	Filename string
	MimeType string
	Content  []byte
}

// TenantAuditLogFilter constrains admin audit log queries by application, action type, and RBAC units.
type TenantAuditLogFilter struct {
	ApplicationID  *uuid.UUID
	AllowedUnitIDs []uuid.UUID
	ActionType     *string
	Limit          int
}

// TenantAuditLogItem exposes an immutable audit entry with optional application/unit context for admin tables.
type TenantAuditLogItem struct {
	ID             uuid.UUID  `json:"id"`
	ApplicationID  *uuid.UUID `json:"application_id,omitempty"`
	RegistrationNo *string    `json:"registration_no,omitempty"`
	StudentName    *string    `json:"student_name,omitempty"`
	UnitID         *uuid.UUID `json:"unit_id,omitempty"`
	ActorUserID    *uuid.UUID `json:"actor_user_id,omitempty"`
	ActorName      *string    `json:"actor_name,omitempty"`
	ActorRole      string     `json:"actor_role"`
	ActionType     string     `json:"action_type"`
	EntityType     string     `json:"entity_type"`
	EntityID       uuid.UUID  `json:"entity_id"`
	FromStatus     *string    `json:"from_status,omitempty"`
	ToStatus       *string    `json:"to_status,omitempty"`
	Reason         *string    `json:"reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

