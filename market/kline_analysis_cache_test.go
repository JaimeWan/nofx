package market

import (
	"nofx/mcp"
	"os"
	"testing"
	"time"
)

// createMockMCPClient 创建模拟的MCP客户端
// 注意：由于 mcp.Client 没有接口，我们创建一个带有预设响应的真实客户端
// 在实际测试中，如果API key未设置，CallWithMessages会返回错误
// 但这个测试主要用于验证逻辑流程，实际LLM调用可以在集成测试中进行
func createMockMCPClient() *mcp.Client {
	client := mcp.New()
	// 设置一个假的API key，这样CallWithMessages不会在API key检查时失败
	// 但实际的HTTP调用会失败，所以我们主要用于测试解析逻辑
	client.SetDeepSeekAPIKey("test-api-key-for-mock")
	return client
}

// getMockAnalysisResponse 获取模拟的分析响应JSON
func getMockAnalysisResponse() string {
	return `{
		"support_levels": [
			{"price": 50000.0, "strength": 4, "timeframe": "4h", "type": "support"}
		],
		"resistance_levels": [
			{"price": 52000.0, "strength": 5, "timeframe": "1d", "type": "resistance"}
		],
		"fibonacci_15m": {
			"swing_high": 51000.0,
			"swing_low": 49500.0,
			"retracement": {"0.236": 49851.0, "0.382": 50023.0, "0.5": 50250.0, "0.618": 50477.0, "0.786": 50826.0},
			"extension": {"1.272": 51408.0, "1.618": 51927.0, "2.0": 52500.0, "2.618": 53427.0}
		},
		"fibonacci_1h": {
			"swing_high": 51000.0,
			"swing_low": 49500.0,
			"retracement": {"0.618": 50477.0},
			"extension": {"1.618": 51927.0}
		},
		"confluence_levels": [
			{"price": 50250.0, "timeframes": ["15m", "1h"], "types": ["fibonacci", "support"], "strength": 2}
		],
		"broken_resistance": [
			{"price": 50000.0, "strength": 4, "timeframe": "1h", "type": "support"}
		],
		"broken_support": [],
		"analysis_summary": "测试分析摘要：识别了关键支撑位和阻力位，斐波那契分析显示多个共振区域"
	}`
}

// createTestKlineData 创建测试用的K线数据
func createTestKlineData(symbol string, currentPrice float64) *Data {
	now := time.Now()
	var klines15m, klines1h, klines4h, klines12h, klines1d []Kline

	// 生成15分钟K线数据（最近50根）
	for i := 49; i >= 0; i-- {
		timestamp := now.Add(-time.Duration(i) * 15 * time.Minute).UnixMilli()
		klines15m = append(klines15m, Kline{
			OpenTime:  timestamp,
			Open:      currentPrice - float64(i)*10,
			High:      currentPrice - float64(i)*10 + 100,
			Low:       currentPrice - float64(i)*10 - 100,
			Close:     currentPrice - float64(i-1)*10,
			Volume:    1000.0 + float64(i)*10,
			CloseTime: timestamp + 15*60*1000 - 1,
		})
	}

	// 生成1小时K线数据（最近50根）
	for i := 49; i >= 0; i-- {
		timestamp := now.Add(-time.Duration(i) * time.Hour).UnixMilli()
		klines1h = append(klines1h, Kline{
			OpenTime:  timestamp,
			Open:      currentPrice - float64(i)*50,
			High:      currentPrice - float64(i)*50 + 200,
			Low:       currentPrice - float64(i)*50 - 200,
			Close:     currentPrice - float64(i-1)*50,
			Volume:    5000.0 + float64(i)*50,
			CloseTime: timestamp + 60*60*1000 - 1,
		})
	}

	// 生成4小时K线数据
	for i := 49; i >= 0; i-- {
		timestamp := now.Add(-time.Duration(i) * 4 * time.Hour).UnixMilli()
		klines4h = append(klines4h, Kline{
			OpenTime:  timestamp,
			Open:      currentPrice - float64(i)*200,
			High:      currentPrice - float64(i)*200 + 500,
			Low:       currentPrice - float64(i)*200 - 500,
			Close:     currentPrice - float64(i-1)*200,
			Volume:    20000.0 + float64(i)*200,
			CloseTime: timestamp + 4*60*60*1000 - 1,
		})
	}

	// 生成12小时K线数据
	for i := 49; i >= 0; i-- {
		timestamp := now.Add(-time.Duration(i) * 12 * time.Hour).UnixMilli()
		klines12h = append(klines12h, Kline{
			OpenTime:  timestamp,
			Open:      currentPrice - float64(i)*500,
			High:      currentPrice - float64(i)*500 + 1000,
			Low:       currentPrice - float64(i)*500 - 1000,
			Close:     currentPrice - float64(i-1)*500,
			Volume:    50000.0 + float64(i)*500,
			CloseTime: timestamp + 12*60*60*1000 - 1,
		})
	}

	// 生成1日K线数据
	for i := 49; i >= 0; i-- {
		timestamp := now.Add(-time.Duration(i) * 24 * time.Hour).UnixMilli()
		klines1d = append(klines1d, Kline{
			OpenTime:  timestamp,
			Open:      currentPrice - float64(i)*1000,
			High:      currentPrice - float64(i)*1000 + 2000,
			Low:       currentPrice - float64(i)*1000 - 2000,
			Close:     currentPrice - float64(i-1)*1000,
			Volume:    100000.0 + float64(i)*1000,
			CloseTime: timestamp + 24*60*60*1000 - 1,
		})
	}

	return &Data{
		Symbol:      symbol,
		CurrentPrice: currentPrice,
		Klines15m:   klines15m,
		Klines1h:    klines1h,
		Klines4h:    klines4h,
		Klines12h:   klines12h,
		Klines1d:    klines1d,
	}
}

