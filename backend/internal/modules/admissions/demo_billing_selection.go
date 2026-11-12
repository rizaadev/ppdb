package admissions

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

func (s *DemoStore) GetTrackByID(_ context.Context, tenantID, trackID uuid.UUID) (*CatalogTrack, error) {
	found, ok := s.cat().FindTrackByID(tenantID, trackID)
	if !ok {
		return nil, nil
	}
	return toCatalogTrack(found), nil
}

func (s *DemoStore) ListPeriodTracks(_ context.Context, tenantID, periodID uuid.UUID) ([]CatalogTrack, error) {
	found, ok := s.cat().FindPeriodTracks(tenantID, periodID)
	if !ok {
		return nil, nil
	}
	out := make([]CatalogTrack, 0, len(found))
	for i := range found {
		out = append(out, *toCatalogTrack(&found[i]))
	}
	return out, nil
}

func (s *DemoStore) CreatePayment(_ context.Context, payment PaymentTransaction) (*PaymentTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[payment.ApplicationID]
	if !ok {
		return nil, ErrApplicationNotFound
	}
	copied := payment
	s.payments[copied.ID] = &copied
	s.syncAppBillingLocked(app)
	res := copied
	return &res, nil
}

func (s *DemoStore) GetPaymentByID(_ context.Context, tenantID, paymentID uuid.UUID) (*PaymentTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.payments[paymentID]
	if !ok {
		return nil, nil
	}
	app, ok := s.apps[p.ApplicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, nil
	}
	copied := *p
	return &copied, nil
}

func (s *DemoStore) GetPaymentByExternalRef(_ context.Context, tenantID uuid.UUID, externalRef string) (*PaymentTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.payments {
		if p.ExternalRef == nil || *p.ExternalRef != externalRef {
			continue
		}
		app, ok := s.apps[p.ApplicationID]
		if ok && s.belongsToTenantLocked(app, tenantID) {
			copied := *p
			copied.TenantID = tenantID
			return &copied, nil
		}
	}
	return nil, nil
}

func (s *DemoStore) ResolvePaymentByExternalRef(_ context.Context, externalRef string) (*PaymentTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.payments {
		if p.ExternalRef == nil || *p.ExternalRef != externalRef {
			continue
		}
		copied := *p
		return &copied, nil
	}
	return nil, nil
}

func (s *DemoStore) ListPaymentsByApplication(_ context.Context, tenantID, applicationID uuid.UUID) ([]PaymentTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}
	return s.collectAppPaymentsLocked(applicationID), nil
}

func (s *DemoStore) AttachManualPaymentProof(
	_ context.Context,
	tenantID, applicationID, paymentID uuid.UUID,
	proofStorageKey string,
) (*PaymentTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}
	p, ok := s.payments[paymentID]
	if !ok || p.ApplicationID != applicationID {
		return nil, ErrPaymentNotFound
	}
	if p.Method != PaymentMethodManualTransfer || p.Status != PaymentStatusPending {
		return nil, ErrApplicationLocked
	}
	keyCopy := proofStorageKey
	p.ProofStorageKey = &keyCopy
	s.syncAppBillingLocked(app)
	res := *p
	return &res, nil
}

