package middleware

import (
	"log"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func ConfigureTrustedProxies(engine *gin.Engine) error {
	trustedProxies, usedDefaults, err := common.ResolveTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return err
	}
	if usedDefaults {
		log.Print("WARNING: TRUSTED_PROXIES is unset or blank; trusting loopback proxy addresses only. Set TRUSTED_PROXIES to the proxy IPs/CIDRs when a reverse proxy runs on another host, or TRUSTED_PROXIES=none to trust no proxies.")
	}
	return common.ConfigureTrustedProxies(engine, trustedProxies)
}