// TestKlineAnalysisCache 测试K线分析缓存基本功能
func TestKlineAnalysisCache(t *testing.T) {
	// 1. 测试设置和获取缓存
	symbol := "BTCUSDT"
	result := &KlineAnalysisResult{
		Symbol:    symbol,
		Timestamp: time.Now(),
		// 注意：不缓存当前价格
		SupportLevels: []PriceLevel{
			{Price: 49000.0, Strength: 4, Timeframe: "4h", Type: "support"},
		},
		ResistanceLevels: []PriceLevel{
			{Price: 51000.0, Strength: 5, Timeframe: "1d", Type: "resistance"},
		},
		AnalysisSummary: "测试分析结果",
	}

	SetKlineAnalysis(symbol, result)

	// 获取缓存
	cached := GetKlineAnalysis(symbol)
	if cached == nil {
		t.Fatal("❌ 缓存设置失败，获取不到结果")
	}

	if cached.Symbol != symbol {
		t.Fatalf("❌ 缓存符号不匹配: 期望 %s, 实际 %s", symbol, cached.Symbol)
	}

	if len(cached.SupportLevels) != 1 {
		t.Fatalf("❌ 支撑位数量不匹配: 期望 1, 实际 %d", len(cached.SupportLevels))
	}

	t.Logf("✅ 缓存设置和获取测试通过")
}

// TestKlineAnalysisCacheExpiry 测试缓存过期机制
func TestKlineAnalysisCacheExpiry(t *testing.T) {
	symbol := "ETHUSDT"
	
	// 创建一个已过期的缓存（直接在缓存中设置，绕过SetKlineAnalysis的时间戳更新）
	klineAnalysisCacheMu.Lock()
	expiredResult := &KlineAnalysisResult{
		Symbol:    symbol,
		Timestamp: time.Now().Add(-20 * time.Minute), // 20分钟前（超过15分钟TTL）
		// 注意：不缓存当前价格
	}
	klineAnalysisCache[symbol] = expiredResult
	klineAnalysisCacheMu.Unlock()

	// 获取缓存（应该过期）
	cached := GetKlineAnalysis(symbol)
	if cached != nil {
		t.Fatal("❌ 缓存应该已过期，但还能获取到")
	}

	// 设置新的有效缓存
	result := &KlineAnalysisResult{
		Symbol:    symbol,
		Timestamp: time.Now(), // 使用当前时间
		// 注意：不缓存当前价格
	}
	SetKlineAnalysis(symbol, result)
	cached = GetKlineAnalysis(symbol)
	if cached == nil {
		t.Fatal("❌ 新设置的缓存应该有效，但获取不到")
	}

	// 验证缓存内容（不再验证价格，因为价格不缓存）

	t.Logf("✅ 缓存过期机制测试通过")
}

