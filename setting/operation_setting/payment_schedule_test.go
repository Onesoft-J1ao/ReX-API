package operation_setting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func scheduleInstant(hour, minute int) time.Time {
	return time.Date(2026, 11, 14, hour, minute, 0, 0, paymentScheduleLocation)
}

func TestEpayGatewayIsAvailableAtDisabledWindow(t *testing.T) {
	gateway := EpayGateway{ScheduleEnabled: false, ScheduleStart: "08:00", ScheduleEnd: "21:00"}
	assert.True(t, gateway.IsAvailableAt(scheduleInstant(3, 0)))
	assert.True(t, gateway.IsAvailableAt(scheduleInstant(12, 0)))
}

func TestEpayGatewayIsAvailableAtDayWindow(t *testing.T) {
	gateway := EpayGateway{ScheduleEnabled: true, ScheduleStart: "08:00", ScheduleEnd: "21:00"}
	assert.False(t, gateway.IsAvailableAt(scheduleInstant(7, 59)))
	assert.True(t, gateway.IsAvailableAt(scheduleInstant(8, 0)))
	assert.True(t, gateway.IsAvailableAt(scheduleInstant(12, 0)))
	assert.True(t, gateway.IsAvailableAt(scheduleInstant(20, 59)))
	assert.False(t, gateway.IsAvailableAt(scheduleInstant(21, 0)))
	assert.False(t, gateway.IsAvailableAt(scheduleInstant(23, 30)))
}

func TestEpayGatewayIsAvailableAtOvernightWindow(t *testing.T) {
	gateway := EpayGateway{ScheduleEnabled: true, ScheduleStart: "22:00", ScheduleEnd: "06:00"}
	assert.True(t, gateway.IsAvailableAt(scheduleInstant(23, 30)))
	assert.True(t, gateway.IsAvailableAt(scheduleInstant(5, 59)))
	assert.False(t, gateway.IsAvailableAt(scheduleInstant(6, 0)))
	assert.False(t, gateway.IsAvailableAt(scheduleInstant(12, 0)))
}

func TestEpayGatewayIsAvailableAtFailsOpenOnInvalidConfig(t *testing.T) {
	gateway := EpayGateway{ScheduleEnabled: true, ScheduleStart: "8am", ScheduleEnd: "21:00"}
	assert.True(t, gateway.IsAvailableAt(scheduleInstant(3, 0)))

	equal := EpayGateway{ScheduleEnabled: true, ScheduleStart: "00:00", ScheduleEnd: "00:00"}
	assert.True(t, equal.IsAvailableAt(scheduleInstant(3, 0)))
}

func TestEpayGatewayScheduleLabel(t *testing.T) {
	assert.Equal(t, "08:00-21:00", EpayGateway{ScheduleEnabled: true, ScheduleStart: "08:00", ScheduleEnd: "21:00"}.ScheduleLabel())
	assert.Equal(t, "", EpayGateway{ScheduleEnabled: false, ScheduleStart: "08:00", ScheduleEnd: "21:00"}.ScheduleLabel())
	assert.Equal(t, "", EpayGateway{ScheduleEnabled: true, ScheduleStart: "bad", ScheduleEnd: "21:00"}.ScheduleLabel())
}

func TestValidateEpayGatewaysJSONRejectsBadSchedule(t *testing.T) {
	valid := `[{"id":"mpay","name":"MPAY","address":"https://pay.example.com","merchant_id":"1001","key":"k","enabled":true,"pay_methods":[{"type":"alipay"}],"schedule_enabled":true,"schedule_start":"08:00","schedule_end":"21:00"}]`
	assert.NoError(t, ValidateEpayGatewaysJSON(valid))

	bad := `[{"id":"mpay","name":"MPAY","address":"https://pay.example.com","merchant_id":"1001","key":"k","enabled":true,"pay_methods":[{"type":"alipay"}],"schedule_enabled":true,"schedule_start":"25:00","schedule_end":"21:00"}]`
	assert.Error(t, ValidateEpayGatewaysJSON(bad))
}
