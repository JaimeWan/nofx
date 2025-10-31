package market

import (
	"math"
	"sort"
)

const (
	supportResistanceLookback = 120
	confluenceTolerance       = 0.002
)

// SupportResistanceSummary 汇总多时间框架的支撑/阻力位
type SupportResistanceSummary struct {
	Timeframes map[string]*SupportResistanceTimeframe `json:"timeframes"`
	Confluence *SupportResistanceConfluence           `json:"confluence"`
}

// SupportResistanceTimeframe 某个时间框架的支撑/阻力位
type SupportResistanceTimeframe struct {
	Supports    []SupportResistanceLevel `json:"supports"`
	Resistances []SupportResistanceLevel `json:"resistances"`
}

// SupportResistanceConfluence 多周期共振结果
type SupportResistanceConfluence struct {
	Supports    []SupportResistanceLevel `json:"supports"`
	Resistances []SupportResistanceLevel `json:"resistances"`
}

// SupportResistanceLevel 单个支撑或阻力位
type SupportResistanceLevel struct {
	Price     float64 `json:"price"`
	Strength  int     `json:"strength"`
	Distance  float64 `json:"distance"`
	Score     float64 `json:"score"`
	IsSupport bool    `json:"is_support"`
}

type levelPoint struct {
	Price float64
	Index int
}

type levelStats struct {
	touches int
	weight  float64
}

type weightedLevelSet struct {
	levels []SupportResistanceLevel
	weight float64
}

func calculateSupportResistanceSummary(klines3m, klines15m, klines1h, klines4h, klines12h, klines1d []Kline, currentPrice float64) *SupportResistanceSummary {
	tfInputs := []struct {
		name   string
		klines []Kline
	}{
		{name: "3m", klines: klines3m},
		{name: "15m", klines: klines15m},
		{name: "1h", klines: klines1h},
		{name: "4h", klines: klines4h},
		{name: "12h", klines: klines12h},
		{name: "1d", klines: klines1d},
	}

	summary := &SupportResistanceSummary{
		Timeframes: make(map[string]*SupportResistanceTimeframe),
	}

	for _, tf := range tfInputs {
		if len(tf.klines) == 0 {
			continue
		}

		supports, resistances := calculateSupportResistanceLevels(tf.klines, supportResistanceLookback)
		if len(supports) == 0 && len(resistances) == 0 {
			continue
		}

		tfPrice := tf.klines[len(tf.klines)-1].Close
		updateLevelDistances(supports, tfPrice, true)
		updateLevelDistances(resistances, tfPrice, false)

		summary.Timeframes[tf.name] = &SupportResistanceTimeframe{
			Supports:    supports,
			Resistances: resistances,
		}
	}

	baseOrder := []string{"3m", "15m", "1h", "4h"}
	var base *SupportResistanceTimeframe
	var baseName string
	for _, name := range baseOrder {
		if tf, ok := summary.Timeframes[name]; ok {
			base = tf
			baseName = name
			break
		}
	}

	if base == nil {
		return summary
	}

	supportSets := []weightedLevelSet{}
	resistanceSets := []weightedLevelSet{}

	weighting := []struct {
		name   string
		weight float64
	}{
		{name: "3m", weight: 0.8},
		{name: "15m", weight: 1.0},
		{name: "1h", weight: 1.2},
		{name: "4h", weight: 1.4},
		{name: "12h", weight: 1.6},
		{name: "1d", weight: 1.8},
	}

	for _, config := range weighting {
		if config.name == baseName {
			continue
		}
		if tf, ok := summary.Timeframes[config.name]; ok {
			supportSets = append(supportSets, weightedLevelSet{levels: tf.Supports, weight: config.weight})
			resistanceSets = append(resistanceSets, weightedLevelSet{levels: tf.Resistances, weight: config.weight})
		}
	}

	if len(supportSets) > 0 {
		summary.Confluence = &SupportResistanceConfluence{
			Supports:    findConfluenceLevels(base.Supports, supportSets, confluenceTolerance, currentPrice, true),
			Resistances: findConfluenceLevels(base.Resistances, resistanceSets, confluenceTolerance, currentPrice, false),
		}
	}

	return summary
}

