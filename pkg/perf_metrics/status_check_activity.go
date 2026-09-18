package perfmetrics

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/status_check_setting"
)

// Passive-activity tracking for flexible status probing.
//
// A group that is already serving real requests has just proven it works, so
// the automatic probe does not need to spend a probe on it. To know that, the
// relay path has to report activity -- but that path runs on every request, so
// the reporting here is deliberately cheap: an in-memory timestamp per group,
// persisted at most once a minute, with Redis carrying it across masters.
const (
	statusCheckPassivePersistInterval = time.Minute
	statusCheckPassiveRedisTTL        = 48 * time.Hour
)

type passiveActivity struct {
	timestamp   int64
	persistedAt int64
}

var (
	statusCheckActivityMu sync.RWMutex
	statusCheckActivities = map[string]passiveActivity{}
)

// statusCheckTrackedGroup reports whether automatic probing paces itself by
// this group's traffic: flexible probing must be on and the group must be one
// of the probed groups.
//
// Reading the settings costs a small allocation, so the feature switch is
// checked first: an installation that does not use flexible probing pays
// nothing on the request path. The settings are read every time rather than
// cached, so turning the feature or the group list on takes effect at once
// instead of after a cache window.
func statusCheckTrackedGroup(group string) bool {
	if group == "" || !status_check_setting.FlexibleProbingEnabled() {
		return false
	}
	for _, candidate := range status_check_setting.GetGroups() {
		if candidate == group {
			return true
		}
	}
	return false
}

// NoteStatusCheckPassiveActivity reports that a real request served this group,
// which restarts that group's probe streak and pushes its next automatic probe
// out by the configured idle window.
//
// A no-op unless flexible probing is on and the group is actually probed, so a
// deployment that does not use it pays nothing on the request path.
func NoteStatusCheckPassiveActivity(group string) {
	if !statusCheckTrackedGroup(group) {
		return
	}

	now := time.Now().Unix()
	persist := false

	statusCheckActivityMu.Lock()
	activity := statusCheckActivities[group]
	activity.timestamp = now
	// Without Redis every master would write its own row, so the durable write
	// is rate-limited; the in-memory timestamp still resets the streak locally.
	if common.RDB == nil || !common.RedisEnabled {
		if now-activity.persistedAt >= int64(statusCheckPassivePersistInterval.Seconds()) {
			activity.persistedAt = now
			persist = true
		}
	}
	statusCheckActivities[group] = activity
	statusCheckActivityMu.Unlock()

	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := common.RDB.Set(ctx, statusCheckPassiveRedisKey(group), now, statusCheckPassiveRedisTTL).Err()
		cancel()
		if err == nil {
			return
		}
		// Redis is configured but unreachable: fall back to the rate-limited
		// durable write rather than losing the observation entirely.
		statusCheckActivityMu.Lock()
		activity = statusCheckActivities[group]
		if now-activity.persistedAt >= int64(statusCheckPassivePersistInterval.Seconds()) {
			activity.persistedAt = now
			persist = true
		}
		statusCheckActivities[group] = activity
		statusCheckActivityMu.Unlock()
	}

	if persist {
		if err := model.TouchStatusCheckProbePassiveActivity(group, now); err != nil {
			common.SysError(fmt.Sprintf("failed to record status check passive activity for group=%s: %v", group, err))
		}
	}
}

// LatestStatusCheckPassiveActivity resolves the most recent activity for a
// group from the durable row, this process, and Redis. Reading all three keeps
// a second master from probing a group that is busy on this one.
func LatestStatusCheckPassiveActivity(group string, stored int64) int64 {
	latest := stored

	statusCheckActivityMu.RLock()
	if activity, ok := statusCheckActivities[group]; ok && activity.timestamp > latest {
		latest = activity.timestamp
	}
	statusCheckActivityMu.RUnlock()

	if !common.RedisEnabled || common.RDB == nil {
		return latest
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	value, err := common.RDB.Get(ctx, statusCheckPassiveRedisKey(group)).Result()
	cancel()
	if err != nil {
		return latest
	}
	if timestamp, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil && timestamp > latest {
		return timestamp
	}
	return latest
}

// statusCheckPassiveRedisKey namespaces the Redis key by the group without
// putting an arbitrary group name into the key.
func statusCheckPassiveRedisKey(group string) string {
	digest := sha256.Sum256([]byte(group))
	return fmt.Sprintf("status-check:passive:%x", digest[:])
}
