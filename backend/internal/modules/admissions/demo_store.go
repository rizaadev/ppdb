package admissions

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/modules/catalog"
)

// DemoStore implements transactional in-memory persistence for local dev without PostgreSQL.
type DemoStore struct {
	mu                sync.RWMutex
	catalog           catalog.Provider
	apps              map[uuid.UUID]*ApplicationDetail
	periodRegSeq      map[uuid.UUID]int
	payments          map[uuid.UUID]*PaymentTransaction
	scores            map[uuid.UUID][]AssessmentScore
	schedules         map[uuid.UUID]*demoScheduleRecord
	periodPublishedAt map[uuid.UUID]time.Time
	exportJobs        map[uuid.UUID]*demoExportRecord
	tenantAudits      []TenantAuditLogItem
}

type demoScheduleRecord struct {
	TenantID uuid.UUID
	Schedule AssessmentSchedule
}

// NewDemoStore creates an empty in-memory admissions store backed by the shared demo catalog.
func NewDemoStore() *DemoStore {
	return &DemoStore{
		catalog:           catalog.DefaultProvider(),
		apps:              make(map[uuid.UUID]*ApplicationDetail),
		periodRegSeq:      make(map[uuid.UUID]int),
		payments:          make(map[uuid.UUID]*PaymentTransaction),
		scores:            make(map[uuid.UUID][]AssessmentScore),
		schedules:         make(map[uuid.UUID]*demoScheduleRecord),
		periodPublishedAt: make(map[uuid.UUID]time.Time),
		exportJobs:        make(map[uuid.UUID]*demoExportRecord),
		tenantAudits:      []TenantAuditLogItem{},
	}
}

// SetCatalogProvider wires a shared live catalog store (e.g. mutable public.DemoStore) into DemoStore.
func (s *DemoStore) SetCatalogProvider(provider catalog.Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if provider != nil {
		s.catalog = provider
	}
}

func (s *DemoStore) cat() catalog.Provider {
	if s.catalog != nil {
		return s.catalog
	}
	return catalog.DefaultProvider()
}

func toCatalogTrack(found *catalog.TrackLookup) *CatalogTrack {
	lat := found.Unit.Latitude
	lon := found.Unit.Longitude
	reqs := make([]TrackRequirement, 0, len(found.Track.Requirements))
	for idx, r := range found.Track.Requirements {
		desc := r.Description
		reqs = append(reqs, TrackRequirement{
			ID:            r.ID,
			TrackID:       found.Track.ID,
			DocCode:       r.DocCode,
			Label:         r.Label,
			Description:   &desc,
			IsRequired:    r.IsRequired,
			AcceptedMimes: slices.Clone(r.AcceptedMimes),
			MaxSizeKB:     r.MaxSizeKB,
			SortOrder:     int16(idx + 1),
		})
	}

	return &CatalogTrack{
		TenantID:             found.Tenant.ID,
		UnitID:               found.Unit.ID,
		UnitName:             found.Unit.Name,
		UnitLat:              &lat,
		UnitLon:              &lon,
		PeriodID:             found.Period.ID,
		PeriodName:           found.Period.Name,
		AcademicYear:         found.Period.AcademicYear,
		TrackID:              found.Track.ID,
		TrackCode:            found.Track.Code,
		TrackName:            found.Track.DisplayName,
		SelectionMode:        found.Track.SelectionMode,
		Quota:                found.Track.Quota,
		SpilloverTargetTrack: found.Track.SpilloverTargetTrackID,
		RegistrationFeeIDR:   found.Track.RegistrationFeeIDR,
		ReEnrollmentFeeIDR:   found.Track.ReEnrollmentFeeIDR,
		MinAgeMonths:         found.Track.MinAgeMonths,
		MaxAgeMonths:         found.Track.MaxAgeMonths,
		AgeReferenceDate:     found.Track.AgeReferenceDate,
		RequiresCoordinates:  found.Track.RequiresCoordinates,
		RequiresAchievements: found.Track.RequiresAchievements,
		ScoringWeights:       maps.Clone(found.Track.ScoringWeights),
		CustomFormSchema:     slices.Clone(found.Track.CustomFormSchema),
		Requirements:         reqs,
	}
}

