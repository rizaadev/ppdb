package public_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/handlers"
	"github.com/pbdb/platform/internal/modules/public"
)

type stubDNSResolver struct {
	txtRecords map[string][]string
	cname      map[string]string
}

func (s *stubDNSResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	return s.txtRecords[strings.ToLower(strings.TrimSpace(name))], nil
}

func (s *stubDNSResolver) LookupCNAME(_ context.Context, host string) (string, error) {
	return s.cname[strings.ToLower(strings.TrimSpace(host))], nil
}

func TestDemoStore_SettingsPaymentMaskLogoDomainAndStaff(t *testing.T) {
	ctx := context.Background()
	store := public.NewDemoStore()
	spmbID := uuid.MustParse("01960000-0000-7000-8000-000000000001")

	// 1. Changing custom domain resets verified flag and issues verification token.
	newDomain := "ppdb.baru.sch.id"
	secret := "SB-Mid-server-SuperSecretKey1234"
	clientKey := "SB-Mid-client-Public1234"
	sandbox := true
	updated, err := store.UpdateTenantSettings(ctx, spmbID, handlers.UpdateTenantSettingsInput{
		CustomDomain: &newDomain,
		PaymentConfig: &handlers.PaymentConfigUpdate{
			MidtransClientKey: &clientKey,
			MidtransServerKey: &secret,
			MidtransSandbox:   &sandbox,
		},
	})
	if err != nil {
		t.Fatalf("UpdateTenantSettings failed: %v", err)
	}
	if updated.DomainVerified {
		t.Fatal("expected DomainVerified to reset to false when custom domain changed")
	}
	if updated.DomainVerificationToken == nil || !strings.HasPrefix(*updated.DomainVerificationToken, "spmb-verify-") {
		t.Fatalf("expected verification token, got %+v", updated.DomainVerificationToken)
	}
	if !updated.PaymentConfig.MidtransServerKeyConfigured || updated.PaymentConfig.MidtransServerKeyMasked != "****1234" {
		t.Fatalf("expected masked Midtrans server key, got %+v", updated.PaymentConfig)
	}

	// Verify DNS against stub resolver
	slug, domPtr, tokPtr, err := store.GetTenantDomainInfo(ctx, spmbID)
	if err != nil || slug != "demo-spmb" || domPtr == nil || tokPtr == nil {
		t.Fatalf("GetTenantDomainInfo failed: slug=%s dom=%v tok=%v err=%v", slug, domPtr, tokPtr, err)
	}
	resolver := &stubDNSResolver{
		txtRecords: map[string][]string{
			"_spmb-verify." + *domPtr: {*tokPtr},
		},
	}
	dnsRes := handlers.VerifyCustomDomainDNS(ctx, resolver, *domPtr, *tokPtr, slug+".spmb.id")
	if !dnsRes.Verified {
		t.Fatalf("expected DNS verification to succeed, got %+v", dnsRes)
	}
	verifiedSettings, err := store.MarkTenantDomainVerified(ctx, spmbID)
	if err != nil || !verifiedSettings.DomainVerified {
		t.Fatalf("MarkTenantDomainVerified failed: %+v err=%v", verifiedSettings, err)
	}

	// 2. Logo storage key computes public proxy URL
	logoKey := spmbID.String() + "/branding/01960000-0000-7000-8000-000000000099-logo.png"
	afterLogo, err := store.SetTenantLogoKey(ctx, spmbID, &logoKey)
	if err != nil {
		t.Fatalf("SetTenantLogoKey failed: %v", err)
	}
	if afterLogo.ThemeConfig.LogoURL == nil || !strings.HasPrefix(*afterLogo.ThemeConfig.LogoURL, "/api/v1/public/logo?v=") {
		t.Fatalf("expected proxy logo URL, got %+v", afterLogo.ThemeConfig.LogoURL)
	}

	// 3. Staff CRUD
	staff, err := store.ListStaff(ctx, spmbID)
	if err != nil || len(staff) == 0 {
		t.Fatalf("expected initial seeded staff, got len=%d err=%v", len(staff), err)
	}
	phone := "081299988877"
	createdStaff, err := store.CreateOrAssignStaff(ctx, spmbID, handlers.CreateStaffInput{
		FullName:      "Panitia Baru",
		Email:         "panitia.baru@smaharapan.sch.id",
		WhatsAppPhone: &phone,
		Password:      "Rahasia99!",
		Role:          domain.RoleVerifier,
	})
	if err != nil {
		t.Fatalf("CreateOrAssignStaff failed: %v", err)
	}
	if createdStaff.Roles[0].Role != domain.RoleVerifier {
		t.Fatalf("expected verifier role, got %+v", createdStaff.Roles)
	}
	inactive := false
	actorID := uuid.Must(uuid.NewV7())
	updatedStaff, err := store.UpdateStaff(ctx, spmbID, actorID, createdStaff.UserID, handlers.UpdateStaffInput{
		IsActive: &inactive,
	})
	if err != nil || updatedStaff.Roles[0].IsActive {
		t.Fatalf("expected staff role deactivated, got %+v err=%v", updatedStaff, err)
	}
}

