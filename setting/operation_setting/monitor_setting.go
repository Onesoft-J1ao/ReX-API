package operation_setting

import (
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes float64 `json:"auto_test_channel_minutes"`
	ChannelTestMode        string  `json:"channel_test_mode"`

	// 全局上游模型同步。每渠道的 upstream_model_update_* 开关要求逐个渠道配置，
	// 这里让定时任务覆盖全部启用中的渠道：上游新增的模型自动追加，上游消失的模型
	// 只做记录不自动删除。列入 UpstreamModelSyncIgnoredModels 的模型永不追加，
	// 因此有意剔除的模型不会被重新加回来。
	UpstreamModelSyncEnabled       bool   `json:"upstream_model_sync_enabled"`
	UpstreamModelSyncAutoAdd       bool   `json:"upstream_model_sync_auto_add"`
	UpstreamModelSyncIgnoredModels string `json:"upstream_model_sync_ignored_models"`
}

// GetUpstreamModelSyncIgnoredModels returns the global never-append list.
func (m *MonitorSetting) GetUpstreamModelSyncIgnoredModels() []string {
	raw := strings.TrimSpace(m.UpstreamModelSyncIgnoredModels)
	if raw == "" {
		return nil
	}
	out := make([]string, 0, 8)
	for _, p := range strings.Split(raw, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

const (
	ChannelTestModeScheduledAll    = "scheduled_all"
	ChannelTestModePassiveRecovery = "passive_recovery"
)

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:  false,
	AutoTestChannelMinutes:  10,
	ChannelTestMode:         ChannelTestModeScheduledAll,
	UpstreamModelSyncEnabled: false,
	UpstreamModelSyncAutoAdd: true,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
			monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_TEST_ENABLED"); ok {
		parsed, err := strconv.ParseBool(enabled)
		if err == nil {
			monitorSetting.AutoTestChannelEnabled = parsed
		}
	}
	if monitorSetting.ChannelTestMode != ChannelTestModePassiveRecovery {
		monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
	}
	return &monitorSetting
}
