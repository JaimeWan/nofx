package market

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"nofx/mcp"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// KlineAnalysisResult K线分析结果（由LLM分析后缓存）
type KlineAnalysisResult struct {
	Symbol       string    `json:"symbol"`
	Timestamp    time.Time `json:"timestamp"` // 分析时间
	CurrentPrice float64   `json:"current_price"`

	// 支撑位/阻力位分析
	SupportLevels    []PriceLevel `json:"support_levels"`    // 支撑位列表
	ResistanceLevels []PriceLevel `json:"resistance_levels"` // 阻力位列表

	// 斐波那契分析（各周期）
	Fibonacci15m *FibonacciAnalysis `json:"fibonacci_15m"` // 15分钟周期斐波那契
	Fibonacci1h  *FibonacciAnalysis `json:"fibonacci_1h"`  // 1小时周期斐波那契
	Fibonacci4h  *FibonacciAnalysis `json:"fibonacci_4h"`  // 4小时周期斐波那契
	Fibonacci12h *FibonacciAnalysis `json:"fibonacci_12h"` // 12小时周期斐波那契
	Fibonacci1d  *FibonacciAnalysis `json:"fibonacci_1d"`  // 1日周期斐波那契

	// 多周期共振分析
	ConfluenceLevels []ConfluenceLevel `json:"confluence_levels"` // 多周期共振位

	// 支撑阻力转换分析
	BrokenResistance []PriceLevel `json:"broken_resistance"` // 已突破转为支撑的阻力位
	BrokenSupport    []PriceLevel `json:"broken_support"`    // 已跌破转为阻力的支撑位

	// 分析摘要（LLM生成的文本描述）
	AnalysisSummary string `json:"analysis_summary"`
}

// PriceLevel 价格位
type PriceLevel struct {
	Price     float64 `json:"price"`     // 价格
	Strength  int     `json:"strength"`  // 强度（1-5）
	Timeframe string  `json:"timeframe"` // 来源周期：15m, 1h, 4h, 12h, 1d
	Type      string  `json:"type"`      // 类型：support, resistance, fibonacci
}

// FibonacciAnalysis 斐波那契分析结果
type FibonacciAnalysis struct {
	SwingHigh   float64            `json:"swing_high"`  // 波段高点
	SwingLow    float64            `json:"swing_low"`   // 波段低点
	Retracement map[string]float64 `json:"retracement"` // 回撤位：0.382, 0.618, 0.786（只计算这3个）
	Extension   map[string]float64 `json:"extension"`   // 扩展位：1.272, 1.618, 2.0, 2.618
}

// ConfluenceLevel 多周期共振位
type ConfluenceLevel struct {
	Price      float64  `json:"price"`      // 价格
	Timeframes []string `json:"timeframes"` // 共振的周期列表
	Types      []string `json:"types"`      // 类型列表：support, resistance, fibonacci
	Strength   int      `json:"strength"`   // 强度（共振周期数）
}

// klineAnalysisCache K线分析缓存
var (
	klineAnalysisCache    = make(map[string]*KlineAnalysisResult) // symbol -> analysis result
	klineAnalysisCacheMu  sync.RWMutex
	klineAnalysisCacheTTL = 15 * time.Minute      // 缓存15分钟
	analysisInProgress    = make(map[string]bool) // 正在分析的币种（避免重复分析）
	analysisInProgressMu  sync.Mutex
	klineAnalysisCacheDir = "kline_analysis_cache" // 缓存文件目录
	cacheDirInitialized   = false                  // 目录是否已初始化
)

// GetKlineAnalysis 获取K线分析结果（从缓存读取）
func GetKlineAnalysis(symbol string) *KlineAnalysisResult {
	symbol = Normalize(symbol)

	// 先尝试从内存缓存读取
	klineAnalysisCacheMu.RLock()
	result, exists := klineAnalysisCache[symbol]
	if exists {
		// 检查缓存是否过期
		if time.Since(result.Timestamp) <= klineAnalysisCacheTTL {
			klineAnalysisCacheMu.RUnlock()
			return result
		}
		// 如果过期了，从内存中移除
		klineAnalysisCacheMu.RUnlock()
		klineAnalysisCacheMu.Lock()
		delete(klineAnalysisCache, symbol)
		klineAnalysisCacheMu.Unlock()
	} else {
		klineAnalysisCacheMu.RUnlock()
	}

	// 如果内存中没有，尝试从文件加载
	result, err := loadKlineAnalysisFromFile(symbol)
	if err != nil {
		log.Printf("⚠️ 从文件加载 %s 的K线分析缓存失败: %v", symbol, err)
		return nil
	}

	if result == nil {
		log.Printf("📋 %s 的K线分析缓存文件不存在", symbol)
		return nil
	}

	// 检查文件中的缓存是否过期
	if time.Since(result.Timestamp) > klineAnalysisCacheTTL {
		// 过期了，删除文件
		log.Printf("⏰ %s 的K线分析缓存已过期（分析时间：%s，已过 %v），删除文件",
			symbol, result.Timestamp.Format("2006-01-02 15:04:05"), time.Since(result.Timestamp))
		filePath := getCacheFilePath(symbol)
		os.Remove(filePath)
		return nil
	}

	// 文件中的缓存有效，加载到内存
	log.Printf("📂 从文件加载 %s 的K线分析缓存（分析时间：%s，剩余有效期：%v）",
		symbol, result.Timestamp.Format("2006-01-02 15:04:05"), klineAnalysisCacheTTL-time.Since(result.Timestamp))
	klineAnalysisCacheMu.Lock()
	klineAnalysisCache[symbol] = result
	klineAnalysisCacheMu.Unlock()

	return result
}

