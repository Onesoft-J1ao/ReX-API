package model

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StatusCheckProbeState is the pacing state for one status-check group's
// automatic probing: when the group last saw real traffic, when it was last
// probed, and how many probes have run since that traffic.
//
// Only timestamps and a counter are stored. Pacing an automatic probe must
// never require keeping anything about the requests that caused the traffic.
type StatusCheckProbeState struct {
	Group                 string `json:"group" gorm:"column:group;size:64;primaryKey"`
	LastPassiveAt         int64  `json:"last_passive_at" gorm:"bigint;default:0"`
	LastProbeAt           int64  `json:"last_probe_at" gorm:"bigint;default:0"`
	ConsecutiveProbeCount int    `json:"consecutive_probe_count" gorm:"default:0"`
}

func (StatusCheckProbeState) TableName() string {
	return "status_check_probe_states"
}

// GetStatusCheckProbeStates reads the state of the named groups. Groups with no
// row yet are simply absent, which the caller treats as "never probed".
func GetStatusCheckProbeStates(groups []string) (map[string]StatusCheckProbeState, error) {
	states := make(map[string]StatusCheckProbeState, len(groups))
	if len(groups) == 0 {
		return states, nil
	}

	var rows []StatusCheckProbeState
	if err := DB.Where(commonGroupCol+" IN ?", groups).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		states[row.Group] = row
	}
	return states, nil
}

// InitializeStatusCheckProbeState opens the idle window when flexible probing
// is first seen for a group, so the first probe waits out the configured quiet
// period instead of spending a probe immediately after it is switched on.
func InitializeStatusCheckProbeState(group string, now int64) error {
	group = strings.TrimSpace(group)
	if group == "" {
		return nil
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&StatusCheckProbeState{
		Group:       group,
		LastProbeAt: now,
	}).Error
}

// TouchStatusCheckProbePassiveActivity advances the group's last real-request
// time. The conditional update keeps a newer timestamp written by another node
// rather than overwriting it with an older one.
func TouchStatusCheckProbePassiveActivity(group string, timestamp int64) error {
	group = strings.TrimSpace(group)
	if group == "" || timestamp <= 0 {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "group"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_passive_at": gorm.Expr("CASE WHEN last_passive_at < ? THEN ? ELSE last_passive_at END", timestamp, timestamp),
		}),
	}).Create(&StatusCheckProbeState{
		Group:         group,
		LastPassiveAt: timestamp,
	}).Error
}

// RecordStatusCheckProbeResult records that an automatic probe just ran, and
// counts it against the group's streak. Real traffic since the previous probe
// restarts the streak at one, so the cap only ever bounds probing that nothing
// has needed.
func RecordStatusCheckProbeResult(group string, timestamp int64) error {
	group = strings.TrimSpace(group)
	if group == "" || timestamp <= 0 {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "group"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_probe_at": timestamp,
			"consecutive_probe_count": gorm.Expr(
				"CASE WHEN last_passive_at > last_probe_at THEN 1 ELSE consecutive_probe_count + 1 END",
			),
		}),
	}).Create(&StatusCheckProbeState{
		Group:                 group,
		LastProbeAt:           timestamp,
		ConsecutiveProbeCount: 1,
	}).Error
}
