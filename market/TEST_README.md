# K线分析缓存测试说明

## 快速开始

运行所有单元测试：
```bash
go test ./market -v -run "TestKline|TestCache|TestParse|TestFormat"
```

## 测试分类

### 1. 基础功能测试

#### TestKlineAnalysisCache
测试缓存的基本设置和获取功能。

```bash
go test ./market -v -run TestKlineAnalysisCache
```

#### TestKlineAnalysisCacheExpiry
测试缓存过期机制（15分钟TTL）。

```bash
go test ./market -v -run TestKlineAnalysisCacheExpiry
```

#### TestKlineAnalysisInProgress
测试分析状态管理（防止重复分析）。

```bash
go test ./market -v -run TestKlineAnalysisInProgress
```

#### TestCacheSkipsAnalysis
测试缓存跳过重复分析的逻辑。

```bash
go test ./market -v -run TestCacheSkipsAnalysis
```

### 2. 解析功能测试

#### TestParseKlineAnalysisResult
测试单个币种的分析结果解析（不依赖真实LLM调用）。

```bash
go test ./market -v -run TestParseKlineAnalysisResult
```

#### TestParseKlineAnalysisBatchResult
测试批量分析结果的解析。

```bash
go test ./market -v -run TestParseKlineAnalysisBatchResult
```

### 3. 格式化输出测试

#### TestFormatKlineAnalysis
测试分析结果的格式化输出。

```bash
go test ./market -v -run TestFormatKlineAnalysis
```

### 4. 集成测试（需要真实API）

#### TestKlineAnalysisIntegration
使用真实的DeepSeek API进行端到端测试。

**使用方法：**
```bash
# 设置环境变量
export DEEPSEEK_API_KEY=your-api-key

# 运行集成测试
go test ./market -v -run TestKlineAnalysisIntegration
```

**注意：**
- 此测试会调用真实的DeepSeek API，会产生费用
- 如果没有设置API key，测试会自动跳过
- 此测试需要网络连接

## 测试数据说明

测试使用模拟的K线数据：
- 15分钟K线：最近50根
- 1小时K线：最近50根
- 4小时K线：最近50根
- 12小时K线：最近50根
- 1日K线：最近50根

## 性能测试

运行性能基准测试：
```bash
go test ./market -bench=BenchmarkKlineAnalysisCache -benchmem
```

## 测试覆盖

当前测试覆盖：
- ✅ 缓存设置和获取
- ✅ 缓存过期机制
- ✅ 分析状态管理
- ✅ JSON解析（单个和批量）
- ✅ 格式化输出
- ✅ 集成测试（可选）

## 注意事项

1. **单元测试**：不依赖真实API，可以快速运行
2. **集成测试**：需要真实的API key，会产生费用
3. **测试数据**：使用模拟数据，不访问真实市场API
4. **缓存隔离**：每个测试都会清理自己的测试数据

## 示例输出

成功运行测试后，你会看到类似的输出：

```
=== RUN   TestKlineAnalysisCache
✅ K线分析缓存已更新: BTCUSDT (缓存有效期15分钟)
    kline_analysis_cache_test.go:173: ✅ 缓存设置和获取测试通过
--- PASS: TestKlineAnalysisCache (0.00s)

=== RUN   TestParseKlineAnalysisResult
    kline_analysis_cache_test.go:242: ✅ 解析分析结果测试通过
    kline_analysis_cache_test.go:243:   支撑位数量: 1
    kline_analysis_cache_test.go:244:   阻力位数量: 1
--- PASS: TestParseKlineAnalysisResult (0.00s)
```