// LoadAllKlineAnalysisFromFiles 从文件系统加载所有缓存（程序启动时调用）
func LoadAllKlineAnalysisFromFiles() error {
	if err := initCacheDir(); err != nil {
		return err
	}

	files, err := os.ReadDir(klineAnalysisCacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在，返回nil
		}
		return fmt.Errorf("读取缓存目录失败: %w", err)
	}

	loadedCount := 0
	expiredCount := 0

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		// 从文件名提取symbol（去掉.json后缀）
		symbol := strings.TrimSuffix(file.Name(), ".json")

		result, err := loadKlineAnalysisFromFile(symbol)
		if err != nil {
			log.Printf("⚠️ 加载 %s 的缓存文件失败: %v", symbol, err)
			continue
		}

		if result == nil {
			continue
		}

		// 检查是否过期
		if time.Since(result.Timestamp) > klineAnalysisCacheTTL {
			// 删除过期文件
			filePath := getCacheFilePath(symbol)
			os.Remove(filePath)
			expiredCount++
			continue
		}

		// 加载到内存
		klineAnalysisCacheMu.Lock()
		klineAnalysisCache[symbol] = result
		klineAnalysisCacheMu.Unlock()

		loadedCount++
	}

	log.Printf("📂 从文件加载K线分析缓存: 成功 %d 个，过期删除 %d 个", loadedCount, expiredCount)
	return nil
}

// logKlineAnalysisResult 在日志中输出K线分析结果
func logKlineAnalysisResult(symbol string, result *KlineAnalysisResult) {
	log.Printf("\n" + strings.Repeat("=", 70))
	log.Printf("📊 %s K线分析结果", symbol)
	log.Printf(strings.Repeat("=", 70))
	log.Printf("📅 分析时间: %s", result.Timestamp.Format("2006-01-02 15:04:05"))
	log.Printf("💰 当前价格: %.4f", result.CurrentPrice)

	// 支撑位
	if len(result.SupportLevels) > 0 {
		log.Printf("🟢 支撑位 (%d个):", len(result.SupportLevels))
		for i, level := range result.SupportLevels {
			if i < 5 { // 只显示前5个
				log.Printf("   %d. %.4f (强度%d, %s周期, %s)", i+1, level.Price, level.Strength, level.Timeframe, level.Type)
			}
		}
		if len(result.SupportLevels) > 5 {
			log.Printf("   ... 还有 %d 个支撑位", len(result.SupportLevels)-5)
		}
	} else {
		log.Printf("🟢 支撑位: 无")
	}

	// 阻力位
	if len(result.ResistanceLevels) > 0 {
		log.Printf("🔴 阻力位 (%d个):", len(result.ResistanceLevels))
		for i, level := range result.ResistanceLevels {
			if i < 5 { // 只显示前5个
				log.Printf("   %d. %.4f (强度%d, %s周期, %s)", i+1, level.Price, level.Strength, level.Timeframe, level.Type)
			}
		}
		if len(result.ResistanceLevels) > 5 {
			log.Printf("   ... 还有 %d 个阻力位", len(result.ResistanceLevels)-5)
		}
	} else {
		log.Printf("🔴 阻力位: 无")
	}

	// 斐波那契分析（重点显示0.618位）
	fibCount := 0
	fib618Levels := make([]string, 0)

	timeframes := []struct {
		name string
		fib  *FibonacciAnalysis
	}{
		{"15m", result.Fibonacci15m},
		{"1h", result.Fibonacci1h},
		{"4h", result.Fibonacci4h},
		{"12h", result.Fibonacci12h},
		{"1d", result.Fibonacci1d},
	}

	for _, tf := range timeframes {
		if tf.fib != nil {
			fibCount++
			if val, ok := tf.fib.Retracement["0.618"]; ok {
				fib618Levels = append(fib618Levels, fmt.Sprintf("%s=%.4f", tf.name, val))
			}
		}
	}

	if fibCount > 0 {
		log.Printf("📐 斐波那契分析 (%d个周期):", fibCount)
		if len(fib618Levels) > 0 {
			log.Printf("   ⭐ 0.618回撤位 (黄金分割): %s", strings.Join(fib618Levels, ", "))
		}
		// 显示每个周期的关键信息
		for _, tf := range timeframes {
			if tf.fib != nil {
				log.Printf("   [%s周期] 高点: %.4f, 低点: %.4f",
					tf.name, tf.fib.SwingHigh, tf.fib.SwingLow)
			}
		}
	} else {
		log.Printf("📐 斐波那契分析: 无")
	}

	// 多周期共振
	if len(result.ConfluenceLevels) > 0 {
		log.Printf("✨ 多周期共振位 (%d个):", len(result.ConfluenceLevels))
		for i, level := range result.ConfluenceLevels {
			if i < 3 { // 只显示前3个
				log.Printf("   %d. %.4f (强度%d, 周期: %s, 类型: %s)",
					i+1, level.Price, level.Strength,
					strings.Join(level.Timeframes, ","), strings.Join(level.Types, ","))
			}
		}
		if len(result.ConfluenceLevels) > 3 {
			log.Printf("   ... 还有 %d 个共振位", len(result.ConfluenceLevels)-3)
		}
	} else {
		log.Printf("✨ 多周期共振位: 无")
	}

	// 支撑阻力转换
	if len(result.BrokenResistance) > 0 {
		log.Printf("⬆️ 已突破转为支撑的阻力位 (%d个):", len(result.BrokenResistance))
		for i, level := range result.BrokenResistance {
			if i < 3 {
				log.Printf("   %d. %.4f (强度%d, %s周期)", i+1, level.Price, level.Strength, level.Timeframe)
			}
		}
		if len(result.BrokenResistance) > 3 {
			log.Printf("   ... 还有 %d 个", len(result.BrokenResistance)-3)
		}
	}

	if len(result.BrokenSupport) > 0 {
		log.Printf("⬇️ 已跌破转为阻力的支撑位 (%d个):", len(result.BrokenSupport))
		for i, level := range result.BrokenSupport {
			if i < 3 {
				log.Printf("   %d. %.4f (强度%d, %s周期)", i+1, level.Price, level.Strength, level.Timeframe)
			}
		}
		if len(result.BrokenSupport) > 3 {
			log.Printf("   ... 还有 %d 个", len(result.BrokenSupport)-3)
		}
	}

	// 分析摘要
	if result.AnalysisSummary != "" {
		log.Printf("📝 分析摘要:")
		// 如果摘要太长，只显示前200字符
		summary := result.AnalysisSummary
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		log.Printf("   %s", summary)
	}

	log.Printf(strings.Repeat("=", 70))
}

