package decision

import (
	"encoding/json"
	"fmt"
	"log"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
	"strings"
	"time"
)

const (
	// tradingFeeRate 表示单边开仓/平仓手续费 (0.0432%)
	tradingFeeRate = 0.000432
)

// PositionInfo 持仓信息
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"` // 持仓更新时间戳（毫秒）
}

// AccountInfo 账户信息
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // 账户净值
	AvailableBalance float64 `json:"available_balance"` // 可用余额
	TotalPnL         float64 `json:"total_pnl"`         // 总盈亏
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // 总盈亏百分比
	MarginUsed       float64 `json:"margin_used"`       // 已用保证金
	MarginUsedPct    float64 `json:"margin_used_pct"`   // 保证金使用率
	PositionCount    int     `json:"position_count"`    // 持仓数量
}

// CandidateCoin 候选币种（来自币种池）
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // 来源: "ai500" 和/或 "oi_top"
}

// OITopData 持仓量增长Top数据（用于AI决策参考）
type OITopData struct {
	Rank              int     // OI Top排名
	OIDeltaPercent    float64 // 持仓量变化百分比（1小时）
	OIDeltaValue      float64 // 持仓量变化价值
	PriceDeltaPercent float64 // 价格变化百分比
	NetLong           float64 // 净多仓
	NetShort          float64 // 净空仓
}

