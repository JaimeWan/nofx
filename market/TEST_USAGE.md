# K线分析测试代码使用指南

## 📋 快速开始

### 运行所有单元测试（不依赖API）
```bash
cd /Users/moquanlin/code/nofx
go test ./market -v -run "TestKline|TestCache|TestParse|TestFormat"
```

### 运行所有测试（包括真实数据测试）
```bash
go test ./market -v
```

## 🧪 测试分类和用法

### 1. 基础功能测试（快速测试，无需API）

#### 测试缓存基本功能
```bash
go test ./market -v -run TestKlineAnalysisCache
```
**功能**：测试缓存的设置、获取、过期机制

#### 测试缓存过期机制
```bash
go test ./market -v -run TestKlineAnalysisCacheExpiry
```
**功能**：验证缓存15分钟过期机制

#### 测试分析状态管理
```bash
go test ./market -v -run TestKlineAnalysisInProgress
```
**功能**：测试防止重复分析的锁定机制

#### 测试缓存跳过逻辑
```bash
go test ./market -v -run TestCacheSkipsAnalysis
```
**功能**：验证有效缓存时会跳过重新分析

---

### 2. 解析功能测试（模拟数据）

#### 测试JSON解析（单个币种）
```bash
go test ./market -v -run TestParseKlineAnalysisResult
```
**功能**：测试LLM返回的JSON解析是否正确

#### 测试批量JSON解析
```bash
go test ./market -v -run TestParseKlineAnalysisBatchResult
```
**功能**：测试批量分析结果的JSON解析

---

### 3. 真实数据测试（需要网络，不调用LLM）

#### 测试真实K线数据获取和格式化
```bash
go test ./market -v -run TestKlineAnalysisWithRealData -timeout 30s
```
**功能**：
- 从Binance获取BTCUSDT的真实K线数据
- 验证各周期K线数据完整性（15m, 1h, 4h, 12h, 1d）
- 测试格式化输出
- **不调用LLM**，只测试数据获取

#### 测试多个币种的真实数据格式化
```bash
go test ./market -v -run TestRealKlineDataFormat -timeout 30s
```
**功能**：
- 测试BTCUSDT, ETHUSDT, SOLUSDT的真实数据
- 验证持仓格式和候选格式的输出差异

---

### 4. 集成测试（需要真实的DeepSeek API Key）

#### 单个币种完整分析测试
```bash
# 设置API key
export DEEPSEEK_API_KEY=your-api-key-here

# 运行集成测试
go test ./market -v -run TestKlineAnalysisIntegration -timeout 60s
```
**功能**：
- 获取真实K线数据
- 调用真实的DeepSeek API进行分析
- 验证分析结果并缓存
- **会产生API费用**

#### 批量分析完整测试
```bash
# 设置API key
export DEEPSEEK_API_KEY=your-api-key-here

# 运行批量分析测试
go test ./market -v -run TestBatchAnalysisWithRealData -timeout 120s
```
**功能**：
- 批量获取多个币种的真实K线数据
- 每批2个币种，调用API分析
- 验证结果合并和缓存
- **会产生API费用**

---

### 5. 查看Prompt内容

#### 查看完整的K线分析Prompt
```bash
go test ./market -v -run TestShowKlineAnalysisPrompt
```
**功能**：
- 显示系统提示词（System Prompt）
- 显示用户提示词示例（使用真实BTCUSDT数据）
- 显示批量分析提示词示例
- 显示Token统计信息

#### 将Prompt保存到文件
```bash
SAVE_PROMPT=1 go test ./market -v -run TestShowPromptToFile
```
**功能**：将prompt保存到 `kline_analysis_prompt.md` 文件

---

### 6. 性能测试

#### 缓存读写性能基准测试
```bash
go test ./market -bench=BenchmarkKlineAnalysisCache -benchmem
```
**功能**：测试缓存读写性能

---

## 📊 测试输出示例

### 真实数据测试输出
```
=== RUN   TestKlineAnalysisWithRealData
    📊 获取 BTCUSDT 的真实K线数据...
    ✅ 获取到真实K线数据:
       当前价格: 109998.00
       15分钟K线: 150 根
       1小时K线: 150 根
       4小时K线: 150 根
       12小时K线: 150 根
       1日K线: 150 根
    ✅ 真实K线数据测试通过
```

### 集成测试输出（需要API key）
```
=== RUN   TestKlineAnalysisIntegration
    📊 获取 BTCUSDT 的真实K线数据...
    ✅ 获取到真实K线数据:
       当前价格: 109998.00
       15分钟K线: 150 根
    🔍 开始分析 BTCUSDT 的K线数据（真实API调用）...
    ✅ K线分析完成并已缓存
    ✅ 集成测试通过
       支撑位数量: 5
       阻力位数量: 3
       分析摘要: ...
```

---

## 🔧 常见问题

### Q: 如何只运行不依赖API的测试？
```bash
# 排除集成测试
go test ./market -v -run "Test.*" -run "!TestKlineAnalysisIntegration" -run "!TestBatchAnalysisWithRealData"
```

### Q: 如何查看详细的测试输出？
```bash
# 使用 -v 参数
go test ./market -v -run TestName
```

### Q: 测试超时怎么办？
```bash
# 增加超时时间
go test ./market -v -run TestName -timeout 5m
```

### Q: 如何运行特定的测试？
```bash
# 运行名称包含 "Cache" 的测试
go test ./market -v -run Test.*Cache

# 运行名称包含 "Real" 的测试
go test ./market -v -run Test.*Real
```

### Q: 集成测试失败怎么办？
1. 检查API key是否正确设置：`echo $DEEPSEEK_API_KEY`
2. 检查网络连接
3. 检查API余额是否充足
4. 查看详细错误信息：`go test ./market -v -run TestKlineAnalysisIntegration`

---

## 📝 测试文件说明

- **`kline_analysis_cache_test.go`**: 主要测试文件
  - 包含所有单元测试、集成测试、性能测试
  
- **`show_prompt_test.go`**: Prompt展示测试
  - 用于查看和验证prompt内容

---

## 🚀 快速测试流程

### 1. 快速验证（无API调用）
```bash
# 运行所有基础测试
go test ./market -v -run "TestKline|TestCache|TestParse"
```

### 2. 真实数据验证（无API调用）
```bash
# 测试真实数据获取
go test ./market -v -run "Test.*Real.*" -timeout 30s
```

### 3. 完整集成测试（需要API key）
```bash
# 设置API key
export DEEPSEEK_API_KEY=your-key

# 运行完整测试
go test ./market -v -timeout 5m
```

---

## 💡 提示

1. **开发阶段**：使用单元测试（不需要API key）
2. **验证阶段**：使用真实数据测试（不需要API key，但需要网络）
3. **集成测试**：只在需要验证完整流程时使用（需要API key和费用）

4. **查看Prompt**：随时运行 `TestShowKlineAnalysisPrompt` 查看当前使用的prompt内容

5. **调试**：使用 `-v` 参数查看详细日志输出

