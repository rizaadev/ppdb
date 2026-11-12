package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pbdb/platform/internal/httpapi/respond"
)

// SecurityHeaders injects baseline defensive HTTP headers against MIME sniffing, framing, and referrer leakage.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(self)")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			next.ServeHTTP(w, r)
		})
	}
}

// TrustedProxyRealIP resolves the real client IP from X-Forwarded-For / X-Real-IP ONLY when
// the immediate TCP peer (r.RemoteAddr) belongs to trustedProxyCIDRs (e.g., Caddy / Docker network).
// Spoofed headers sent by untrusted peers are completely ignored.
func TrustedProxyRealIP(trustedProxyCIDRs []string) func(http.Handler) http.Handler {
	nets := make([]*net.IPNet, 0, len(trustedProxyCIDRs))
	for _, cidr := range trustedProxyCIDRs {
		if _, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr)); err == nil {
			nets = append(nets, ipNet)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peerIP := parseIPOnly(r.RemoteAddr)
			if peerIP != nil && isTrustedIP(peerIP, nets) {
				if resolved := resolveClientIPFromHeaders(r, nets); resolved != "" {
					r.RemoteAddr = resolved
				} else {
					r.RemoteAddr = peerIP.String()
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func resolveClientIPFromHeaders(r *http.Request, trusted []*net.IPNet) string {
	// Parse X-Forwarded-For right-to-left, skipping trusted reverse proxies, so spoofed leftmost values are ignored.
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := parseIPOnly(parts[i])
			if candidate == nil {
				continue
			}
			if !isTrustedIP(candidate, trusted) {
				return candidate.String()
			}
		}
		// All hops were trusted proxies; pick the first valid IP.
		for _, part := range parts {
			if candidate := parseIPOnly(part); candidate != nil {
				return candidate.String()
			}
		}
	}

	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if candidate := parseIPOnly(xri); candidate != nil {
			return candidate.String()
		}
	}
	return ""
}

func isTrustedIP(ip net.IP, trusted []*net.IPNet) bool {
	for _, cidr := range trusted {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func parseIPOnly(raw string) net.IP {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = host
	}
	trimmed = strings.Trim(trimmed, "[]")
	return net.ParseIP(trimmed)
}

// IPRateLimiter enforces a sliding-window per-client-IP request cap on abuse-sensitive public/auth endpoints.
type IPRateLimiter struct {
	mu          sync.Mutex
	window      time.Duration
	maxRequests int
	buckets     map[string][]time.Time
	now         func() time.Time
}

// NewIPRateLimiter constructs an in-memory per-IP rate limiter for a sliding time window.
func NewIPRateLimiter(window time.Duration, maxRequests int) *IPRateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	if maxRequests <= 0 {
		maxRequests = 40
	}
	return &IPRateLimiter{
		window:      window,
		maxRequests: maxRequests,
		buckets:     make(map[string][]time.Time),
		now:         time.Now,
	}
}

// Middleware wraps an HTTP handler and rejects bursts beyond maxRequests within the sliding window.
func (l *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractClientIP(r)
		allowed, remaining, retryAfter := l.allow(ip)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(l.maxRequests))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			secs := int(retryAfter.Seconds())
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			respond.Error(w, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Terlalu banyak permintaan dalam waktu singkat. Silakan coba lagi sebentar.", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *IPRateLimiter) allow(ip string) (bool, int, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now().UTC()
	cutoff := now.Add(-l.window)

	// Guard unbounded growth on high-cardinality internet traffic.
	if len(l.buckets) > 4096 {
		for k, stamps := range l.buckets {
			if len(stamps) == 0 || stamps[len(stamps)-1].Before(cutoff) {
				delete(l.buckets, k)
			}
		}
	}

	stamps := l.buckets[ip]
	kept := stamps[:0]
	for _, ts := range stamps {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}

	if len(kept) >= l.maxRequests {
		l.buckets[ip] = kept
		retryAfter := kept[0].Add(l.window).Sub(now)
		return false, 0, retryAfter
	}

	kept = append(kept, now)
	l.buckets[ip] = kept
	remaining := l.maxRequests - len(kept)
	if remaining < 0 {
		remaining = 0
	}
	return true, remaining, 0
}

func extractClientIP(r *http.Request) string {
	if ip := parseIPOnly(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	if addr := strings.TrimSpace(r.RemoteAddr); addr != "" {
		return addr
	}
	return "unknown"
}