func (s *DemoStore) GetCatalogTrack(_ context.Context, tenantID, unitID, periodID, trackID uuid.UUID) (*CatalogTrack, error) {
	found, ok := s.cat().FindTrack(tenantID, unitID, periodID, trackID)
	if !ok {
		return nil, nil
	}
	return toCatalogTrack(found), nil
}

func (s *DemoStore) ListApplicationsByGuardian(_ context.Context, tenantID, guardianUserID uuid.UUID) ([]ApplicationSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []ApplicationSummary
	for _, item := range s.apps {
		if len(item.AuditLogs) == 0 && item.GuardianUserID != guardianUserID {
			continue
		}
		if item.GuardianUserID == guardianUserID && s.belongsToTenantLocked(item, tenantID) {
			list = append(list, cloneSummary(item.Application, item.Profile))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].ID.String() > list[j].ID.String()
		}
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	if list == nil {
		list = []ApplicationSummary{}
	}
	return list, nil
}

func (s *DemoStore) belongsToTenantLocked(app *ApplicationDetail, tenantID uuid.UUID) bool {
	return s.cat().UnitBelongsToTenant(tenantID, app.Application.UnitID)
}

func (s *DemoStore) CreateApplication(
	_ context.Context,
	appID uuid.UUID,
	tenantID uuid.UUID,
	catalog CatalogTrack,
	guardianUserID uuid.UUID,
	initialStatus domain.ApplicationStatus,
	studentName string,
	extraAnswers map[string]any,
	actor ActorContext,
) (*ApplicationDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.periodRegSeq[catalog.PeriodID]++
	seq := s.periodRegSeq[catalog.PeriodID]
	regNo := formatRegistrationNo(catalog.TrackCode, catalog.AcademicYear, seq)
	now := time.Now().UTC()

	extras := map[string]any{}
	if extraAnswers != nil {
		extras = maps.Clone(extraAnswers)
	}

	nameCopy := studentName
	toStatusStr := string(initialStatus)
	detail := &ApplicationDetail{
		Application: ApplicationSummary{
			ID:             appID,
			RegistrationNo: regNo,
			Status:         initialStatus,
			UnitID:         catalog.UnitID,
			UnitName:       catalog.UnitName,
			PeriodID:       catalog.PeriodID,
			PeriodName:     catalog.PeriodName,
			AcademicYear:   catalog.AcademicYear,
			TrackID:        catalog.TrackID,
			TrackName:      catalog.TrackName,
			StudentName:    &nameCopy,
			Version:        1,
			CreatedAt:      now,
		},
		GuardianUserID:       guardianUserID,
		UnitLatitude:         catalog.UnitLat,
		UnitLongitude:        catalog.UnitLon,
		RequiresCoordinates:  catalog.RequiresCoordinates,
		RequiresAchievements: catalog.RequiresAchievements,
		RegistrationFeeIDR:   catalog.RegistrationFeeIDR,
		ReEnrollmentFeeIDR:   catalog.ReEnrollmentFeeIDR,
		ExtraAnswers:         extras,
		Profile: &ApplicantProfile{
			FullName:    studentName,
			Citizenship: "WNI",
		},
		Guardians:           []Guardian{},
		Achievements:        []Achievement{},
		Requirements:        slices.Clone(catalog.Requirements),
		Documents:           []Document{},
		Payments:            []PaymentTransaction{},
		Scores:              []AssessmentScore{},
		Schedules:           []AssessmentSchedule{},
		TrackScoringWeights: maps.Clone(catalog.ScoringWeights),
		CustomFormSchema:    slices.Clone(catalog.CustomFormSchema),
		AuditLogs: []AuditEntry{
			{
				ID:          uuid.Must(uuid.NewV7()),
				ActorUserID: &actor.UserID,
				ActorRole:   actor.RoleName,
				ActionType:  "application_created",
				EntityType:  "application",
				EntityID:    appID,
				ToStatus:    &toStatusStr,
				CreatedAt:   now,
			},
		},
	}

	s.apps[appID] = detail
	return cloneDetail(detail), nil
}

func (s *DemoStore) GetApplicationDetail(_ context.Context, tenantID, applicationID uuid.UUID) (*ApplicationDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, nil
	}
	return cloneDetail(app), nil
}

