package handlers

import (
	"strings"
	"testing"

	"github.com/pbdb/platform/internal/httpapi/middleware"
)

func TestInjectTenantMetaEscapesHTML(t *testing.T) {
	baseHTML := `<html><head><!-- TENANT_META --></head><body></body></html>`
	out := injectTenantMeta(baseHTML, &middleware.ResolvedTenant{
		Name:    `SMA <script>alert(1)</script>`,
		Tagline: `Portal "Resmi" & Aman`,
	})

	if strings.Contains(out, "<script>") {
		t.Fatalf("expected HTML escaping on injected title, got: %s", out)
	}
	if !strings.Contains(out, `og:title`) || !strings.Contains(out, `Portal &#34;Resmi&#34; &amp; Aman`) {
		t.Fatalf("expected escaped og:description in output, got: %s", out)
	}
}

