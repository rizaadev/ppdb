package admissions

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

// Store abstracts multi-tenant persistence for applicant workflows and staff verification.
type Store interface {
	GetCatalogTrack(ctx context.Context, tenantID, unitID, periodID, trackID uuid.UUID) (*CatalogTrack, error)

	ListApplicationsByGuardian(ctx context.Context, tenantID, guardianUserID uuid.UUID) ([]ApplicationSummary, error)
	CreateApplication(
		ctx context.Context,
		appID uuid.UUID,
		tenantID uuid.UUID,
		catalog CatalogTrack,
		guardianUserID uuid.UUID,
		initialStatus domain.ApplicationStatus,
		studentName string,
		extraAnswers map[string]any,
		actor ActorContext,
	) (*ApplicationDetail, error)

	GetApplicationDetail(ctx context.Context, tenantID, applicationID uuid.UUID) (*ApplicationDetail, error)

	SaveApplicantProfile(
		ctx context.Context,
		tenantID, applicationID uuid.UUID,
		expectedVersion *int,
		profile ApplicantProfile,
		extraAnswers map[string]any,
		distanceMeters *int,
	) (*ApplicationDetail, error)

	ReplaceGuardians(ctx context.Context, tenantID, applicationID uuid.UUID, guardians []Guardian) ([]Guardian, error)
	ReplaceAchievements(ctx context.Context, tenantID, applicationID uuid.UUID, achievements []Achievement) ([]Achievement, error)

	UpsertDocument(
		ctx context.Context,
		tenantID, applicationID, docID uuid.UUID,
		input ConfirmDocumentInput,
	) (*Document, error)
	GetDocumentByID(ctx context.Context, tenantID, applicationID, documentID uuid.UUID) (*Document, error)

	SubmitApplication(
		ctx context.Context,
		tenantID, applicationID uuid.UUID,
		expectedVersion int,
		fromStatus domain.ApplicationStatus,
		audit AuditWrite,
		notify NotificationWrite,
	) (*ApplicationDetail, error)

	ListApplicationsForStaff(ctx context.Context, tenantID uuid.UUID, filter StaffListFilter) ([]ApplicationSummary, error)
	ClaimNextForVerification(
		ctx context.Context,
		tenantID, unitID, verifierUserID uuid.UUID,
		audit AuditWrite,
	) (*ApplicationDetail, error)

	ReviewDocument(
		ctx context.Context,
		tenantID, applicationID, documentID, reviewerUserID uuid.UUID,
		status domain.DocumentReviewStatus,
		note *string,
		audit AuditWrite,
	) (*Document, error)

	CompleteVerification(
		ctx context.Context,
		tenantID, applicationID uuid.UUID,
		expectedVersion int,
		fromStatus, toStatus domain.ApplicationStatus,
		reviewedAt time.Time,
		audit AuditWrite,
		notify NotificationWrite,
	) (*ApplicationDetail, error)

	GetTrackByID(ctx context.Context, tenantID, trackID uuid.UUID) (*CatalogTrack, error)
	ListPeriodTracks(ctx context.Context, tenantID, periodID uuid.UUID) ([]CatalogTrack, error)

	CreatePayment(ctx context.Context, payment PaymentTransaction) (*PaymentTransaction, error)
	GetPaymentByID(ctx context.Context, tenantID, paymentID uuid.UUID) (*PaymentTransaction, error)
	GetPaymentByExternalRef(ctx context.Context, tenantID uuid.UUID, externalRef string) (*PaymentTransaction, error)
	ResolvePaymentByExternalRef(ctx context.Context, externalRef string) (*PaymentTransaction, error)
	ListPaymentsByApplication(ctx context.Context, tenantID, applicationID uuid.UUID) ([]PaymentTransaction, error)
	AttachManualPaymentProof(ctx context.Context, tenantID, applicationID, paymentID uuid.UUID, proofStorageKey string) (*PaymentTransaction, error)
	ListAdminPayments(ctx context.Context, tenantID uuid.UUID, filter AdminPaymentFilter) ([]PaymentTransaction, error)
	VerifyPayment(
		ctx context.Context,
		tenantID, paymentID uuid.UUID,
		decision string,
		verifierUserID *uuid.UUID,
		note *string,
		paidAt time.Time,
		rawWebhook []byte,
		actor ActorContext,
	) (*PaymentTransaction, *ApplicationDetail, error)

	ConfirmReEnrollment(
		ctx context.Context,
		tenantID, applicationID uuid.UUID,
		expectedVersion int,
		fromStatus, toStatus domain.ApplicationStatus,
		extraAnswers map[string]any,
		audit AuditWrite,
		notify *NotificationWrite,
	) (*ApplicationDetail, error)

	UpsertApplicationScores(
		ctx context.Context,
		tenantID, applicationID uuid.UUID,
		examinerUserID uuid.UUID,
		inputs []ScoreComponentInput,
		actor ActorContext,
	) (*UpsertScoresResult, error)

	SimulateTrackRanking(ctx context.Context, tenantID, trackID uuid.UUID) (*TrackRankingSimulation, error)
	PublishPeriodResults(ctx context.Context, tenantID, periodID uuid.UUID, actor ActorContext, publishedAt time.Time) (*PublishResultsResult, error)
	PromoteWaitlist(ctx context.Context, tenantID, applicationID uuid.UUID, actor ActorContext) (*ApplicationDetail, error)

	ListAssessmentSchedules(ctx context.Context, tenantID uuid.UUID, filter AssessmentScheduleFilter) ([]AssessmentSchedule, error)
	CreateAssessmentSchedules(ctx context.Context, tenantID uuid.UUID, inputs []AssessmentSchedule, actor ActorContext) ([]AssessmentSchedule, error)
	DeleteAssessmentSchedule(ctx context.Context, tenantID, scheduleID uuid.UUID, actor ActorContext) error

	CreateExportJob(ctx context.Context, tenantID, unitID, periodID uuid.UUID, format string, actor ActorContext) (*ExportJob, error)
	GetExportJob(ctx context.Context, tenantID, jobID uuid.UUID) (*ExportJob, error)
	ListExportJobs(ctx context.Context, tenantID uuid.UUID, allowedUnitIDs []uuid.UUID, limit int) ([]ExportJob, error)
	GetDemoExportFile(ctx context.Context, tenantID, jobID uuid.UUID) (*DemoExportFile, error)
	ListTenantAuditLogs(ctx context.Context, tenantID uuid.UUID, filter TenantAuditLogFilter) ([]TenantAuditLogItem, error)
}