func (s *DemoStore) SaveApplicantProfile(
	_ context.Context,
	tenantID, applicationID uuid.UUID,
	expectedVersion *int,
	profile ApplicantProfile,
	extraAnswers map[string]any,
	distanceMeters *int,
) (*ApplicationDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}
	if expectedVersion != nil && *expectedVersion != app.Application.Version {
		return nil, ErrVersionConflict
	}

	profCopy := profile
	if strings.TrimSpace(profCopy.Citizenship) == "" {
		profCopy.Citizenship = "WNI"
	}
	app.Profile = &profCopy
	nameCopy := profCopy.FullName
	app.Application.StudentName = &nameCopy
	app.Application.NISN = profCopy.NISN
	app.Application.DistanceMeters = distanceMeters
	if extraAnswers != nil {
		app.ExtraAnswers = maps.Clone(extraAnswers)
	}
	app.Application.Version++
	return cloneDetail(app), nil
}

func (s *DemoStore) ReplaceGuardians(_ context.Context, tenantID, applicationID uuid.UUID, guardians []Guardian) ([]Guardian, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}
	app.Guardians = slices.Clone(guardians)
	app.Application.Version++
	return slices.Clone(app.Guardians), nil
}

func (s *DemoStore) ReplaceAchievements(_ context.Context, tenantID, applicationID uuid.UUID, achievements []Achievement) ([]Achievement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}
	app.Achievements = slices.Clone(achievements)
	app.Application.Version++
	return slices.Clone(app.Achievements), nil
}

func (s *DemoStore) UpsertDocument(
	_ context.Context,
	tenantID, applicationID, docID uuid.UUID,
	input ConfirmDocumentInput,
) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}

	var req *TrackRequirement
	for i := range app.Requirements {
		if app.Requirements[i].ID == input.RequirementID {
			req = &app.Requirements[i]
			break
		}
	}
	if req == nil {
		return nil, ErrRequirementNotFound
	}

	for i := range app.Documents {
		if app.Documents[i].RequirementID == input.RequirementID {
			doc := &app.Documents[i]
			doc.StorageKey = input.StorageKey
			doc.OriginalFilename = input.OriginalFilename
			doc.MimeType = input.MimeType
			doc.SizeBytes = input.SizeBytes
			doc.Status = domain.DocStatusPending
			doc.ReviewerUserID = nil
			doc.ReviewNote = nil
			doc.ReviewedAt = nil
			doc.Version++
			copied := *doc
			return &copied, nil
		}
	}

	doc := Document{
		ID:               docID,
		RequirementID:    req.ID,
		DocCode:          req.DocCode,
		RequirementLabel: req.Label,
		IsRequired:       req.IsRequired,
		StorageKey:       input.StorageKey,
		OriginalFilename: input.OriginalFilename,
		MimeType:         input.MimeType,
		SizeBytes:        input.SizeBytes,
		Status:           domain.DocStatusPending,
		Version:          1,
	}
	app.Documents = append(app.Documents, doc)
	copied := doc
	return &copied, nil
}

func (s *DemoStore) GetDocumentByID(_ context.Context, tenantID, applicationID, documentID uuid.UUID) (*Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, nil
	}
	for _, doc := range app.Documents {
		if doc.ID == documentID {
			copied := doc
			return &copied, nil
		}
	}
	return nil, nil
}

func (s *DemoStore) SubmitApplication(
	_ context.Context,
	tenantID, applicationID uuid.UUID,
	expectedVersion int,
	fromStatus domain.ApplicationStatus,
	audit AuditWrite,
	_ NotificationWrite,
) (*ApplicationDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}
	if app.Application.Version != expectedVersion {
		return nil, ErrVersionConflict
	}
	if app.Application.Status != fromStatus {
		return nil, ErrApplicationLocked
	}

	now := time.Now().UTC()
	app.Application.Status = domain.StatusSubmitted
	if app.Application.SubmittedAt == nil {
		app.Application.SubmittedAt = &now
	}
	app.Application.Version++
	appendAuditLocked(app, audit, now)
	return cloneDetail(app), nil
}

