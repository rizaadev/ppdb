package httpapi

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pbdb/platform/internal/config"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/handlers"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/modules/admissions"
	"github.com/pbdb/platform/internal/modules/auth"
	"github.com/pbdb/platform/internal/storage"
)

// Dependencies bundles runtime collaborators needed by the HTTP router.
type Dependencies struct {
	Config            config.Config
	Pool              *pgxpool.Pool
	TenantLookup      middleware.TenantLookup
	PublicStore       handlers.PublicPortalStore
	PlatformStore     handlers.PlatformTenantStore
	SetupStore        handlers.AdminSetupStore
	ObjectStorage     storage.ObjectStorage
	DNSResolver       handlers.DNSResolver
	AuthService       *auth.Service
	AdmissionsService *admissions.Service
	JWTManager        *auth.JWTManager
	FrontendFS        fs.FS
}

// NewRouter builds the chi router for infrastructure, public API, protected routes, and SPA fallback.
func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.TrustedProxyRealIP(deps.Config.TrustedProxyCIDRs))
	if deps.Config.LogRequests {
		r.Use(chimiddleware.Logger)
	}
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.SecurityHeaders())
	r.Use(chimiddleware.Timeout(30 * time.Second))

	publicCheckLimiter := middleware.NewIPRateLimiter(time.Minute, 60)
	authBurstLimiter := middleware.NewIPRateLimiter(time.Minute, 60)

	infra := &handlers.InfraHandler{
		Pool:         deps.Pool,
		TenantLookup: deps.TenantLookup,
	}
	r.Get("/healthz", infra.Healthz)
	r.Get("/internal/tls/verify", infra.VerifyCaddyTLS)

	publicHandler := &handlers.PublicHandler{
		Store:          deps.PublicStore,
		Storage:        deps.ObjectStorage,
		Admissions:     deps.AdmissionsService,
		DocumentSecret: deps.Config.DocumentSecret,
	}
	authHandler := &handlers.AuthHandler{Service: deps.AuthService}
	applicantHandler := &handlers.ApplicantHandler{Service: deps.AdmissionsService}
	staffAppsHandler := &handlers.StaffApplicationsHandler{Service: deps.AdmissionsService}
	paymentsHandler := &handlers.PaymentsHandler{
		Service:           deps.AdmissionsService,
		WebhookSecret:     deps.Config.PaymentWebhookSecret,
		MidtransServerKey: deps.Config.MidtransServerKey,
	}
	selectionHandler := &handlers.SelectionHandler{Service: deps.AdmissionsService}
	exportsAuditHandler := &handlers.ExportsAuditHandler{Service: deps.AdmissionsService}
	adminTenantHandler := &handlers.AdminTenantHandler{
		PublicStore: deps.PublicStore,
		SetupStore:  deps.SetupStore,
		Storage:     deps.ObjectStorage,
		DNSResolver: deps.DNSResolver,
		BaseDomain:  deps.Config.BaseDomain,
	}
	platformHandler := &handlers.PlatformHandler{
		Store:       deps.PlatformStore,
		Auth:        deps.AuthService,
		DNSResolver: deps.DNSResolver,
		BaseDomain:  deps.Config.BaseDomain,
	}
	spaHandler := &handlers.SPAHandler{Assets: deps.FrontendFS}

	r.Handle("/assets/*", spaHandler)
	r.Handle("/platform", spaHandler)
	r.Handle("/platform/*", spaHandler)

	if localStore, ok := deps.ObjectStorage.(*storage.LocalStorage); ok && localStore != nil {
		r.Method(http.MethodPut, "/api/v1/storage/object", localStore)
		r.Method(http.MethodGet, "/api/v1/storage/object", localStore)
	}

	// Payment webhooks operate both centrally (Midtrans server notification URL without subdomain)
	// and when a tenant host or X-Tenant-Slug is provided in tests/local callbacks.
	r.With(middleware.OptionalTenantResolver(
		deps.TenantLookup,
		deps.Config.BaseDomain,
		deps.Config.AppEnv,
	)).Post("/api/v1/webhooks/payments/{provider}", paymentsHandler.HandleWebhook)

	r.With(authBurstLimiter.Middleware).Post("/api/v1/platform/auth/login", platformHandler.Login)
	r.Route("/api/v1/platform", func(pf chi.Router) {
		pf.Use(middleware.Authenticate(deps.JWTManager))
		pf.Use(middleware.RequirePlatformAdmin())
		pf.Get("/tenants", platformHandler.ListTenants)
		pf.Post("/tenants", platformHandler.CreateTenant)
		pf.Patch("/tenants/{tenantId}", platformHandler.UpdateTenant)
		pf.Post("/tenants/{tenantId}/verify-domain", platformHandler.VerifyDomain)
	})

	r.Group(func(tenantScoped chi.Router) {
		tenantScoped.Use(middleware.TenantResolver(
			deps.TenantLookup,
			deps.Config.BaseDomain,
			deps.Config.AppEnv,
			deps.Config.DevFallbackSlug,
		))

		tenantScoped.Route("/api/v1", func(api chi.Router) {
			api.Get("/public/portal", publicHandler.GetPortal)
			api.Get("/public/logo", publicHandler.GetLogo)
			api.With(publicCheckLimiter.Middleware).Get("/public/verify-document", publicHandler.VerifyDocument)
			api.With(publicCheckLimiter.Middleware).Post("/public/check-status", publicHandler.CheckStatus)

			api.With(authBurstLimiter.Middleware).Route("/auth", func(authRoutes chi.Router) {
				authRoutes.Post("/otp/request", authHandler.RequestOTP)
				authRoutes.Post("/otp/verify", authHandler.VerifyOTP)
				authRoutes.Post("/password/reset", authHandler.ResetPassword)
				authRoutes.Post("/login", authHandler.Login)
				authRoutes.Post("/refresh", authHandler.Refresh)
				authRoutes.Post("/logout", authHandler.Logout)
			})

			api.Group(func(protected chi.Router) {
				protected.Use(middleware.Authenticate(deps.JWTManager))
				protected.Get("/me", authHandler.Me)

				protected.Route("/applicant/applications", func(applicant chi.Router) {
					applicant.Get("/", applicantHandler.ListApplications)
					applicant.Post("/", applicantHandler.CreateApplication)
					applicant.Route("/{applicationId}", func(one chi.Router) {
						one.Get("/", applicantHandler.GetApplication)
						one.Patch("/", applicantHandler.UpdateProfile)
						one.Patch("/profile", applicantHandler.UpdateProfile)
						one.Put("/guardians", applicantHandler.ReplaceGuardians)
						one.Put("/achievements", applicantHandler.ReplaceAchievements)
						one.Post("/documents/presign-upload", applicantHandler.PresignUpload)
						one.Post("/documents/confirm", applicantHandler.ConfirmDocument)
						one.Get("/documents/{documentId}/presign-download", applicantHandler.PresignDownload)
						one.Post("/submit", applicantHandler.SubmitApplication)
						one.Get("/payments", paymentsHandler.ListApplicantPayments)
						one.Post("/payments", paymentsHandler.CreatePayment)
						one.Post("/payments/{paymentId}/presign-proof", paymentsHandler.PresignManualProof)
						one.Post("/payments/{paymentId}/manual-proof", paymentsHandler.AttachManualProof)
						one.Post("/payments/{paymentId}/sync", paymentsHandler.SyncApplicantPayment)
						one.Post("/re-enroll", paymentsHandler.ReEnroll)
					})
				})

				protected.Route("/admin", func(admin chi.Router) {
					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
						domain.RoleUnitAdmin,
						domain.RolePrincipal,
					)).Get("/tenant/settings", adminTenantHandler.GetSettings)

					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
					)).Patch("/tenant/settings", adminTenantHandler.UpdateSettings)

					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
					)).Post("/tenant/logo/presign", adminTenantHandler.PresignLogoUpload)

					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
					)).Post("/tenant/logo/confirm", adminTenantHandler.ConfirmLogoUpload)

					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
					)).Post("/tenant/domain/verify", adminTenantHandler.VerifyDomain)

					admin.Route("/staff", func(staff chi.Router) {
						staff.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RolePrincipal,
							domain.RoleExaminer,
						)).Get("/", adminTenantHandler.ListStaff)

						staff.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
						)).Post("/", adminTenantHandler.CreateStaff)

						staff.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
						)).Patch("/{userId}", adminTenantHandler.UpdateStaff)
					})

					admin.Route("/units", func(units chi.Router) {
						units.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RoleVerifier,
							domain.RoleExaminer,
							domain.RoleTreasurer,
							domain.RolePrincipal,
						)).Get("/", adminTenantHandler.ListUnits)

						units.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
						)).Post("/", adminTenantHandler.CreateUnit)

						units.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
						)).Patch("/{unitId}", adminTenantHandler.UpdateUnit)
					})

					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
						domain.RoleUnitAdmin,
						domain.RolePrincipal,
					)).Route("/periods", func(periods chi.Router) {
						periods.Post("/", adminTenantHandler.CreatePeriod)
						periods.Patch("/{periodId}", adminTenantHandler.UpdatePeriod)
						periods.Post("/{periodId}/publish-results", selectionHandler.PublishPeriodResults)
					})

					admin.Route("/tracks", func(tracks chi.Router) {
						tracks.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RolePrincipal,
						)).Post("/", adminTenantHandler.CreateTrack)

						tracks.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RolePrincipal,
						)).Patch("/{trackId}", adminTenantHandler.UpdateTrack)

						tracks.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RolePrincipal,
						)).Post("/{trackId}/simulate-ranking", selectionHandler.SimulateTrackRanking)

						tracks.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
						)).Put("/{trackId}/requirements", adminTenantHandler.ReplaceTrackRequirements)
					})

					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
						domain.RoleUnitAdmin,
						domain.RoleTreasurer,
						domain.RolePrincipal,
					)).Route("/payments", func(pay chi.Router) {
						pay.Get("/", paymentsHandler.ListAdminPayments)
						pay.Get("/{paymentId}/presign-proof", paymentsHandler.PresignAdminPaymentProof)
						pay.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RoleTreasurer,
						)).Post("/{paymentId}/verify-manual", paymentsHandler.VerifyManualPayment)
						pay.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RoleTreasurer,
						)).Post("/{paymentId}/sync", paymentsHandler.SyncAdminPayment)
					})

					admin.Route("/schedules", func(sched chi.Router) {
						sched.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RoleVerifier,
							domain.RoleExaminer,
							domain.RoleTreasurer,
							domain.RolePrincipal,
						)).Get("/", selectionHandler.ListSchedules)

						sched.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RoleExaminer,
							domain.RolePrincipal,
						)).Post("/", selectionHandler.CreateSchedule)

						sched.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RoleExaminer,
							domain.RolePrincipal,
						)).Delete("/{scheduleId}", selectionHandler.DeleteSchedule)
					})

					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
						domain.RoleUnitAdmin,
						domain.RoleVerifier,
						domain.RoleExaminer,
						domain.RoleTreasurer,
						domain.RolePrincipal,
					)).Route("/applications", func(apps chi.Router) {
						apps.Get("/", staffAppsHandler.List)
						apps.Get("/{applicationId}", staffAppsHandler.GetDetail)
						apps.Get("/{applicationId}/documents/{documentId}/presign-download", staffAppsHandler.PresignDocumentDownload)

						apps.Group(func(verifiers chi.Router) {
							verifiers.Use(middleware.RequireRoles(
								domain.RoleTenantAdmin,
								domain.RoleUnitAdmin,
								domain.RoleVerifier,
							))
							verifiers.Post("/claim-next", staffAppsHandler.ClaimNext)
							verifiers.Patch("/{applicationId}/documents/{documentId}/review", staffAppsHandler.ReviewDocument)
							verifiers.Post("/{applicationId}/complete-verification", staffAppsHandler.CompleteVerification)
						})

						apps.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RoleExaminer,
						)).Put("/{applicationId}/scores", selectionHandler.UpsertScores)

						apps.With(middleware.RequireRoles(
							domain.RoleTenantAdmin,
							domain.RoleUnitAdmin,
							domain.RolePrincipal,
						)).Post("/{applicationId}/promote-waitlist", selectionHandler.PromoteWaitlist)
					})

					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
						domain.RoleUnitAdmin,
						domain.RoleVerifier,
						domain.RoleTreasurer,
						domain.RolePrincipal,
					)).Route("/exports", func(exp chi.Router) {
						exp.Get("/", exportsAuditHandler.ListExports)
						exp.Post("/", exportsAuditHandler.CreateExport)
						exp.Get("/{jobId}", exportsAuditHandler.GetExport)
						exp.Get("/{jobId}/download", exportsAuditHandler.DownloadExport)
					})

					admin.With(middleware.RequireRoles(
						domain.RoleTenantAdmin,
						domain.RoleUnitAdmin,
						domain.RoleVerifier,
						domain.RoleTreasurer,
						domain.RolePrincipal,
					)).Get("/audit-logs", exportsAuditHandler.ListAuditLogs)
				})
			})
		})

		tenantScoped.Handle("/*", spaHandler)
	})

	return r
}