// TestParseKlineAnalysisResult 测试解析分析结果（不依赖实际的LLM调用）
func TestParseKlineAnalysisResult(t *testing.T) {
	symbol := "BTCUSDT"
	currentPrice := 50000.0
	
	// 使用模拟的响应数据
	mockResponse := getMockAnalysisResponse()
	
	// 解析结果
	result, err := parseKlineAnalysisResult(mockResponse, symbol, currentPrice)
	if err != nil {
		t.Fatalf("❌ 解析分析结果失败: %v", err)
	}

	if result == nil {
		t.Fatal("❌ 解析结果为空")
	}

	if result.Symbol != symbol {
		t.Fatalf("❌ 符号不匹配: 期望 %s, 实际 %s", symbol, result.Symbol)
	}

	// 不再验证当前价格，因为价格不缓存

	if len(result.SupportLevels) == 0 {
		t.Fatal("❌ 应该包含支撑位")
	}

	if len(result.ResistanceLevels) == 0 {
		t.Fatal("❌ 应该包含阻力位")
	}

	if result.Fibonacci15m == nil {
		t.Fatal("❌ 应该包含15分钟斐波那契分析")
	}

	t.Logf("✅ 解析分析结果测试通过")
	t.Logf("   支撑位数量: %d", len(result.SupportLevels))
	t.Logf("   阻力位数量: %d", len(result.ResistanceLevels))
	t.Logf("   15m斐波那契高点: %.2f", result.Fibonacci15m.SwingHigh)
	t.Logf("   分析摘要: %s", result.AnalysisSummary)
}

// TestRealKlineDataFormat 测试使用真实K线数据格式化输出
func TestRealKlineDataFormat(t *testing.T) {
	symbols := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	
	for _, symbol := range symbols {
		t.Logf("📊 测试 %s 的真实K线数据格式化...", symbol)
		
		// 获取真实的市场数据
		data, err := Get(symbol, "binance")
		if err != nil {
			t.Logf("⚠️ 获取 %s 市场数据失败: %v，跳过", symbol, err)
			continue
		}

		// 测试持仓币种格式（显示更多数据）
		formattedPosition := Format(data, true)
		if len(formattedPosition) == 0 {
			t.Fatalf("❌ %s 持仓格式输出为空", symbol)
		}

		// 测试候选币种格式（显示精简数据）
		formattedCandidate := Format(data, false)
		if len(formattedCandidate) == 0 {
			t.Fatalf("❌ %s 候选格式输出为空", symbol)
		}

		t.Logf("✅ %s 格式化测试通过", symbol)
		t.Logf("   当前价格: %.2f", data.CurrentPrice)
		t.Logf("   持仓格式长度: %d 字符", len(formattedPosition))
		t.Logf("   候选格式长度: %d 字符", len(formattedCandidate))
		t.Logf("   是否使用缓存: %v", GetKlineAnalysis(symbol) != nil)
	}
}

