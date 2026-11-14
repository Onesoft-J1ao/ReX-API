package operation_setting

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type EpayGateway struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Address    string              `json:"address"`
	MerchantID string              `json:"merchant_id"`
	Key        string              `json:"key"`
	Enabled    bool                `json:"enabled"`
	PayMethods []map[string]string `json:"pay_methods"`

	// ScheduleEnabled 开启后，该网关只在 ScheduleStart~ScheduleEnd 之间对用户展示并允许下单；
	// 回调与对账不受窗口限制，避免窗口边界到账的订单漏结算。
	ScheduleEnabled  bool   `json:"schedule_enabled"`
	ScheduleStart    string `json:"schedule_start"`
	ScheduleEnd      string `json:"schedule_end"`
	ScheduleTimezone string `json:"schedule_timezone"`
}

// 默认按北京时间判断营业窗口：容器内 TZ 常为 UTC，直接用 time.Now() 会让 08:00-21:00 整体偏移 8 小时。
var paymentScheduleLocation = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}()

func parseScheduleClock(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, errHour := strconv.Atoi(strings.TrimSpace(parts[0]))
	minute, errMinute := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errHour != nil || errMinute != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

// IsAvailableAt 判断网关在给定时刻是否处于营业窗口内；未开启窗口限制时始终可用。
func (g EpayGateway) IsAvailableAt(t time.Time) bool {
	if !g.ScheduleEnabled {
		return true
	}
	start, okStart := parseScheduleClock(g.ScheduleStart)
	end, okEnd := parseScheduleClock(g.ScheduleEnd)
	if !okStart || !okEnd || start == end {
		return true
	}
	loc := paymentScheduleLocation
	if name := strings.TrimSpace(g.ScheduleTimezone); name != "" {
		if custom, err := time.LoadLocation(name); err == nil {
			loc = custom
		}
	}
	local := t.In(loc)
	now := local.Hour()*60 + local.Minute()
	if start < end {
		return now >= start && now < end
	}
	return now >= start || now < end
}

// ScheduleLabel 返回可读的营业窗口文案，未开启或配置非法时返回空串。
func (g EpayGateway) ScheduleLabel() string {
	if !g.ScheduleEnabled {
		return ""
	}
	start, okStart := parseScheduleClock(g.ScheduleStart)
	end, okEnd := parseScheduleClock(g.ScheduleEnd)
	if !okStart || !okEnd {
		return ""
	}
	return fmt.Sprintf("%02d:%02d-%02d:%02d", start/60, start%60, end/60, end%60)
}

var EpayGateways = []EpayGateway{}

type PaymentSetting struct {
	AmountOptions  []int           `json:"amount_options"`
	AmountDiscount map[int]float64 `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`

	// RedemptionPurchaseEnabled sells redemption codes through the payment
	// providers: the buyer pays real money and receives codes, never wallet
	// balance. Off by default because it moves real money.
	RedemptionPurchaseEnabled bool `json:"redemption_purchase_enabled"`
}

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:             []int{10, 20, 50, 100, 200, 500},
	AmountDiscount:            map[int]float64{},
	RedemptionPurchaseEnabled: false,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}

func UpdateEpayGatewaysByJsonString(jsonString string) error {
	if err := ValidateEpayGatewaysJSON(jsonString); err != nil {
		return err
	}
	var gateways []EpayGateway
	if err := common.Unmarshal([]byte(jsonString), &gateways); err != nil {
		return err
	}
	EpayGateways = gateways
	return nil
}

