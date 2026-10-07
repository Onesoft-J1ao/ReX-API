package pricesentinel

import (
	"fmt"
	"html"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// maxRowsPerSection caps how many models a report lists. A hundred-row digest
// is unreadable in a mail client and the tail is rarely what needs a decision,
// so the report shows the largest moves and says how many were left out.
const maxRowsPerSection = 12

// ReportSubject is the mail subject line. The timestamp is the check time, so
// two reports in one inbox are distinguishable without opening them.
func ReportSubject(checkedAt int64) string {
	return fmt.Sprintf("上游价格巡检报告 %s", formatTimestamp(checkedAt))
}

// BuildReport renders the digest as HTML.
//
// The two sections are split by direction on purpose: a model we charge more
// for risks losing customers, one we charge less for is margin being given
// away. They call for opposite responses, so they are never mixed into one
// list. The edit link is appended by the caller, which is the only part that
// needs the token.
//
// Every value that reaches the mail is escaped. Model names and the edit URL
// come from the upstream document and the server address setting, neither of
// which is safe to splice into HTML raw.
func BuildReport(comparison *Comparison, checkedAt int64, editUrl string) string {
	var body strings.Builder
	body.WriteString(`<h1 style="margin:0 0 6px;font-size:20px;line-height:28px;font-weight:600;color:#111827;">上游价格巡检报告</h1>`)
	summary := fmt.Sprintf(
		"%s · 对比 %d 个模型，%d 个价格不一致",
		html.EscapeString(formatTimestamp(checkedAt)),
		comparison.Compared,
		len(comparison.Changes),
	)
	// Say the expression-priced models were left out rather than letting a
	// short report read as "nothing else changed": their ratios are leftovers
	// the sentinel cannot compare, and silence about them would be misleading.
	if comparison.TieredSkipped > 0 {
		summary += fmt.Sprintf("，另有 %d 个表达式计费模型未参与对比", comparison.TieredSkipped)
	}
	body.WriteString(fmt.Sprintf(
		`<p style="margin:0 0 20px;font-size:13px;line-height:20px;color:#6b7280;">%s</p>`,
		summary,
	))

	higher, lower := splitByDirection(comparison.Changes)
	writeSection(&body, "我们更贵", "可能流失客户", "#b45309", higher)
	writeSection(&body, "我们更便宜", "毛利被压缩", "#047857", lower)

	if editUrl != "" {
		escaped := html.EscapeString(editUrl)
		body.WriteString(fmt.Sprintf(
			`<p style="margin:24px 0 0;"><a href="%s" style="display:inline-block;padding:10px 18px;background:#111827;color:#ffffff;border-radius:8px;font-size:14px;font-weight:600;text-decoration:none;">打开改价页面</a></p>`,
			escaped,
		))
		body.WriteString(fmt.Sprintf(
			`<p style="margin:10px 0 0;font-size:12px;line-height:18px;color:#9ca3af;word-break:break-all;">如果按钮打不开，复制这个地址：<br>%s</p>`,
			escaped,
		))
		body.WriteString(`<p style="margin:10px 0 0;font-size:12px;line-height:18px;color:#6b7280;">链接在有效期内可反复使用，打开后需要管理员访问令牌。页面上可以设置毛利率并一键同步整份报告。</p>`)
	}

	body.WriteString(`<p style="margin:20px 0 0;padding-top:16px;border-top:1px solid #f3f4f6;font-size:12px;line-height:18px;color:#6b7280;">本报告只做对比，不会自动改价。上游涨价时跟进保利润、不跟进保客户；上游降价时正好相反。</p>`)

	rendered, err := common.RenderEmailLayout(template.HTML(body.String()), "这封邮件由价格巡检自动发送。")
	if err != nil {
		// The layout is a fixed string, so this only fails if that string was
		// edited into something unparseable. Fall back to the body alone rather
		// than dropping the digest.
		return body.String()
	}
	return rendered
}

// BuildFailureReport renders the mail sent when the upstream could not be read.
//
// A failed check still mails, because an operator who stops receiving the digest
// cannot tell "no changes" from "the check is broken". The error text comes from
// an HTTP client and may contain the upstream's own wording, so it is escaped.
func BuildFailureReport(checkedAt int64, reason string) (string, error) {
	body := fmt.Sprintf(
		`<h1 style="margin:0 0 6px;font-size:20px;line-height:28px;font-weight:600;color:#111827;">上游价格巡检报告</h1>`+
			`<p style="margin:0 0 16px;font-size:13px;line-height:20px;color:#6b7280;">%s</p>`+
			`<p style="margin:0 0 12px;font-size:14px;line-height:22px;color:#1f2937;">本次检查没能读到上游价格：</p>`+
			`<div style="margin:0 0 16px;padding:12px 14px;background:#fef2f2;border:1px solid #fecaca;border-radius:8px;font-family:monospace;font-size:12px;line-height:18px;color:#991b1b;word-break:break-all;">%s</div>`+
			`<p style="margin:0;font-size:13px;line-height:20px;color:#6b7280;">下一轮会自动重试，不需要你做任何操作。</p>`,
		html.EscapeString(formatTimestamp(checkedAt)),
		html.EscapeString(reason),
	)
	return common.RenderEmailLayout(template.HTML(body), "这封邮件由价格巡检自动发送。")
}

// splitByDirection orders each side by how large the move is, so the top of a
// section is the model that most needs a decision.
func splitByDirection(changes []model.PriceChange) (higher, lower []model.PriceChange) {
	for _, change := range changes {
		// Percent is measured from ours to theirs, so a positive value means
		// the upstream now charges more than we do — we are the cheaper one.
		if change.Percent > 0 {
			lower = append(lower, change)
		} else {
			higher = append(higher, change)
		}
	}
	bySize := func(rows []model.PriceChange) {
		sort.SliceStable(rows, func(i, j int) bool {
			return abs(rows[i].Percent) > abs(rows[j].Percent)
		})
	}
	bySize(higher)
	bySize(lower)
	return higher, lower
}

// writeSection renders one direction of the report. The colour marks which side
// of the trade the rows sit on, so the two sections read differently at a glance
// instead of as two copies of the same table.
func writeSection(builder *strings.Builder, title, hint, colour string, rows []model.PriceChange) {
	builder.WriteString(fmt.Sprintf(
		`<h2 style="margin:22px 0 8px;font-size:15px;line-height:22px;font-weight:600;color:%s;">%s <span style="font-weight:400;color:#9ca3af;">· %s · %d</span></h2>`,
		colour, html.EscapeString(title), html.EscapeString(hint), len(rows),
	))
	if len(rows) == 0 {
		builder.WriteString(`<p style="margin:0;font-size:13px;color:#9ca3af;">无</p>`)
		return
	}
	builder.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="border-collapse:collapse;font-size:13px;">`)
	builder.WriteString(`<tr style="color:#9ca3af;font-size:11px;text-transform:uppercase;">`)
	for _, heading := range []string{"模型", "维度", "我们", "上游", "差异"} {
		align := "left"
		if heading != "模型" && heading != "维度" {
			align = "right"
		}
		builder.WriteString(fmt.Sprintf(
			`<th align="%s" style="padding:6px 8px;border-bottom:1px solid #e5e7eb;font-weight:500;">%s</th>`,
			align, heading,
		))
	}
	builder.WriteString(`</tr>`)

	shown := min(len(rows), maxRowsPerSection)
	for _, change := range rows[:shown] {
		builder.WriteString(`<tr>`)
		builder.WriteString(fmt.Sprintf(
			`<td style="padding:7px 8px;border-bottom:1px solid #f3f4f6;color:#111827;word-break:break-all;">%s</td>`,
			html.EscapeString(change.ModelName),
		))
		builder.WriteString(fmt.Sprintf(
			`<td style="padding:7px 8px;border-bottom:1px solid #f3f4f6;color:#6b7280;font-family:monospace;font-size:11px;">%s</td>`,
			html.EscapeString(change.Field),
		))
		builder.WriteString(cell(formatValue(change.Ours)))
		builder.WriteString(cell(formatValue(change.Theirs)))
		builder.WriteString(fmt.Sprintf(
			`<td align="right" style="padding:7px 8px;border-bottom:1px solid #f3f4f6;color:%s;font-variant-numeric:tabular-nums;">%+.2f%%</td>`,
			colour, change.Percent,
		))
		builder.WriteString(`</tr>`)
	}
	builder.WriteString(`</table>`)
	if len(rows) > shown {
		builder.WriteString(fmt.Sprintf(
			`<p style="margin:8px 0 0;font-size:12px;color:#9ca3af;">其余 %d 个变动较小，已省略。</p>`,
			len(rows)-shown,
		))
	}
}

func cell(value string) string {
	return fmt.Sprintf(
		`<td align="right" style="padding:7px 8px;border-bottom:1px solid #f3f4f6;font-variant-numeric:tabular-nums;">%s</td>`,
		html.EscapeString(value),
	)
}

func formatValue(value *float64) string {
	if value == nil {
		return "未配置"
	}
	// Prices range from 1e-6 to ~1e2; %g keeps both ends readable without a
	// fixed decimal count that would show 0.000001 as 0.00.
	return fmt.Sprintf("%g", *value)
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

// formatTimestamp renders a check time for the subject line and body. Local
// time, because the operator reads it in their own day.
func formatTimestamp(unix int64) string {
	return time.Unix(unix, 0).Format("2006-01-02 15:04")
}
