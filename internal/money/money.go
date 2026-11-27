// Package money 只做「分 → 元」的展示格式化（AGENTS.md：金额统一存分）。
package money

import "fmt"

// Yuan 把分格式化成两位小数的元，用于给代理/管理员看的提示文案。
func Yuan(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}