func ValidateEpayGatewaysJSON(jsonString string) error {
	var gateways []EpayGateway
	if err := common.Unmarshal([]byte(jsonString), &gateways); err != nil {
		return err
	}
	ids := make(map[string]struct{}, len(gateways))
	for _, gateway := range gateways {
		id := strings.TrimSpace(gateway.ID)
		if id == "" || len(id) > 100 {
			return errors.New("Epay gateway id is required and must not exceed 100 characters")
		}
		if _, exists := ids[id]; exists {
			return errors.New("Epay gateway ids must be unique")
		}
		ids[id] = struct{}{}
		address, err := url.Parse(strings.TrimSpace(gateway.Address))
		if err != nil || (address.Scheme != "http" && address.Scheme != "https") || address.Host == "" {
			return errors.New("Epay gateway address must be a valid HTTP URL")
		}
		if strings.TrimSpace(gateway.MerchantID) == "" || strings.TrimSpace(gateway.Key) == "" {
			return errors.New("Epay gateway merchant id and key are required")
		}
		if gateway.ScheduleEnabled {
			if _, ok := parseScheduleClock(gateway.ScheduleStart); !ok {
				return errors.New("Epay gateway schedule start must be HH:MM")
			}
			if _, ok := parseScheduleClock(gateway.ScheduleEnd); !ok {
				return errors.New("Epay gateway schedule end must be HH:MM")
			}
		}
		methodTypes := make(map[string]struct{}, len(gateway.PayMethods))
		for _, method := range gateway.PayMethods {
			methodType := strings.TrimSpace(method["type"])
			if methodType == "" {
				return errors.New("Epay payment method type is required")
			}
			if _, exists := methodTypes[methodType]; exists {
				return errors.New("Epay payment method types must be unique within a gateway")
			}
			methodTypes[methodType] = struct{}{}
			for _, field := range []string{"fee", "fee_rate"} {
				if strings.TrimSpace(method[field]) == "" {
					continue
				}
				value, err := strconv.ParseFloat(method[field], 64)
				if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
					return errors.New("Epay payment fees must be finite non-negative numbers")
				}
			}
		}
	}
	return nil
}

func EpayGateways2JsonString() string {
	jsonBytes, err := common.Marshal(EpayGateways)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func GetEpayGateways() []EpayGateway {
	if len(EpayGateways) > 0 {
		result := make([]EpayGateway, 0, len(EpayGateways))
		for _, gateway := range EpayGateways {
			if !gateway.Enabled {
				continue
			}
			result = append(result, gateway)
		}
		return result
	}
	if PayAddress == "" || EpayId == "" || EpayKey == "" {
		return nil
	}
	return []EpayGateway{{
		ID: "default", Name: "Epay", Address: PayAddress, MerchantID: EpayId,
		Key: EpayKey, Enabled: true, PayMethods: PayMethods,
	}}
}

// GetAvailableEpayGateways 只返回当前处于营业窗口内的网关，用于对用户展示支付方式。
func GetAvailableEpayGateways() []EpayGateway {
	now := time.Now()
	gateways := GetEpayGateways()
	result := make([]EpayGateway, 0, len(gateways))
	for _, gateway := range gateways {
		if gateway.IsAvailableAt(now) {
			result = append(result, gateway)
		}
	}
	return result
}

// IsEpayGatewayAvailable 供下单入口复用同一套窗口判断。
func IsEpayGatewayAvailable(gateway *EpayGateway) bool {
	if gateway == nil {
		return false
	}
	return gateway.IsAvailableAt(time.Now())
}

func GetEpayGateway(id string) *EpayGateway {
	if id != "" && len(EpayGateways) > 0 {
		for i := range EpayGateways {
			if EpayGateways[i].ID == id {
				gateway := EpayGateways[i]
				return &gateway
			}
		}
		return nil
	}
	gateways := GetEpayGateways()
	for i := range gateways {
		if gateways[i].ID == id || id == "" {
			gateway := gateways[i]
			return &gateway
		}
	}
	return nil
}

// MigrateLegacyEpayGateway converts the old single-gateway settings into the
// first provider once, while leaving the legacy fields untouched for rollback.
func MigrateLegacyEpayGateway() bool {
	if len(EpayGateways) > 0 || PayAddress == "" || EpayId == "" || EpayKey == "" {
		return false
	}
	EpayGateways = []EpayGateway{{
		ID: "default", Name: "Epay", Address: PayAddress, MerchantID: EpayId,
		Key: EpayKey, Enabled: true, PayMethods: PayMethods,
	}}
	return true
}