func calculateSupportResistanceLevels(klines []Kline, lookback int) (supports []SupportResistanceLevel, resistances []SupportResistanceLevel) {
	if len(klines) == 0 {
		return nil, nil
	}

	start := len(klines) - lookback
	if start < 0 {
		start = 0
	}

	recent := klines[start:]
	if len(recent) == 0 {
		return nil, nil
	}

	currentPrice := recent[len(recent)-1].Close
	localMins := findLocalMinima(recent)
	localMaxs := findLocalMaxima(recent)

	supportStats := make(map[int]*levelStats)
	resistanceStats := make(map[int]*levelStats)
	total := len(recent)

	for _, point := range localMins {
		supportStats[point.Index] = &levelStats{
			touches: 1,
			weight:  calcRecencyWeight(total, point.Index),
		}
	}

	for _, point := range localMaxs {
		resistanceStats[point.Index] = &levelStats{
			touches: 1,
			weight:  calcRecencyWeight(total, point.Index),
		}
	}

	tolerance := 0.002

	for idx, candle := range recent {
		for _, point := range localMins {
			if idx == point.Index {
				continue
			}
			stats := supportStats[point.Index]
			if stats == nil || point.Price <= 0 {
				continue
			}
			if candle.Low <= point.Price*(1+tolerance) && candle.Low >= point.Price*(1-tolerance) {
				stats.touches++
				stats.weight += calcRecencyWeight(total, idx)
			}
		}

		for _, point := range localMaxs {
			if idx == point.Index {
				continue
			}
			stats := resistanceStats[point.Index]
			if stats == nil || point.Price <= 0 {
				continue
			}
			if candle.High >= point.Price*(1-tolerance) && candle.High <= point.Price*(1+tolerance) {
				stats.touches++
				stats.weight += calcRecencyWeight(total, idx)
			}
		}
	}

	for _, point := range localMins {
		stats := supportStats[point.Index]
		if stats == nil {
			continue
		}
		if point.Price < currentPrice {
			supports = append(supports, SupportResistanceLevel{
				Price:     point.Price,
				Strength:  stats.touches,
				Score:     stats.weight,
				IsSupport: true,
			})
		}
	}

	for _, point := range localMaxs {
		stats := resistanceStats[point.Index]
		if stats == nil {
			continue
		}
		if point.Price > currentPrice {
			resistances = append(resistances, SupportResistanceLevel{
				Price:     point.Price,
				Strength:  stats.touches,
				Score:     stats.weight,
				IsSupport: false,
			})
		}
	}

	supports = mergeNearbyLevels(supports, 0.002)
	resistances = mergeNearbyLevels(resistances, 0.002)

	updateLevelDistances(supports, currentPrice, true)
	updateLevelDistances(resistances, currentPrice, false)

	supports = sortAndFilterLevels(supports, true)
	resistances = sortAndFilterLevels(resistances, false)

	return supports, resistances
}

func findLocalMinima(klines []Kline) []levelPoint {
	if len(klines) < 3 {
		return []levelPoint{}
	}

	window := 3
	minima := make([]levelPoint, 0)

	for i := window; i < len(klines)-window; i++ {
		currentLow := klines[i].Low
		isLocalMin := true

		for j := i - window; j <= i+window; j++ {
			if j == i {
				continue
			}
			if klines[j].Low < currentLow {
				isLocalMin = false
				break
			}
		}

		if isLocalMin {
			duplicate := false
			for _, existing := range minima {
				if math.Abs(existing.Price-currentLow)/currentLow < 0.001 {
					duplicate = true
					break
				}
			}
			if !duplicate {
				minima = append(minima, levelPoint{Price: currentLow, Index: i})
			}
		}
	}

	return minima
}

func findLocalMaxima(klines []Kline) []levelPoint {
	if len(klines) < 3 {
		return []levelPoint{}
	}

	window := 3
	maxima := make([]levelPoint, 0)

	for i := window; i < len(klines)-window; i++ {
		currentHigh := klines[i].High
		isLocalMax := true

		for j := i - window; j <= i+window; j++ {
			if j == i {
				continue
			}
			if klines[j].High > currentHigh {
				isLocalMax = false
				break
			}
		}

		if isLocalMax {
			duplicate := false
			for _, existing := range maxima {
				if math.Abs(existing.Price-currentHigh)/currentHigh < 0.001 {
					duplicate = true
					break
				}
			}
			if !duplicate {
				maxima = append(maxima, levelPoint{Price: currentHigh, Index: i})
			}
		}
	}

	return maxima
}

