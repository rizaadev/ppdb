package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSecurityHeadersAndIPRateLimiter(t *testing.T) {
	limiter := NewIPRateLimiter(time.Minute, 2)
	handler := TrustedProxyRealIP([]string{"172.16.0.0/12", "127.0.0.1/32"})(
		SecurityHeaders()(limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))),
	)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		// Untrusted peer attempting X-Forwarded-For spoofing must still share its real socket bucket.
		req.Header.Set("X-Forwarded-For", "198.51.100."+string(rune('1'+i)))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d", i+1, rec.Code)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("missing nosniff header")
		}
		if rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("missing DENY frame header")
		}
	}

	blockedReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	blockedReq.RemoteAddr = "203.0.113.10:12345"
	blockedReq.Header.Set("X-Forwarded-For", "198.51.100.99")
	blockedRec := httptest.NewRecorder()
	handler.ServeHTTP(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd burst request expected 429 despite spoofed XFF, got %d", blockedRec.Code)
	}
	if blockedRec.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header on 429 response")
	}

	// Requests arriving via trusted Caddy container IP must use the rightmost untrusted XFF hop.
	for i := 0; i < 2; i++ {
		proxyReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
		proxyReq.RemoteAddr = "172.18.0.5:44123"
		proxyReq.Header.Set("X-Forwarded-For", "9.9.9.9, 198.51.100.50")
		proxyRec := httptest.NewRecorder()
		handler.ServeHTTP(proxyRec, proxyReq)
		if proxyRec.Code != http.StatusOK {
			t.Fatalf("proxied client %d expected 200, got %d", i+1, proxyRec.Code)
		}
	}

	otherProxiedClient := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	otherProxiedClient.RemoteAddr = "172.18.0.5:44124"
	otherProxiedClient.Header.Set("X-Forwarded-For", "198.51.100.51")
	otherRec := httptest.NewRecorder()
	handler.ServeHTTP(otherRec, otherProxiedClient)
	if otherRec.Code != http.StatusOK {
		t.Fatalf("different client behind same trusted proxy should not share rate limit bucket, got %d", otherRec.Code)
	}
}
