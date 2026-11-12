package admissions

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/modules/catalog"
)

// NewSeededDemoStore returns a DemoStore pre-populated with realistic sample applications,
// manual payment invoices, and verified selection candidates for interactive UI preview without Postgres.
func NewSeededDemoStore() *DemoStore {
	s := NewDemoStore()
	now := time.Now().UTC()
	guardianID := uuid.MustParse("01960000-0000-7000-8001-000000000001")

	seedForTenant := func(tenantSlug string, includeManualPay bool) {
		var tenant catalog.TenantDef
		var found bool
		for _, t := range catalog.DemoTenants() {
			if t.Slug == tenantSlug {
				tenant = t
				found = true
				break
			}
		}
		if !found {
			return
		}
		units, periods := catalog.PortalForTenant(tenant.Slug, tenant.Name, tenant.TerminologyPreset)
		if len(units) == 0 || len(periods) == 0 || len(periods[0].Tracks) == 0 {
			return
		}
		unit := units[0]
		period := periods[0]
		track0 := period.Tracks[0]

		lat0 := unit.Latitude + 0.0042
		lon0 := unit.Longitude + 0.0031
		dist0 := int(domain.HaversineMeters(unit.Latitude, unit.Longitude, lat0, lon0))
		nisn1 := "0098765432"
		bdate1 := "2010-08-14"
		gender1 := "L"
		sub1 := now.Add(-18 * time.Hour)
		app1ID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-seed-app-1"))

		s.periodRegSeq[period.ID] = 1
		reg1 := formatRegistrationNo(track0.Code, period.AcademicYear, 1)
		reqs1 := toCatalogTrack(&catalog.TrackLookup{Tenant: tenant, Unit: unit, Period: period, Track: track0}).Requirements

		docID1 := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-doc-kk"))
		docID2 := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-doc-akta"))
		name1 := "Ahmad Fadhil Ramadhan"
		app1 := &ApplicationDetail{
			Application: ApplicationSummary{
				ID:             app1ID,
				RegistrationNo: reg1,
				Status:         domain.StatusSubmitted,
				UnitID:         unit.ID,
				UnitName:       unit.Name,
				PeriodID:       period.ID,
				PeriodName:     period.Name,
				AcademicYear:   period.AcademicYear,
				TrackID:        track0.ID,
				TrackName:      track0.DisplayName,
				StudentName:    &name1,
				NISN:           &nisn1,
				DistanceMeters: &dist0,
				Version:        3,
				SubmittedAt:    &sub1,
				CreatedAt:      now.Add(-24 * time.Hour),
			},
			GuardianUserID:       guardianID,
			UnitLatitude:         &unit.Latitude,
			UnitLongitude:        &unit.Longitude,
			RequiresCoordinates:  track0.RequiresCoordinates,
			RequiresAchievements: track0.RequiresAchievements,
			RegistrationFeeIDR:   track0.RegistrationFeeIDR,
			ReEnrollmentFeeIDR:   track0.ReEnrollmentFeeIDR,
			ExtraAnswers:         map[string]any{},
			Profile: &ApplicantProfile{
				FullName:    name1,
				NISN:        &nisn1,
				BirthDate:   &bdate1,
				Gender:      &gender1,
				Citizenship: "WNI",
				Latitude:    &lat0,
				Longitude:   &lon0,
			},
			Guardians:    []Guardian{},
			Achievements: []Achievement{},
			Requirements: reqs1,
			Documents: []Document{
				{
					ID:               docID1,
					RequirementID:    reqs1[0].ID,
					DocCode:          reqs1[0].DocCode,
					RequirementLabel: reqs1[0].Label,
					IsRequired:       reqs1[0].IsRequired,
					StorageKey:       fmt.Sprintf("%s/applications/%s/%s/kk-fadhil.pdf", tenant.ID, app1ID, reqs1[0].ID),
					OriginalFilename: "kk-fadhil.pdf",
					MimeType:         "application/pdf",
					SizeBytes:        412000,
					Status:           domain.DocStatusPending,
					Version:          1,
				},
				{
					ID:               docID2,
					RequirementID:    reqs1[1].ID,
					DocCode:          reqs1[1].DocCode,
					RequirementLabel: reqs1[1].Label,
					IsRequired:       reqs1[1].IsRequired,
					StorageKey:       fmt.Sprintf("%s/applications/%s/%s/akta-fadhil.jpg", tenant.ID, app1ID, reqs1[1].ID),
					OriginalFilename: "akta-fadhil.jpg",
					MimeType:         "image/jpeg",
					SizeBytes:        289000,
					Status:           domain.DocStatusPending,
					Version:          1,
				},
			},
			Payments: []PaymentTransaction{},
			Scores:   []AssessmentScore{},
			AuditLogs: []AuditEntry{
				{
					ID:          uuid.Must(uuid.NewV7()),
					ActorUserID: &guardianID,
					ActorRole:   "applicant",
					ActionType:  "application_submitted",
					EntityType:  "application",
					EntityID:    app1ID,
					CreatedAt:   sub1,
				},
			},
		}
		s.apps[app1ID] = app1

		// Verified candidate ready for scoring & ranking simulation
		s.periodRegSeq[period.ID] = 2
		app2ID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-seed-app-2"))
		name2 := "Nabila Azzahra Putri"
		nisn2 := "0099123456"
		bdate2 := "2010-05-11"
		verAt := now.Add(-6 * time.Hour)
		score2 := "88.50"
		dist2 := 480
		app2 := &ApplicationDetail{
			Application: ApplicationSummary{
				ID:             app2ID,
				RegistrationNo: formatRegistrationNo(track0.Code, period.AcademicYear, 2),
				Status:         domain.StatusVerified,
				UnitID:         unit.ID,
				UnitName:       unit.Name,
				PeriodID:       period.ID,
				PeriodName:     period.Name,
				AcademicYear:   period.AcademicYear,
				TrackID:        track0.ID,
				TrackName:      track0.DisplayName,
				StudentName:    &name2,
				NISN:           &nisn2,
				DistanceMeters: &dist2,
				FinalScore:     &score2,
				Version:        4,
				SubmittedAt:    &sub1,
				VerifiedAt:     &verAt,
				CreatedAt:      now.Add(-30 * time.Hour),
			},
			GuardianUserID:       uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-guardian-2")),
			UnitLatitude:         &unit.Latitude,
			UnitLongitude:        &unit.Longitude,
			RequiresCoordinates:  track0.RequiresCoordinates,
			RequiresAchievements: track0.RequiresAchievements,
			RegistrationFeeIDR:   track0.RegistrationFeeIDR,
			ReEnrollmentFeeIDR:   track0.ReEnrollmentFeeIDR,
			ExtraAnswers:         map[string]any{},
			Profile: &ApplicantProfile{
				FullName:    name2,
				NISN:        &nisn2,
				BirthDate:   &bdate2,
				Citizenship: "WNI",
			},
			Guardians:    []Guardian{},
			Achievements: []Achievement{},
			Requirements: slices.Clone(reqs1),
			Documents:    []Document{},
			Payments:     []PaymentTransaction{},
			Scores:       []AssessmentScore{},
			AuditLogs:    []AuditEntry{},
		}
		s.apps[app2ID] = app2

		if includeManualPay && track0.RegistrationFeeIDR > 0 {
			s.periodRegSeq[period.ID] = 3
			app3ID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-seed-app-3"))
			name3 := "Bintang Pratama Putra"
			nisn3 := "0099887766"
			app3 := &ApplicationDetail{
				Application: ApplicationSummary{
					ID:             app3ID,
					RegistrationNo: formatRegistrationNo(track0.Code, period.AcademicYear, 3),
					Status:         domain.StatusAwaitingPayment,
					UnitID:         unit.ID,
					UnitName:       unit.Name,
					PeriodID:       period.ID,
					PeriodName:     period.Name,
					AcademicYear:   period.AcademicYear,
					TrackID:        track0.ID,
					TrackName:      track0.DisplayName,
					StudentName:    &name3,
					NISN:           &nisn3,
					Version:        1,
					CreatedAt:      now.Add(-4 * time.Hour),
				},
				GuardianUserID:       guardianID,
				UnitLatitude:         &unit.Latitude,
				UnitLongitude:        &unit.Longitude,
				RequiresCoordinates:  track0.RequiresCoordinates,
				RequiresAchievements: track0.RequiresAchievements,
				RegistrationFeeIDR:   track0.RegistrationFeeIDR,
				ReEnrollmentFeeIDR:   track0.ReEnrollmentFeeIDR,
				ExtraAnswers:         map[string]any{},
				Profile:              &ApplicantProfile{FullName: name3, NISN: &nisn3, Citizenship: "WNI"},
				Guardians:            []Guardian{},
				Achievements:         []Achievement{},
				Requirements:         slices.Clone(reqs1),
				Documents:            []Document{},
				AuditLogs:            []AuditEntry{},
			}
			payID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-seed-pay-3"))
			extRef := fmt.Sprintf("INV-%s-DEMO01", app3.Application.RegistrationNo)
			proofKey := fmt.Sprintf("%s/payments/%s/%s/struk-transfer.jpg", tenant.ID, app3ID, payID)
			expires := now.Add(18 * time.Hour)
			pay := &PaymentTransaction{
				ID:              payID,
				ApplicationID:   app3ID,
				RegistrationNo:  app3.Application.RegistrationNo,
				StudentName:     &name3,
				UnitID:          unit.ID,
				UnitName:        unit.Name,
				TrackName:       track0.DisplayName,
				Purpose:         PaymentPurposeRegistration,
				Method:          PaymentMethodManualTransfer,
				ExternalRef:     &extRef,
				AmountIDR:       track0.RegistrationFeeIDR,
				Status:          PaymentStatusPending,
				ProofStorageKey: &proofKey,
				ExpiresAt:       &expires,
				CreatedAt:       now.Add(-3 * time.Hour),
			}
			s.payments[payID] = pay
			s.apps[app3ID] = app3
			s.syncAppBillingLocked(app3)
		}

		// Enrolled graduate sample so national exports (Dapodik/EMIS/CSV) produce complete rows immediately.
		s.periodRegSeq[period.ID] = 4
		app4ID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-seed-app-4"))
		name4 := "Rafa Aditya Pratama"
		nik4 := "3174011203110004"
		kk4 := "3174010809100021"
		nisn4 := "0102233445"
		bplace4 := "Jakarta"
		bdate4 := "2010-08-19"
		gender4 := "L"
		rel4 := "Islam"
		addr4 := "Jl. Ahmad Yani No. 18"
		rt4 := "003"
		rw4 := "005"
		vil4 := "Cawang"
		dist4 := "Kramat Jati"
		reg4 := "Jakarta Timur"
		prov4 := "DKI Jakarta"
		zip4 := "13630"
		prev4 := "SD Negeri 01 Cawang"
		npsn4 := "20103412"
		gradYr4 := int16(2026)
		score4 := "92.75"
		rank4 := 1
		sub4 := now.Add(-72 * time.Hour)
		ver4 := now.Add(-48 * time.Hour)
		dec4 := now.Add(-24 * time.Hour)
		fNik := "3174011405810002"
		fOcc := "Pegawai Swasta"
		fInc := "5_10jt"
		fWA := "6281290001104"
		mNik := "3174015408840003"
		mOcc := "Guru"
		mInc := "3_5jt"
		mWA := "6281290001105"
		fromAcc := string(domain.StatusReEnrollment)
		toEnr := string(domain.StatusEnrolled)
		app4 := &ApplicationDetail{
			Application: ApplicationSummary{
				ID:                 app4ID,
				RegistrationNo:     formatRegistrationNo(track0.Code, period.AcademicYear, 4),
				Status:             domain.StatusEnrolled,
				UnitID:             unit.ID,
				UnitName:           unit.Name,
				PeriodID:           period.ID,
				PeriodName:         period.Name,
				AcademicYear:       period.AcademicYear,
				TrackID:            track0.ID,
				TrackName:          track0.DisplayName,
				StudentName:        &name4,
				NISN:               &nisn4,
				DistanceMeters:     &dist2,
				FinalScore:         &score4,
				RankPosition:       &rank4,
				Version:            6,
				SubmittedAt:        &sub4,
				VerifiedAt:         &ver4,
				DecidedAt:          &dec4,
				ResultsPublishedAt: &dec4,
				CreatedAt:          now.Add(-96 * time.Hour),
			},
			GuardianUserID:       uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-guardian-4")),
			UnitLatitude:         &unit.Latitude,
			UnitLongitude:        &unit.Longitude,
			RequiresCoordinates:  track0.RequiresCoordinates,
			RequiresAchievements: track0.RequiresAchievements,
			RegistrationFeeIDR:   track0.RegistrationFeeIDR,
			ReEnrollmentFeeIDR:   track0.ReEnrollmentFeeIDR,
			ExtraAnswers:         map[string]any{"ukuran_seragam": "M"},
			Profile: &ApplicantProfile{
				FullName:       name4,
				NIK:            &nik4,
				KKNumber:       &kk4,
				NISN:           &nisn4,
				BirthPlace:     &bplace4,
				BirthDate:      &bdate4,
				Gender:         &gender4,
				Religion:       &rel4,
				Citizenship:    "WNI",
				AddressStreet:  &addr4,
				RT:             &rt4,
				RW:             &rw4,
				VillageName:    &vil4,
				DistrictName:   &dist4,
				RegencyName:    &reg4,
				ProvinceName:   &prov4,
				PostalCode:     &zip4,
				PrevSchoolName: &prev4,
				PrevSchoolNPSN: &npsn4,
				GraduationYear: &gradYr4,
			},
			Guardians: []Guardian{
				{
					ID:               uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-father-4")),
					Relation:         "father",
					FullName:         "Budi Santoso",
					NIK:              &fNik,
					Occupation:       &fOcc,
					IncomeBracket:    &fInc,
					PhoneWhatsApp:    &fWA,
					IsPrimaryContact: true,
				},
				{
					ID:            uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantSlug+"-mother-4")),
					Relation:      "mother",
					FullName:      "Rina Kartika",
					NIK:           &mNik,
					Occupation:    &mOcc,
					IncomeBracket: &mInc,
					PhoneWhatsApp: &mWA,
				},
			},
			Achievements: []Achievement{},
			Requirements: slices.Clone(reqs1),
			Documents:    []Document{},
			Payments:     []PaymentTransaction{},
			Scores:       []AssessmentScore{},
			AuditLogs: []AuditEntry{
				{
					ID:         uuid.Must(uuid.NewV7()),
					ActorRole:  "applicant",
					ActionType: "re_enrollment_confirmed",
					EntityType: "application",
					EntityID:   app4ID,
					FromStatus: &fromAcc,
					ToStatus:   &toEnr,
					CreatedAt:  now.Add(-12 * time.Hour),
				},
			},
		}
		s.apps[app4ID] = app4
	}

	seedForTenant("demo-spmb", false)
	seedForTenant("demo-pmbm", true)
	seedForTenant("demo-mandiri", true)
	return s
}

