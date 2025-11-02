package market

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestShowKlineAnalysisPrompt 展示K线分析的完整prompt（用于查看）
func TestShowKlineAnalysisPrompt(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("K线分析 - 系统提示词 (System Prompt)")
	fmt.Println(strings.Repeat("=", 80))
	
	systemPrompt := buildKlineAnalysisPrompt()
	fmt.Println(systemPrompt)
	
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("K线分析 - 用户提示词示例 (User Prompt Example)")
	fmt.Println(strings.Repeat("=", 80))
	
	// 获取真实的K线数据作为示例
	symbol := "BTCUSDT"
	data, err := Get(symbol, "binance")
	if err != nil {
		t.Logf("⚠️ 无法获取真实数据，使用说明：需要从Binance获取K线数据")
		fmt.Println("\n示例用户提示词结构：")
		fmt.Println("- 币种名称和当前价格")
		fmt.Println("- 15分钟K线数据（格式：时间戳|开|高|低|收|成交量）")
		fmt.Println("- 1小时K线数据")
		fmt.Println("- 4小时K线数据")
		fmt.Println("- 12小时K线数据（如果可用）")
		fmt.Println("- 1日K线数据")
		return
	}
	
	// 显示单个币种的用户提示词
	userPrompt := buildKlineAnalysisUserPrompt(symbol, data)
	fmt.Println(userPrompt)
	
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("提示词统计信息")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("系统提示词长度: %d 字符\n", len(systemPrompt))
	fmt.Printf("用户提示词长度: %d 字符\n", len(userPrompt))
	fmt.Printf("总长度: %d 字符\n", len(systemPrompt)+len(userPrompt))
	fmt.Printf("估算token数（约）: %d tokens\n", (len(systemPrompt)+len(userPrompt))/4) // 粗略估算：1 token ≈ 4字符
	
	// 显示批量分析的提示词示例
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("批量分析 - 用户提示词示例（2个币种）")
	fmt.Println(strings.Repeat("=", 80))
	
	ethData, err := Get("ETHUSDT", "binance")
	if err == nil {
		dataMap := map[string]*Data{
			symbol:  data,
			"ETHUSDT": ethData,
		}
		batchPrompt := buildKlineAnalysisBatchUserPrompt([]string{symbol, "ETHUSDT"}, dataMap)
		fmt.Printf("批量提示词长度: %d 字符\n", len(batchPrompt))
		fmt.Printf("估算token数（约）: %d tokens\n", (len(systemPrompt)+len(batchPrompt))/4)
		
		// 只显示前500字符作为示例
		if len(batchPrompt) > 500 {
			fmt.Println("\n批量提示词前500字符：")
			fmt.Println(batchPrompt[:500] + "...")
		}
	}
	
	fmt.Println("\n" + strings.Repeat("=", 80))
}

// TestShowPromptToFile 将prompt保存到文件（方便查看）
func TestShowPromptToFile(t *testing.T) {
	if os.Getenv("SAVE_PROMPT") == "" {
		t.Skip("跳过：设置 SAVE_PROMPT=1 环境变量以保存prompt到文件")
		return
	}
	
	systemPrompt := buildKlineAnalysisPrompt()
	
	// 获取真实数据
	symbol := "BTCUSDT"
	data, err := Get(symbol, "binance")
	if err != nil {
		t.Fatalf("获取数据失败: %v", err)
	}
	
	userPrompt := buildKlineAnalysisUserPrompt(symbol, data)
	
	// 保存到文件
	content := fmt.Sprintf("# K线分析系统提示词\n\n%s\n\n# K线分析用户提示词示例\n\n%s", systemPrompt, userPrompt)
	
	err = os.WriteFile("kline_analysis_prompt.md", []byte(content), 0644)
	if err != nil {
		t.Fatalf("保存文件失败: %v", err)
	}
	
	t.Logf("✅ Prompt已保存到 kline_analysis_prompt.md")
}

