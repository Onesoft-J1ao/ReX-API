package registrar

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// UsableKeyCount counts keys a multi-key channel can still use. It mirrors
// (*Channel).HasEnabledMultiKey: a key is usable when it has no status entry or
// is marked enabled, so the replenishment floor counts the same keys routing
// would pick.
func UsableKeyCount(channel *model.Channel) int {
	if channel == nil || !channel.ChannelInfo.IsMultiKey {
		return 0
	}
	usable := 0
	for index := range channel.GetKeys() {
		if status, ok := channel.ChannelInfo.MultiKeyStatusList[index]; !ok || status == common.ChannelStatusEnabled || status == 0 {
			usable++
		}
	}
	return usable
}

// AppendKeysToChannel adds the keys a channel does not already carry and returns
// how many were new. The existing list is preserved as-is (a duplicate already
// stored stays where it is) and only the incoming keys are deduplicated against
// it, matching the append semantics of the channel edit endpoint.
func AppendKeysToChannel(channelID int, keys []string) (int, error) {
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return 0, err
	}
	if channel == nil {
		return 0, fmt.Errorf("channel %d not found", channelID)
	}
	if !channel.ChannelInfo.IsMultiKey {
		return 0, fmt.Errorf("channel %d is not a multi-key channel", channelID)
	}

	existing := channel.GetKeys()
	seen := make(map[string]struct{}, len(existing)+len(keys))
	for _, key := range existing {
		seen[strings.TrimSpace(key)] = struct{}{}
	}
	all := make([]string, 0, len(existing)+len(keys))
	all = append(all, existing...)
	added := 0
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		all = append(all, key)
		added++
	}
	if added == 0 {
		return 0, nil
	}

	channel.Key = strings.Join(all, "\n")
	// Keep the transient slice consistent so an immediate HasEnabledMultiKey on
	// this instance sees the new count; Update persists Key and recomputes
	// MultiKeySize from it.
	channel.Keys = all
	if err := channel.Update(); err != nil {
		return 0, err
	}
	model.CacheUpdateChannel(channel)
	return added, nil
}
