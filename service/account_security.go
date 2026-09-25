package service

import (
	"fmt"
	"html"
	"html/template"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func UnbindAccountOAuth(identity AuthIdentity, providerID int) error {
	enabled := model.AccountLoginMethods{
		Password: common.PasswordLoginEnabled,
		Passkey:  system_setting.PasskeySettingsSnapshot().Enabled,
		WeChat:   common.WeChatAuthEnabled,
	}
	for _, provider := range oauth.GetAllProviders() {
		if !provider.IsEnabled() {
			continue
		}
		if custom, ok := provider.(*oauth.GenericOAuthProvider); ok {
			enabled.CustomProviderIDs = append(enabled.CustomProviderIDs, custom.GetProviderId())
		} else {
			enabled.OAuthColumns = append(enabled.OAuthColumns, provider.ProviderUserIDColumn())
		}
	}
	return model.UnbindUserOAuthForSession(identity, providerID, enabled)
}

// NotifyAccountSecurityChange never includes credentials or tokens. The caller
// records delivery failure independently from the already-committed change.
func NotifyAccountSecurityChange(email, event string) error {
	if email == "" {
		return nil
	}
	subject := common.SystemName + " 账号安全通知"
	body := fmt.Sprintf(
		`<h1 style="margin:0 0 8px;font-size:20px;line-height:28px;font-weight:600;color:#111827;">账号安全变更</h1>`+
			`<p style="margin:0 0 16px;font-size:14px;line-height:22px;color:#4b5563;">你的账号刚刚发生了一项安全设置变更：</p>`+
			`<div style="margin:0 0 16px;padding:12px 14px;background:#f9fafb;border:1px solid #e5e7eb;border-radius:8px;font-size:14px;color:#111827;">%s</div>`+
			`<p style="margin:0;font-size:13px;line-height:20px;color:#6b7280;">如果这不是你本人的操作，请立刻打开账号安全设置，退出其他登录会话，并联系管理员。</p>`,
		html.EscapeString(event))
	content, err := common.RenderEmailLayout(template.HTML(body), "这封邮件由系统自动发送，用于通知账号安全变更。")
	if err != nil {
		return err
	}
	return common.SendEmail(subject, email, content)
}