// SetKlineAnalysis 设置K线分析结果（写入缓存）
func SetKlineAnalysis(symbol string, result *KlineAnalysisResult) {
	symbol = Normalize(symbol)
	klineAnalysisCacheMu.Lock()
	defer klineAnalysisCacheMu.Unlock()

	result.Timestamp = time.Now()
	klineAnalysisCache[symbol] = result

	// 同时保存到JSON文件
	if err := saveKlineAnalysisToFile(symbol, result); err != nil {
		log.Printf("⚠️ 保存 %s 的K线分析缓存到文件失败: %v", symbol, err)
	}

	// 在日志中输出分析结果
	logKlineAnalysisResult(symbol, result)

	log.Printf("✅ K线分析缓存已更新: %s (缓存有效期15分钟)", symbol)
}

// initCacheDir 初始化缓存目录
func initCacheDir() error {
	if cacheDirInitialized {
		return nil
	}

	if err := os.MkdirAll(klineAnalysisCacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}

	cacheDirInitialized = true
	return nil
}

// getCacheFilePath 获取缓存文件路径
func getCacheFilePath(symbol string) string {
	return filepath.Join(klineAnalysisCacheDir, fmt.Sprintf("%s.json", symbol))
}

// saveKlineAnalysisToFile 保存K线分析结果到JSON文件
func saveKlineAnalysisToFile(symbol string, result *KlineAnalysisResult) error {
	if err := initCacheDir(); err != nil {
		return err
	}

	filePath := getCacheFilePath(symbol)
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// loadKlineAnalysisFromFile 从JSON文件加载K线分析结果
func loadKlineAnalysisFromFile(symbol string) (*KlineAnalysisResult, error) {
	filePath := getCacheFilePath(symbol)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 文件不存在，返回nil
		}
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	var result KlineAnalysisResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	return &result, nil
}

// IsAnalysisInProgress 检查是否正在分析该币种
func IsAnalysisInProgress(symbol string) bool {
	symbol = Normalize(symbol)
	analysisInProgressMu.Lock()
	defer analysisInProgressMu.Unlock()
	return analysisInProgress[symbol]
}

// SetAnalysisInProgress 设置分析状态
func SetAnalysisInProgress(symbol string, inProgress bool) {
	symbol = Normalize(symbol)
	analysisInProgressMu.Lock()
	defer analysisInProgressMu.Unlock()
	if inProgress {
		analysisInProgress[symbol] = true
	} else {
		delete(analysisInProgress, symbol)
	}
}

// AnalyzeKlinesWithLLM 使用LLM分析K线数据并缓存结果（单个币种）
func AnalyzeKlinesWithLLM(symbol string, data *Data, mcpClient *mcp.Client) error {
	symbol = Normalize(symbol)

	// 检查是否正在分析
	if IsAnalysisInProgress(symbol) {
		return fmt.Errorf("正在分析中，跳过")
	}

	// 检查缓存是否仍然有效
	cached := GetKlineAnalysis(symbol)
	if cached != nil {
		log.Printf("⏭️  %s 的K线分析缓存仍然有效，跳过", symbol)
		return nil
	}

	// 标记为正在分析
	SetAnalysisInProgress(symbol, true)
	defer SetAnalysisInProgress(symbol, false)

	log.Printf("🔍 开始分析 %s 的K线数据（通过LLM）...", symbol)

	// 构建分析提示词
	systemPrompt := buildKlineAnalysisPrompt()
	userPrompt := buildKlineAnalysisUserPrompt(symbol, data)

	// 调用LLM进行分析（K线分析需要更多tokens，设置max_tokens=4000以确保完整输出）
	aiResponse, err := mcpClient.CallWithMessagesWithMaxTokens(systemPrompt, userPrompt, 4000)
	if err != nil {
		return fmt.Errorf("LLM分析K线失败: %w", err)
	}

	// 记录响应长度
	responseLen := len(aiResponse)
	log.Printf("📥 [%s] LLM响应长度: %d 字符", symbol, responseLen)

	// 解析分析结果
	result, err := parseKlineAnalysisResult(aiResponse, symbol, data.CurrentPrice)
	if err != nil {
		// 解析失败，记录详细的错误信息
		log.Printf("❌ [%s] 解析分析结果失败: %v", symbol, err)
		log.Printf("❌ [%s] LLM响应长度: %d 字符", symbol, responseLen)

		// 如果是JSON解析错误，可能是token限制导致响应被截断
		if strings.Contains(err.Error(), "unexpected end of JSON") ||
			strings.Contains(err.Error(), "JSON格式不完整") {
			log.Printf("⚠️ [%s] 可能原因：LLM响应被截断（token限制），响应长度: %d", symbol, responseLen)
			log.Printf("⚠️ [%s] 建议：增加max_tokens或减少K线数据量", symbol)

			// 检查是否接近token限制（DeepSeek的max_tokens是4000）
			// 如果响应很短但包含不完整JSON，可能是被截断了
			if responseLen < 500 {
				log.Printf("⚠️ [%s] 响应过短，可能是LLM未完成输出", symbol)
			}
		}

		return fmt.Errorf("解析分析结果失败: %w", err)
	}

	// 验证解析结果是否有实际内容
	if len(result.SupportLevels) == 0 && len(result.ResistanceLevels) == 0 &&
		result.Fibonacci15m == nil && result.Fibonacci1h == nil &&
		result.Fibonacci4h == nil && result.Fibonacci1d == nil {
		log.Printf("⚠️ [%s] 解析结果为空（所有字段都为null），可能LLM未返回有效数据", symbol)
		log.Printf("⚠️ [%s] 分析摘要: %s", symbol, result.AnalysisSummary)
		// 不返回错误，但记录警告
	}

	// 缓存结果
	SetKlineAnalysis(symbol, result)

	log.Printf("✅ %s 的K线分析完成并已缓存", symbol)
	return nil
}

