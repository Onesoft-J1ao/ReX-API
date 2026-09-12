package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignMjImage signs a Midjourney forwarded image id for the unauthenticated
// GET /mj/image/:id route. HMAC(SessionSecret, "mjimg:"+id) truncated to 16
// hex chars is enough to stop unauthenticated enumeration (CVE-2026-9306)
// while keeping <img>/fetch clients token-less.
func SignMjImage(mjId string) string {
	mac := hmac.New(sha256.New, []byte(SessionSecret))
	mac.Write([]byte("mjimg:" + mjId))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}
