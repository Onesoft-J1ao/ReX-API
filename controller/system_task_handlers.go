package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/registrar"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/registrar_setting"
)

// RegisterScheduledSystemTasks wires the periodic channel test, upstream model
// update, and async task polling (Midjourney / Suno / video) jobs into the
// system task framework so a DB lease dedups execution across multiple master
// instances and each run is recorded as one task row. Call this before
// service.StartSystemTaskRunner.
func RegisterScheduledSystemTasks() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
	service.RegisterSystemTaskHandler(groupHealthProbeHandler{})
	service.RegisterSystemTaskHandler(modelUpdateHandler{})
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
	service.RegisterSystemTaskHandler(asyncTaskPollHandler{})
	service.RegisterSystemTaskHandler(channelStatusSyncHandler{})
	service.RegisterSystemTaskHandler(channelStatusSyncHandler{})
	service.RegisterSystemTaskHandler(priceSentinelHandler{})
	service.RegisterSystemTaskHandler(registrarRunHandler{})
	service.RegisterSystemTaskHandler(registrarReplenishHandler{})
}

// channelTestHandler runs the scheduled "test all channels" job. Enablement and
// cadence still come from the monitor settings; only the execution path moved
// into the system task runner.
type channelTestHandler struct{}

func (channelTestHandler) Type() string { return model.SystemTaskTypeChannelTest }

func (channelTestHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoTestChannelEnabled
}

func (channelTestHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelTestHandler) NewPayload() any { return nil }

// channelTestTaskPayload controls one channel_test run. A nil/empty payload is a
// scheduled run, which uses the configured monitor ChannelTestMode and does not
// notify. A manual "test all channels" trigger sets Mode=scheduled_all and
// Notify=true to reproduce the legacy manual behavior (test every channel and
// notify root on completion).
type channelTestTaskPayload struct {
	Mode   string `json:"mode,omitempty"`
	Notify bool   `json:"notify,omitempty"`
}

