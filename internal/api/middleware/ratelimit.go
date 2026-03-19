package middleware

import (
"net/http"
"sync"
"time"

"golang.org/x/time/rate"
)

type RateLimiter struct {
limiters map[string]*rate.Limiter
mu       sync.Mutex
rps      int
burst    int
}

func NewRateLimiter(rps, burst int) *RateLimiter {
return &RateLimiter{
limiters: make(map[string]*rate.Limiter),
rps:      rps,
burst:    burst,
}
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
rl.mu.Lock()
defer rl.mu.Unlock()

limiter, exists := rl.limiters[ip]
if !exists {
limiter = rate.NewLimiter(rate.Limit(rl.rps), rl.burst)
rl.limiters[ip] = limiter
}

return limiter
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
go func() {
ticker := time.NewTicker(5 * time.Minute)
defer ticker.Stop()
for range ticker.C {
rl.mu.Lock()
for ip, limiter := range rl.limiters {
if limiter.Tokens() == float64(rl.burst) {
delete(rl.limiters, ip)
}
}
rl.mu.Unlock()
}
}()

return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
ip := r.Header.Get("X-Forwarded-For")
if ip == "" {
ip = r.Header.Get("X-Real-IP")
}
if ip == "" {
ip = r.RemoteAddr
}

limiter := rl.getLimiter(ip)
if !limiter.Allow() {
http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
return
}

next.ServeHTTP(w, r)
})
}