// Context 交易上下文（传递给AI的完整信息）
type Context struct {
	CurrentTime            string                       `json:"current_time"`
	RuntimeMinutes         int                          `json:"runtime_minutes"`
	CallCount              int                          `json:"call_count"`
	Account                AccountInfo                  `json:"account"`
	Positions              []PositionInfo               `json:"positions"`
	CandidateCoins         []CandidateCoin              `json:"candidate_coins"`
	MarketDataMap          map[string]*market.Data      `json:"-"` // 不序列化，但内部使用
	OITopDataMap           map[string]*market.OITopData `json:"-"` // OI Top数据映射
	Performance            interface{}                  `json:"-"` // 历史表现分析（logger.PerformanceAnalysis）
	BTCETHLeverage         int                          `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage        int                          `json:"-"` // 山寨币杠杆倍数（从配置读取）
	CoinWhitelistEnabled   bool                         `json:"-"` // 是否启用币种白名单
	CoinWhitelist          []string                     `json:"-"` // 币种白名单列表
	Exchange               string                       `json:"-"` // 交易所类型: "binance", "hyperliquid", "aster"
	MaxPositionCount       int                          `json:"-"` // 最多持仓币种数量
	SingleTradeMarginRatio float64                      `json:"-"` // 单笔开仓保证金比例（0-1）
	DecisionLogger         *logger.DecisionLogger       `json:"-"` // 决策日志记录器，用于获取历史思维链
}

// Decision AI的交易决策
type Decision struct {
	Symbol             string  `json:"symbol"`
	Action             string  `json:"action"` // "open_long", "open_short", "close_long", "close_short", "hold", "wait"
	Leverage           int     `json:"leverage,omitempty"`
	PositionSizeUSD    float64 `json:"position_size_usd,omitempty"`
	StopLoss           float64 `json:"stop_loss,omitempty"`
	TakeProfit         float64 `json:"take_profit,omitempty"`
	Confidence         int     `json:"confidence,omitempty"`           // 信心度 (0-100)
	RiskUSD            float64 `json:"risk_usd,omitempty"`             // 最大美元风险
	ObservationTimeMin int     `json:"observation_time_min,omitempty"` // 持仓观察时间（分钟），开仓时必填
	Reasoning          string  `json:"reasoning"`
}

// FullDecision AI的完整决策（包含思维链）
type FullDecision struct {
	UserPrompt string     `json:"user_prompt"` // 发送给AI的输入prompt
	CoTTrace   string     `json:"cot_trace"`   // 思维链分析（AI输出）
	Decisions  []Decision `json:"decisions"`   // 具体决策列表
	Timestamp  time.Time  `json:"timestamp"`
}

// GetFullDecision 获取AI的完整交易决策（一次性分析所有币种和持仓）
// 由于使用了K线分析缓存，token消耗已大大减少，可以一次性发送所有币种信息
func GetFullDecision(ctx *Context, mcpClient *mcp.Client) (*FullDecision, error) {
	// 1. 为所有币种获取市场数据
	if err := fetchMarketDataForContext(ctx); err != nil {
		return nil, fmt.Errorf("获取市场数据失败: %w", err)
	}

	// 2. 构建 System Prompt（固定规则）
	systemPrompt := buildSystemPrompt(ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, ctx.MaxPositionCount, ctx.SingleTradeMarginRatio)

	// 3. 构建 User Prompt（包含所有持仓和候选币种）
	userPrompt := buildUserPrompt(ctx)

	// 4. 调用AI API获取决策
	log.Printf("📊 分析 %d 个持仓币种 + %d 个候选币种（一次性处理）", len(ctx.Positions), len(ctx.CandidateCoins))
	aiResponse, err := mcpClient.CallWithMessages(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("调用AI API失败: %w", err)
	}

	// 5. 解析AI响应
	fullDecision, err := parseFullDecisionResponse(aiResponse, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, ctx.Account.AvailableBalance, ctx.SingleTradeMarginRatio)
	if err != nil {
		return nil, fmt.Errorf("解析AI决策失败: %w", err)
	}

	// 6. 验证决策
	if err := validateDecisions(fullDecision.Decisions, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, ctx.Account.AvailableBalance, ctx.SingleTradeMarginRatio); err != nil {
		log.Printf("⚠️ 决策验证失败: %v，将返回部分决策", err)
		// 不直接返回错误，而是记录日志并继续
	}

	// 7. 最终验证：检查总持仓数限制、总保证金使用等
	newOpenPositions := 0
	totalRequiredMargin := 0.0
	for _, decision := range fullDecision.Decisions {
		if decision.Action == "open_long" || decision.Action == "open_short" {
			newOpenPositions++
			requiredMargin := decision.PositionSizeUSD / float64(decision.Leverage)
			totalRequiredMargin += requiredMargin
		}
	}

	if newOpenPositions > ctx.MaxPositionCount {
		log.Printf("⚠️ 新开仓数量 %d 超过限制 %d", newOpenPositions, ctx.MaxPositionCount)
	}

	if totalRequiredMargin > ctx.Account.AvailableBalance*1.1 {
		log.Printf("⚠️ 总所需保证金 %.2f USDT 超过可用余额 %.2f USDT（含10%%容差）", totalRequiredMargin, ctx.Account.AvailableBalance)
	}

	// 8. 构建最终结果
	result := &FullDecision{
		Decisions:  fullDecision.Decisions,
		CoTTrace:   fullDecision.CoTTrace,
		Timestamp:  time.Now(),
		UserPrompt: userPrompt,
	}

	log.Printf("✅ 决策完成: 共 %d 个决策", len(result.Decisions))
	return result, nil
}

// fetchMarketDataForContext 为上下文中的所有币种获取市场数据和OI数据
func fetchMarketDataForContext(ctx *Context) error {
	ctx.MarketDataMap = make(map[string]*market.Data)
	ctx.OITopDataMap = make(map[string]*market.OITopData)

	// 收集所有需要获取数据的币种
	symbolSet := make(map[string]bool)

	// 1. 优先获取持仓币种的数据（这是必须的）
	for _, pos := range ctx.Positions {
		symbolSet[pos.Symbol] = true
	}

	// 2. 候选币种数量根据账户状态动态调整，并应用白名单过滤
	maxCandidates := calculateMaxCandidates(ctx)
	for i, coin := range ctx.CandidateCoins {
		if i >= maxCandidates {
			break
		}

		// 应用白名单过滤
		if ctx.CoinWhitelistEnabled {
			if !isCoinInWhitelist(coin.Symbol, ctx.CoinWhitelist) {
				log.Printf("🚫 %s 不在白名单中，跳过", coin.Symbol)
				continue
			}
		}

		symbolSet[coin.Symbol] = true
	}

	// 并发获取市场数据
	// 持仓币种集合（用于判断是否跳过OI检查）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	for symbol := range symbolSet {
		data, err := market.Get(symbol, ctx.Exchange)
		if err != nil {
			// 单个币种失败不影响整体，只记录错误
			continue
		}

		// ⚠️ 流动性过滤：持仓价值低于15M USD的币种不做（多空都不做）
		// 持仓价值 = 持仓量 × 当前价格
		// 但现有持仓必须保留（需要决策是否平仓）
		isExistingPosition := positionSymbols[symbol]
		if !isExistingPosition && data.OpenInterest != nil && data.CurrentPrice > 0 {
			// 计算持仓价值（USD）= 持仓量 × 当前价格
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			oiValueInMillions := oiValue / 1_000_000 // 转换为百万美元单位
			if oiValueInMillions < 15 {
				log.Printf("⚠️  %s 持仓价值过低(%.2fM USD < 15M)，跳过此币种 [持仓量:%.0f × 价格:%.4f]",
					symbol, oiValueInMillions, data.OpenInterest.Latest, data.CurrentPrice)
				continue
			}
		}

		ctx.MarketDataMap[symbol] = data
	}

	// OI Top数据已禁用，暂时不使用
	// 如需启用，请取消以下注释
	/*
		// 加载OI Top数据（不影响主流程）
		oiPositions, err := pool.GetOITopPositions()
		if err == nil {
			for _, pos := range oiPositions {
				// 标准化符号匹配
				symbol := pos.Symbol
				oiData := &market.OITopData{
					Rank:              pos.Rank,
					OIDeltaPercent:    pos.OIDeltaPercent,
					OIDeltaValue:      pos.OIDeltaValue,
					PriceDeltaPercent: pos.PriceDeltaPercent,
					NetLong:           pos.NetLong,
					NetShort:          pos.NetShort,
				}
				ctx.OITopDataMap[symbol] = oiData

				// 将OI Top数据添加到对应的MarketData中
				if marketData, exists := ctx.MarketDataMap[symbol]; exists {
					marketData.OITopData = oiData
				}
			}
		}

		// 加载Hyperliquid OI数据（不影响主流程）
		hyperliquidOIPositions, err := pool.GetHyperliquidOIData()
		if err == nil {
			for _, pos := range hyperliquidOIPositions {
				// 检查是否在白名单中
				if ctx.CoinWhitelistEnabled {
					if !isCoinInWhitelist(pos.Symbol, ctx.CoinWhitelist) {
						continue
					}
				}

				// 添加到OI Top数据映射中（使用Hyperliquid数据）
				// 将OI转换为USD：OI * 当前价格
				oiValueUSD := pos.OI
				if marketData, exists := ctx.MarketDataMap[pos.Symbol]; exists && marketData.CurrentPrice > 0 {
					oiValueUSD = pos.OI * marketData.CurrentPrice
				}

				oiData := &market.OITopData{
					Rank:              0, // Hyperliquid数据没有排名
					OIDeltaPercent:    0, // Hyperliquid数据没有变化百分比
					OIDeltaValue:      oiValueUSD, // 使用转换为USD的OI值
					PriceDeltaPercent: 0, // Hyperliquid数据没有价格变化
					NetLong:           0, // Hyperliquid数据没有净多空
					NetShort:          0,
				}
				ctx.OITopDataMap[pos.Symbol] = oiData

				// 将Hyperliquid OI数据添加到对应的MarketData中
				if marketData, exists := ctx.MarketDataMap[pos.Symbol]; exists {
					marketData.OITopData = oiData
				}
			}
		}
	*/

	return nil
}

// calculateMaxCandidates 根据账户状态计算需要分析的候选币种数量
func calculateMaxCandidates(ctx *Context) int {
	// 直接返回候选池的全部币种数量
	// 因为候选池已经在 auto_trader.go 中筛选过了
	// 固定分析前20个评分最高的币种（来自AI500）
	return len(ctx.CandidateCoins)
}

// isCoinInWhitelist 检查币种是否在白名单中
func isCoinInWhitelist(coin string, whitelist []string) bool {
	for _, whitelistCoin := range whitelist {
		if whitelistCoin == coin {
			return true
		}
	}
	return false
}

// buildSystemPrompt 构建 System Prompt（固定规则，可缓存）
func buildSystemPrompt(accountEquity float64, btcEthLeverage, altcoinLeverage int, maxPositionCount int, singleTradeMarginRatio float64) string {
	var sb strings.Builder

	// === 核心使命 ===
	sb.WriteString("你是专业的加密货币交易AI，在加密货币合约市场进行自主交易。\n\n")
	sb.WriteString("# 🎯 核心目标\n\n")
	sb.WriteString("**最大化夏普比率（Sharpe Ratio）**\n\n")
	sb.WriteString("夏普比率 = 平均收益 / 收益波动率\n\n")
	sb.WriteString("**这意味着**：\n")
	sb.WriteString("- ✅ 高质量交易（高胜率、大盈亏比）→ 提升夏普\n")
	sb.WriteString("- ✅ 稳定收益、控制回撤 → 提升夏普\n")
	sb.WriteString("- ✅ 耐心持仓、让利润奔跑 → 提升夏普\n")
	sb.WriteString("- ❌ 频繁交易、小盈小亏 → 增加波动，严重降低夏普\n")
	sb.WriteString("- ❌ 过度交易、手续费损耗 → 直接亏损\n")
	sb.WriteString("- ❌ 过早平仓、频繁进出 → 错失大行情\n\n")
	sb.WriteString("**关键认知**: 系统每3分钟扫描一次，但不意味着每次都要交易！\n")
	sb.WriteString("大多数时候应该是 `wait` 或 `hold`，只在极佳机会时才开仓。\n\n")

	// === 硬约束（风险控制）===
	sb.WriteString("# ⚖️ 硬约束（风险控制）\n\n")
	sb.WriteString("1. **风险回报比**: 必须 ≥ 1:3（冒1%风险，赚3%+收益）\n")
	sb.WriteString(fmt.Sprintf("2. **最多持仓**: %d个币种（质量>数量）\n", maxPositionCount))
	sb.WriteString(fmt.Sprintf("3. **单币仓位大小**（position_size_usd，USD计价）: 山寨币 %.0f-%.0f USDT（%dx杠杆）| BTC/ETH %.0f-%.0f USDT（%dx杠杆）\n",
		accountEquity*0.8, accountEquity*1.5, altcoinLeverage, accountEquity*5, accountEquity*10, btcEthLeverage))
	// === 保证金与资金管理 ===
	sb.WriteString("# 🛡️ 资金管理 (铁律)\n\n")
	sb.WriteString("1. **永远以现金流安全为先**：亏损前不要扩仓\n\n")
	sb.WriteString("2. **严格遵守单笔保证金比例**：\n")
	sb.WriteString(fmt.Sprintf("   - 单笔开仓保证金比例：%.0f%%\n", singleTradeMarginRatio*100))
	sb.WriteString("   - ⚠️ 绝对不允许超过可用余额！\n")
	sb.WriteString(fmt.Sprintf("   - 📏 标准单笔保证金 = 账户净值 × %.0f%%\n", singleTradeMarginRatio*100))
	sb.WriteString("   - ⚠️ 在保证金充足时应优先以此为目标\n\n")
	sb.WriteString("3. **总使用率限制**：\n")
	sb.WriteString(fmt.Sprintf("   - 总使用率上限：≤ %.0f%%\n", 90.0))
	sb.WriteString("   - 🚨 超过此限制将触发警报\n\n")
	sb.WriteString("4. **保证金管理**（⚠️ 严格约束！）：\n")
	sb.WriteString("   - 总使用率上限：≤ 90%\n")
	sb.WriteString(fmt.Sprintf("   - ⚠️ **每笔新开仓**：单笔保证金必须≤账户净值的%.0f%%（硬性上限）\n", singleTradeMarginRatio*100))
	standardMargin := accountEquity * singleTradeMarginRatio
	if standardMargin > 0 {
		if standardMargin > accountEquity {
			standardMargin = accountEquity
		}
		sb.WriteString(fmt.Sprintf("   - 📏 **标准单笔保证金**：账户净值%.2f × %.0f%% = %.2f USDT；在保证金充足时应优先以此为目标\n", accountEquity, singleTradeMarginRatio*100, standardMargin))
	}
	sb.WriteString("   - 🚨 **绝对硬约束**：单笔保证金绝对不能超过可用余额（AvailableBalance）！\n")
	sb.WriteString("   - 📊 **计算公式**：单笔保证金 = position_size_usd / leverage\n")
	sb.WriteString("   - 🚨 **最大仓位限制**：position_size_usd ≤ 可用余额 × leverage\n")
	sb.WriteString("   - ⚠️ **必须检查**：开仓前务必确认 (position_size_usd / leverage) ≤ 可用余额\n")
	sb.WriteString("   - ⚠️ **避免全仓**：不要一次性使用过多保证金，应分散风险\n")
	sb.WriteString("   - ⚠️ **预留缓冲**：始终保持至少10-20%的可用保证金，以应对波动和追加保证金需求\n")
	sb.WriteString("   - 💡 **示例**：如果可用余额是837.14 USDT，杠杆5倍，则最大仓位 = 837.14 × 5 = 4185.70 USDT\n")
	sb.WriteString("   - 🚨 **验证失败将被拒绝**：如果保证金超过可用余额，整个决策将被拒绝，必须重新计算\n\n")
	sb.WriteString("5. **禁止突破追价**：在阻力位不许追多，在支撑位不许追空；若出现突破/跌破，必须等待回踩确认后再评估，严禁直接追单。\n\n")
	sb.WriteString("6. **交易手续费**：开仓和平仓各收0.0432%，往返约0.0864%；所有风险/收益计算必须先扣除这部分成本。\n\n")

	// === 交易哲学 & 最佳实践 ===
	sb.WriteString("# 🎯 交易哲学 & 最佳实践\n\n")
	sb.WriteString("## 核心原则：\n\n")
	sb.WriteString("**资金保全第一**：保护资本比追求收益更重要\n\n")
	sb.WriteString("**纪律胜于情绪**：执行你的退出方案，不随意移动止损或目标\n\n")
	sb.WriteString("**质量优于数量**：少量高信念交易胜过大量低信念交易\n\n")
	sb.WriteString("**适应波动性**：根据市场条件调整仓位\n\n")
	sb.WriteString("**尊重趋势**：不要与强趋势作对\n\n")
	sb.WriteString("**🎯 核心分析方法（最高优先级）**：\n")
	sb.WriteString("- 🔥 **支撑位/阻力位分析是决策的核心基础**：系统提供K线数据，你必须首先识别和分析关键支撑/阻力位\n")
	sb.WriteString("- 🔥 **斐波那契分析是必执行步骤**：所有交易决策都必须基于斐波那契回撤和扩展分析\n")
	sb.WriteString("- 🔥 **多周期共振是关键**：优先寻找多周期（15分钟、1小时、4小时、1日）斐波那契位与支撑/阻力位重合的区域\n")
	sb.WriteString("- ⚠️ **重要**：支撑位/阻力位和斐波那契分析应该是你决策的主要依据，其他指标（MACD、RSI等）仅作为辅助确认\n\n")
	sb.WriteString("**斐波那契分析**（⚠️ 必须执行）：\n")
	sb.WriteString("- 📊 **使用斐波那契回撤和扩展**：基于15分钟、1小时、4小时、1日K线数据进行分析\n")
	sb.WriteString("- 📈 **识别关键高低点**：从各周期K线中找出明显的波段高点和低点\n")
	sb.WriteString("- 🔢 **计算斐波那契位**：使用经典斐波那契回撤位（0.236, 0.382, 0.5, 0.618, 0.786）和扩展位（1.272, 1.618, 2.0, 2.618）\n")
	sb.WriteString("- 🎯 **多周期共振**：重点识别多个周期（15分钟、1小时、4小时、1日）斐波那契位重合的区域，这些是关键支撑/阻力位\n")
	sb.WriteString("- ⚠️ **交易应用**：\n")
	sb.WriteString("  • 在上涨趋势中，回撤到斐波那契支撑位（0.382, 0.5, 0.618）附近寻找做多机会\n")
	sb.WriteString("  • 在下跌趋势中，反弹到斐波那契阻力位附近寻找做空机会\n")
	sb.WriteString("  • 突破关键斐波那契位后，使用扩展位（1.272, 1.618）作为目标位\n\n")
	sb.WriteString("**斐波那契0.618共振交易信号**（⚠️ 重要交易规则）：\n")
	sb.WriteString("- 🎯 **做多信号**：如果0.618斐波那契回撤位与某个强有力的支撑位（如历史低点、前支撑位、多周期共振支撑位等）形成共振，价格回撤到该位置附近时，这是强做多信号\n")
	sb.WriteString("- 🎯 **做空信号**：如果0.618斐波那契回撤位（在下跌趋势中相当于反弹阻力位）与某个强有力的阻力位（如历史高点、前阻力位、多周期共振阻力位等）形成共振，价格反弹到该位置附近时，这是强做空信号\n")
	sb.WriteString("- ⚠️ **共振判断标准**：\n")
	sb.WriteString("  • 斐波那契位与关键支撑/阻力位的价格差异在0.5%以内，视为共振\n")
	sb.WriteString("  • 多周期（至少2个周期）同时出现共振，信号更强\n")
	sb.WriteString("  • 结合成交量、K线形态（如锤子线、吞没形态等）确认信号强度\n")
	sb.WriteString("- 📊 **交易优先级**：0.618共振信号是高质量交易机会，优先级高于单一斐波那契位或单一支撑/阻力位\n\n")
	sb.WriteString("**支撑阻力位转换分析**（⚠️ 必须执行）：\n")
	sb.WriteString("- 📊 **结合K线和斐波那契**：根据K线数据和高低点，使用斐波那契分析识别关键支撑/阻力位\n")
	sb.WriteString("- 🔍 **必须分析**：哪些阻力位（包括斐波那契阻力位）已经被突破转为支撑位（当前价格已在该阻力位上方）\n")
	sb.WriteString("- 🔍 **必须分析**：哪些支撑位（包括斐波那契支撑位）已经被跌破转为阻力位（当前价格已在该支撑位下方）\n")
	sb.WriteString("- 📊 **判断依据**：通过比较当前价格与K线数据中的关键高低点和斐波那契位的位置关系\n")
	sb.WriteString("- 🎯 **交易意义**：突破后的阻力转支撑位成为新的支撑，跌破后的支撑转阻力位成为新的阻力\n")
	sb.WriteString("- ⚠️ **重要**：重点关注多周期共振的关键价位（特别是斐波那契位重合的区域），这些位置通常更重要\n\n")
	sb.WriteString("**级别匹配策略**：当信号来自4h/12h/1d等较高周期的支撑或阻力时，必须相应拉大止盈距离、延长持有时间，不得只顾短时波动；至少计划风险回报≥1:4，并给出持仓时间目标。\n\n")
	sb.WriteString("**持仓观察时间要求**：\n")
	sb.WriteString("- 每次开仓时必须在JSON中返回 `observation_time_min` 字段（分钟）\n")
	sb.WriteString("- 表示你计划持有该仓位多长时间进行观察和评估\n")
	sb.WriteString("- 最短30分钟，根据信号级别调整：\n")
	sb.WriteString("  • 3分钟级别信号：30-60分钟\n")
	sb.WriteString("  • 15分钟级别信号：60-120分钟\n")
	sb.WriteString("  • 1小时级别信号：120-240分钟\n")
	sb.WriteString("  • 4小时及以上级别信号：240分钟以上\n")
	sb.WriteString("- 在观察时间内，除非触发止损或止盈，否则应保持持仓\n\n")
	sb.WriteString("## 常见误区避免：\n\n")
	sb.WriteString("⚠️ **过度交易**：频繁交易导致费用侵蚀利润\n\n")
	sb.WriteString("⚠️ **复仇式交易**：亏损后立即加码试图\"翻本\"\n\n")
	sb.WriteString("⚠️ **分析瘫痪**：过度等待完美信号，导致失机\n\n")
	sb.WriteString("⚠️ **忽视相关性**：BTC常引领山寨币，须优先观察BTC\n\n")
	sb.WriteString("⚠️ **过度杠杆**：放大收益同时放大亏损\n\n")

	// === 交易频率认知 ===
	sb.WriteString("# ⏱️ 交易频率认知\n\n")
	sb.WriteString("**量化标准**:\n")
	sb.WriteString("- 优秀交易员：每天2-4笔 = 每小时0.1-0.2笔\n")
	sb.WriteString("- 过度交易：每小时>2笔 = 严重问题\n")
	// 这里原本写的30-60分钟，感觉会使llm误以为60分钟后就可以平仓
	sb.WriteString("- 最佳节奏：开仓后持有至少30分钟以上\n\n")
	sb.WriteString("**自查**:\n")
	sb.WriteString("如果你发现自己每个周期都在交易 → 说明标准太低\n")
	sb.WriteString("如果你发现持仓<30分钟就平仓 → 说明太急躁\n\n")

	// === 开仓信号强度 ===
	sb.WriteString("# 🎯 开仓标准（严格）\n\n")
	sb.WriteString("只在**强信号**时开仓，不确定就观望。\n\n")
	sb.WriteString("**🔥 核心分析方法（必须优先执行）**：\n\n")
	sb.WriteString("**第一步：支撑位/阻力位识别（必执行）**：\n")
	sb.WriteString("- 📊 仔细分析K线数据（15分钟、1小时、4小时、12小时、1日），识别关键的高点和低点\n")
	sb.WriteString("- 📊 标记历史价格多次触及但未突破的位置（这些是强支撑/阻力位）\n")
	sb.WriteString("- 📊 识别整数位、心理价位等关键位置\n")
	sb.WriteString("- 📊 分析哪些阻力位已突破转为支撑，哪些支撑位已跌破转为阻力\n\n")
	sb.WriteString("**第二步：斐波那契分析（必执行）**：\n")
	sb.WriteString("- 🔢 基于15分钟、1小时、4小时、1日K线数据，识别关键波段高点和低点\n")
	sb.WriteString("- 🔢 计算斐波那契回撤位（0.236, 0.382, 0.5, 0.618, 0.786）和扩展位（1.272, 1.618, 2.0, 2.618）\n")
	sb.WriteString("- 🔢 识别多周期斐波那契位重合的区域（这些是关键支撑/阻力位）\n")
	sb.WriteString("- 🔢 分析当前价格相对于各周期斐波那契位的位置\n")
	sb.WriteString("- 🔢 特别关注0.618斐波那契位与其他支撑/阻力位的共振\n\n")
	sb.WriteString("**第三步：多周期共振分析（必执行）**：\n")
	sb.WriteString("- 🎯 找出多个周期（至少2个周期）的斐波那契位与支撑/阻力位重合的区域\n")
	sb.WriteString("- 🎯 这些共振区域是最高质量的交易机会\n")
	sb.WriteString("- 🎯 优先考虑这些共振区域附近的交易信号\n\n")
	sb.WriteString("**辅助数据**（用于确认信号强度）：\n")
	sb.WriteString("- 📈 **技术序列**：EMA20序列、MACD序列、RSI7序列、RSI14序列\n")
	sb.WriteString("- 💰 **资金序列**：成交量序列、持仓量(OI)序列、资金费率\n")
	sb.WriteString("- 🎯 **筛选标记**：AI500评分 / OI_Top排名（如果有标注）\n\n")
	sb.WriteString("**⚠️ 重要决策原则**：\n")
	sb.WriteString("- 🔥 **优先依据**：支撑位/阻力位和斐波那契分析应该是你决策的主要依据\n")
	sb.WriteString("- 🔥 **辅助确认**：其他技术指标（MACD、RSI等）仅作为辅助确认，不应作为主要决策依据\n")
	sb.WriteString("- 🔥 **交易规则**：支撑位只寻找做多机会，阻力位只考虑做空或减仓\n")
	sb.WriteString("- 🔥 **共振优先**：多周期斐波那契位与支撑/阻力位共振的信号优先级最高\n")
	sb.WriteString("- 如果计划的止损/止盈基于4h及以上周期，请同步拉大持仓时长与目标收益，保持耐心，不可在短期波动中仓促退出\n")
	sb.WriteString("- 综合信心度 ≥ 75 才开仓\n\n")
	sb.WriteString("**避免低质量信号**：\n")
	sb.WriteString("- 单一维度（只看一个指标）\n")
	sb.WriteString("- 相互矛盾（涨但量萎缩）\n")
	sb.WriteString("- 横盘震荡\n")
	sb.WriteString("- 刚平仓不久（<15分钟）\n\n")

	// === 夏普比率自我进化 ===
	sb.WriteString("# 🧬 夏普比率自我进化\n\n")
	sb.WriteString("每次你会收到**夏普比率**作为绩效反馈（周期级别）：\n\n")
	sb.WriteString("**夏普比率 < -0.5** (持续亏损):\n")
	sb.WriteString("  → 🛑 停止交易，连续观望至少6个周期（18分钟）\n")
	sb.WriteString("  → 🔍 深度反思：\n")
	sb.WriteString("     • 交易频率过高？（每小时>2次就是过度）\n")
	sb.WriteString("     • 持仓时间过短？（<30分钟就是过早平仓）\n")
	sb.WriteString("     • 信号强度不足？（信心度<75）\n")
	sb.WriteString("     • 风险回报比是否达标？（必须≥1:3）\n\n")
	sb.WriteString("**夏普比率 -0.5 ~ 0** (轻微亏损):\n")
	sb.WriteString("  → ⚠️ 严格控制：只做信心度>80的交易\n")
	sb.WriteString("  → 减少交易频率：每小时最多1笔新开仓\n")
	sb.WriteString("  → 耐心持仓：至少持有30分钟以上\n\n")
	sb.WriteString("**夏普比率 0 ~ 0.7** (正收益):\n")
	sb.WriteString("  → ✅ 维持当前策略\n\n")
	sb.WriteString("**夏普比率 > 0.7** (优异表现):\n")
	sb.WriteString("  → 🚀 可适度扩大仓位\n\n")
	sb.WriteString("**关键**: 夏普比率是唯一指标，它会自然惩罚频繁交易和过度进出。\n\n")

	// === 决策流程 ===
	sb.WriteString("# 📋 决策流程\n\n")
	sb.WriteString("1. **分析夏普比率**: 当前策略是否有效？需要调整吗？\n")
	sb.WriteString("2. **评估持仓**: 趋势是否改变？是否该止盈/止损？\n")
	sb.WriteString("3. **寻找新机会**（🔥 核心步骤）：\n")
	sb.WriteString("   a. **首先分析支撑位/阻力位**：基于K线数据识别关键价位\n")
	sb.WriteString("   b. **执行斐波那契分析**：计算各周期的斐波那契回撤和扩展位\n")
	sb.WriteString("   c. **寻找多周期共振**：找出斐波那契位与支撑/阻力位重合的区域\n")
	sb.WriteString("   d. **评估交易信号**：优先考虑共振区域的交易机会\n")
	sb.WriteString("   e. **辅助确认**：使用其他技术指标（MACD、RSI等）确认信号强度\n")
	sb.WriteString("4. **输出决策**: 思维链分析 + JSON（必须包含对支撑位/阻力位和斐波那契的分析说明）\n\n")

	// === 输出格式 ===
	sb.WriteString("# 📤 输出格式\n\n")
	sb.WriteString("**第一步: 思维链（纯文本）**\n")
	sb.WriteString("简洁分析你的思考过程\n")
	sb.WriteString("🔥 **必须包含以下分析内容**：\n")
	sb.WriteString("- 支撑位/阻力位识别：你识别了哪些关键支撑位和阻力位？\n")
	sb.WriteString("- 斐波那契分析：各周期的斐波那契回撤位和扩展位在哪里？\n")
	sb.WriteString("- 多周期共振：是否有斐波那契位与支撑/阻力位重合的区域？\n")
	sb.WriteString("- 当前价格位置：当前价格相对于这些关键价位的位置如何？\n")
	sb.WriteString("- 交易信号判断：基于支撑位/阻力位和斐波那契分析得出的交易信号\n\n")
	sb.WriteString("**第二步: JSON决策数组**\n\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_long\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 90000, \"take_profit\": 97000, \"confidence\": 85, \"risk_usd\": 300, \"observation_time_min\": 60, \"reasoning\": \"价格回撤至0.618斐波那契支撑位+多周期共振+4h前阻力位转支撑，MACD确认\"},\n", btcEthLeverage, accountEquity*5))
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"ETHUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 3200, \"take_profit\": 3000, \"confidence\": 80, \"risk_usd\": 200, \"observation_time_min\": 90, \"reasoning\": \"价格反弹至0.618斐波那契阻力位+1h和4h周期共振+前高点阻力位，RSI超买确认\"},\n", btcEthLeverage, accountEquity*3))
	sb.WriteString("  {\"symbol\": \"SOLUSDT\", \"action\": \"close_long\", \"reasoning\": \"达到1.618斐波那契扩展目标位+前阻力位，止盈离场\"}\n")
	sb.WriteString("]\n```\n\n")
	sb.WriteString("**字段说明**:\n")
	sb.WriteString("- `action`: open_long | open_short | close_long | close_short | hold | wait\n")
	sb.WriteString("- `confidence`: 0-100（开仓建议≥75）\n")
	sb.WriteString("- `observation_time_min`: 持仓观察时间（分钟），开仓时必填。表示你计划持有该仓位多长时间进行观察，至少30分钟以上。\n")
	sb.WriteString("- 开仓时必填: leverage, position_size_usd, stop_loss, take_profit, confidence, risk_usd, observation_time_min, reasoning\n\n")

	// === 关键提醒 ===
	sb.WriteString("---\n\n")
	sb.WriteString("**记住**: \n")
	sb.WriteString("- 目标是夏普比率，不是交易频率\n")
	sb.WriteString("- 做多和做空都是平等的交易工具，根据市场信号选择方向，不要有方向偏好\n")
	sb.WriteString("- 宁可错过，不做低质量交易\n")
	sb.WriteString("- 风险回报比1:3是底线\n")

	sb.WriteString("**最终指令**: \n")
	sb.WriteString("- 仔细阅读整个用户 Prompt 后再进行决策\n")
	sb.WriteString("- 核实你的头寸规模计算（双重检查数学）\n")
	sb.WriteString("- 确保你的 JSON 输出合法且完整\n")
	sb.WriteString("- 提供诚实的 confidence 分数（不要夸大信念）\n")
	sb.WriteString("- 与你的退出方案保持一致（不要提前取消止损或目标）\n")

	return sb.String()
}

