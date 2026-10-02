package middleware

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// cloudflareConnectingIP is the header Cloudflare sets to the visitor address.
// When traffic arrives through a Cloudflare Tunnel the request reaches the app
// from the tunnel's own loopback connection, so Gin's ClientIP sees the origin
// address the tunnel was configured with rather than the visitor.
const cloudflareConnectingIP = "CF-Connecting-IP"

// ResolveClientIP rewrites the request's forwarded-for headers when Cloudflare
// supplied the visitor address, so every later c.ClientIP() call — audit logs,
// registration records, rate-limit buckets — sees the real client.
//
// It only trusts the header when the direct peer is itself a trusted proxy;
// otherwise any caller could spoof its own address.
func ResolveClientIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfIP := strings.TrimSpace(c.GetHeader(cloudflareConnectingIP))
		if cfIP == "" {
			c.Next()
			return
		}
		peer := net.ParseIP(c.RemoteIP())
		if peer == nil || !isTrustedPeer(peer) {
			c.Next()
			return
		}
		if net.ParseIP(cfIP) == nil {
			c.Next()
			return
		}

		// Gin's ClientIP walks X-Forwarded-For from the right, skipping trusted
		// entries. Presenting Cloudflare's address there makes it the first
		// untrusted hop, which is exactly the visitor.
		c.Request.Header.Set("X-Forwarded-For", cfIP)
		c.Request.Header.Set("X-Real-IP", cfIP)
		c.Next()
	}
}

func isTrustedPeer(ip net.IP) bool {
	for _, cidr := range trustedPeerCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// trustedPeerCIDRs mirrors the loopback and private ranges a tunnel or reverse
// proxy connects from.
var trustedPeerCIDRs = func() []*net.IPNet {
	parsed := make([]*net.IPNet, 0, 8)
	for _, raw := range []string{
		"127.0.0.0/8",
		"::1/128",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7",
	} {
		if _, block, err := net.ParseCIDR(raw); err == nil {
			parsed = append(parsed, block)
		}
	}
	return parsed
}()