func (s *DemoStore) ListAdminPayments(_ context.Context, tenantID uuid.UUID, filter AdminPaymentFilter) ([]PaymentTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []PaymentTransaction
	for _, p := range s.payments {
		app, ok := s.apps[p.ApplicationID]
		if !ok || !s.belongsToTenantLocked(app, tenantID) {
			continue
		}
		if filter.UnitID != nil && app.Application.UnitID != *filter.UnitID {
			continue
		}
		if len(filter.AllowedUnitIDs) > 0 && !slices.Contains(filter.AllowedUnitIDs, app.Application.UnitID) {
			continue
		}
		if filter.Status != nil && p.Status != *filter.Status {
			continue
		}
		if filter.Method != nil && p.Method != *filter.Method {
			continue
		}
		if filter.Purpose != nil && p.Purpose != *filter.Purpose {
			continue
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID.String() > out[j].ID.String()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []PaymentTransaction{}
	}
	return out, nil
}

func (s *DemoStore) VerifyPayment(
	_ context.Context,
	tenantID, paymentID uuid.UUID,
	decision string,
	verifierUserID *uuid.UUID,
	note *string,
	paidAt time.Time,
	_ []byte,
	actor ActorContext,
) (*PaymentTransaction, *ApplicationDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.payments[paymentID]
	if !ok {
		return nil, nil, ErrPaymentNotFound
	}
	app, ok := s.apps[p.ApplicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, nil, ErrPaymentNotFound
	}
	if p.Status != PaymentStatusPending {
		return nil, nil, ErrPaymentAlreadyFinal
	}

	p.Status = decision
	p.VerifiedByUserID = verifierUserID
	p.VerificationNote = note
	if decision == PaymentStatusPaid {
		ts := paidAt.UTC()
		p.PaidAt = &ts
	}

	fromPay := PaymentStatusPending
	appendAuditLocked(app, AuditWrite{
		TenantID:    tenantID,
		Application: &app.Application.ID,
		Actor:       actor,
		ActionType:  "payment_" + decision,
		EntityType:  "payment_transaction",
		EntityID:    p.ID,
		FromStatus:  &fromPay,
		ToStatus:    &decision,
		Reason:      note,
	}, paidAt.UTC())

	if decision == PaymentStatusPaid {
		switch p.Purpose {
		case PaymentPurposeRegistration:
			if app.Application.Status == domain.StatusAwaitingPayment {
				fromStr := string(domain.StatusAwaitingPayment)
				midStr := string(domain.StatusPaymentVerified)
				toStr := string(domain.StatusDraftForm)
				appendAuditLocked(app, AuditWrite{
					TenantID:    tenantID,
					Application: &app.Application.ID,
					Actor:       actor,
					ActionType:  "payment_verified",
					EntityType:  "application",
					EntityID:    app.Application.ID,
					FromStatus:  &fromStr,
					ToStatus:    &midStr,
				}, paidAt.UTC())
				app.Application.Status = domain.StatusDraftForm
				app.Application.Version++
				appendAuditLocked(app, AuditWrite{
					TenantID:    tenantID,
					Application: &app.Application.ID,
					Actor:       actor,
					ActionType:  "form_unlocked",
					EntityType:  "application",
					EntityID:    app.Application.ID,
					FromStatus:  &midStr,
					ToStatus:    &toStr,
				}, paidAt.UTC())
			}
		case PaymentPurposeReEnrollment:
			if app.Application.Status == domain.StatusReEnrollment {
				fromStr := string(domain.StatusReEnrollment)
				toStr := string(domain.StatusEnrolled)
				app.Application.Status = domain.StatusEnrolled
				app.Application.Version++
				appendAuditLocked(app, AuditWrite{
					TenantID:    tenantID,
					Application: &app.Application.ID,
					Actor:       actor,
					ActionType:  "re_enrollment_paid",
					EntityType:  "application",
					EntityID:    app.Application.ID,
					FromStatus:  &fromStr,
					ToStatus:    &toStr,
				}, paidAt.UTC())
			}
		}
	}

	s.syncAppBillingLocked(app)
	pCopy := *p
	return &pCopy, cloneDetail(app), nil
}

func (s *DemoStore) ConfirmReEnrollment(
	_ context.Context,
	tenantID, applicationID uuid.UUID,
	expectedVersion int,
	fromStatus, toStatus domain.ApplicationStatus,
	extraAnswers map[string]any,
	audit AuditWrite,
	_ *NotificationWrite,
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
	if extraAnswers != nil {
		if app.ExtraAnswers == nil {
			app.ExtraAnswers = map[string]any{}
		}
		maps.Copy(app.ExtraAnswers, extraAnswers)
	}
	app.Application.Status = toStatus
	app.Application.Version++
	appendAuditLocked(app, audit, time.Now().UTC())
	return cloneDetail(app), nil
}

func (s *DemoStore) UpsertApplicationScores(
	_ context.Context,
	tenantID, applicationID uuid.UUID,
	examinerUserID uuid.UUID,
	inputs []ScoreComponentInput,
	actor ActorContext,
) (*UpsertScoresResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}

	now := time.Now().UTC()
	existing := s.scores[applicationID]
	byCode := make(map[string]AssessmentScore, len(existing))
	for _, sc := range existing {
		byCode[sc.ComponentCode] = sc
	}

	for _, in := range inputs {
		weighted := in.RawScore * (in.WeightPct / 100.0)
		var notesPtr *string
		if in.Notes != "" {
			n := in.Notes
			notesPtr = &n
		}
		item, exists := byCode[in.ComponentCode]
		if !exists {
			item = AssessmentScore{
				ID:            uuid.Must(uuid.NewV7()),
				ApplicationID: applicationID,
				ComponentCode: in.ComponentCode,
			}
		}
		item.RawScore = in.RawScore
		item.WeightPct = in.WeightPct
		item.WeightedScore = weighted
		item.Notes = notesPtr
		item.ExaminerUserID = &examinerUserID
		item.UpdatedAt = now
		byCode[in.ComponentCode] = item
	}

	merged := make([]AssessmentScore, 0, len(byCode))
	for _, sc := range byCode {
		merged = append(merged, sc)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].ComponentCode < merged[j].ComponentCode
	})
	s.scores[applicationID] = merged
	app.Scores = slices.Clone(merged)

	total := calculateWeightedTotal(merged)
	scoreStr := fmt.Sprintf("%.2f", total)
	app.Application.FinalScore = &scoreStr

	if app.Application.Status == domain.StatusVerified {
		fromStr := string(domain.StatusVerified)
		toStr := string(domain.StatusSelectionInProgress)
		app.Application.Status = domain.StatusSelectionInProgress
		appendAuditLocked(app, AuditWrite{
			TenantID:    tenantID,
			Application: &app.Application.ID,
			Actor:       actor,
			ActionType:  "assessment_scored",
			EntityType:  "application",
			EntityID:    app.Application.ID,
			FromStatus:  &fromStr,
			ToStatus:    &toStr,
		}, now)
	} else {
		appendAuditLocked(app, AuditWrite{
			TenantID:    tenantID,
			Application: &app.Application.ID,
			Actor:       actor,
			ActionType:  "assessment_scored",
			EntityType:  "application",
			EntityID:    app.Application.ID,
		}, now)
	}
	app.Application.Version++

	return &UpsertScoresResult{
		Application: cloneDetail(app),
		Scores:      slices.Clone(merged),
		FinalScore:  total,
	}, nil
}