func (s *DemoStore) ListApplicationsForStaff(_ context.Context, tenantID uuid.UUID, filter StaffListFilter) ([]ApplicationSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	searchLower := strings.ToLower(strings.TrimSpace(filter.Search))
	var results []ApplicationSummary

	for _, app := range s.apps {
		if !s.belongsToTenantLocked(app, tenantID) {
			continue
		}
		if filter.UnitID != nil && app.Application.UnitID != *filter.UnitID {
			continue
		}
		if len(filter.AllowedUnitIDs) > 0 && !slices.Contains(filter.AllowedUnitIDs, app.Application.UnitID) {
			continue
		}
		if filter.PeriodID != nil && app.Application.PeriodID != *filter.PeriodID {
			continue
		}
		if filter.TrackID != nil && app.Application.TrackID != *filter.TrackID {
			continue
		}
		if filter.Status != nil && app.Application.Status != *filter.Status {
			continue
		}
		if searchLower != "" && !matchesSearch(app, searchLower) {
			continue
		}
		if filter.CursorCreatedAt != nil && filter.CursorID != nil {
			if !beforeCursor(app.Application.CreatedAt, app.Application.ID, *filter.CursorCreatedAt, *filter.CursorID) {
				continue
			}
		}
		results = append(results, cloneSummary(app.Application, app.Profile))
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].CreatedAt.Equal(results[j].CreatedAt) {
			return results[i].ID.String() > results[j].ID.String()
		}
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if len(results) > limit {
		results = results[:limit]
	}
	if results == nil {
		results = []ApplicationSummary{}
	}
	return results, nil
}

func (s *DemoStore) ClaimNextForVerification(
	_ context.Context,
	tenantID, unitID, verifierUserID uuid.UUID,
	audit AuditWrite,
) (*ApplicationDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldest *ApplicationDetail
	for _, app := range s.apps {
		if !s.belongsToTenantLocked(app, tenantID) || app.Application.UnitID != unitID {
			continue
		}
		if app.Application.Status != domain.StatusSubmitted {
			continue
		}
		if oldest == nil || submittedTime(app).Before(submittedTime(oldest)) {
			oldest = app
		}
	}
	if oldest == nil {
		return nil, nil
	}

	now := time.Now().UTC()
	oldest.Application.Status = domain.StatusUnderVerification
	oldest.ClaimedByVerifierID = &verifierUserID
	oldest.ClaimedAt = &now
	oldest.Application.Version++
	audit.EntityID = oldest.Application.ID
	audit.Application = &oldest.Application.ID
	appendAuditLocked(oldest, audit, now)
	return cloneDetail(oldest), nil
}

func (s *DemoStore) ReviewDocument(
	_ context.Context,
	tenantID, applicationID, documentID, reviewerUserID uuid.UUID,
	status domain.DocumentReviewStatus,
	note *string,
	audit AuditWrite,
) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}
	if app.Application.Status != domain.StatusSubmitted && app.Application.Status != domain.StatusUnderVerification {
		return nil, ErrApplicationLocked
	}

	now := time.Now().UTC()
	if app.Application.Status == domain.StatusSubmitted {
		app.Application.Status = domain.StatusUnderVerification
		app.ClaimedByVerifierID = &reviewerUserID
		app.ClaimedAt = &now
		app.Application.Version++
	}

	for i := range app.Documents {
		if app.Documents[i].ID == documentID {
			doc := &app.Documents[i]
			doc.Status = status
			doc.ReviewerUserID = &reviewerUserID
			doc.ReviewNote = note
			doc.ReviewedAt = &now
			appendAuditLocked(app, audit, now)
			copied := *doc
			return &copied, nil
		}
	}
	return nil, ErrDocumentNotFound
}

func (s *DemoStore) CompleteVerification(
	_ context.Context,
	tenantID, applicationID uuid.UUID,
	expectedVersion int,
	fromStatus, toStatus domain.ApplicationStatus,
	reviewedAt time.Time,
	audit AuditWrite,
	_ NotificationWrite,
) (*ApplicationDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}
	if app.Application.Version != expectedVersion {
		return nil, ErrVersionConflict
	}
	if app.Application.Status != fromStatus {
		return nil, ErrApplicationLocked
	}

	app.Application.Status = toStatus
	if toStatus == domain.StatusVerified {
		ts := reviewedAt.UTC()
		app.Application.VerifiedAt = &ts
	}
	app.Application.Version++
	appendAuditLocked(app, audit, reviewedAt.UTC())
	return cloneDetail(app), nil
}

