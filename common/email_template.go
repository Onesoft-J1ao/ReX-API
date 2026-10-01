package common

import (
	"fmt"
	"html/template"
	"strings"
)

const DefaultEmailVerificationTemplate = `<h1 style="margin:0 0 8px;font-size:20px;line-height:28px;font-weight:600;color:#111827;">验证你的邮箱</h1>
<p style="margin:0 0 20px;font-size:14px;line-height:22px;color:#4b5563;">你正在 {{.SystemName}} 进行邮箱验证，请使用下面的验证码：</p>
<div style="margin:0 0 20px;padding:16px;background:#f9fafb;border:1px solid #e5e7eb;border-radius:8px;font-family:'SFMono-Regular',Consolas,monospace;font-size:30px;letter-spacing:8px;text-align:center;color:#111827;">{{.Code}}</div>
<p style="margin:0;font-size:13px;line-height:20px;color:#6b7280;">验证码 {{.ValidMinutes}} 分钟内有效。如果这不是你的操作，忽略这封邮件即可，你的账号不会有任何变化。</p>`

type emailVerificationTemplateData struct {
	SystemName   string
	Code         string
	ValidMinutes int
}

func RenderEmailVerificationTemplate(code string) (string, error) {
	return renderEmailVerificationTemplate(EmailVerificationTemplate, code)
}

func ValidateEmailVerificationTemplate(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("email verification template cannot be empty")
	}
	_, err := renderEmailVerificationTemplate(value, "123456")
	return err
}

func renderEmailVerificationTemplate(value string, code string) (string, error) {
	tmpl, err := template.New("email-verification").Option("missingkey=error").Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid email verification template: %w", err)
	}

	var content strings.Builder
	err = tmpl.Execute(&content, emailVerificationTemplateData{
		SystemName:   SystemName,
		Code:         code,
		ValidMinutes: VerificationValidMinutes,
	})
	if err != nil {
		return "", fmt.Errorf("invalid email verification template: %w", err)
	}
	return RenderEmailLayout(template.HTML(content.String()), "这封邮件由系统自动发送，用于验证邮箱所有权。")
}