// AnalyzeKlinesBatchWithLLM 批量分析多个币种的K线数据（分批处理，避免token过长）
// symbols: 币种列表（已标准化）
// dataMap: symbol -> Data 映射
// mcpClient: MCP客户端
// batchSize: 每批分析的币种数量（建议2-3个）
func AnalyzeKlinesBatchWithLLM(symbols []string, dataMap map[string]*Data, mcpClient *mcp.Client, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 2 // 默认每批2个币种
	}

	log.Printf("📦 开始批量分析 %d 个币种的K线数据（每批 %d 个）", len(symbols), batchSize)

	// 过滤掉已缓存和正在分析的币种
	pendingSymbols := make([]string, 0)
	for _, symbol := range symbols {
		symbol = Normalize(symbol)

		// 检查缓存
		if cached := GetKlineAnalysis(symbol); cached != nil {
			continue
		}

		// 检查是否正在分析
		if IsAnalysisInProgress(symbol) {
			continue
		}

		// 检查是否有数据
		if _, hasData := dataMap[symbol]; !hasData {
			continue
		}

		pendingSymbols = append(pendingSymbols, symbol)
	}

	if len(pendingSymbols) == 0 {
		log.Printf("✅ 所有币种的K线分析缓存都有效，无需重新分析")
		return nil
	}

	log.Printf("📋 待分析币种数量: %d（已过滤缓存和正在分析的币种）", len(pendingSymbols))

	// 分批处理
	successCount := 0
	failCount := 0

	for i := 0; i < len(pendingSymbols); i += batchSize {
		end := i + batchSize
		if end > len(pendingSymbols) {
			end = len(pendingSymbols)
		}

		batchSymbols := pendingSymbols[i:end]
		log.Printf("📊 批次 %d/%d: 分析 %d 个币种 (%s)",
			(i/batchSize)+1, (len(pendingSymbols)+batchSize-1)/batchSize,
			len(batchSymbols), strings.Join(batchSymbols, ", "))

		// 标记为正在分析
		for _, symbol := range batchSymbols {
			SetAnalysisInProgress(symbol, true)
		}

		// 构建批量分析的提示词
		systemPrompt := buildKlineAnalysisPrompt()
		userPrompt := buildKlineAnalysisBatchUserPrompt(batchSymbols, dataMap)

		// 调用LLM进行分析（批量分析需要更多tokens，设置max_tokens=6000）
		aiResponse, err := mcpClient.CallWithMessagesWithMaxTokens(systemPrompt, userPrompt, 6000)

		// 解析批量分析结果
		results, parseErr := parseKlineAnalysisBatchResult(aiResponse, batchSymbols, dataMap)

		// 取消正在分析标记并缓存结果
		for _, symbol := range batchSymbols {
			SetAnalysisInProgress(symbol, false)

			if err != nil {
				log.Printf("⚠️ 批次中 %s 的LLM调用失败: %v", symbol, err)
				failCount++
				continue
			}

			if parseErr != nil {
				log.Printf("⚠️ 批次中 %s 的解析失败: %v", symbol, parseErr)
				failCount++
				continue
			}

			// 查找对应的分析结果
			if result, ok := results[symbol]; ok {
				SetKlineAnalysis(symbol, result)
				successCount++
				log.Printf("✅ %s 的K线分析完成并已缓存", symbol)
			} else {
				failCount++
				log.Printf("⚠️ 批次中 %s 的分析结果未找到", symbol)
			}
		}

		// 批次间短暂延迟，避免API限流
		if end < len(pendingSymbols) {
			time.Sleep(2 * time.Second)
		}
	}

	log.Printf("✅ 批量K线分析完成: 成功 %d 个，失败 %d 个", successCount, failCount)
	return nil
}

// buildKlineAnalysisPrompt 构建K线分析的系统提示词
func buildKlineAnalysisPrompt() string {
	var sb strings.Builder

	sb.WriteString("你是一个专业的加密货币技术分析师，专门分析K线数据以识别支撑位、阻力位和斐波那契位。\n\n")
	sb.WriteString("## 任务\n\n")
	sb.WriteString("分析提供的K线数据，识别以下内容：\n\n")
	sb.WriteString("1. **支撑位和阻力位**：\n")
	sb.WriteString("   - 识别关键高点和低点\n")
	sb.WriteString("   - 标记多次触及但未突破的位置\n")
	sb.WriteString("   - 整数位、心理价位等关键位置\n")
	sb.WriteString("   - 按强度排序（1-5，5最强）\n\n")

	sb.WriteString("2. **斐波那契分析**（各周期独立分析，⚠️ 必须执行）：\n")
	sb.WriteString("   - **识别波段高点和低点**：从K线数据中找出明显的波段高点和低点作为计算基准，必须覆盖足够大的波动区间，不能只取最近几根K线的微小波动\n")
	sb.WriteString("   - **波段有效性要求**：所选波段必须是明显的转折，满足下列最小跨度，否则需要继续向前回溯重新选择：\n")
	sb.WriteString("     • 15m 周期：高低点价差 ≥ 当前价格的 1.5%\n")
	sb.WriteString("     • 1h 周期：高低点价差 ≥ 当前价格的 3%\n")
	sb.WriteString("     • 4h 周期：高低点价差 ≥ 当前价格的 5%\n")
	sb.WriteString("     • 12h 周期：高低点价差 ≥ 当前价格的 8%\n")
	sb.WriteString("     • 1d 周期：高低点价差 ≥ 当前价格的 10%\n")
	sb.WriteString("     ⚠️ 如果当前可见数据无法满足上述跨度，请继续向更早的K线回看，直到找到足够大的波段；若仍无法满足，请将该周期的结果设为 null\n")
	sb.WriteString("   - **计算斐波那契回撤位**：这是关键分析步骤，**只计算以下3个回撤位**：\n")
	sb.WriteString("     • 0.382（弱回撤）\n")
	sb.WriteString("     • 0.618（黄金分割，最重要的回撤位）\n")
	sb.WriteString("     • 0.786（深度回撤）\n")
	sb.WriteString("     ⚠️ **重要**：不要计算0.236和0.5回撤位，只计算上述3个回撤位\n")
	sb.WriteString("   - **计算斐波那契扩展位**（用于目标位预测）：1.272, 1.618, 2.0, 2.618\n")
	sb.WriteString("   - **各周期独立分析**：15m, 1h, 4h, 12h, 1d（每个周期都需要计算上述3个回撤位）\n")
	sb.WriteString("   - ⚠️ **特别强调**：0.618斐波那契回撤位是最重要的技术位，必须重点关注并与其他支撑/阻力位进行共振分析\n\n")

	sb.WriteString("3. **多周期共振**：\n")
	sb.WriteString("   - 找出多个周期的斐波那契位或支撑/阻力位重合的位置\n")
	sb.WriteString("   - 这些是最高质量的交易信号位置\n\n")

	sb.WriteString("4. **支撑阻力转换**：\n")
	sb.WriteString("   - 哪些阻力位已被突破转为支撑（当前价格在该位置上方）\n")
	sb.WriteString("   - 哪些支撑位已被跌破转为阻力（当前价格在该位置下方）\n\n")

	sb.WriteString("## 输出格式（JSON）\n\n")
	sb.WriteString("请以JSON格式输出分析结果：\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"support_levels\": [\n")
	sb.WriteString("    {\"price\": 50000.0, \"strength\": 4, \"timeframe\": \"4h\", \"type\": \"support\"}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"resistance_levels\": [\n")
	sb.WriteString("    {\"price\": 52000.0, \"strength\": 5, \"timeframe\": \"1d\", \"type\": \"resistance\"}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"fibonacci_15m\": {\n")
	sb.WriteString("    \"swing_high\": 51000.0,\n")
	sb.WriteString("    \"swing_low\": 49500.0,\n")
	sb.WriteString("    \"retracement\": {\"0.382\": 50023.0, \"0.618\": 50477.0, \"0.786\": 50826.0},\n")
	sb.WriteString("    \"extension\": {\"1.272\": 51408.0, \"1.618\": 51927.0, \"2.0\": 52500.0, \"2.618\": 53427.0}\n")
	sb.WriteString("  },\n")
	sb.WriteString("  \"fibonacci_1h\": {...},\n")
	sb.WriteString("  \"fibonacci_4h\": {...},\n")
	sb.WriteString("  \"fibonacci_12h\": {...},\n")
	sb.WriteString("  \"fibonacci_1d\": {...},\n")
	sb.WriteString("  \"confluence_levels\": [\n")
	sb.WriteString("    {\"price\": 50250.0, \"timeframes\": [\"15m\", \"1h\", \"4h\"], \"types\": [\"fibonacci\", \"support\"], \"strength\": 3}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"broken_resistance\": [\n")
	sb.WriteString("    {\"price\": 50000.0, \"strength\": 4, \"timeframe\": \"1h\", \"type\": \"support\"}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"broken_support\": [\n")
	sb.WriteString("    {\"price\": 49000.0, \"strength\": 3, \"timeframe\": \"4h\", \"type\": \"resistance\"}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"analysis_summary\": \"简要分析摘要\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## 注意事项\n\n")
	sb.WriteString("- ⚠️ **必须输出完整的JSON**：确保所有字段都填充完整，JSON结构必须完整闭合\n")
	sb.WriteString("- 只输出JSON，不要额外文本（前后不要有其他文字说明）\n")
	sb.WriteString("- 如果某个字段没有数据，使用空数组 `[]` 或 `null`，但JSON结构必须完整\n")
	sb.WriteString("- 价格精度保留2-4位小数\n")
	sb.WriteString("- 强度值范围1-5\n")
	sb.WriteString("- 周期标识：15m, 1h, 4h, 12h, 1d\n")
	sb.WriteString("- ⚠️ **重要**：JSON必须从 `{` 开始到 `}` 结束，完整且有效\n")

	return sb.String()
}