func (channelTestHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelTestTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runChannelTestTask(ctx, payload.Mode, payload.Notify, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// modelUpdateHandler runs the scheduled upstream model update detection job.
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string { return model.SystemTaskTypeModelUpdate }

func (modelUpdateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", true)
}

func (modelUpdateHandler) Interval() time.Duration {
	intervalMinutes := common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES",
		channelUpstreamModelUpdateTaskDefaultIntervalMinutes,
	)
	if intervalMinutes < 1 {
		intervalMinutes = channelUpstreamModelUpdateTaskDefaultIntervalMinutes
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (modelUpdateHandler) NewPayload() any { return nil }

// modelUpdateTaskPayload controls one model_update run. A scheduled run
// (Manual=false) respects the per-channel minimum check interval and may
// auto-apply detected models when a channel has auto-sync enabled. A manual
// "detect all" trigger sets Manual=true to reproduce the legacy detect-all
// semantics: force a re-check regardless of the interval and never auto-apply,
// so the admin reviews and applies changes explicitly.
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := runChannelUpstreamModelUpdateTaskOnce(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// midjourneyPollHandler runs one Midjourney polling pass per scheduled run.
// Enabled() folds the "are there unfinished tasks?" check into enablement so the
// scheduler creates no row when the system is idle; only when at least one
// Midjourney task is in progress does a row get scheduled.
type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string { return model.SystemTaskTypeMidjourneyPoll }

func (midjourneyPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedMidjourneyTasks()
}

func (midjourneyPollHandler) Interval() time.Duration { return 15 * time.Second }

func (midjourneyPollHandler) NewPayload() any { return nil }

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runMidjourneyTaskUpdateOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// asyncTaskPollHandler runs one async-task (Suno/video) polling pass per
// scheduled run. Like midjourneyPollHandler, Enabled() folds in the unfinished
// task existence check so an idle system schedules no rows.
type asyncTaskPollHandler struct{}

func (asyncTaskPollHandler) Type() string { return model.SystemTaskTypeAsyncTaskPoll }

func (asyncTaskPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedSyncTasks()
}

func (asyncTaskPollHandler) Interval() time.Duration { return 15 * time.Second }

func (asyncTaskPollHandler) NewPayload() any { return nil }

func (asyncTaskPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunTaskPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysLog(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}

// registrarRunHandler executes one registration run requested from the admin
// console. The payload names the provider and the account count; the count is
// clamped to the provider's MaxBatch so an API caller cannot ask for an
// unbounded registration burst.
type registrarRunHandler struct{}

func (registrarRunHandler) Type() string { return model.SystemTaskTypeRegistrarRun }

type registrarRunPayload struct {
	ProviderID string `json:"provider_id"`
	Count      int    `json:"count"`
}

func (registrarRunHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := registrarRunPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	provider, ok := registrar_setting.GetProvider(payload.ProviderID)
	if !ok {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, fmt.Errorf("unknown registrar provider %q", payload.ProviderID))
		return
	}
	count := payload.Count
	if provider.MaxBatch > 0 && (count <= 0 || count > provider.MaxBatch) {
		count = provider.MaxBatch
	}
	if count <= 0 {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, fmt.Errorf("provider %s has no positive run size", provider.ID))
		return
	}
	result, err := registrar.RunProvider(ctx, provider, count, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, result, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
}

// registrarReplenishHandler tops up any provider whose channel has fallen below
// its usable-key floor. Enabled() folds that check in, so an idle system
// schedules no row and the history stays quiet.
type registrarReplenishHandler struct{}

func (registrarReplenishHandler) Type() string { return model.SystemTaskTypeRegistrarReplenish }

func (registrarReplenishHandler) Enabled() bool {
	setting := registrar_setting.GetSetting()
	if !setting.Enabled {
		return false
	}
	for _, provider := range setting.Providers {
		if !provider.Enabled || provider.ChannelID <= 0 {
			continue
		}
		channel, err := model.GetChannelById(provider.ChannelID, true)
		if err != nil || channel == nil {
			continue
		}
		if provider.MissingFor(registrar.UsableKeyCount(channel)) > 0 {
			return true
		}
	}
	return false
}

func (registrarReplenishHandler) Interval() time.Duration { return registrar_setting.GetInterval() }

func (registrarReplenishHandler) NewPayload() any { return nil }

func (registrarReplenishHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runRegistrarReplenish(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// registrarReplenishSummary records one entry per provider per run so the admin
// page and the task history show why a provider did or did not register.
type registrarReplenishSummary struct {
	Providers []registrarProviderReplenish `json:"providers"`
}

type registrarProviderReplenish struct {
	ProviderID string `json:"provider_id"`
	UsableKeys int    `json:"usable_keys"`
	Requested  int    `json:"requested"`
	Added      int    `json:"added"`
	Error      string `json:"error,omitempty"`
}

// runRegistrarReplenish tops providers up in turn. One provider failing is
// recorded in its own entry rather than failing the whole pass, so a broken site
// does not stop the others from being replenished.
func runRegistrarReplenish(ctx context.Context, progress func(processed, total int)) registrarReplenishSummary {
	var summary registrarReplenishSummary
	setting := registrar_setting.GetSetting()
	if !setting.Enabled {
		return summary
	}
	for _, provider := range setting.Providers {
		if !provider.Enabled || provider.ChannelID <= 0 {
			continue
		}
		if ctx.Err() != nil {
			return summary
		}
		entry := registrarProviderReplenish{ProviderID: provider.ID}
		channel, err := model.GetChannelById(provider.ChannelID, true)
		if err != nil || channel == nil {
			entry.Error = fmt.Sprintf("channel %d is not available", provider.ChannelID)
			summary.Providers = append(summary.Providers, entry)
			continue
		}
		entry.UsableKeys = registrar.UsableKeyCount(channel)
		missing := provider.MissingFor(entry.UsableKeys)
		entry.Requested = missing
		if missing > 0 {
			result, runErr := registrar.RunProvider(ctx, provider, missing, progress)
			entry.Added = result.Added
			if runErr != nil {
				entry.Error = runErr.Error()
			}
		}
		summary.Providers = append(summary.Providers, entry)
	}
	return summary
}
