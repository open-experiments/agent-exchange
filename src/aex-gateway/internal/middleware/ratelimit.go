package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiter implements in-memory rate limiting using token bucket algorithm
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*bucket
	limit      int
	burstSize  int
	windowSize time.Duration
}

type bucket struct {
	tokens     int
	lastRefill time.Time
}

func NewRateLimiter(limitPerMinute, burstSize int) *RateLimiter {
	rl := &RateLimiter{
		buckets:    make(map[string]*bucket),
		limit:      limitPerMinute,
		burstSize:  burstSize,
		windowSize: time.Minute,
	}
	// Start cleanup goroutine
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, b := range rl.buckets {
			if now.Sub(b.lastRefill) > 10*time.Minute {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Allow(key string) (allowed bool, remaining int, resetAt time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]

	if !exists {
		b = &bucket{
			tokens:     rl.limit,
			lastRefill: now,
		}
		rl.buckets[key] = b
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(b.lastRefill)
	if elapsed >= rl.windowSize {
		b.tokens = rl.limit
		b.lastRefill = now
	} else {
		// Partial refill
		tokensToAdd := int(float64(rl.limit) * (float64(elapsed) / float64(rl.windowSize)))
		b.tokens = min(b.tokens+tokensToAdd, rl.limit)
		if tokensToAdd > 0 {
			b.lastRefill = now
		}
	}

	resetAt = b.lastRefill.Add(rl.windowSize)

	if b.tokens > 0 {
		b.tokens--
		return true, b.tokens, resetAt
	}

	return false, 0, resetAt
}

func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := GetTenantID(r.Context())
			if tenantID == "" {
				tenantID = "anonymous"
			}

			allowed, remaining, resetAt := limiter.Allow(tenantID)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limiter.limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			if !allowed {
				retryAfter := int(time.Until(resetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":       "rate_limit_exceeded",
						"message":    "Rate limit exceeded. Please retry after " + strconv.Itoa(retryAfter) + " seconds.",
						"request_id": GetRequestID(r.Context()),
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

