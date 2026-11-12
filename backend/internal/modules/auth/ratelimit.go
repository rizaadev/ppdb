package auth

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RateLimiter checks and enforces cooldown + sliding-window quotas for OTP requests.
type RateLimiter interface {
	AllowCtx(ctx context.Context, key string, now time.Time) (bool, time.Duration)
}

type rateBucket struct {
	lastSentAt time.Time
	windowHits []time.Time
}

// OTPRateLimiter prevents WhatsApp gateway abuse using per-phone cooldown and sliding window limits in memory.
type OTPRateLimiter struct {
	mu          sync.Mutex
	cooldown    time.Duration
	window      time.Duration
	maxInWindow int
	buckets     map[string]*rateBucket
}

// NewOTPRateLimiter initializes an in-memory OTP limiter with safe default thresholds.
func NewOTPRateLimiter(cooldown, window time.Duration, maxInWindow int) *OTPRateLimiter {
	return &OTPRateLimiter{
		cooldown:    cooldown,
		window:      window,
		maxInWindow: maxInWindow,
		buckets:     make(map[string]*rateBucket),
	}
}

// AllowCtx implements RateLimiter using in-memory sliding window tracking.
func (l *OTPRateLimiter) AllowCtx(_ context.Context, key string, now time.Time) (bool, time.Duration) {
	return l.Allow(key, now)
}

// Allow checks and records an OTP request for the given key (e.g. phone number).
func (l *OTPRateLimiter) Allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) > 4096 {
		l.pruneLocked(now)
	}

	b, exists := l.buckets[key]
	if !exists {
		l.buckets[key] = &rateBucket{
			lastSentAt: now,
			windowHits: []time.Time{now},
		}
		return true, 0
	}

	if sinceLast := now.Sub(b.lastSentAt); sinceLast < l.cooldown {
		return false, l.cooldown - sinceLast
	}

	cutoff := now.Add(-l.window)
	kept := b.windowHits[:0]
	for _, ts := range b.windowHits {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	b.windowHits = kept

	if len(b.windowHits) >= l.maxInWindow {
		retryAfter := b.windowHits[0].Add(l.window).Sub(now)
		return false, retryAfter
	}

	b.lastSentAt = now
	b.windowHits = append(b.windowHits, now)
	return true, 0
}

func (l *OTPRateLimiter) pruneLocked(now time.Time) {
	cutoff := now.Add(-l.window)
	for k, b := range l.buckets {
		if b.lastSentAt.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

// PGOTPRateLimiter enforces distributed OTP cooldown and sliding-window limits across replicas via otp_challenges.
type PGOTPRateLimiter struct {
	pool        *pgxpool.Pool
	cooldown    time.Duration
	window      time.Duration
	maxInWindow int
}

// NewPGOTPRateLimiter constructs a PostgreSQL-backed distributed OTP rate limiter.
func NewPGOTPRateLimiter(pool *pgxpool.Pool, cooldown, window time.Duration, maxInWindow int) *PGOTPRateLimiter {
	return &PGOTPRateLimiter{
		pool:        pool,
		cooldown:    cooldown,
		window:      window,
		maxInWindow: maxInWindow,
	}
}

// AllowCtx queries recent otp_challenges rows for the phone number and enforces cooldown + window caps.
func (l *PGOTPRateLimiter) AllowCtx(ctx context.Context, key string, now time.Time) (bool, time.Duration) {
	if l.pool == nil {
		return true, 0
	}

	var count int
	var minCreatedAt, maxCreatedAt *time.Time
	cutoff := now.Add(-l.window)

	err := l.pool.QueryRow(ctx, `
		SELECT COUNT(*), MIN(created_at), MAX(created_at)
		FROM otp_challenges
		WHERE whatsapp_phone = $1 AND created_at > $2
	`, key, cutoff).Scan(&count, &minCreatedAt, &maxCreatedAt)
	if err != nil {
		// Fail closed briefly on unexpected DB errors so gateway spam cannot slip through undetected.
		return false, 5 * time.Second
	}

	if maxCreatedAt != nil {
		if sinceLast := now.Sub(*maxCreatedAt); sinceLast < l.cooldown {
			return false, l.cooldown - sinceLast
		}
	}
	if count >= l.maxInWindow && minCreatedAt != nil {
		retryAfter := minCreatedAt.Add(l.window).Sub(now)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter
	}

	return true, 0
}
