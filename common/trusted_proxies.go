package common

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// defaultTrustedProxyCIDRs is what an unset TRUSTED_PROXIES falls back to.
//
// Only loopback: a reverse proxy or tunnel on the same host connects from
// 127.0.0.1, which is the common shape, while trusting every RFC 1918 range
// would let any host on the same private network set X-Forwarded-For and
// thereby choose its own client IP -- which is what the token IP allowlist and
// the rate-limit buckets are keyed on. A proxy on a different host is a
// deliberate deployment and declares itself with TRUSTED_PROXIES.
var defaultTrustedProxyCIDRs = []string{
	"127.0.0.0/8",
	"::1",
}

// ResolveTrustedProxies parses TRUSTED_PROXIES without applying it to an
// engine. The returned slice can be reused by the outer and plugin engines.
func ResolveTrustedProxies(raw string) (trustedProxies []string, usedDefaults bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), defaultTrustedProxyCIDRs...), true, nil
	}
	if strings.EqualFold(raw, "none") {
		return nil, false, nil
	}

	parts := strings.Split(raw, ",")
	trustedProxies = make([]string, 0, len(parts))
	for _, part := range parts {
		trustedProxy := strings.TrimSpace(part)
		if trustedProxy == "" {
			continue
		}
		if strings.EqualFold(trustedProxy, "none") {
			return nil, false, errors.New("TRUSTED_PROXIES=none must be used alone")
		}
		trustedProxies = append(trustedProxies, trustedProxy)
	}
	if len(trustedProxies) == 0 {
		return nil, false, errors.New("TRUSTED_PROXIES does not contain an IP address or CIDR")
	}
	return trustedProxies, false, nil
}

func ConfigureTrustedProxies(engine *gin.Engine, trustedProxies []string) error {
	if err := engine.SetTrustedProxies(trustedProxies); err != nil {
		return fmt.Errorf("invalid TRUSTED_PROXIES: %w", err)
	}
	return nil
}
