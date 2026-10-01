package common

import (
	"fmt"
	"html/template"
	"strings"
)

// emailShell wraps one email's body in the layout every mail we send shares:
// a centered card on a quiet background, the system name as the header, and a
// footer that says why the mail arrived. Mail clients ignore <style> blocks and
// class names, so every rule is inline.
const emailShell = `<!doctype html>
<html lang="zh">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin:0;padding:24px;background:#f4f5f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif;color:#1f2937;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0">
    <tr>
      <td align="center">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="max-width:600px;">
          <tr>
            <td style="padding:0 4px 12px;font-size:15px;font-weight:600;letter-spacing:0.2px;color:#111827;">{{.SystemName}}</td>
          </tr>
          <tr>
            <td style="background:#ffffff;border:1px solid #e5e7eb;border-radius:12px;padding:32px;">
              {{.Body}}
            </td>
          </tr>
          <tr>
            <td style="padding:14px 4px 0;font-size:12px;line-height:18px;color:#9ca3af;">{{.Footer}}</td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`

type emailShellData struct {
	SystemName string
	Body       template.HTML
	Footer     string
}

// RenderEmailLayout wraps an already-rendered body in the shared layout. The
// body is trusted HTML assembled by the caller, so it is passed through
// unescaped; the caller is responsible for escaping any value that came from
// outside the program.
func RenderEmailLayout(body template.HTML, footer string) (string, error) {
	tmpl, err := template.New("email").Parse(emailShell)
	if err != nil {
		return "", fmt.Errorf("invalid email layout: %w", err)
	}
	var content strings.Builder
	err = tmpl.Execute(&content, emailShellData{
		SystemName: SystemName,
		Body:       body,
		Footer:     footer,
	})
	if err != nil {
		return "", fmt.Errorf("invalid email layout: %w", err)
	}
	return content.String(), nil
}
