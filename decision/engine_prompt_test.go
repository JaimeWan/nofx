package decision

import (
	"testing"
	"time"

	"nofx/market"
)

func TestBuildUserPromptSample(t *testing.T) {
	symbols := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	marketData := make(map[string]*market.Data)

	for _, symbol := range symbols {
		data, err := market.Get(symbol, "binance")
		if err != nil {
			t.Fatalf("获取%s市场数据失败: %v", symbol, err)
		}
		marketData[symbol] = data
	}

	btcData := marketData["BTCUSDT"]
	entryPrice := btcData.CurrentPrice * 0.97
	markPrice := btcData.CurrentPrice

	ctx := &Context{
		CurrentTime:    time.Now().Format("2006-01-02 15:04"),
		RuntimeMinutes: 180,
		CallCount:      42,
		Account: AccountInfo{
			TotalEquity:      20000,
			AvailableBalance: 12000,
			TotalPnL:         350,
			TotalPnLPct:      1.75,
			MarginUsed:       8000,
			MarginUsedPct:    40,
			PositionCount:    1,
		},
		Positions: []PositionInfo{
			{
				Symbol:           "BTCUSDT",
				Side:             "long",
				EntryPrice:       entryPrice,
				MarkPrice:        markPrice,
				Quantity:         0.1,
				Leverage:         5,
				UnrealizedPnL:    (markPrice - entryPrice) * 0.1,
				UnrealizedPnLPct: (markPrice/entryPrice - 1) * 100,
				LiquidationPrice: entryPrice * 0.9,
				MarginUsed:       2000,
				UpdateTime:       time.Now().Add(-45 * time.Minute).UnixMilli(),
			},
		},
		CandidateCoins: []CandidateCoin{
			{Symbol: "ETHUSDT", Sources: []string{"ai500"}},
			{Symbol: "SOLUSDT", Sources: []string{"oi_top"}},
		},
		MarketDataMap:          marketData,
		Performance:            map[string]interface{}{"sharpe_ratio": 0.92},
		BTCETHLeverage:         5,
		AltcoinLeverage:        3,
		CoinWhitelistEnabled:   false,
		CoinWhitelist:          nil,
		Exchange:               "binance",
		MaxPositionCount:       3,
		SingleTradeMarginRatio: 0.05,
	}

	prompt := buildUserPrompt(ctx)
	t.Logf("\n===== Generated User Prompt =====\n%s\n", prompt)
}
