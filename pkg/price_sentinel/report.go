package pricesentinel

import (
	"fmt"
	"sort"
	"strings"
	"time"

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

// BuildReport renders the digest body.
//
// The two sections are split by direction on purpose: a model we charge more
// for risks losing customers, one we charge less for is margin being given
// away. They call for opposite responses, so they are never mixed into one
// list. The edit link is appended by the caller, which is the only part that
// needs the token.
func BuildReport(comparison *Comparison, checkedAt int64, editUrl string) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("上游价格巡检报告（%s）\n", formatTimestamp(checkedAt)))
	builder.WriteString(fmt.Sprintf(
		"\n对比模型 %d 个，价格不一致 %d 个。\n",
		comparison.Compared,
		len(comparison.Changes),
	))

	higher, lower := splitByDirection(comparison.Changes)

	builder.WriteString("\n一、我们更贵（可能流失客户）\n")
	if len(higher) == 0 {
		builder.WriteString("  无\n")
	} else {
		writeRows(&builder, higher)
	}

	builder.WriteString("\n二、我们更便宜（毛利被压缩）\n")
	if len(lower) == 0 {
		builder.WriteString("  无\n")
	} else {
		writeRows(&builder, lower)
	}

	if editUrl != "" {
		builder.WriteString("\n三、改价入口\n")
		builder.WriteString("  " + editUrl + "\n")
		builder.WriteString("  链接一次性有效，点击后需输入管理员访问令牌才能改价。\n")
	}

	builder.WriteString("\n本报告只做对比，不会自动改价。是否跟进由你决定：\n")
	builder.WriteString("  上游涨价 → 跟进保利润，不跟进保客户\n")
	builder.WriteString("  上游降价 → 跟进保客户，不跟进保利润\n")
	return builder.String()
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

func writeRows(builder *strings.Builder, rows []model.PriceChange) {
	shown := min(len(rows), maxRowsPerSection)
	for _, change := range rows[:shown] {
		builder.WriteString(fmt.Sprintf(
			"  %-44s %-18s 我们 %-12s 上游 %-12s (%+.2f%%)\n",
			truncate(change.ModelName, 44),
			change.Field,
			formatValue(change.Ours),
			formatValue(change.Theirs),
			change.Percent,
		))
		if len(change.Groups) > 0 {
			builder.WriteString(fmt.Sprintf("      分组: %s\n", strings.Join(change.Groups, ", ")))
		}
	}
	if len(rows) > shown {
		builder.WriteString(fmt.Sprintf("  ... 其余 %d 个已省略\n", len(rows)-shown))
	}
}

func formatValue(value *float64) string {
	if value == nil {
		return "未配置"
	}
	// Prices range from 1e-6 to ~1e2; %g keeps both ends readable without a
	// fixed decimal count that would show 0.000001 as 0.00.
	return fmt.Sprintf("%g", *value)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit-1] + "…"
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