// buildKlineAnalysisUserPrompt 构建K线分析的用户提示词（单个币种）
func buildKlineAnalysisUserPrompt(symbol string, data *Data) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("请分析 %s 的K线数据（当前价格：%.4f）\n\n", symbol, data.CurrentPrice))

	// 提供关键周期的K线数据（每个周期最近100根，提供更多历史数据以便准确识别支撑阻力位和斐波那契位）
	klinesToShow := 100

	sb.WriteString(formatKlinesForAnalysis(symbol, data, klinesToShow))

	return sb.String()
}

// buildKlineAnalysisBatchUserPrompt 构建批量K线分析的用户提示词（多个币种）
func buildKlineAnalysisBatchUserPrompt(symbols []string, dataMap map[string]*Data) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("请分析以下 %d 个币种的K线数据：%s\n\n",
		len(symbols), strings.Join(symbols, ", ")))

	// 提供关键周期的K线数据（每个周期最近100根，提供更多历史数据以便准确识别支撑阻力位和斐波那契位）
	klinesToShow := 100

	for i, symbol := range symbols {
		data, ok := dataMap[symbol]
		if !ok {
			continue
		}

		sb.WriteString(fmt.Sprintf("=== 币种 %d: %s (当前价格：%.4f) ===\n\n", i+1, symbol, data.CurrentPrice))
		sb.WriteString(formatKlinesForAnalysis(symbol, data, klinesToShow))
		sb.WriteString("\n")
	}

	sb.WriteString("⚠️ 请为每个币种单独输出JSON结果，格式如下：\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"BTCUSDT\": {\n")
	sb.WriteString("    \"support_levels\": [...],\n")
	sb.WriteString("    \"resistance_levels\": [...],\n")
	sb.WriteString("    \"fibonacci_15m\": {...},\n")
	sb.WriteString("    ...\n")
	sb.WriteString("  },\n")
	sb.WriteString("  \"ETHUSDT\": {\n")
	sb.WriteString("    ...\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n\n")

	return sb.String()
}

