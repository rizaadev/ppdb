package handlers

import (
	"context"
	"net"
	"strings"
	"time"
)

// DNSResolver abstracts DNS TXT/CNAME lookups for custom domain ownership verification.
type DNSResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
}

// NetDNSResolver implements DNSResolver using Go's standard network resolver with a bounded timeout.
type NetDNSResolver struct{}

func (NetDNSResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return (&net.Resolver{}).LookupTXT(c, name)
}

func (NetDNSResolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return (&net.Resolver{}).LookupCNAME(c, host)
}

// DomainVerificationResult summarizes a real DNS TXT/CNAME verification check for a custom domain.
type DomainVerificationResult struct {
	Domain              string   `json:"domain"`
	Verified            bool     `json:"verified"`
	ExpectedTXTHost     string   `json:"expected_txt_host"`
	ExpectedTXTValue    string   `json:"expected_txt_value"`
	ExpectedCNAMETarget string   `json:"expected_cname_target,omitempty"`
	ObservedTXT         []string `json:"observed_txt"`
	ObservedCNAME       string   `json:"observed_cname,omitempty"`
	Message             string   `json:"message"`
}

// VerifyCustomDomainDNS executes DNS TXT and CNAME lookups to confirm domain ownership before activating host routing.
func VerifyCustomDomainDNS(
	ctx context.Context,
	resolver DNSResolver,
	customDomain, verificationToken, expectedCNAMETarget string,
) DomainVerificationResult {
	if resolver == nil {
		resolver = NetDNSResolver{}
	}
	cleanDomain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(customDomain), "."))
	txtHost := "_spmb-verify." + cleanDomain
	cleanToken := strings.TrimSpace(verificationToken)
	cleanCNAME := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(expectedCNAMETarget), "."))

	res := DomainVerificationResult{
		Domain:              cleanDomain,
		ExpectedTXTHost:     txtHost,
		ExpectedTXTValue:    cleanToken,
		ExpectedCNAMETarget: cleanCNAME,
		ObservedTXT:         []string{},
	}

	txtMatched := false
	if txtRecords, err := resolver.LookupTXT(ctx, txtHost); err == nil {
		for _, rec := range txtRecords {
			trimmed := strings.TrimSpace(rec)
			res.ObservedTXT = append(res.ObservedTXT, trimmed)
			if cleanToken != "" && trimmed == cleanToken {
				txtMatched = true
			}
		}
	}
	if !txtMatched {
		if rootTXT, err := resolver.LookupTXT(ctx, cleanDomain); err == nil {
			for _, rec := range rootTXT {
				trimmed := strings.TrimSpace(rec)
				res.ObservedTXT = append(res.ObservedTXT, trimmed)
				if cleanToken != "" && trimmed == cleanToken {
					txtMatched = true
				}
			}
		}
	}

	cnameMatched := false
	if cname, err := resolver.LookupCNAME(ctx, cleanDomain); err == nil {
		observed := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cname), "."))
		if observed != "" && observed != cleanDomain {
			res.ObservedCNAME = observed
			if cleanCNAME != "" && observed == cleanCNAME {
				cnameMatched = true
			}
		}
	}

	if txtMatched || cnameMatched {
		res.Verified = true
		res.Message = "Verifikasi DNS berhasil. Domain kustom siap digunakan."
		return res
	}

	res.Message = "Record DNS belum cocok. Tambahkan TXT record di " + txtHost + " atau arahkan CNAME ke target platform lalu coba lagi."
	return res
}

