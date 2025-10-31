package market

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// defaultCoins 默认币种列表（从配置读取）
var defaultCoins = make(map[string]bool)

// hyperliquidOICache Hyperliquid OI数据缓存
var (
	hyperliquidOICache     = make(map[string]float64) // symbol -> OI
	hyperliquidOICacheTime time.Time
	hyperliquidOICacheMu   sync.RWMutex
	hyperliquidOICacheTTL  = 30 * time.Second // 缓存30秒
)

// SetDefaultCoins 设置默认币种列表（从配置读取）
func SetDefaultCoins(coins []string) {
	defaultCoins = make(map[string]bool)
	for _, coin := range coins {
		normalizedCoin := Normalize(coin)
		defaultCoins[normalizedCoin] = true
	}
}

// isInDefaultCoins 检查币种是否在default_coins中
func isInDefaultCoins(symbol string) bool {
	if len(defaultCoins) == 0 {
		// 如果没有配置default_coins，返回true（向后兼容）
		return true
	}
	normalizedSymbol := Normalize(symbol)
	return defaultCoins[normalizedSymbol]
}

// Data 市场数据结构
type Data struct {
	Symbol            string
	CurrentPrice      float64
	PriceChange1h     float64 // 1小时价格变化百分比
	PriceChange4h     float64 // 4小时价格变化百分比
	CurrentEMA20      float64
	CurrentMACD       float64
	CurrentRSI7       float64
	OpenInterest      *OIData
	FundingRate       float64
	IntradaySeries    *IntradayData
	LongerTermContext *LongerTermData
	OITopData         *OITopData // OI Top数据
	SupportResistance *SupportResistanceSummary
}

// OITopData OI Top数据结构
type OITopData struct {
	Rank              int     `json:"rank"`
	OIDeltaPercent    float64 `json:"oi_delta_percent"`
	OIDeltaValue      float64 `json:"oi_delta_value"`
	PriceDeltaPercent float64 `json:"price_delta_percent"`
	NetLong           float64 `json:"net_long"`
	NetShort          float64 `json:"net_short"`
}

// OIData Open Interest数据
type OIData struct {
	Latest  float64
	Average float64
}

// IntradayData 日内数据(3分钟间隔)
type IntradayData struct {
	MidPrices   []float64
	EMA20Values []float64
	MACDValues  []float64
	RSI7Values  []float64
	RSI14Values []float64
}

// LongerTermData 长期数据(4小时时间框架)
type LongerTermData struct {
	EMA20         float64
	EMA50         float64
	ATR3          float64
	ATR14         float64
	CurrentVolume float64
	AverageVolume float64
	MACDValues    []float64
	RSI14Values   []float64
}

// Kline K线数据
type Kline struct {
	OpenTime  int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime int64
}