func sortAndFilterLevels(levels []SupportResistanceLevel, isSupport bool) []SupportResistanceLevel {
	if len(levels) == 0 {
		return []SupportResistanceLevel{}
	}

	sort.SliceStable(levels, func(i, j int) bool {
		if levels[i].Score == levels[j].Score {
			if levels[i].Strength == levels[j].Strength {
				if levels[i].Distance == levels[j].Distance {
					if isSupport {
						return levels[i].Price > levels[j].Price
					}
					return levels[i].Price < levels[j].Price
				}
				return levels[i].Distance < levels[j].Distance
			}
			return levels[i].Strength > levels[j].Strength
		}
		return levels[i].Score > levels[j].Score
	})

	if len(levels) > 5 {
		levels = levels[:5]
	}

	sort.SliceStable(levels, func(i, j int) bool {
		if levels[i].Distance == levels[j].Distance {
			if isSupport {
				return levels[i].Price > levels[j].Price
			}
			return levels[i].Price < levels[j].Price
		}
		return levels[i].Distance < levels[j].Distance
	})

	return levels
}

func calcRecencyWeight(total, index int) float64 {
	if total <= 1 {
		return 1
	}
	if index < 0 {
		index = 0
	}
	if index >= total {
		index = total - 1
	}
	normalized := float64(index) / float64(total-1)
	return 0.5 + 0.5*normalized
}

func mergeNearbyLevels(levels []SupportResistanceLevel, tolerance float64) []SupportResistanceLevel {
	if len(levels) == 0 {
		return levels
	}

	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Price < levels[j].Price
	})

	merged := make([]SupportResistanceLevel, 0, len(levels))
	current := levels[0]
	current.Distance = 0

	for i := 1; i < len(levels); i++ {
		denom := (current.Price + levels[i].Price) / 2
		if denom == 0 {
			denom = 1
		}
		if math.Abs(levels[i].Price-current.Price)/denom <= tolerance {
			totalScore := current.Score + levels[i].Score
			if totalScore <= 0 {
				totalScore = float64(current.Strength + levels[i].Strength)
				if totalScore == 0 {
					totalScore = 1
				}
			}
			current.Price = (current.Price*current.Score + levels[i].Price*levels[i].Score) / totalScore
			current.Strength += levels[i].Strength
			current.Score += levels[i].Score
		} else {
			merged = append(merged, current)
			current = levels[i]
			current.Distance = 0
		}
	}

	merged = append(merged, current)

	return merged
}

func updateLevelDistances(levels []SupportResistanceLevel, currentPrice float64, isSupport bool) {
	if currentPrice <= 0 {
		return
	}

	for i := range levels {
		if isSupport {
			levels[i].Distance = math.Abs((currentPrice - levels[i].Price) / currentPrice * 100)
		} else {
			levels[i].Distance = math.Abs((levels[i].Price - currentPrice) / currentPrice * 100)
		}
	}
}

func findConfluenceLevels(base []SupportResistanceLevel, sets []weightedLevelSet, tolerance float64, currentPrice float64, isSupport bool) []SupportResistanceLevel {
	if len(base) == 0 || len(sets) == 0 {
		return []SupportResistanceLevel{}
	}

	result := make([]SupportResistanceLevel, 0)
	const baseWeight = 0.5

	for _, level := range base {
		combined := level
		combined.Score *= baseWeight
		combined.Strength = int(math.Round(float64(combined.Strength) * baseWeight))

		matched := false
		for _, set := range sets {
			for _, other := range set.levels {
				denom := (level.Price + other.Price) / 2
				if denom == 0 {
					continue
				}
				if math.Abs(level.Price-other.Price)/denom <= tolerance {
					matched = true
					combined.Score += other.Score * set.weight
					combined.Strength += int(math.Round(float64(other.Strength) * set.weight))
					break
				}
			}
			if matched {
				break
			}
		}

		if matched {
			result = append(result, combined)
		}
	}

	if len(result) == 0 {
		return result
	}

	updateLevelDistances(result, currentPrice, isSupport)
	result = sortAndFilterLevels(result, isSupport)

	return result
}