func appendAuditLocked(app *ApplicationDetail, audit AuditWrite, now time.Time) {
	id := audit.ID
	if id == uuid.Nil {
		id = uuid.Must(uuid.NewV7())
	}
	entry := AuditEntry{
		ID:          id,
		ActorUserID: &audit.Actor.UserID,
		ActorRole:   audit.Actor.RoleName,
		ActionType:  audit.ActionType,
		EntityType:  audit.EntityType,
		EntityID:    audit.EntityID,
		FromStatus:  audit.FromStatus,
		ToStatus:    audit.ToStatus,
		Reason:      audit.Reason,
		CreatedAt:   now,
	}
	app.AuditLogs = append([]AuditEntry{entry}, app.AuditLogs...)
}

func submittedTime(app *ApplicationDetail) time.Time {
	if app.Application.SubmittedAt != nil {
		return *app.Application.SubmittedAt
	}
	return app.Application.CreatedAt
}

func matchesSearch(app *ApplicationDetail, needle string) bool {
	if strings.Contains(strings.ToLower(app.Application.RegistrationNo), needle) {
		return true
	}
	if app.Application.StudentName != nil && strings.Contains(strings.ToLower(*app.Application.StudentName), needle) {
		return true
	}
	if app.Profile != nil && app.Profile.NISN != nil && strings.Contains(strings.ToLower(*app.Profile.NISN), needle) {
		return true
	}
	return false
}

func beforeCursor(createdAt time.Time, id uuid.UUID, cursorAt time.Time, cursorID uuid.UUID) bool {
	if createdAt.Before(cursorAt) {
		return true
	}
	return createdAt.Equal(cursorAt) && id.String() < cursorID.String()
}

func cloneSummary(sum ApplicationSummary, prof *ApplicantProfile) ApplicationSummary {
	out := sum
	if prof != nil {
		name := prof.FullName
		out.StudentName = &name
		out.NISN = prof.NISN
	}
	return out
}

func cloneDetail(src *ApplicationDetail) *ApplicationDetail {
	if src == nil {
		return nil
	}
	out := *src
	out.Application = cloneSummary(src.Application, src.Profile)
	if src.Profile != nil {
		p := *src.Profile
		out.Profile = &p
	}
	if src.ExtraAnswers != nil {
		out.ExtraAnswers = maps.Clone(src.ExtraAnswers)
	} else {
		out.ExtraAnswers = map[string]any{}
	}
	out.Guardians = slices.Clone(src.Guardians)
	out.Achievements = slices.Clone(src.Achievements)
	out.Requirements = slices.Clone(src.Requirements)
	out.Documents = slices.Clone(src.Documents)
	if src.Payments != nil {
		out.Payments = slices.Clone(src.Payments)
	} else {
		out.Payments = []PaymentTransaction{}
	}
	if src.Scores != nil {
		out.Scores = slices.Clone(src.Scores)
	} else {
		out.Scores = []AssessmentScore{}
	}
	if src.Schedules != nil {
		out.Schedules = slices.Clone(src.Schedules)
	} else {
		out.Schedules = []AssessmentSchedule{}
	}
	if src.TrackScoringWeights != nil {
		out.TrackScoringWeights = maps.Clone(src.TrackScoringWeights)
	}
	out.CustomFormSchema = slices.Clone(src.CustomFormSchema)
	out.AuditLogs = slices.Clone(src.AuditLogs)
	return &out
}

func formatRegistrationNo(trackCode, academicYear string, seq int) string {
	prefix := strings.ToUpper(strings.ReplaceAll(trackCode, "_", ""))
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	if prefix == "" {
		prefix = "REG"
	}
	yearPart := "26"
	if len(academicYear) >= 4 {
		if _, err := strconv.Atoi(academicYear[:4]); err == nil {
			yearPart = academicYear[2:4]
		}
	}
	return fmt.Sprintf("%s-%s-%04d", prefix, yearPart, seq)
}
