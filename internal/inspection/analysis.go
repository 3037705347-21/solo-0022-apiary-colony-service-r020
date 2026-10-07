package inspection

import (
	"sort"

	"example.com/apiary-colony-service/internal/domain"
)

const (
	ConcernMiteLoad    = "mite_load"
	ConcernBroodGap    = "brood_gap"
	ConcernTemperRisk  = "temper_risk"
	ConcernLowStrength = "low_strength"
)

// TrendMiteRising 表示螨计数在连续多次巡检中持续上升。
const TrendMiteRising = "mite_rising"

const (
	MiteLoadThreshold  = 9
	LowStrengthCeiling = 2
)

const (
	// MiteModerateFloor 是中等螨计数下限：低于 mite_load 阈值、但已值得跟踪的螨计数水平。
	MiteModerateFloor = 4
	// MiteRiseWatchStreak 是趋势进入关注所需的最小连续上升次数（含本次巡检）。
	MiteRiseWatchStreak = 2
	// MiteRiseEscalationStreak 是触发升级所需的最小连续上升次数（含本次巡检）。
	MiteRiseEscalationStreak = 3
)

// DeriveConcerns 按固定阈值从一次巡检派生出关注项，顺序稳定。
func DeriveConcerns(record domain.Inspection) []string {
	concerns := make([]string, 0, 4)
	if record.MiteCount >= MiteLoadThreshold {
		concerns = append(concerns, ConcernMiteLoad)
	}
	if record.BroodPattern == domain.BroodNone {
		concerns = append(concerns, ConcernBroodGap)
	}
	if record.Temper == domain.TemperAggressive {
		concerns = append(concerns, ConcernTemperRisk)
	}
	if record.ColonyStrength <= LowStrengthCeiling {
		concerns = append(concerns, ConcernLowStrength)
	}
	return concerns
}

// MiteRiseStreak 计算截至 current 观察时刻螨计数连续严格递增的巡检次数（含 current）。
// 只依据观察时间不晚于 current 的已保存记录，因此补录更早的观察不会改写本次趋势；
// 相同观察时刻的重复记录只计一次（取保存较后者），重复提交不会打断上升趋势，
// 也不会被重复计数。
func MiteRiseStreak(history []domain.Inspection, current domain.Inspection) int {
	points := make([]domain.Inspection, 0, len(history)+1)
	for _, item := range history {
		if item.ObservedAt.After(current.ObservedAt) {
			continue
		}
		points = append(points, item)
	}
	points = append(points, current)
	sort.SliceStable(points, func(i, j int) bool {
		if points[i].ObservedAt.Equal(points[j].ObservedAt) {
			return points[i].ID < points[j].ID
		}
		return points[i].ObservedAt.Before(points[j].ObservedAt)
	})
	deduped := make([]domain.Inspection, 0, len(points))
	for index, point := range points {
		if index+1 < len(points) && points[index+1].ObservedAt.Equal(point.ObservedAt) {
			continue
		}
		deduped = append(deduped, point)
	}
	streak := 1
	for index := len(deduped) - 1; index > 0; index-- {
		if deduped[index].MiteCount <= deduped[index-1].MiteCount {
			break
		}
		streak++
	}
	return streak
}

// DeriveTrends 按连续上升次数派生趋势标记，顺序稳定。
func DeriveTrends(streak int) []string {
	trends := make([]string, 0, 1)
	if streak >= MiteRiseWatchStreak {
		trends = append(trends, TrendMiteRising)
	}
	return trends
}

// DeriveWatchLevel 综合阈值关注项与螨计数上升趋势给出关注级别：
// 有关注项或螨计数开始连续上升时为 watch；连续上升达到升级次数且螨计数
// 已进入中等区间时提升为 escalated。
func DeriveWatchLevel(record domain.Inspection, streak int) domain.WatchLevel {
	level := domain.WatchNone
	if len(record.Concerns) > 0 || streak >= MiteRiseWatchStreak {
		level = domain.WatchRaised
	}
	if streak >= MiteRiseEscalationStreak && record.MiteCount >= MiteModerateFloor {
		level = domain.WatchEscalated
	}
	return level
}