// TestParseKlineAnalysisBatchResult 测试批量解析分析结果
func TestParseKlineAnalysisBatchResult(t *testing.T) {
	symbols := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	dataMap := make(map[string]*Data)

	// 创建测试数据
	for i, symbol := range symbols {
		price := 50000.0 + float64(i)*10000.0
		dataMap[symbol] = createTestKlineData(symbol, price)
	}

	// 模拟批量返回JSON
	mockBatchResponse := `{
		"BTCUSDT": {
			"support_levels": [{"price": 49000.0, "strength": 4, "timeframe": "4h", "type": "support"}],
			"resistance_levels": [{"price": 51000.0, "strength": 5, "timeframe": "1d", "type": "resistance"}],
			"fibonacci_15m": {"swing_high": 51000.0, "swing_low": 49500.0, "retracement": {"0.618": 50477.0}, "extension": {}},
			"analysis_summary": "BTC分析"
		},
		"ETHUSDT": {
			"support_levels": [{"price": 2900.0, "strength": 3, "timeframe": "1h", "type": "support"}],
			"resistance_levels": [{"price": 3100.0, "strength": 4, "timeframe": "4h", "type": "resistance"}],
			"fibonacci_1h": {"swing_high": 3100.0, "swing_low": 2900.0, "retracement": {"0.618": 3023.6}, "extension": {}},
			"analysis_summary": "ETH分析"
		},
		"SOLUSDT": {
			"support_levels": [{"price": 140.0, "strength": 2, "timeframe": "15m", "type": "support"}],
			"resistance_levels": [{"price": 160.0, "strength": 3, "timeframe": "1h", "type": "resistance"}],
			"analysis_summary": "SOL分析"
		}
	}`

	// 解析批量结果
	results, err := parseKlineAnalysisBatchResult(mockBatchResponse, symbols, dataMap)
	if err != nil {
		t.Fatalf("❌ 批量解析失败: %v", err)
	}

	// 验证所有币种的结果
	for _, symbol := range symbols {
		result, ok := results[symbol]
		if !ok {
			t.Fatalf("❌ %s 的解析结果缺失", symbol)
		}

		if result.Symbol != symbol {
			t.Fatalf("❌ %s 的符号不匹配", symbol)
		}

		if len(result.SupportLevels) == 0 {
			t.Fatalf("❌ %s 应该包含支撑位", symbol)
		}

		t.Logf("✅ %s 解析成功: 支撑位 %d 个, 阻力位 %d 个", 
			symbol, len(result.SupportLevels), len(result.ResistanceLevels))
	}

	t.Logf("✅ 批量解析分析结果测试通过")
}

// TestFormatKlineAnalysis 测试格式化输出
func TestFormatKlineAnalysis(t *testing.T) {
	symbol := "BTCUSDT"
	data := createTestKlineData(symbol, 50000.0)

	// 设置测试缓存
	result := &KlineAnalysisResult{
		Symbol:    symbol,
		Timestamp: time.Now(),
		// 注意：不缓存当前价格
		SupportLevels: []PriceLevel{
			{Price: 49000.0, Strength: 4, Timeframe: "4h", Type: "support"},
			{Price: 48000.0, Strength: 3, Timeframe: "1h", Type: "support"},
		},
		ResistanceLevels: []PriceLevel{
			{Price: 51000.0, Strength: 5, Timeframe: "1d", Type: "resistance"},
			{Price: 52000.0, Strength: 4, Timeframe: "4h", Type: "resistance"},
		},
		Fibonacci15m: &FibonacciAnalysis{
			SwingHigh:   51000.0,
			SwingLow:    49500.0,
			Retracement: map[string]float64{"0.618": 50477.0},
			Extension:   map[string]float64{"1.618": 51927.0},
		},
		ConfluenceLevels: []ConfluenceLevel{
			{Price: 50250.0, Timeframes: []string{"15m", "1h"}, Types: []string{"fibonacci"}, Strength: 2},
		},
		AnalysisSummary: "测试分析摘要",
	}

	SetKlineAnalysis(symbol, result)

	// 格式化输出
	formatted := FormatKlineAnalysis(symbol, data, true)
	if formatted == "" {
		t.Fatal("❌ 格式化输出为空")
	}

	// 检查是否包含关键信息
	if !contains(formatted, "支撑位") {
		t.Fatal("❌ 格式化输出应该包含'支撑位'")
	}
	if !contains(formatted, "阻力位") {
		t.Fatal("❌ 格式化输出应该包含'阻力位'")
	}
	if !contains(formatted, "斐波那契") {
		t.Fatal("❌ 格式化输出应该包含'斐波那契'")
	}

	t.Logf("✅ 格式化输出测试通过")
	t.Logf("\n格式化输出示例:\n%s", formatted[:200]+"...")
}

// TestKlineAnalysisInProgress 测试分析状态管理
func TestKlineAnalysisInProgress(t *testing.T) {
	symbol := "TESTUSDT"

	// 设置正在分析状态
	SetAnalysisInProgress(symbol, true)
	if !IsAnalysisInProgress(symbol) {
		t.Fatal("❌ 应该标记为正在分析")
	}

	// 取消分析状态
	SetAnalysisInProgress(symbol, false)
	if IsAnalysisInProgress(symbol) {
		t.Fatal("❌ 应该已取消分析状态")
	}

	t.Logf("✅ 分析状态管理测试通过")
}

