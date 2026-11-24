// Package pricing 提供项目内统一的模型 token 价格估算。
package pricing

import "strings"

// USDToCNY 是当前项目账单展示使用的折算汇率。
const USDToCNY = 7.30

// EstimateCNY 返回一次模型调用的成本估算，单位人民币。
// 未收录的模型返回 0，调用方应把 0 视为“未知价格”而不是免费。
func EstimateCNY(modelName string, inputTokens, outputTokens int64) float64 {
	inputPrice, outputPrice := PriceCNYPerMillion(modelName)
	return float64(inputTokens)/1_000_000*inputPrice + float64(outputTokens)/1_000_000*outputPrice
}

// EstimateUSD 返回美元成本，便于 RunBudget 使用 MaxCostUSD 约束。
func EstimateUSD(modelName string, inputTokens, outputTokens int64) float64 {
	return EstimateCNY(modelName, inputTokens, outputTokens) / USDToCNY
}

// PriceCNYPerMillion 返回模型每百万 token 的输入/输出价格，单位人民币。
// 价格表集中在这里，避免 manager 和 runtime 各自维护一份。
func PriceCNYPerMillion(modelName string) (inputCNY, outputCNY float64) {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.Contains(normalized, "glm-4.7") || strings.Contains(normalized, "glm4.7"):
		// GLM-4.7：输入 $0.6/M，输出 $2.2/M。
		return 0.6 * USDToCNY, 2.2 * USDToCNY
	case strings.Contains(normalized, "glm-4-flash"), strings.Contains(normalized, "glm-4.5-flash"):
		return 0, 0
	default:
		return 0, 0
	}
}
