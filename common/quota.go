package common

func GetTrustQuota() int {
	// Disable the trust-quota fast path. The old 10*QuotaPerUnit threshold
	// skipped pre-consume for high-balance / unlimited-token requests and
	// only charged at settle, so concurrent bursts could overspend.
	// shouldTrust() already returns false when trustQuota <= 0.
	return 0
}