// Get 获取指定代币的市场数据
func Get(symbol string, exchange string) (*Data, error) {
	// 标准化symbol
	symbol = Normalize(symbol)

	// 获取3分钟K线数据
	klines3m, err := getKlines(symbol, "3m", 150)
	if err != nil {
		return nil, fmt.Errorf("获取3分钟K线失败: %v", err)
	}

	// 获取15分钟K线数据
	klines15m, err := getKlines(symbol, "15m", 150)
	if err != nil {
		return nil, fmt.Errorf("获取15分钟K线失败: %v", err)
	}

	// 获取1小时K线数据
	klines1h, err := getKlines(symbol, "1h", 150)
	if err != nil {
		return nil, fmt.Errorf("获取1小时K线失败: %v", err)
	}

	// 获取4小时K线数据
	klines4h, err := getKlines(symbol, "4h", 150)
	if err != nil {
		return nil, fmt.Errorf("获取4小时K线失败: %v", err)
	}

	// 计算当前指标 (基于3分钟最新数据)
	currentPrice := klines3m[len(klines3m)-1].Close
	currentEMA20 := calculateEMA(klines3m, 20)
	currentMACD := calculateMACD(klines3m)
	currentRSI7 := calculateRSI(klines3m, 7)

	// 计算价格变化百分比
	// 1小时价格变化 = 20个3分钟K线前的价格
	priceChange1h := 0.0
	if len(klines3m) >= 21 { // 至少需要21根K线 (当前 + 20根前)
		price1hAgo := klines3m[len(klines3m)-21].Close
		if price1hAgo > 0 {
			priceChange1h = ((currentPrice - price1hAgo) / price1hAgo) * 100
		}
	}

	// 4小时价格变化 = 1个4小时K线前的价格
	priceChange4h := 0.0
	if len(klines4h) >= 2 {
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			priceChange4h = ((currentPrice - price4hAgo) / price4hAgo) * 100
		}
	}

	// 获取OI数据（只对default_coins中的币种请求）
	var oiData *OIData
	if isInDefaultCoins(symbol) {
		var err error
		if exchange == "hyperliquid" {
			oiData, err = getHyperliquidOpenInterestData(symbol)
		} else {
			// 默认使用Binance API
			oiData, err = getOpenInterestData(symbol)
		}
		if err != nil {
			// OI失败不影响整体,使用默认值
			oiData = &OIData{Latest: 0, Average: 0}
		}
	} else {
		// 不在default_coins中，不请求OI数据
		oiData = &OIData{Latest: 0, Average: 0}
	}

	// 获取Funding Rate
	fundingRate, _ := getFundingRate(symbol)

	// 计算日内系列数据
	intradayData := calculateIntradaySeries(klines3m)

	// 计算长期数据
	longerTermData := calculateLongerTermData(klines4h)

	// 计算支撑阻力摘要
	supportResistance := calculateSupportResistanceSummary(klines3m, klines15m, klines1h, klines4h, currentPrice)

	return &Data{
		Symbol:            symbol,
		CurrentPrice:      currentPrice,
		PriceChange1h:     priceChange1h,
		PriceChange4h:     priceChange4h,
		CurrentEMA20:      currentEMA20,
		CurrentMACD:       currentMACD,
		CurrentRSI7:       currentRSI7,
		OpenInterest:      oiData,
		FundingRate:       fundingRate,
		IntradaySeries:    intradayData,
		LongerTermContext: longerTermData,
		SupportResistance: supportResistance,
	}, nil
}

// getKlines 从Binance获取K线数据
func getKlines(symbol, interval string, limit int) ([]Kline, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/klines?symbol=%s&interval=%s&limit=%d",
		symbol, interval, limit)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rawData [][]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, err
	}

	klines := make([]Kline, len(rawData))
	for i, item := range rawData {
		openTime := int64(item[0].(float64))
		open, _ := parseFloat(item[1])
		high, _ := parseFloat(item[2])
		low, _ := parseFloat(item[3])
		close, _ := parseFloat(item[4])
		volume, _ := parseFloat(item[5])
		closeTime := int64(item[6].(float64))

		klines[i] = Kline{
			OpenTime:  openTime,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
			CloseTime: closeTime,
		}
	}

	return klines, nil
}

// calculateEMA 计算EMA
func calculateEMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}

	// 计算SMA作为初始EMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)

	// 计算EMA
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}

	return ema
}

// calculateMACD 计算MACD
func calculateMACD(klines []Kline) float64 {
	if len(klines) < 26 {
		return 0
	}

	// 计算12期和26期EMA
	ema12 := calculateEMA(klines, 12)
	ema26 := calculateEMA(klines, 26)

	// MACD = EMA12 - EMA26
	return ema12 - ema26
}

// calculateRSI 计算RSI
func calculateRSI(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	gains := 0.0
	losses := 0.0

	// 计算初始平均涨跌幅
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// 使用Wilder平滑方法计算后续RSI
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + (-change)) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateATR 计算ATR
func calculateATR(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	trs := make([]float64, len(klines))
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// 计算初始ATR
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += trs[i]
	}
	atr := sum / float64(period)

	// Wilder平滑
	for i := period + 1; i < len(klines); i++ {
		atr = (atr*float64(period-1) + trs[i]) / float64(period)
	}

	return atr
}

