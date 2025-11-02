# K线分析缓存存储位置说明

## 📍 缓存存储位置

K线分析缓存存储在 **程序内存（RAM）** 中，**不持久化到磁盘**。

## 💾 存储方式

### 数据结构

```go
// 全局变量（内存存储）
var (
    klineAnalysisCache = make(map[string]*KlineAnalysisResult) // symbol -> analysis result
    klineAnalysisCacheMu sync.RWMutex                          // 读写锁
    klineAnalysisCacheTTL = 15 * time.Minute                   // 缓存有效期15分钟
    analysisInProgress = make(map[string]bool)                  // 正在分析的币种（避免重复分析）
    analysisInProgressMu sync.Mutex
)
```

### 存储位置详情

- **位置**: 程序运行时的内存（RAM）
- **结构**: `map[string]*KlineAnalysisResult`
  - Key: 币种符号（如 "BTCUSDT"）
  - Value: K线分析结果对象
- **同步机制**: 使用 `sync.RWMutex` 保证并发安全
- **有效期**: 15分钟（超过时间自动过期）

## 🔍 缓存内容

每个缓存项包含：

```go
type KlineAnalysisResult struct {
    Symbol              string    // 币种符号
    Timestamp           time.Time // 分析时间
    CurrentPrice        float64   // 当前价格
    
    // 支撑位/阻力位
    SupportLevels       []PriceLevel
    ResistanceLevels    []PriceLevel
    
    // 斐波那契分析（各周期）
    Fibonacci15m        *FibonacciAnalysis
    Fibonacci1h         *FibonacciAnalysis
    Fibonacci4h         *FibonacciAnalysis
    Fibonacci12h        *FibonacciAnalysis
    Fibonacci1d         *FibonacciAnalysis
    
    // 多周期共振
    ConfluenceLevels    []ConfluenceLevel
    
    // 支撑阻力转换
    BrokenResistance    []PriceLevel
    BrokenSupport       []PriceLevel
    
    // 分析摘要
    AnalysisSummary     string
}
```

## ⚠️ 重要特性

### 1. 内存存储
- ✅ **优点**: 访问速度快，无需磁盘IO
- ⚠️ **缺点**: 程序重启后缓存会丢失

### 2. 自动过期
- 缓存有效期：**15分钟**
- 超过15分钟的缓存会被自动清除
- 下次使用时会重新分析

### 3. 并发安全
- 使用 `sync.RWMutex` 保证多 goroutine 安全访问
- 支持多个并发读取，写入时互斥

### 4. 分析状态锁定
- 使用 `analysisInProgress` map 防止重复分析
- 分析中的币种不会再次触发分析

## 🔄 缓存生命周期

```
1. 程序启动
   └─> 缓存为空

2. 第一次分析
   └─> 调用 LLM 分析
   └─> 结果存入内存缓存
   └─> 缓存有效期：15分钟

3. 15分钟内再次使用
   └─> 直接从内存缓存读取
   └─> 不调用 LLM（节省 token）

4. 超过15分钟
   └─> 缓存自动过期
   └─> 下次使用时会重新分析

5. 程序关闭
   └─> 内存缓存清空
   └─> 下次启动需要重新分析
```

## 📊 查看缓存状态

### 在代码中查看

```go
// 获取缓存
analysis := market.GetKlineAnalysis("BTCUSDT")
if analysis != nil {
    fmt.Printf("缓存存在，分析时间: %v\n", analysis.Timestamp)
    fmt.Printf("支撑位数量: %d\n", len(analysis.SupportLevels))
    fmt.Printf("阻力位数量: %d\n", len(analysis.ResistanceLevels))
} else {
    fmt.Println("缓存不存在或已过期")
}
```

### 在测试中查看

```bash
# 运行缓存测试
go test ./market -v -run TestKlineAnalysisCache
```

## 🚀 未来可能的改进

如果需要持久化缓存，可以考虑：

1. **文件缓存**: 保存到 JSON 文件
2. **数据库缓存**: 使用 SQLite 或 Redis
3. **配置文件**: 保存到配置文件目录

但目前的设计是内存缓存，因为：
- 缓存有效期只有15分钟，重启后重新分析即可
- 内存访问速度快
- 实现简单，无需处理文件IO

## 📝 相关文件

- `market/kline_analysis_cache.go` - 缓存实现文件
- `market/kline_analysis_cache_test.go` - 缓存测试文件