// summarizeConfluenceLevels 已移除，不再使用支撑阻力位数据

// buildUserPromptBatch 构建单批币种的 User Prompt（用于分批处理）
// positionSymbols: 本批要分析的持仓币种符号列表
// candidateSymbols: 本批要分析的候选币种符号列表
// candidateCoinMap: 候选币种映射表（用于获取source信息等）
// totalCandidates: 候选币种总数（用于显示进度）
// batchNum: 批次编号（用于显示进度）
func buildUserPromptBatch(ctx *Context, positionSymbols []string, candidateSymbols []string, candidateCoinMap map[string]CandidateCoin, totalCandidates int, batchNum int) string {
	var sb strings.Builder

	// 系统状态
	sb.WriteString(fmt.Sprintf("**时间**: %s | **周期**: #%d | **运行**: %d分钟\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// 批次信息
	if batchNum > 0 {
		sb.WriteString(fmt.Sprintf("**批次信息**: 这是第 %d 批次分析（候选币种总计 %d 个）\n\n", batchNum, totalCandidates))
	}

	// 白名单状态
	if ctx.CoinWhitelistEnabled {
		sb.WriteString(fmt.Sprintf("**币种白名单**: 已启用，仅交易以下%d个币种: %s\n\n",
			len(ctx.CoinWhitelist), strings.Join(ctx.CoinWhitelist, ", ")))
	} else {
		sb.WriteString("**币种白名单**: 未启用，可交易所有币种\n\n")
	}

	// BTC 市场
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		sb.WriteString(fmt.Sprintf("**BTC**: %.2f (1h: %+.2f%%, 4h: %+.2f%%) | MACD: %.4f | RSI: %.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}

	// 账户
	sb.WriteString(fmt.Sprintf("**账户**: 净值%.2f | 余额%.2f (%.1f%%) | 盈亏%+.2f%% | 保证金%.1f%% | 持仓%d个\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100,
		ctx.Account.TotalPnLPct,
		ctx.Account.MarginUsedPct,
		ctx.Account.PositionCount))

	// 保证金使用信息
	recommendedSingleTradeMargin := ctx.Account.TotalEquity * ctx.SingleTradeMarginRatio
	actualAvailableBalance := ctx.Account.AvailableBalance

	// 显示保证金基本信息
	sb.WriteString(fmt.Sprintf("**保证金信息**: 可用余额%.2f USDT | 单笔标准保证金=%.0f USDT（账户净值的%.0f%%）\n\n",
		actualAvailableBalance, recommendedSingleTradeMargin, ctx.SingleTradeMarginRatio*100))

	// 保证金约束说明
	if actualAvailableBalance > 0 {
		sb.WriteString("**保证金约束**:\n")
		sb.WriteString(fmt.Sprintf("   单笔保证金 = position_size_usd / leverage ≤ min(%.2f USDT（标准额度）, %.2f USDT（可用余额）)\n",
			recommendedSingleTradeMargin, actualAvailableBalance))
		sb.WriteString(fmt.Sprintf("   最大仓位大小 ≤ min(%.2f × leverage, %.2f × leverage)\n",
			recommendedSingleTradeMargin, actualAvailableBalance))

		// BTC/ETH 的最大仓位
		if ctx.BTCETHLeverage > 0 {
			maxBTCETHPosition := actualAvailableBalance * float64(ctx.BTCETHLeverage)
			sb.WriteString(fmt.Sprintf("   BTC/ETH（%dx杠杆）最大仓位 = %.2f × %d = %.2f USDT\n", ctx.BTCETHLeverage, actualAvailableBalance, ctx.BTCETHLeverage, maxBTCETHPosition))
		}

		// 山寨币的最大仓位
		if ctx.AltcoinLeverage > 0 {
			maxAltcoinPosition := actualAvailableBalance * float64(ctx.AltcoinLeverage)
			sb.WriteString(fmt.Sprintf("   山寨币（%dx杠杆）最大仓位 = %.2f × %d = %.2f USDT\n", ctx.AltcoinLeverage, actualAvailableBalance, ctx.AltcoinLeverage, maxAltcoinPosition))
		}

		sb.WriteString("\n")
	}

	sb.WriteString("⚠️ 手续费提醒：开仓和平仓各收0.0432%，往返约0.0864%；净收益需扣除该成本后再评估。\n\n")

	// 本批持仓币种（完整市场数据）
	if len(positionSymbols) > 0 {
		sb.WriteString(fmt.Sprintf("## 当前持仓（本批分析 %d 个）\n", len(positionSymbols)))
		for i, symbol := range positionSymbols {
			// 查找对应的持仓信息
			var pos *PositionInfo
			for j := range ctx.Positions {
				if ctx.Positions[j].Symbol == symbol {
					pos = &ctx.Positions[j]
					break
				}
			}
			if pos == nil {
				continue
			}

			// 计算持仓时长
			holdingDuration := ""
			if pos.UpdateTime > 0 {
				durationMs := time.Now().UnixMilli() - pos.UpdateTime
				durationMin := durationMs / (1000 * 60)
				if durationMin < 60 {
					holdingDuration = fmt.Sprintf(" | 持仓时长%d分钟", durationMin)
				} else {
					durationHour := durationMin / 60
					durationMinRemainder := durationMin % 60
					holdingDuration = fmt.Sprintf(" | 持仓时长%d小时%d分钟", durationHour, durationMinRemainder)
				}
			}

			sb.WriteString(fmt.Sprintf("%d. %s %s | 入场价%.4f 当前价%.4f | 盈亏%+.2f%% | 杠杆%dx | 保证金%.0f | 强平价%.4f%s\n\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side),
				pos.EntryPrice, pos.MarkPrice, pos.UnrealizedPnLPct,
				pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, holdingDuration))

			// 使用FormatMarketData输出完整市场数据（持仓币种显示更多数据）
			if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
				sb.WriteString(market.Format(marketData, true)) // true表示是持仓币种
				sb.WriteString("\n")
			}
		}
	} else if batchNum == 0 {
		// 只有批次0（持仓批次）才显示"无持仓"
		sb.WriteString("**当前持仓**: 无\n\n")
	}

	// 本批候选币种（完整市场数据）
	if len(candidateSymbols) > 0 {
		sb.WriteString(fmt.Sprintf("## 候选币种（本批分析 %d 个）\n\n", len(candidateSymbols)))
		displayedCount := 0
		for _, symbol := range candidateSymbols {
			marketData, hasData := ctx.MarketDataMap[symbol]
			if !hasData {
				continue
			}
			displayedCount++

			// 获取候选币种信息
			coin, hasCoin := candidateCoinMap[symbol]
			sourceTags := ""
			if hasCoin {
				if len(coin.Sources) > 1 {
					sourceTags = " (AI500+OI_Top双重信号)"
				} else if len(coin.Sources) == 1 && coin.Sources[0] == "oi_top" {
					sourceTags = " (OI_Top持仓增长)"
				}

				// 检查是否有Hyperliquid OI数据
				if oiData, hasOIData := ctx.OITopDataMap[symbol]; hasOIData && oiData.OIDeltaValue > 0 {
					if sourceTags == "" {
						sourceTags = " (Hyperliquid OI数据)"
					} else {
						sourceTags += "+Hyperliquid OI"
					}
				}
			}

			// 使用FormatMarketData输出完整市场数据（候选币种显示精简数据以节省tokens）
			sb.WriteString(fmt.Sprintf("### %d. %s%s\n\n", displayedCount, symbol, sourceTags))
			sb.WriteString(market.Format(marketData, false)) // false表示是候选币种
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// 夏普比率（直接传值，不要复杂格式化）
	if ctx.Performance != nil {
		type PerformanceData struct {
			SharpeRatio float64 `json:"sharpe_ratio"`
		}
		var perfData PerformanceData
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfData); err == nil {
				sb.WriteString(fmt.Sprintf("## 📊 夏普比率: %.2f\n\n", perfData.SharpeRatio))
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析本批币种并输出决策（思维链 + JSON）\n")

	return sb.String()
}

// buildUserPrompt 构建 User Prompt（动态数据）- 保留原函数用于兼容
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// 系统状态
	sb.WriteString(fmt.Sprintf("**时间**: %s | **周期**: #%d | **运行**: %d分钟\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// 白名单状态
	if ctx.CoinWhitelistEnabled {
		sb.WriteString(fmt.Sprintf("**币种白名单**: 已启用，仅交易以下%d个币种: %s\n\n",
			len(ctx.CoinWhitelist), strings.Join(ctx.CoinWhitelist, ", ")))
	} else {
		sb.WriteString("**币种白名单**: 未启用，可交易所有币种\n\n")
	}

	// BTC 市场
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		sb.WriteString(fmt.Sprintf("**BTC**: %.2f (1h: %+.2f%%, 4h: %+.2f%%) | MACD: %.4f | RSI: %.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}

	// 账户
	sb.WriteString(fmt.Sprintf("**账户**: 净值%.2f | 余额%.2f (%.1f%%) | 盈亏%+.2f%% | 保证金%.1f%% | 持仓%d个\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100,
		ctx.Account.TotalPnLPct,
		ctx.Account.MarginUsedPct,
		ctx.Account.PositionCount))

	// 保证金使用信息
	recommendedSingleTradeMargin := ctx.Account.TotalEquity * ctx.SingleTradeMarginRatio // 使用配置的比例
	actualAvailableBalance := ctx.Account.AvailableBalance                               // 实际可用余额

	// 显示保证金基本信息
	sb.WriteString(fmt.Sprintf("**保证金信息**: 可用余额%.2f USDT | 单笔标准保证金=%.0f USDT（账户净值的%.0f%%）\n\n",
		actualAvailableBalance, recommendedSingleTradeMargin, ctx.SingleTradeMarginRatio*100))

	// 保证金约束说明
	if actualAvailableBalance > 0 {
		sb.WriteString("**保证金约束**:\n")
		sb.WriteString(fmt.Sprintf("   单笔保证金 = position_size_usd / leverage ≤ min(%.2f USDT（标准额度）, %.2f USDT（可用余额）)\n",
			recommendedSingleTradeMargin, actualAvailableBalance))
		sb.WriteString(fmt.Sprintf("   最大仓位大小 ≤ min(%.2f × leverage, %.2f × leverage)\n",
			recommendedSingleTradeMargin, actualAvailableBalance))

		// BTC/ETH 的最大仓位
		if ctx.BTCETHLeverage > 0 {
			maxBTCETHPosition := actualAvailableBalance * float64(ctx.BTCETHLeverage)
			sb.WriteString(fmt.Sprintf("   BTC/ETH（%dx杠杆）最大仓位 = %.2f × %d = %.2f USDT\n", ctx.BTCETHLeverage, actualAvailableBalance, ctx.BTCETHLeverage, maxBTCETHPosition))
		}

		// 山寨币的最大仓位
		if ctx.AltcoinLeverage > 0 {
			maxAltcoinPosition := actualAvailableBalance * float64(ctx.AltcoinLeverage)
			sb.WriteString(fmt.Sprintf("   山寨币（%dx杠杆）最大仓位 = %.2f × %d = %.2f USDT\n", ctx.AltcoinLeverage, actualAvailableBalance, ctx.AltcoinLeverage, maxAltcoinPosition))
		}

		sb.WriteString("\n")
	}

	// K线数据已包含在市场数据中，AI可以自己分析支撑阻力位

	sb.WriteString("⚠️ 手续费提醒：开仓和平仓各收0.0432%，往返约0.0864%；净收益需扣除该成本后再评估。\n\n")

	// 持仓（完整市场数据）
	if len(ctx.Positions) > 0 {
		sb.WriteString("## 当前持仓\n")
		for i, pos := range ctx.Positions {
			// 计算持仓时长
			holdingDuration := ""
			if pos.UpdateTime > 0 {
				durationMs := time.Now().UnixMilli() - pos.UpdateTime
				durationMin := durationMs / (1000 * 60) // 转换为分钟
				if durationMin < 60 {
					holdingDuration = fmt.Sprintf(" | 持仓时长%d分钟", durationMin)
				} else {
					durationHour := durationMin / 60
					durationMinRemainder := durationMin % 60
					holdingDuration = fmt.Sprintf(" | 持仓时长%d小时%d分钟", durationHour, durationMinRemainder)
				}
			}

			sb.WriteString(fmt.Sprintf("%d. %s %s | 入场价%.4f 当前价%.4f | 盈亏%+.2f%% | 杠杆%dx | 保证金%.0f | 强平价%.4f%s\n\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side),
				pos.EntryPrice, pos.MarkPrice, pos.UnrealizedPnLPct,
				pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, holdingDuration))

			// 使用FormatMarketData输出完整市场数据（持仓币种显示更多数据）
			if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
				sb.WriteString(market.Format(marketData, true)) // true表示是持仓币种
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("**当前持仓**: 无\n\n")
	}

	// 候选币种（完整市场数据）
	sb.WriteString(fmt.Sprintf("## 候选币种 (%d个)\n\n", len(ctx.MarketDataMap)))
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}
		displayedCount++

		sourceTags := ""
		if len(coin.Sources) > 1 {
			sourceTags = " (AI500+OI_Top双重信号)"
		} else if len(coin.Sources) == 1 && coin.Sources[0] == "oi_top" {
			sourceTags = " (OI_Top持仓增长)"
		}

		// 检查是否有Hyperliquid OI数据
		if oiData, hasOIData := ctx.OITopDataMap[coin.Symbol]; hasOIData && oiData.OIDeltaValue > 0 {
			if sourceTags == "" {
				sourceTags = " (Hyperliquid OI数据)"
			} else {
				sourceTags += "+Hyperliquid OI"
			}
		}

		// 使用FormatMarketData输出完整市场数据（候选币种显示精简数据以节省tokens）
		sb.WriteString(fmt.Sprintf("### %d. %s%s\n\n", displayedCount, coin.Symbol, sourceTags))
		sb.WriteString(market.Format(marketData, false)) // false表示是候选币种
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// 夏普比率（直接传值，不要复杂格式化）
	if ctx.Performance != nil {
		// 直接从interface{}中提取SharpeRatio
		type PerformanceData struct {
			SharpeRatio float64 `json:"sharpe_ratio"`
		}
		var perfData PerformanceData
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfData); err == nil {
				sb.WriteString(fmt.Sprintf("## 📊 夏普比率: %.2f\n\n", perfData.SharpeRatio))
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析并输出决策（思维链 + JSON）\n")

	return sb.String()
}

// parseFullDecisionResponse 解析AI的完整决策响应
func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int, availableBalance float64, singleTradeMarginRatio float64) (*FullDecision, error) {
	// 1. 提取思维链
	cotTrace := extractCoTTrace(aiResponse)

	// 2. 提取JSON决策列表
	decisions, err := extractDecisions(aiResponse)
	if err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: []Decision{},
		}, fmt.Errorf("提取决策失败: %w\n\n=== AI思维链分析 ===\n%s", err, cotTrace)
	}

	// 3. 验证决策
	if err := validateDecisions(decisions, accountEquity, btcEthLeverage, altcoinLeverage, availableBalance, singleTradeMarginRatio); err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: decisions,
		}, fmt.Errorf("决策验证失败: %w\n\n=== AI思维链分析 ===\n%s", err, cotTrace)
	}

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

// extractCoTTrace 提取思维链分析
func extractCoTTrace(response string) string {
	// 查找JSON数组的开始位置
	jsonStart := strings.Index(response, "[")

	if jsonStart > 0 {
		// 思维链是JSON数组之前的内容
		return strings.TrimSpace(response[:jsonStart])
	}

	// 如果找不到JSON，整个响应都是思维链
	return strings.TrimSpace(response)
}

// extractDecisions 提取JSON决策列表
func extractDecisions(response string) ([]Decision, error) {
	// 直接查找JSON数组 - 找第一个完整的JSON数组
	arrayStart := strings.Index(response, "[")
	if arrayStart == -1 {
		return nil, fmt.Errorf("无法找到JSON数组起始")
	}

	// 从 [ 开始，匹配括号找到对应的 ]
	arrayEnd := findMatchingBracket(response, arrayStart)
	if arrayEnd == -1 {
		return nil, fmt.Errorf("无法找到JSON数组结束")
	}

	jsonContent := strings.TrimSpace(response[arrayStart : arrayEnd+1])

	// 🔧 修复常见的JSON格式错误：缺少引号的字段值
	// 匹配: "reasoning": 内容"}  或  "reasoning": 内容}  (没有引号)
	// 修复为: "reasoning": "内容"}
	// 使用简单的字符串扫描而不是正则表达式
	jsonContent = fixMissingQuotes(jsonContent)

	// 解析JSON
	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w\nJSON内容: %s", err, jsonContent)
	}

	return decisions, nil
}

// fixMissingQuotes 替换中文引号为英文引号（避免输入法自动转换）
func fixMissingQuotes(jsonStr string) string {
	jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u2018", "'")  // '
	jsonStr = strings.ReplaceAll(jsonStr, "\u2019", "'")  // '
	return jsonStr
}

// validateDecisions 验证所有决策（需要账户信息和杠杆配置）
func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, availableBalance float64, singleTradeMarginRatio float64) error {
	for i, decision := range decisions {
		if err := validateDecision(&decision, accountEquity, btcEthLeverage, altcoinLeverage, availableBalance, singleTradeMarginRatio); err != nil {
			return fmt.Errorf("决策 #%d 验证失败: %w", i+1, err)
		}
	}
	return nil
}

// findMatchingBracket 查找匹配的右括号
func findMatchingBracket(s string, start int) int {
	if start >= len(s) || s[start] != '[' {
		return -1
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// validateDecision 验证单个决策的有效性
func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, availableBalance float64, singleTradeMarginRatio float64) error {
	// 验证action
	validActions := map[string]bool{
		"open_long":   true,
		"open_short":  true,
		"close_long":  true,
		"close_short": true,
		"hold":        true,
		"wait":        true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("无效的action: %s", d.Action)
	}

	// 开仓操作必须提供完整参数
	if d.Action == "open_long" || d.Action == "open_short" {
		// 根据币种使用配置的杠杆上限
		maxLeverage := altcoinLeverage          // 山寨币使用配置的杠杆
		maxPositionValue := accountEquity * 1.5 // 山寨币最多1.5倍账户净值
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage          // BTC和ETH使用配置的杠杆
			maxPositionValue = accountEquity * 10 // BTC/ETH最多10倍账户净值
		}

		if d.Leverage <= 0 || d.Leverage > maxLeverage {
			return fmt.Errorf("杠杆必须在1-%d之间（%s，当前配置上限%d倍）: %d", maxLeverage, d.Symbol, maxLeverage, d.Leverage)
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("仓位大小必须大于0: %.2f", d.PositionSizeUSD)
		}

		// 验证持仓观察时间（开仓时必填）
		if d.ObservationTimeMin <= 0 {
			return fmt.Errorf("开仓时必须提供observation_time_min（持仓观察时间，分钟），至少30分钟")
		}
		if d.ObservationTimeMin < 30 {
			return fmt.Errorf("持仓观察时间至少30分钟，当前: %d分钟", d.ObservationTimeMin)
		}

		// 验证保证金不超过可用余额（关键约束）
		requiredMargin := d.PositionSizeUSD / float64(d.Leverage)
		// if singleTradeMarginRatio > 0 {
		// 	maxAllowedMargin := accountEquity * singleTradeMarginRatio
		// 	if requiredMargin > maxAllowedMargin {
		// 		return fmt.Errorf("单笔保证金%.2f USDT超过限制%.2f USDT（账户净值%.2f的%.0f%%）",
		// 			requiredMargin, maxAllowedMargin, accountEquity, singleTradeMarginRatio*100)
		// 	}
		// }
		if requiredMargin > availableBalance {
			return fmt.Errorf("保证金不足：所需保证金%.2f USDT（仓位%.2f / 杠杆%d）超过可用余额%.2f USDT",
				requiredMargin, d.PositionSizeUSD, d.Leverage, availableBalance)
		}
		if availableBalance <= 0 {
			return fmt.Errorf("可用余额不足：当前可用余额为%.2f USDT，无法开仓", availableBalance)
		}

		// 验证仓位价值上限（加1%容差以避免浮点数精度问题）
		tolerance := maxPositionValue * 0.01 // 1%容差
		if d.PositionSizeUSD > maxPositionValue+tolerance {
			if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
				return fmt.Errorf("BTC/ETH单币种仓位价值不能超过%.0f USDT（10倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			} else {
				return fmt.Errorf("山寨币单币种仓位价值不能超过%.0f USDT（1.5倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			}
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("止损和止盈必须大于0")
		}

		// 验证止损止盈的合理性
		if d.Action == "open_long" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("做多时止损价必须小于止盈价")
			}
		} else {
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("做空时止损价必须大于止盈价")
			}
		}

		// 验证风险回报比（必须≥1:3）
		// 计算入场价（假设当前市价）
		var entryPrice float64
		if d.Action == "open_long" {
			// 做多：入场价在止损和止盈之间
			entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2 // 假设在20%位置入场
		} else {
			// 做空：入场价在止损和止盈之间
			entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2 // 假设在20%位置入场
		}

		var riskPercent, rewardPercent float64
		var netRewardPercent, effectiveRiskPercent float64
		roundTripFeePercent := tradingFeeRate * 100 * 2 // 开仓+平仓总成本 (百分比)
		if d.Action == "open_long" {
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
		} else {
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
		}

		effectiveRiskPercent = riskPercent + roundTripFeePercent
		netRewardPercent = rewardPercent - roundTripFeePercent

		if netRewardPercent <= 0 {
			return fmt.Errorf("考虑手续费(%.4f%%)后，潜在收益%.2f%%不足以覆盖成本", roundTripFeePercent, rewardPercent)
		}

		if effectiveRiskPercent <= 0 {
			return fmt.Errorf("风险评估异常：止损距离过近或数据无效（风险%.4f%%）", effectiveRiskPercent)
		}

		// 验证风险回报比必须≥1:3（严格强制执行）
		rewardRiskRatio := netRewardPercent / effectiveRiskPercent
		if rewardRiskRatio < 3.0 {
			return fmt.Errorf("风险回报比不达标：当前%.2f:1，必须≥3:1（风险%.2f%%，收益%.2f%%）", 
				rewardRiskRatio, effectiveRiskPercent, netRewardPercent)
		}

		// 同时验证价格差异的风险回报比（基于止损止盈价格设置）
		var priceRisk, priceReward float64
		if d.Action == "open_long" {
			// 做多：风险 = 入场价 - 止损价，收益 = 止盈价 - 入场价
			priceRisk = entryPrice - d.StopLoss
			priceReward = d.TakeProfit - entryPrice
		} else {
			// 做空：风险 = 止损价 - 入场价，收益 = 入场价 - 止盈价
			priceRisk = d.StopLoss - entryPrice
			priceReward = entryPrice - d.TakeProfit
		}

		if priceRisk <= 0 || priceReward <= 0 {
			return fmt.Errorf("止损止盈价格设置异常：风险%.4f，收益%.4f", priceRisk, priceReward)
		}

		priceRewardRiskRatio := priceReward / priceRisk
		if priceRewardRiskRatio < 3.0 {
			return fmt.Errorf("止盈止损价格比不达标：当前%.2f:1（收益价格%.4f:风险价格%.4f），必须≥3:1。请调整止损价或止盈价以满足要求", 
				priceRewardRiskRatio, priceReward, priceRisk)
		}

	}

	return nil
}

// deduplicateAndResolveConflicts 去重和解决冲突：同一币种只保留第一个决策
func deduplicateAndResolveConflicts(decisions []Decision) []Decision {
	seenSymbols := make(map[string]bool)
	result := make([]Decision, 0)

	for _, decision := range decisions {
		// 对于 wait 和 hold 操作，不需要去重（可以重复）
		if decision.Action == "wait" || decision.Action == "hold" {
			result = append(result, decision)
			continue
		}

		// 对于其他操作（开仓、平仓），每个币种只保留第一个决策
		if !seenSymbols[decision.Symbol] {
			seenSymbols[decision.Symbol] = true
			result = append(result, decision)
		} else {
			log.Printf("⚠️ 检测到重复决策，已忽略：%s %s（已存在该币种的其他决策）", decision.Symbol, decision.Action)
		}
	}

	return result
}

// validateFinalDecisions 验证最终汇总的决策（检查总持仓数、总保证金等全局限制）
func validateFinalDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, availableBalance float64, maxPositionCount int, singleTradeMarginRatio float64) error {
	// 统计新开仓数量
	newOpenPositions := 0
	totalRequiredMargin := 0.0

	for _, decision := range decisions {
		if decision.Action == "open_long" || decision.Action == "open_short" {
			newOpenPositions++
			requiredMargin := decision.PositionSizeUSD / float64(decision.Leverage)
			totalRequiredMargin += requiredMargin
		}
	}

	// 检查总持仓数限制
	if newOpenPositions > maxPositionCount {
		return fmt.Errorf("新开仓数量 %d 超过限制 %d", newOpenPositions, maxPositionCount)
	}

	// 检查总保证金是否超过可用余额（允许一定的累积）
	if totalRequiredMargin > availableBalance*1.1 { // 允许10%的容差（考虑订单执行时的价格波动）
		return fmt.Errorf("总所需保证金 %.2f USDT 超过可用余额 %.2f USDT（含10%%容差）", totalRequiredMargin, availableBalance)
	}

	return nil
}

// buildSupportResistanceDigest 已移除，不再提供支撑阻力位摘要
// 现在AI需要根据K线数据自己分析支撑阻力位