// calculateIntradaySeries 计算日内系列数据
func calculateIntradaySeries(klines []Kline) *IntradayData {
	data := &IntradayData{
		MidPrices:   make([]float64, 0, 10),
		EMA20Values: make([]float64, 0, 10),
		MACDValues:  make([]float64, 0, 10),
		RSI7Values:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 获取最近10个数据点
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		data.MidPrices = append(data.MidPrices, klines[i].Close)

		// 计算每个点的EMA20
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// 计算每个点的MACD
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// 计算每个点的RSI
		if i >= 7 {
			rsi7 := calculateRSI(klines[:i+1], 7)
			data.RSI7Values = append(data.RSI7Values, rsi7)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// calculateLongerTermData 计算长期数据
func calculateLongerTermData(klines []Kline) *LongerTermData {
	data := &LongerTermData{
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 计算EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)

	// 计算ATR
	data.ATR3 = calculateATR(klines, 3)
	data.ATR14 = calculateATR(klines, 14)

	// 计算成交量
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume
		// 计算平均成交量
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// 计算MACD和RSI序列
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// getOpenInterestData 获取OI数据
func getOpenInterestData(symbol string) (*OIData, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/openInterest?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		OpenInterest string `json:"openInterest"`
		Symbol       string `json:"symbol"`
		Time         int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	oi, _ := strconv.ParseFloat(result.OpenInterest, 64)

	return &OIData{
		Latest:  oi,
		Average: oi * 0.999, // 近似平均值
	}, nil
}

// getHyperliquidOpenInterestData 从Hyperliquid获取OI数据
func getHyperliquidOpenInterestData(symbol string) (*OIData, error) {
	// 检查缓存是否有效
	hyperliquidOICacheMu.RLock()
	cacheValid := time.Since(hyperliquidOICacheTime) < hyperliquidOICacheTTL
	if cacheValid {
		if oi, exists := hyperliquidOICache[symbol]; exists {
			hyperliquidOICacheMu.RUnlock()
			return &OIData{
				Latest:  oi,
				Average: oi * 0.999, // 近似平均值
			}, nil
		}
	}
	hyperliquidOICacheMu.RUnlock()

	// 缓存失效或不存在，重新获取所有币种的OI数据
	hyperliquidOICacheMu.Lock()
	defer hyperliquidOICacheMu.Unlock()

	// 双重检查，可能其他goroutine已经更新了缓存
	if time.Since(hyperliquidOICacheTime) < hyperliquidOICacheTTL {
		if oi, exists := hyperliquidOICache[symbol]; exists {
			return &OIData{
				Latest:  oi,
				Average: oi * 0.999,
			}, nil
		}
	}

	// 从Hyperliquid API获取所有币种的OI数据
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	requestBody := map[string]string{
		"type": "metaAndAssetCtxs",
	}
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("构建请求体失败: %w", err)
	}

	resp, err := client.Post("https://api.hyperliquid.xyz/info", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("请求hyperliquid OI API失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取hyperliquid oi响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hyperliquid oi api返回错误 (status %d): %s", resp.StatusCode, string(body))
	}

	// 解析API响应
	var response []interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("hyperliquid oi json解析失败: %w", err)
	}

	if len(response) < 2 {
		return nil, fmt.Errorf("hyperliquid oi响应格式错误")
	}

	// 解析universe数据
	universeData, ok := response[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("hyperliquid oi universe数据格式错误")
	}

	universe, ok := universeData["universe"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("hyperliquid oi universe数组格式错误")
	}

	// 解析assetCtxs数据
	assetCtxs, ok := response[1].([]interface{})
	if !ok {
		return nil, fmt.Errorf("hyperliquid oi assetCtxs数据格式错误")
	}

	if len(universe) != len(assetCtxs) {
		return nil, fmt.Errorf("hyperliquid oi数据长度不匹配")
	}

	// 更新缓存
	newCache := make(map[string]float64)
	for i, universeItem := range universe {
		universeMap, ok := universeItem.(map[string]interface{})
		if !ok {
			continue
		}

		assetCtxMap, ok := assetCtxs[i].(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := universeMap["name"].(string)
		if !ok {
			continue
		}

		openInterest, ok := assetCtxMap["openInterest"].(string)
		if !ok {
			continue
		}

		oi, err := strconv.ParseFloat(openInterest, 64)
		if err != nil {
			continue
		}

		// 转换为USDT交易对格式
		symbolHL := name + "USDT"
		symbolHL = Normalize(symbolHL)
		newCache[symbolHL] = oi
	}

	hyperliquidOICache = newCache
	hyperliquidOICacheTime = time.Now()

	// 查找目标币种的OI
	if oi, exists := hyperliquidOICache[symbol]; exists {
		return &OIData{
			Latest:  oi,
			Average: oi * 0.999,
		}, nil
	}

	// 未找到该币种，返回0
	return &OIData{
		Latest:  0,
		Average: 0,
	}, nil
}

// getFundingRate 获取资金费率
func getFundingRate(symbol string) (float64, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/premiumIndex?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Symbol          string `json:"symbol"`
		MarkPrice       string `json:"markPrice"`
		IndexPrice      string `json:"indexPrice"`
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
		InterestRate    string `json:"interestRate"`
		Time            int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	rate, _ := strconv.ParseFloat(result.LastFundingRate, 64)
	return rate, nil
}

// Format 格式化输出市场数据
func Format(data *Data) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("当前价格 = %.2f, EMA20 = %.3f, MACD = %.3f, RSI(7) = %.3f\n\n",
		data.CurrentPrice, data.CurrentEMA20, data.CurrentMACD, data.CurrentRSI7))

	sb.WriteString(fmt.Sprintf("以下为 %s 的持仓量与资金费率信息:\n\n",
		data.Symbol))

	if data.OpenInterest != nil {
		// 显示原始OI数量和USD价值
		oiValueUSD := data.OpenInterest.Latest * data.CurrentPrice
		sb.WriteString(fmt.Sprintf("合约持仓量: 最新 %.2f (张) | 价值 %.2f USD | 平均 %.2f\n\n",
			data.OpenInterest.Latest, oiValueUSD, data.OpenInterest.Average))
	}

	sb.WriteString(fmt.Sprintf("资金费率: %.2e\n\n", data.FundingRate))

	// 添加OI Top数据
	if data.OITopData != nil {
		sb.WriteString("持仓量排名数据:\n\n")

		if data.OITopData.Rank > 0 {
			sb.WriteString(fmt.Sprintf("排名: #%d\n", data.OITopData.Rank))
		}

		if data.OITopData.OIDeltaValue > 0 {
			sb.WriteString(fmt.Sprintf("持仓量变化值: %.2f\n", data.OITopData.OIDeltaValue))
		}

		if data.OITopData.OIDeltaPercent != 0 {
			sb.WriteString(fmt.Sprintf("持仓量变化幅度: %.2f%%\n", data.OITopData.OIDeltaPercent))
		}

		if data.OITopData.PriceDeltaPercent != 0 {
			sb.WriteString(fmt.Sprintf("价格变化幅度: %.2f%%\n", data.OITopData.PriceDeltaPercent))
		}

		if data.OITopData.NetLong > 0 || data.OITopData.NetShort > 0 {
			sb.WriteString(fmt.Sprintf("净多头: %.2f | 净空头: %.2f\n", data.OITopData.NetLong, data.OITopData.NetShort))
		}

		sb.WriteString("\n")
	}

	if data.IntradaySeries != nil {
		sb.WriteString("3分钟级别序列（从旧到新）:\n\n")

		if len(data.IntradaySeries.MidPrices) > 0 {
			sb.WriteString(fmt.Sprintf("收盘价序列: %s\n\n", formatFloatSlice(data.IntradaySeries.MidPrices)))
		}

		if len(data.IntradaySeries.EMA20Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA(20周期): %s\n\n", formatFloatSlice(data.IntradaySeries.EMA20Values)))
		}

		if len(data.IntradaySeries.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD序列: %s\n\n", formatFloatSlice(data.IntradaySeries.MACDValues)))
		}

		if len(data.IntradaySeries.RSI7Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI(7周期): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI7Values)))
		}

		if len(data.IntradaySeries.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI(14周期): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI14Values)))
		}
	}

	if data.LongerTermContext != nil {
		sb.WriteString("4小时级别背景数据:\n\n")

		sb.WriteString(fmt.Sprintf("EMA: 20周期 %.3f vs. 50周期 %.3f\n\n",
			data.LongerTermContext.EMA20, data.LongerTermContext.EMA50))

		sb.WriteString(fmt.Sprintf("ATR: 3周期 %.3f vs. 14周期 %.3f\n\n",
			data.LongerTermContext.ATR3, data.LongerTermContext.ATR14))

		sb.WriteString(fmt.Sprintf("成交量: 当前 %.3f vs. 均值 %.3f\n\n",
			data.LongerTermContext.CurrentVolume, data.LongerTermContext.AverageVolume))

		if len(data.LongerTermContext.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD序列: %s\n\n", formatFloatSlice(data.LongerTermContext.MACDValues)))
		}

		if len(data.LongerTermContext.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI(14周期): %s\n\n", formatFloatSlice(data.LongerTermContext.RSI14Values)))
		}
	}

	if data.SupportResistance != nil && len(data.SupportResistance.Timeframes) > 0 {
		sb.WriteString("支撑/阻力结构:\n\n")

		ordered := []string{"3m", "15m", "1h", "4h"}
		for _, tf := range ordered {
			if tfData, ok := data.SupportResistance.Timeframes[tf]; ok {
				sb.WriteString(fmt.Sprintf("[%s] 支撑: %s\n", tf, formatSupportResistanceSlice(tfData.Supports, 3)))
				sb.WriteString(fmt.Sprintf("[%s] 阻力: %s\n\n", tf, formatSupportResistanceSlice(tfData.Resistances, 3)))
			}
		}

		if data.SupportResistance.Confluence != nil {
			sb.WriteString("多周期共振 (3m 与长周期重合):\n")
			sb.WriteString(fmt.Sprintf("共振支撑: %s\n", formatSupportResistanceSlice(data.SupportResistance.Confluence.Supports, 3)))
			sb.WriteString(fmt.Sprintf("共振阻力: %s\n\n", formatSupportResistanceSlice(data.SupportResistance.Confluence.Resistances, 3)))
		}

		sb.WriteString("⚠️ 交易原则: 禁止在阻力位追多，也禁止在支撑位追空。\n\n")
	}

	return sb.String()
}

// formatFloatSlice 格式化float64切片为字符串
func formatFloatSlice(values []float64) string {
	strValues := make([]string, len(values))
	for i, v := range values {
		strValues[i] = fmt.Sprintf("%.3f", v)
	}
	return "[" + strings.Join(strValues, ", ") + "]"
}

func formatSupportResistanceSlice(levels []SupportResistanceLevel, limit int) string {
	if len(levels) == 0 {
		return "-"
	}

	if len(levels) > limit {
		levels = levels[:limit]
	}

	parts := make([]string, len(levels))
	for i, level := range levels {
		parts[i] = fmt.Sprintf("%.2f (强度%d, 得分%.2f, 距离%.2f%%)", level.Price, level.Strength, level.Score, level.Distance)
	}

	return strings.Join(parts, " | ")
}

// Normalize 标准化symbol,确保是USDT交易对
func Normalize(symbol string) string {
	symbol = strings.ToUpper(symbol)
	if strings.HasSuffix(symbol, "USDT") {
		return symbol
	}
	return symbol + "USDT"
}

// parseFloat 解析float值
func parseFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case string:
		return strconv.ParseFloat(val, 64)
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}
