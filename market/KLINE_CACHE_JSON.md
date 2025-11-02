# K线分析缓存JSON文件说明

## 📁 缓存文件位置

缓存文件保存在：**`kline_analysis_cache/`** 目录下

每个币种对应一个JSON文件：
- `BTCUSDT.json` - BTC的分析结果
- `ETHUSDT.json` - ETH的分析结果
- `SOLUSDT.json` - SOL的分析结果
- ... 等等

## 📄 JSON文件格式

每个JSON文件包含完整的K线分析结果：

```json
{
  "symbol": "BTCUSDT",
  "timestamp": "2025-11-02T11:30:00Z",
  "current_price": 109998.5,
  "support_levels": [
    {
      "price": 109000.0,
      "strength": 4,
      "timeframe": "4h",
      "type": "support"
    }
  ],
  "resistance_levels": [
    {
      "price": 111000.0,
      "strength": 5,
      "timeframe": "1d",
      "type": "resistance"
    }
  ],
  "fibonacci_15m": {
    "swing_high": 110500.0,
    "swing_low": 109500.0,
    "retracement": {
      "0.236": 109764.0,
      "0.382": 109882.0,
      "0.5": 110000.0,
      "0.618": 110118.0,
      "0.786": 110236.0
    },
    "extension": {
      "1.272": 110636.0,
      "1.618": 111118.0,
      "2.0": 111500.0,
      "2.618": 112182.0
    }
  },
  "fibonacci_1h": { ... },
  "fibonacci_4h": { ... },
  "fibonacci_12h": { ... },
  "fibonacci_1d": { ... },
  "confluence_levels": [
    {
      "price": 110000.0,
      "timeframes": ["15m", "1h", "4h"],
      "types": ["fibonacci", "support"],
      "strength": 3
    }
  ],
  "broken_resistance": [
    {
      "price": 109500.0,
      "strength": 4,
      "timeframe": "1h",
      "type": "support"
    }
  ],
  "broken_support": [],
  "analysis_summary": "BTC当前价格位于关键支撑位上方，0.618斐波那契回撤位与4小时支撑位形成共振..."
}
```

## 🔍 查看缓存文件

### 方法1：直接查看文件

```bash
# 查看所有缓存文件
ls -lh kline_analysis_cache/

# 查看特定币种的缓存
cat kline_analysis_cache/BTCUSDT.json

# 使用jq格式化查看（如果安装了jq）
cat kline_analysis_cache/BTCUSDT.json | jq .

# 查看支撑位
cat kline_analysis_cache/BTCUSDT.json | jq '.support_levels'

# 查看斐波那契分析
cat kline_analysis_cache/BTCUSDT.json | jq '.fibonacci_1h'
```

### 方法2：程序自动加载

程序启动时会自动从JSON文件加载缓存：

```
📂 从文件加载K线分析缓存: 成功 3 个，过期删除 0 个
✅ K线分析缓存已从文件加载
```

## ⏰ 缓存有效期

- **有效期**：15分钟
- **自动过期**：超过15分钟的缓存会被自动删除
- **自动清理**：程序启动时会删除所有过期的缓存文件

## 📝 使用示例

### 查看所有缓存

```bash
cd /Users/moquanlin/code/nofx
ls -lh kline_analysis_cache/
```

### 查看BTC的分析结果

```bash
cat kline_analysis_cache/BTCUSDT.json | jq '.'
```

### 查看支撑位和阻力位

```bash
# 支撑位
cat kline_analysis_cache/BTCUSDT.json | jq '.support_levels'

# 阻力位
cat kline_analysis_cache/BTCUSDT.json | jq '.resistance_levels'

# 一起查看
cat kline_analysis_cache/BTCUSDT.json | jq '{support: .support_levels, resistance: .resistance_levels}'
```

### 查看斐波那契分析

```bash
# 1小时斐波那契
cat kline_analysis_cache/BTCUSDT.json | jq '.fibonacci_1h'

# 所有周期的斐波那契
cat kline_analysis_cache/BTCUSDT.json | jq '{fib_15m: .fibonacci_15m, fib_1h: .fibonacci_1h, fib_4h: .fibonacci_4h, fib_1d: .fibonacci_1d}'
```

### 查看分析摘要

```bash
cat kline_analysis_cache/BTCUSDT.json | jq -r '.analysis_summary'
```

## 🔄 缓存更新时机

缓存在以下情况会自动更新：

1. **LLM分析完成后**：每次K线分析完成，结果会自动保存到JSON文件
2. **程序启动时**：自动从JSON文件加载有效缓存到内存
3. **缓存过期时**：自动删除过期的JSON文件

## 🗑️ 清理缓存

### 手动删除单个币种缓存

```bash
rm kline_analysis_cache/BTCUSDT.json
```

### 删除所有缓存

```bash
rm -rf kline_analysis_cache/
```

### 删除过期缓存（程序会自动处理）

过期的缓存会在以下时机自动删除：
- 程序启动时加载缓存时
- 读取缓存发现过期时

## 💡 提示

1. **缓存目录位置**：`kline_analysis_cache/`（项目根目录下）
2. **文件格式**：UTF-8编码的JSON文件
3. **文件权限**：644（可读可写，其他人只读）
4. **Git忽略**：缓存目录已添加到`.gitignore`，不会被提交到Git

## 🔍 调试

如果缓存文件有问题，可以：

1. **删除并重新生成**：
   ```bash
   rm kline_analysis_cache/BTCUSDT.json
   # 程序下次分析时会重新生成
   ```

2. **验证JSON格式**：
   ```bash
   cat kline_analysis_cache/BTCUSDT.json | jq . > /dev/null
   # 如果返回错误，说明JSON格式有问题
   ```

3. **查看日志**：
   程序运行时的日志会显示缓存加载和保存的情况

