package ratio_setting

// IsModelFreeInGroup reports whether serving modelName through group costs
// nothing. It answers the pricing question on its own terms rather than reusing
// the relay's FreeModel flag, which means "skip pre-consume" and is forced false
// whenever EnableFreeModelPreConsume is on (the default) — useless for deciding
// whether a probe would cost money.
//
// A group priced at zero is free regardless of the model. Otherwise the model's
// own entry decides, and only an explicit zero counts: an unconfigured model
// falls back to a default ratio, which is not a price of zero and must never be
// treated as free.
func IsModelFreeInGroup(modelName string, group string) bool {
	if GetGroupRatio(group) == 0 {
		return true
	}
	value, _, exist := GetModelRatioOrPrice(modelName)
	if !exist {
		return false
	}
	return value == 0
}
