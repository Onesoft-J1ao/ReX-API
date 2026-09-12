package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Register anti-abuse
//
// The per-IP CriticalRateLimit on /api/user/register stops one address from
// scripting registrations, but a rotated proxy pool walks straight around it
// (each IP stays under the limit while the fleet keeps signing up). Two
// additional layers close that gap:
//
//  1. RegisterTicket: the client must fetch a one-time, IP-bound, short-lived
//     HMAC ticket from /api/user/register_ticket before registering. A script
//     that fires POST /register directly gets 403; fetching a ticket per
//     account costs one extra round-trip that is itself IP-rate-limited.
//  2. Global register throttle: a rolling count of registrations per window
//     across ALL IPs. A proxy pool that rotates addresses still accumulates
//     on the shared counter; once the window total crosses the threshold,
//     registration is refused until the window rolls over.
// ---------------------------------------------------------------------------

const (
	registerTicketMark       = "RT"
	registerTicketMaxPerIP   = 5       // tickets per IP per window
	registerTicketWindow     = 30 * 60 // ticket window: 30 minutes
	registerTicketTTL        = 15 * 60 // ticket validity after issue
	registerTicketMaxGlobal  = 30      // registrations per global window
	registerGlobalWindow     = 60 * 60 // global registration window: 1 hour
	registerTicketSecretZone = "rex-register"
)

// registerTicketSecret binds tickets to the deployed SESSION_SECRET so they
// cannot be forged offline and do not survive a secret rotation.
func registerTicketSecret() []byte {
	return []byte(registerTicketSecretZone + ":" + common.SessionSecret)
}

// signRegisterTicket produces an HMAC over ip|expiresAt so a ticket issued to
// one IP cannot be replayed from another and cannot be extended in time.
func signRegisterTicket(ip string, expiresAt int64) string {
	mac := hmac.New(sha256.New, registerTicketSecret())
	fmt.Fprintf(mac, "%s|%d", ip, expiresAt)
	return hex.EncodeToString(mac.Sum(nil))
}

// registerTicketMessage builds the signed payload for one ticket.
func registerTicketMessage(ip string, expiresAt int64) string {
	return fmt.Sprintf("%d|%s", expiresAt, ip)
}

// RegisterTicket issues a one-time registration ticket bound to the caller IP.
func RegisterTicket() gin.HandlerFunc {
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		ip := c.ClientIP()

		// Per-IP ticket quota: fetching a ticket per account is fine for a
		// human, but a ticket farm burning hundreds per hour is not.
		var allowed bool
		var ttlSeconds int64
		redisCounted := false
		if common.RedisEnabled {
			var err error
			allowed, _, ttlSeconds, err = redisFixedWindowTake(
				c.Request.Context(),
				redisIPRateLimitKey(registerTicketMark, ip),
				registerTicketMaxPerIP,
				registerTicketWindow,
			)
			if err == nil {
				redisCounted = true
				if !allowed {
					c.JSON(http.StatusTooManyRequests, gin.H{
						"success": false,
						"message": fmt.Sprintf("注册过于频繁，请等待 %d 秒后再试", ttlSeconds),
					})
					c.Abort()
					return
				}
			}
			// Redis failure falls through to the in-memory limiter below.
		}
		// In-memory limiter runs as the local fallback; skip when Redis is
		// already the source of truth so the human path is not double-charged.
		if !redisCounted && !inMemoryRateLimiter.Request(
			registerTicketMark+":"+ip,
			registerTicketMaxPerIP,
			registerTicketWindow,
		) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "注册过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		expiresAt := time.Now().Add(registerTicketTTL).Unix()
		ticket := signRegisterTicket(ip, expiresAt)
		c.Set("register_ticket", gin.H{
			"ticket":     fmt.Sprintf("%d:%s", expiresAt, ticket),
			"expires_at": expiresAt,
		})
		c.Next()
	}
}

// verifyRegisterTicket checks an IP-bound ticket carried as ?register_ticket=
// or the X-Register-Ticket header.
func verifyRegisterTicket(c *gin.Context) bool {
	ticket := c.Query("register_ticket")
	if ticket == "" {
		ticket = c.GetHeader("X-Register-Ticket")
	}
	if ticket == "" {
		return false
	}
	var expiresAt int64
	var sig string
	if _, err := fmt.Sscanf(ticket, "%d:%s", &expiresAt, &sig); err != nil {
		return false
	}
	if time.Now().Unix() > expiresAt {
		return false
	}
	expected := signRegisterTicket(c.ClientIP(), expiresAt)
	return subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) == 1
}

// RegisterThrottle enforces the global rolling registration count and the
// ticket check on POST /api/user/register.
func RegisterThrottle() gin.HandlerFunc {
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		// 1) One-time IP-bound ticket: blocks direct scripted POSTs.
		if !verifyRegisterTicket(c) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "注册校验失败，请刷新页面后重试",
			})
			c.Abort()
			return
		}

		// 2) Global rolling count across all IPs: a rotated proxy pool still
		// accumulates here even when each address stays under its own limit.
		allowed := true
		var waitSeconds int64 = registerGlobalWindow
		redisCounted := false
		if common.RedisEnabled {
			ok, _, ttl, err := redisFixedWindowTake(
				c.Request.Context(),
				fmt.Sprintf("%s:global", redisRateLimitNamespace),
				registerTicketMaxGlobal,
				registerGlobalWindow,
			)
			if err == nil {
				redisCounted = true
				allowed = ok
				if ttl > 0 {
					waitSeconds = ttl
				}
			}
			// Redis failure: fall through to the in-memory counter below.
		}
		if allowed && !redisCounted {
			if !inMemoryRateLimiter.Request(
				registerTicketMark+":global",
				registerTicketMaxGlobal,
				registerGlobalWindow,
			) {
				allowed = false
			}
		}
		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": fmt.Sprintf("当前注册人数过多，请等待 %d 秒后再试", waitSeconds),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