func (s *DemoStore) SimulateTrackRanking(ctx context.Context, tenantID, trackID uuid.UUID) (*TrackRankingSimulation, error) {
	track, err := s.GetTrackByID(ctx, tenantID, trackID)
	if err != nil {
		return nil, err
	}
	if track == nil {
		return nil, ErrTrackNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sim := s.simulateTrackLocked(*track)
	return &sim, nil
}

func (s *DemoStore) PublishPeriodResults(
	ctx context.Context,
	tenantID, periodID uuid.UUID,
	actor ActorContext,
	publishedAt time.Time,
) (*PublishResultsResult, error) {
	tracks, err := s.ListPeriodTracks(ctx, tenantID, periodID)
	if err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		return nil, ErrPeriodNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ts := publishedAt.UTC()
	s.periodPublishedAt[periodID] = ts

	out := PublishResultsResult{
		PeriodID:    periodID,
		PublishedAt: ts,
		Tracks:      make([]TrackRankingSimulation, 0, len(tracks)),
	}

	for _, tr := range tracks {
		sim := s.simulateTrackLocked(tr)
		for _, cand := range sim.Candidates {
			app := s.apps[cand.ApplicationID]
			if app == nil {
				continue
			}
			app.Application.ResultsPublishedAt = &ts
			// Only transition candidates still in pre-decision pipeline states.
			if isPreDecisionStatus(app.Application.Status) {
				fromStr := string(app.Application.Status)
				toStr := string(cand.SimulatedDecision)
				rankPos := cand.RankPosition
				app.Application.RankPosition = &rankPos
				app.Application.DecidedAt = &ts
				app.Application.Status = cand.SimulatedDecision
				app.Application.Version++
				appendAuditLocked(app, AuditWrite{
					TenantID:    tenantID,
					Application: &app.Application.ID,
					Actor:       actor,
					ActionType:  "results_published",
					EntityType:  "application",
					EntityID:    app.Application.ID,
					FromStatus:  &fromStr,
					ToStatus:    &toStr,
				}, ts)
			}
		}
		out.TotalAccepted += sim.AcceptedCount
		out.TotalWaitlisted += sim.WaitlistedCount
		out.TotalRejected += sim.RejectedCount
		out.Tracks = append(out.Tracks, sim)
	}

	return &out, nil
}

func (s *DemoStore) PromoteWaitlist(_ context.Context, tenantID, applicationID uuid.UUID, actor ActorContext) (*ApplicationDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[applicationID]
	if !ok || !s.belongsToTenantLocked(app, tenantID) {
		return nil, ErrApplicationNotFound
	}
	if app.Application.Status != domain.StatusWaitlisted {
		return nil, ErrApplicationLocked
	}
	now := time.Now().UTC()
	fromStr := string(domain.StatusWaitlisted)
	toStr := string(domain.StatusAccepted)
	app.Application.Status = domain.StatusAccepted
	app.Application.DecidedAt = &now
	app.Application.Version++
	appendAuditLocked(app, AuditWrite{
		TenantID:    tenantID,
		Application: &app.Application.ID,
		Actor:       actor,
		ActionType:  "waitlist_promoted",
		EntityType:  "application",
		EntityID:    app.Application.ID,
		FromStatus:  &fromStr,
		ToStatus:    &toStr,
	}, now)
	return cloneDetail(app), nil
}

func (s *DemoStore) simulateTrackLocked(track CatalogTrack) TrackRankingSimulation {
	var candidates []*ApplicationDetail
	for _, app := range s.apps {
		if app.Application.TrackID != track.TrackID || !s.belongsToTenantLocked(app, track.TenantID) {
			continue
		}
		if isRankingPoolStatus(app.Application.Status) {
			candidates = append(candidates, app)
		}
	}
	return computeTrackSimulation(track, candidates)
}

func (s *DemoStore) collectAppPaymentsLocked(applicationID uuid.UUID) []PaymentTransaction {
	var list []PaymentTransaction
	for _, p := range s.payments {
		if p.ApplicationID == applicationID {
			list = append(list, *p)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].ID.String() > list[j].ID.String()
		}
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	if list == nil {
		return []PaymentTransaction{}
	}
	return list
}

func (s *DemoStore) syncAppBillingLocked(app *ApplicationDetail) {
	app.Payments = s.collectAppPaymentsLocked(app.Application.ID)
	if scores, ok := s.scores[app.Application.ID]; ok {
		app.Scores = slices.Clone(scores)
	}
	var appSchedules []AssessmentSchedule
	for _, rec := range s.schedules {
		if rec.Schedule.ApplicationID == app.Application.ID {
			appSchedules = append(appSchedules, rec.Schedule)
		}
	}
	sort.Slice(appSchedules, func(i, j int) bool {
		return appSchedules[i].ScheduledAt.Before(appSchedules[j].ScheduledAt)
	})
	if appSchedules == nil {
		appSchedules = []AssessmentSchedule{}
	}
	app.Schedules = appSchedules
}

func (s *DemoStore) ListAssessmentSchedules(_ context.Context, tenantID uuid.UUID, filter AssessmentScheduleFilter) ([]AssessmentSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []AssessmentSchedule
	for _, rec := range s.schedules {
		if rec.TenantID != tenantID {
			continue
		}
		if filter.ApplicationID != nil && rec.Schedule.ApplicationID != *filter.ApplicationID {
			continue
		}
		if filter.UnitID != nil && rec.Schedule.UnitID != *filter.UnitID {
			continue
		}
		if len(filter.AllowedUnitIDs) > 0 && !slices.Contains(filter.AllowedUnitIDs, rec.Schedule.UnitID) {
			continue
		}
		if filter.TrackID != nil {
			app, ok := s.apps[rec.Schedule.ApplicationID]
			if !ok || app.Application.TrackID != *filter.TrackID {
				continue
			}
		}
		out = append(out, rec.Schedule)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ScheduledAt.Equal(out[j].ScheduledAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ScheduledAt.Before(out[j].ScheduledAt)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	if out == nil {
		out = []AssessmentSchedule{}
	}
	return out, nil
}

func (s *DemoStore) CreateAssessmentSchedules(_ context.Context, tenantID uuid.UUID, inputs []AssessmentSchedule, actor ActorContext) ([]AssessmentSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	out := make([]AssessmentSchedule, 0, len(inputs))
	for _, in := range inputs {
		app, ok := s.apps[in.ApplicationID]
		if !ok || !s.belongsToTenantLocked(app, tenantID) {
			return nil, ErrApplicationNotFound
		}
		rec := in
		rec.RegistrationNo = app.Application.RegistrationNo
		rec.StudentName = app.Application.StudentName
		rec.UnitID = app.Application.UnitID
		rec.UnitName = app.Application.UnitName
		rec.TrackName = app.Application.TrackName
		rec.CreatedAt = now
		rec.UpdatedAt = now
		s.schedules[rec.ID] = &demoScheduleRecord{TenantID: tenantID, Schedule: rec}
		reason := fmt.Sprintf("Jadwal %s (%s)", rec.SessionName, rec.ScheduledAt.UTC().Format("02 Jan 2006 15:04 UTC"))
		appendAuditLocked(app, AuditWrite{
			TenantID: tenantID, Application: &app.Application.ID, Actor: actor,
			ActionType: "assessment_scheduled", EntityType: "assessment_schedule", EntityID: rec.ID,
			Reason: &reason,
		}, now)
		s.syncAppBillingLocked(app)
		out = append(out, rec)
	}
	return out, nil
}

func (s *DemoStore) DeleteAssessmentSchedule(_ context.Context, tenantID, scheduleID uuid.UUID, actor ActorContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.schedules[scheduleID]
	if !ok || rec.TenantID != tenantID {
		return ErrScheduleNotFound
	}
	appID := rec.Schedule.ApplicationID
	delete(s.schedules, scheduleID)
	if app, exists := s.apps[appID]; exists {
		appendAuditLocked(app, AuditWrite{
			TenantID: tenantID, Application: &appID, Actor: actor,
			ActionType: "assessment_schedule_deleted", EntityType: "assessment_schedule", EntityID: scheduleID,
		}, time.Now().UTC())
		s.syncAppBillingLocked(app)
	}
	return nil
}

func isPreDecisionStatus(st domain.ApplicationStatus) bool {
	return st == domain.StatusVerified || st == domain.StatusSelectionInProgress || st == domain.StatusRanked
}

func isRankingPoolStatus(st domain.ApplicationStatus) bool {
	return isPreDecisionStatus(st) || st == domain.StatusAccepted || st == domain.StatusWaitlisted || st == domain.StatusRejected
}

