package handlers

import (
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/pbdb/platform/internal/httpapi/middleware"
)

// SPAHandler serves the embedded static React Vite bundle and injects per-tenant HTML metadata on index.html.
type SPAHandler struct {
	Assets fs.FS
}

func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if cleanPath != "" && cleanPath != "." {
		if f, err := h.Assets.Open(cleanPath); err == nil {
			defer f.Close()
			if stat, statErr := f.Stat(); statErr == nil && !stat.IsDir() {
				if strings.HasPrefix(cleanPath, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, h.Assets, cleanPath)
				return
			}
		}
	}

	rawHTML, err := fs.ReadFile(h.Assets, "index.html")
	if err != nil {
		http.Error(w, "Frontend bundle not built", http.StatusServiceUnavailable)
		return
	}

	rendered := injectTenantMeta(string(rawHTML), middlewareTenantOrDefault(r))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(rendered))
}

func middlewareTenantOrDefault(r *http.Request) *middleware.ResolvedTenant {
	if t, ok := middleware.TenantFromContext(r.Context()); ok {
		return t
	}
	return &middleware.ResolvedTenant{
		Name:    "Portal Penerimaan Murid Baru",
		Tagline: "Sistem Penerimaan Murid Baru Terpadu (SPMB / PMBM / Mandiri)",
	}
}

func injectTenantMeta(htmlDoc string, t *middleware.ResolvedTenant) string {
	safeTitle := html.EscapeString(t.Name)
	desc := t.Tagline
	if desc == "" {
		desc = "Portal resmi penerimaan murid baru."
	}
	safeDesc := html.EscapeString(desc)

	metaBlock := fmt.Sprintf(
		"<title>%s</title>\n    <meta name=\"description\" content=\"%s\" />\n    <meta property=\"og:title\" content=\"%s\" />\n    <meta property=\"og:description\" content=\"%s\" />",
		safeTitle, safeDesc, safeTitle, safeDesc,
	)

	const marker = "<!-- TENANT_META -->"
	if strings.Contains(htmlDoc, marker) {
		return strings.Replace(htmlDoc, marker, metaBlock, 1)
	}
	return strings.Replace(htmlDoc, "</head>", "    "+metaBlock+"\n  </head>", 1)
}