// formatKlinesForAnalysis 格式化K线数据用于分析（通用函数）
func formatKlinesForAnalysis(symbol string, data *Data, klinesToShow int) string {
	var sb strings.Builder

	// 15分钟K线
	if len(data.Klines15m) > 0 {
		sb.WriteString(fmt.Sprintf("[15分钟K线] 最近%d根:\n", klinesToShow))
		start := len(data.Klines15m) - klinesToShow
		if start < 0 {
			start = 0
		}
		for i := start; i < len(data.Klines15m); i++ {
			k := data.Klines15m[i]
			sb.WriteString(fmt.Sprintf("  %d|%.4f|%.4f|%.4f|%.4f|%.2f\n", k.OpenTime, k.Open, k.High, k.Low, k.Close, k.Volume))
		}
		sb.WriteString("\n")
	}

	if len(data.Klines1h) > 0 {
		sb.WriteString(fmt.Sprintf("[1小时K线] 最近%d根:\n", klinesToShow))
		start := len(data.Klines1h) - klinesToShow
		if start < 0 {
			start = 0
		}
		for i := start; i < len(data.Klines1h); i++ {
			k := data.Klines1h[i]
			sb.WriteString(fmt.Sprintf("  %d|%.4f|%.4f|%.4f|%.4f|%.2f\n", k.OpenTime, k.Open, k.High, k.Low, k.Close, k.Volume))
		}
		sb.WriteString("\n")
	}

	if len(data.Klines4h) > 0 {
		sb.WriteString(fmt.Sprintf("[4小时K线] 最近%d根:\n", klinesToShow))
		start := len(data.Klines4h) - klinesToShow
		if start < 0 {
			start = 0
		}
		for i := start; i < len(data.Klines4h); i++ {
			k := data.Klines4h[i]
			sb.WriteString(fmt.Sprintf("  %d|%.4f|%.4f|%.4f|%.4f|%.2f\n", k.OpenTime, k.Open, k.High, k.Low, k.Close, k.Volume))
		}
		sb.WriteString("\n")
	}

	if len(data.Klines12h) > 0 {
		sb.WriteString(fmt.Sprintf("[12小时K线] 最近%d根:\n", klinesToShow))
		start := len(data.Klines12h) - klinesToShow
		if start < 0 {
			start = 0
		}
		for i := start; i < len(data.Klines12h); i++ {
			k := data.Klines12h[i]
			sb.WriteString(fmt.Sprintf("  %d|%.4f|%.4f|%.4f|%.4f|%.2f\n", k.OpenTime, k.Open, k.High, k.Low, k.Close, k.Volume))
		}
		sb.WriteString("\n")
	}

	if len(data.Klines1d) > 0 {
		sb.WriteString(fmt.Sprintf("[1日K线] 最近%d根:\n", klinesToShow))
		start := len(data.Klines1d) - klinesToShow
		if start < 0 {
			start = 0
		}
		for i := start; i < len(data.Klines1d); i++ {
			k := data.Klines1d[i]
			sb.WriteString(fmt.Sprintf("  %d|%.4f|%.4f|%.4f|%.4f|%.2f\n", k.OpenTime, k.Open, k.High, k.Low, k.Close, k.Volume))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// parseKlineAnalysisResult 解析LLM返回的分析结果
func parseKlineAnalysisResult(response, symbol string, currentPrice float64) (*KlineAnalysisResult, error) {
	// 记录原始响应用于调试（只记录前500字符，避免日志过长）
	responsePreview := response
	if len(responsePreview) > 500 {
		responsePreview = responsePreview[:500] + "..."
	}
	log.Printf("🔍 [%s] LLM响应预览（前500字符）: %s", symbol, responsePreview)

	// 提取JSON部分 - 尝试多种方法
	var jsonStr string

	// 方法1：查找代码块中的JSON
	if startIdx := strings.Index(response, "```json"); startIdx != -1 {
		// 从 ```json 之后开始查找结束标记 ```
		searchStart := startIdx + 7 // ```json 是7个字符
		endMarkerIdx := strings.Index(response[searchStart:], "```")
		if endMarkerIdx != -1 {
			// endMarkerIdx 是相对位置，需要加上 searchStart 得到绝对位置
			absoluteEndIdx := searchStart + endMarkerIdx
			jsonStr = response[searchStart:absoluteEndIdx]
			jsonStr = strings.TrimSpace(jsonStr)
		}
	}

	// 方法2：如果方法1失败，查找第一个{到最后一个}
	if jsonStr == "" {
		jsonStart := strings.Index(response, "{")
		if jsonStart == -1 {
			log.Printf("❌ [%s] 未找到JSON起始标记，原始响应长度: %d", symbol, len(response))
			return nil, fmt.Errorf("未找到JSON数据，响应长度: %d", len(response))
		}

		// 尝试找到匹配的结束}
		braceCount := 0
		jsonEnd := -1
		for i := jsonStart; i < len(response); i++ {
			if response[i] == '{' {
				braceCount++
			} else if response[i] == '}' {
				braceCount--
				if braceCount == 0 {
					jsonEnd = i
					break
				}
			}
		}

		if jsonEnd == -1 || jsonEnd <= jsonStart {
			log.Printf("❌ [%s] JSON格式不完整，起始位置: %d，未找到匹配的结束}", symbol, jsonStart)
			// 尝试使用最后一个}作为结束
			lastBrace := strings.LastIndex(response, "}")
			if lastBrace > jsonStart {
				jsonEnd = lastBrace
				log.Printf("⚠️ [%s] 使用最后一个}作为结束: %d", symbol, jsonEnd)
			} else {
				return nil, fmt.Errorf("JSON格式不完整，未找到匹配的结束}")
			}
		}

		jsonStr = response[jsonStart : jsonEnd+1]
	}

	// 清理JSON字符串（移除可能的代码块标记）
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	// 记录提取的JSON用于调试
	if len(jsonStr) > 300 {
		log.Printf("🔍 [%s] 提取的JSON预览（前300字符）: %s...", symbol, jsonStr[:300])
	} else {
		log.Printf("🔍 [%s] 提取的JSON: %s", symbol, jsonStr)
	}

	// 解析JSON
	var result KlineAnalysisResult
	result.Symbol = symbol
	result.CurrentPrice = currentPrice

	// 使用临时结构解析JSON
	var jsonData struct {
		SupportLevels    []PriceLevel       `json:"support_levels"`
		ResistanceLevels []PriceLevel       `json:"resistance_levels"`
		Fibonacci15m     *FibonacciAnalysis `json:"fibonacci_15m"`
		Fibonacci1h      *FibonacciAnalysis `json:"fibonacci_1h"`
		Fibonacci4h      *FibonacciAnalysis `json:"fibonacci_4h"`
		Fibonacci12h     *FibonacciAnalysis `json:"fibonacci_12h"`
		Fibonacci1d      *FibonacciAnalysis `json:"fibonacci_1d"`
		ConfluenceLevels []ConfluenceLevel  `json:"confluence_levels"`
		BrokenResistance []PriceLevel       `json:"broken_resistance"`
		BrokenSupport    []PriceLevel       `json:"broken_support"`
		AnalysisSummary  string             `json:"analysis_summary"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &jsonData); err != nil {
		log.Printf("❌ [%s] JSON解析失败: %v", symbol, err)
		log.Printf("❌ [%s] 尝试解析的JSON长度: %d 字符", symbol, len(jsonStr))

		// 尝试修复常见的JSON问题
		// 1. 尝试修复未闭合的字符串
		// 2. 尝试修复缺失的逗号
		// 3. 尝试修复多余的逗号

		// 如果JSON被截断，尝试部分解析
		if strings.Contains(err.Error(), "unexpected end of JSON input") {
			log.Printf("⚠️ [%s] JSON可能被截断，尝试部分解析...", symbol)

			// 尝试使用json.Valid检查JSON是否至少部分有效
			if json.Valid([]byte(jsonStr)) == false {
				// JSON无效，尝试找到最后一个有效的JSON片段
				log.Printf("⚠️ [%s] JSON无效，尝试修复或使用空结果", symbol)
			}
		}

		// 返回错误，不再返回空结果，让调用者决定如何处理
		return nil, fmt.Errorf("JSON解析失败: %w，JSON长度: %d，预览: %s", err, len(jsonStr),
			func() string {
				if len(jsonStr) > 200 {
					return jsonStr[:200] + "..."
				}
				return jsonStr
			}())
	}

	// 填充结果
	result.SupportLevels = jsonData.SupportLevels
	result.ResistanceLevels = jsonData.ResistanceLevels
	result.Fibonacci15m = jsonData.Fibonacci15m
	result.Fibonacci1h = jsonData.Fibonacci1h
	result.Fibonacci4h = jsonData.Fibonacci4h
	result.Fibonacci12h = jsonData.Fibonacci12h
	result.Fibonacci1d = jsonData.Fibonacci1d
	result.ConfluenceLevels = jsonData.ConfluenceLevels
	result.BrokenResistance = jsonData.BrokenResistance
	result.BrokenSupport = jsonData.BrokenSupport
	result.AnalysisSummary = jsonData.AnalysisSummary

	if result.AnalysisSummary == "" {
		result.AnalysisSummary = fmt.Sprintf("已识别 %d 个支撑位，%d 个阻力位，%d 个共振位",
			len(result.SupportLevels), len(result.ResistanceLevels), len(result.ConfluenceLevels))
	}

	// 过滤不满足最小波段跨度要求的斐波那契分析
	filterInvalidFibonacci(&result, currentPrice)

	return &result, nil
}

// filterInvalidFibonacci 移除波段跨度过小的斐波那契分析（避免无效结果）
func filterInvalidFibonacci(result *KlineAnalysisResult, currentPrice float64) {
	if result == nil || currentPrice <= 0 {
		return
	}

	thresholds := []struct {
		name    string
		fib     **FibonacciAnalysis
		percent float64
	}{
		{"15m", &result.Fibonacci15m, 0.015},
		{"1h", &result.Fibonacci1h, 0.03},
		{"4h", &result.Fibonacci4h, 0.05},
		{"12h", &result.Fibonacci12h, 0.08},
		{"1d", &result.Fibonacci1d, 0.10},
	}

	for _, tf := range thresholds {
		if tf.fib == nil || *tf.fib == nil {
			continue
		}

		fib := *tf.fib
		diff := math.Abs(fib.SwingHigh - fib.SwingLow)
		minDiff := currentPrice * tf.percent

		if diff < minDiff {
			log.Printf("⚠️ [%s] %s 周期斐波那契分析波段过小：高点=%.4f, 低点=%.4f, 差值=%.4f < %.4f (%.1f%%)，已移除",
				result.Symbol, tf.name, fib.SwingHigh, fib.SwingLow, diff, minDiff, tf.percent*100)
			*tf.fib = nil
		}
	}
}

// parseKlineAnalysisBatchResult 解析批量分析结果（多个币种）
func parseKlineAnalysisBatchResult(response string, symbols []string, dataMap map[string]*Data) (map[string]*KlineAnalysisResult, error) {
	results := make(map[string]*KlineAnalysisResult)

	// 提取JSON部分
	jsonStart := strings.Index(response, "{")
	if jsonStart == -1 {
		return nil, fmt.Errorf("未找到JSON数据")
	}

	jsonEnd := strings.LastIndex(response, "}")
	if jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, fmt.Errorf("JSON格式不完整")
	}

	jsonStr := response[jsonStart : jsonEnd+1]

	// 清理JSON字符串
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	// 解析为 map[string]interface{}，每个键是一个币种
	var batchData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &batchData); err != nil {
		log.Printf("⚠️ 批量JSON解析失败，尝试逐个解析: %v", err)
		// 如果批量解析失败，尝试逐个解析（LLM可能返回了多个单独的JSON）
		return parseKlineAnalysisBatchResultFallback(response, symbols, dataMap)
	}

	// 为每个币种解析对应的分析结果
	for _, symbol := range symbols {
		symbol = Normalize(symbol)

		coinData, ok := batchData[symbol].(map[string]interface{})
		if !ok {
			continue
		}

		data, hasData := dataMap[symbol]
		if !hasData {
			continue
		}

		// 转换为JSON再解析（使用和单个解析相同的逻辑）
		coinDataJSON, err := json.Marshal(coinData)
		if err != nil {
			log.Printf("⚠️ %s 数据序列化失败: %v", symbol, err)
			continue
		}

		// 使用临时结构解析
		var jsonData struct {
			SupportLevels    []PriceLevel       `json:"support_levels"`
			ResistanceLevels []PriceLevel       `json:"resistance_levels"`
			Fibonacci15m     *FibonacciAnalysis `json:"fibonacci_15m"`
			Fibonacci1h      *FibonacciAnalysis `json:"fibonacci_1h"`
			Fibonacci4h      *FibonacciAnalysis `json:"fibonacci_4h"`
			Fibonacci12h     *FibonacciAnalysis `json:"fibonacci_12h"`
			Fibonacci1d      *FibonacciAnalysis `json:"fibonacci_1d"`
			ConfluenceLevels []ConfluenceLevel  `json:"confluence_levels"`
			BrokenResistance []PriceLevel       `json:"broken_resistance"`
			BrokenSupport    []PriceLevel       `json:"broken_support"`
			AnalysisSummary  string             `json:"analysis_summary"`
		}

		if err := json.Unmarshal(coinDataJSON, &jsonData); err != nil {
			log.Printf("⚠️ %s 分析结果解析失败: %v", symbol, err)
			continue
		}

		result := &KlineAnalysisResult{
			Symbol:           symbol,
			CurrentPrice:     data.CurrentPrice,
			SupportLevels:    jsonData.SupportLevels,
			ResistanceLevels: jsonData.ResistanceLevels,
			Fibonacci15m:     jsonData.Fibonacci15m,
			Fibonacci1h:      jsonData.Fibonacci1h,
			Fibonacci4h:      jsonData.Fibonacci4h,
			Fibonacci12h:     jsonData.Fibonacci12h,
			Fibonacci1d:      jsonData.Fibonacci1d,
			ConfluenceLevels: jsonData.ConfluenceLevels,
			BrokenResistance: jsonData.BrokenResistance,
			BrokenSupport:    jsonData.BrokenSupport,
			AnalysisSummary:  jsonData.AnalysisSummary,
		}

		if result.AnalysisSummary == "" {
			result.AnalysisSummary = fmt.Sprintf("已识别 %d 个支撑位，%d 个阻力位，%d 个共振位",
				len(result.SupportLevels), len(result.ResistanceLevels), len(result.ConfluenceLevels))
		}

		// 过滤不满足最小波段跨度要求的斐波那契分析
		filterInvalidFibonacci(result, data.CurrentPrice)

		results[symbol] = result
	}

	return results, nil
}

// parseKlineAnalysisBatchResultFallback 备用解析方法（如果批量解析失败，尝试逐个解析）
func parseKlineAnalysisBatchResultFallback(response string, symbols []string, dataMap map[string]*Data) (map[string]*KlineAnalysisResult, error) {
	results := make(map[string]*KlineAnalysisResult)

	// 尝试为每个币种单独解析
	for _, symbol := range symbols {
		symbol = Normalize(symbol)
		data, hasData := dataMap[symbol]
		if !hasData {
			continue
		}

		result, err := parseKlineAnalysisResult(response, symbol, data.CurrentPrice)
		if err == nil {
			results[symbol] = result
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("无法解析任何币种的分析结果")
	}

	return results, nil
}

// FormatKlineAnalysis 格式化K线分析结果用于显示（替代原始K线数据）
func FormatKlineAnalysis(symbol string, data *Data, isPosition bool) string {
	var sb strings.Builder

	analysis := GetKlineAnalysis(symbol)
	if analysis == nil {
		// 如果没有缓存，回退到显示原始K线数据（兼容模式）
		sb.WriteString("⚠️ K线分析缓存未就绪，显示原始K线数据（兼容模式）\n\n")
		// 这里可以调用原来的Format函数显示原始数据
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("📊 %s K线分析结果（分析时间：%s）\n\n", symbol, analysis.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("当前价格：%.4f\n\n", analysis.CurrentPrice))

	// 支撑位
	if len(analysis.SupportLevels) > 0 {
		sb.WriteString("**支撑位**：\n")
		for _, level := range analysis.SupportLevels {
			sb.WriteString(fmt.Sprintf("  %.4f (强度%d, %s, %s)\n", level.Price, level.Strength, level.Timeframe, level.Type))
		}
		sb.WriteString("\n")
	}

	// 阻力位
	if len(analysis.ResistanceLevels) > 0 {
		sb.WriteString("**阻力位**：\n")
		for _, level := range analysis.ResistanceLevels {
			sb.WriteString(fmt.Sprintf("  %.4f (强度%d, %s, %s)\n", level.Price, level.Strength, level.Timeframe, level.Type))
		}
		sb.WriteString("\n")
	}

	// 斐波那契分析（各周期）
	timeframes := []struct {
		name string
		fib  *FibonacciAnalysis
	}{
		{"15分钟", analysis.Fibonacci15m},
		{"1小时", analysis.Fibonacci1h},
		{"4小时", analysis.Fibonacci4h},
		{"12小时", analysis.Fibonacci12h},
		{"1日", analysis.Fibonacci1d},
	}

	for _, tf := range timeframes {
		if tf.fib != nil {
			sb.WriteString(fmt.Sprintf("**%s周期斐波那契分析**：\n", tf.name))
			sb.WriteString(fmt.Sprintf("  波段高点：%.4f | 波段低点：%.4f\n", tf.fib.SwingHigh, tf.fib.SwingLow))
			if len(tf.fib.Retracement) > 0 {
				sb.WriteString("  回撤位：")
				levels := []string{"0.382", "0.618", "0.786"} // 只显示这3个回撤位
				for _, key := range levels {
					if val, ok := tf.fib.Retracement[key]; ok {
						sb.WriteString(fmt.Sprintf(" %s=%.4f", key, val))
					}
				}
				sb.WriteString("\n")
			}
			if len(tf.fib.Extension) > 0 {
				sb.WriteString("  扩展位：")
				levels := []string{"1.272", "1.618", "2.0", "2.618"}
				for _, key := range levels {
					if val, ok := tf.fib.Extension[key]; ok {
						sb.WriteString(fmt.Sprintf(" %s=%.4f", key, val))
					}
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

	// 多周期共振
	if len(analysis.ConfluenceLevels) > 0 {
		sb.WriteString("**多周期共振位**：\n")
		for _, level := range analysis.ConfluenceLevels {
			sb.WriteString(fmt.Sprintf("  %.4f (强度%d, 周期：%s, 类型：%s)\n",
				level.Price, level.Strength, strings.Join(level.Timeframes, ","), strings.Join(level.Types, ",")))
		}
		sb.WriteString("\n")
	}

	// 支撑阻力转换
	if len(analysis.BrokenResistance) > 0 {
		sb.WriteString("**已突破转为支撑的阻力位**：\n")
		for _, level := range analysis.BrokenResistance {
			sb.WriteString(fmt.Sprintf("  %.4f (强度%d, %s)\n", level.Price, level.Strength, level.Timeframe))
		}
		sb.WriteString("\n")
	}

	if len(analysis.BrokenSupport) > 0 {
		sb.WriteString("**已跌破转为阻力的支撑位**：\n")
		for _, level := range analysis.BrokenSupport {
			sb.WriteString(fmt.Sprintf("  %.4f (强度%d, %s)\n", level.Price, level.Strength, level.Timeframe))
		}
		sb.WriteString("\n")
	}

	// 分析摘要
	if analysis.AnalysisSummary != "" {
		sb.WriteString(fmt.Sprintf("**分析摘要**：%s\n\n", analysis.AnalysisSummary))
	}

	sb.WriteString("⚠️ 此分析结果每15分钟更新一次\n\n")

	return sb.String()
}