// TestCacheSkipsAnalysis 测试缓存跳过重复分析
func TestCacheSkipsAnalysis(t *testing.T) {
	symbol := "CACHEDUSDT"

	// 设置有效缓存
	result := &KlineAnalysisResult{
		Symbol:    symbol,
		Timestamp: time.Now(),
		// 注意：不缓存当前价格
		AnalysisSummary: "已缓存的分析",
	}
	SetKlineAnalysis(symbol, result)

	// 验证缓存有效时，GetKlineAnalysis 应该返回结果
	cached := GetKlineAnalysis(symbol)
	if cached == nil {
		t.Fatal("❌ 缓存应该有效")
	}

	if cached.AnalysisSummary != "已缓存的分析" {
		t.Fatalf("❌ 缓存内容不匹配")
	}

	t.Logf("✅ 缓存跳过重复分析测试通过")
}

// contains 辅助函数：检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		 contains(s[1:], substr)))
}

// BenchmarkKlineAnalysisCache 性能测试
func BenchmarkKlineAnalysisCache(b *testing.B) {
	symbol := "BTCUSDT"
	result := &KlineAnalysisResult{
		Symbol:    symbol,
		Timestamp: time.Now(),
		// 注意：不缓存当前价格
		AnalysisSummary: "基准测试",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SetKlineAnalysis(symbol, result)
		GetKlineAnalysis(symbol)
	}
}

// TestKlineAnalysisWithRealData 使用真实K线数据测试（不调用LLM）
func TestKlineAnalysisWithRealData(t *testing.T) {
	symbol := "BTCUSDT"
	
	t.Logf("📊 获取 %s 的真实K线数据...", symbol)
	
	// 获取真实的市场数据
	data, err := Get(symbol, "binance")
	if err != nil {
		t.Fatalf("❌ 获取市场数据失败: %v", err)
	}

	// 验证K线数据
	if len(data.Klines15m) == 0 {
		t.Fatal("❌ 15分钟K线数据为空")
	}
	if len(data.Klines1h) == 0 {
		t.Fatal("❌ 1小时K线数据为空")
	}
	if len(data.Klines4h) == 0 {
		t.Fatal("❌ 4小时K线数据为空")
	}
	if len(data.Klines1d) == 0 {
		t.Fatal("❌ 1日K线数据为空")
	}

	t.Logf("✅ 获取到真实K线数据:")
	t.Logf("   当前价格: %.2f", data.CurrentPrice)
	t.Logf("   15分钟K线: %d 根", len(data.Klines15m))
	t.Logf("   1小时K线: %d 根", len(data.Klines1h))
	t.Logf("   4小时K线: %d 根", len(data.Klines4h))
	t.Logf("   12小时K线: %d 根", len(data.Klines12h))
	t.Logf("   1日K线: %d 根", len(data.Klines1d))

	// 测试格式化输出（使用真实数据）
	formatted := Format(data, true)
	if len(formatted) == 0 {
		t.Fatal("❌ 格式化输出为空")
	}

	t.Logf("✅ 真实K线数据测试通过")
	t.Logf("   格式化输出长度: %d 字符", len(formatted))
}

// TestKlineAnalysisIntegration 集成测试（需要真实的API key）
// 使用方法：设置环境变量 DEEPSEEK_API_KEY
// 注意：此测试会调用真实的DeepSeek API，会产生费用
func TestKlineAnalysisIntegration(t *testing.T) {
	// 检查是否提供了API key
	apiKey := getEnvOrSkip(t, "DEEPSEEK_API_KEY")

	symbol := "BTCUSDT"
	
	// 获取真实的市场数据
	t.Logf("📊 获取 %s 的真实K线数据...", symbol)
	data, err := Get(symbol, "binance")
	if err != nil {
		t.Fatalf("❌ 获取市场数据失败: %v", err)
	}

	t.Logf("✅ 获取到真实K线数据:")
	t.Logf("   当前价格: %.2f", data.CurrentPrice)
	t.Logf("   15分钟K线: %d 根", len(data.Klines15m))
	t.Logf("   1小时K线: %d 根", len(data.Klines1h))
	t.Logf("   4小时K线: %d 根", len(data.Klines4h))

	// 创建MCP客户端
	client := mcp.New()
	client.SetDeepSeekAPIKey(apiKey)

	// 清除可能的缓存
	klineAnalysisCacheMu.Lock()
	delete(klineAnalysisCache, symbol)
	klineAnalysisCacheMu.Unlock()

	t.Logf("🔍 开始分析 %s 的K线数据（真实API调用）...", symbol)
	
	// 执行分析
	err = AnalyzeKlinesWithLLM(symbol, data, client)
	if err != nil {
		t.Fatalf("❌ K线分析失败: %v", err)
	}

	// 验证结果
	cached := GetKlineAnalysis(symbol)
	if cached == nil {
		t.Fatal("❌ 分析后应该生成缓存，但获取不到")
	}

	t.Logf("✅ 集成测试通过")
	t.Logf("   支撑位数量: %d", len(cached.SupportLevels))
	t.Logf("   阻力位数量: %d", len(cached.ResistanceLevels))
	t.Logf("   分析摘要: %s", cached.AnalysisSummary)
	
	// 显示部分分析结果
	if len(cached.SupportLevels) > 0 {
		t.Logf("   支撑位示例: %.2f (强度%d, %s)", 
			cached.SupportLevels[0].Price, 
			cached.SupportLevels[0].Strength, 
			cached.SupportLevels[0].Timeframe)
	}
	if len(cached.ResistanceLevels) > 0 {
		t.Logf("   阻力位示例: %.2f (强度%d, %s)", 
			cached.ResistanceLevels[0].Price, 
			cached.ResistanceLevels[0].Strength, 
			cached.ResistanceLevels[0].Timeframe)
	}
}

// TestBatchAnalysisWithRealData 使用真实K线数据进行批量分析测试
func TestBatchAnalysisWithRealData(t *testing.T) {
	apiKey := getEnvOrSkip(t, "DEEPSEEK_API_KEY")
	
	symbols := []string{"BTCUSDT", "ETHUSDT"}
	dataMap := make(map[string]*Data)

	t.Logf("📊 获取 %d 个币种的真实K线数据...", len(symbols))
	
	// 获取真实的市场数据
	for _, symbol := range symbols {
		data, err := Get(symbol, "binance")
		if err != nil {
			t.Fatalf("❌ 获取 %s 市场数据失败: %v", symbol, err)
		}
		dataMap[symbol] = data
		t.Logf("   ✅ %s: 价格 %.2f, 15mK线 %d根, 1hK线 %d根", 
			symbol, data.CurrentPrice, len(data.Klines15m), len(data.Klines1h))
	}

	// 清除缓存
	klineAnalysisCacheMu.Lock()
	for _, symbol := range symbols {
		delete(klineAnalysisCache, symbol)
	}
	klineAnalysisCacheMu.Unlock()

	// 创建MCP客户端
	client := mcp.New()
	client.SetDeepSeekAPIKey(apiKey)

	// 执行批量分析（每批2个）
	t.Logf("🔍 开始批量分析（真实API调用）...")
	batchSize := 2
	err := AnalyzeKlinesBatchWithLLM(symbols, dataMap, client, batchSize)
	if err != nil {
		t.Fatalf("❌ 批量分析失败: %v", err)
	}

	// 验证所有币种的缓存
	for _, symbol := range symbols {
		cached := GetKlineAnalysis(symbol)
		if cached == nil {
			t.Fatalf("❌ %s 的分析结果应该被缓存，但获取不到", symbol)
		}
		t.Logf("✅ %s 分析完成: 支撑位 %d 个, 阻力位 %d 个", 
			symbol, len(cached.SupportLevels), len(cached.ResistanceLevels))
	}

	t.Logf("✅ 批量分析测试通过")
}

// getEnvOrSkip 获取环境变量，如果不存在则跳过测试
func getEnvOrSkip(t *testing.T, key string) string {
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("跳过测试：需要设置环境变量 %s", key)
	}
	return value
}

