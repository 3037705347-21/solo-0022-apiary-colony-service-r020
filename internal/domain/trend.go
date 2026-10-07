package domain

import "time"

// AttentionLevel 是协调人视角的关注级别，随趋势只升不降。
type AttentionLevel string

const (
	AttentionNormal    AttentionLevel = "normal"
	AttentionConcern   AttentionLevel = "concern"
	AttentionEscalated AttentionLevel = "escalated"
)

// MiteTrendDirection 描述最近一次巡检相对上一次的螨计数走向。
type MiteTrendDirection string

const (
	MiteTrendFlat    MiteTrendDirection = "flat"
	MiteTrendRising  MiteTrendDirection = "rising"
	MiteTrendFalling MiteTrendDirection = "falling"
	MiteTrendUnknown MiteTrendDirection = "unknown"
)

// MiteTrendConsecutiveRises 是触发升级所需的连续上升段长度，
// 3、5、7 三次巡检逐次严格升高时，连续上升段为 2，达到升级条件。
const MiteTrendConsecutiveRises = 2

// MetricMiteLoad 是螨计数趋势升级所对应的指标名，与关注项 mite_load 对齐。
const MetricMiteLoad = "mite_load"

// MiteTrend 是同一蜂箱全部已保存巡检事实在当前提交时刻的螨计数趋势视图。
type MiteTrend struct {
	Direction        MiteTrendDirection `json:"direction"`
	ConsecutiveRises int                `json:"consecutiveRises"`
	LatestMiteCount  int                `json:"latestMiteCount"`
}

// TrendEscalation 是一次趋势升级的不可变事实，只能追加、不能改写或删除。
type TrendEscalation struct {
	ID                string    `json:"id"`
	HiveID            string    `json:"hiveId"`
	Metric            string    `json:"metric"`
	TriggerInspection string    `json:"triggerInspectionId"`
	ObservedAt        time.Time `json:"observedAt"`
	EscalatedAt       time.Time `json:"escalatedAt"`
}

// HiveTrend 是随提交巡检固化下来的蜂箱趋势状态，与状态汇总读取同一份存储。
type HiveTrend struct {
	HiveID           string         `json:"hiveId"`
	Mite             MiteTrend      `json:"mite"`
	Attention        AttentionLevel `json:"attention"`
	EscalationCount  int            `json:"escalationCount"`
	EscalationMetric string         `json:"escalationMetric,omitempty"`
	FirstEscalatedAt *time.Time     `json:"firstEscalatedAt,omitempty"`
	LastEscalationID string         `json:"lastEscalationId,omitempty"`
	LastEscalationAt *time.Time     `json:"lastEscalatedAt,omitempty"`
}

// SortedInspectionsByObservedAt 返回按观察时间、再按编号稳定排序的巡检副本，
// 趋势派生始终基于这一全序，补录的较早巡检只会落到它应有的时间位置。
func SortedInspectionsByObservedAt(inspections []Inspection) []Inspection {
	sorted := append([]Inspection{}, inspections...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && inspectionBefore(sorted[j], sorted[j-1]); j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	return sorted
}

func inspectionBefore(a, b Inspection) bool {
	if !a.ObservedAt.Equal(b.ObservedAt) {
		return a.ObservedAt.Before(b.ObservedAt)
	}
	return a.ID < b.ID
}

// DeriveMiteTrend 依据已保存巡检事实派生螨计数趋势：
// 方向取末次与前次的比较，连续上升段从序列尾部向前累计严格升高的边数。
func DeriveMiteTrend(inspections []Inspection) MiteTrend {
	sorted := SortedInspectionsByObservedAt(inspections)
	if len(sorted) == 0 {
		return MiteTrend{Direction: MiteTrendUnknown}
	}
	latest := sorted[len(sorted)-1]
	trend := MiteTrend{Direction: MiteTrendUnknown, LatestMiteCount: latest.MiteCount}
	if len(sorted) == 1 {
		trend.Direction = MiteTrendFlat
		return trend
	}
	previous := sorted[len(sorted)-2].MiteCount
	switch {
	case latest.MiteCount > previous:
		trend.Direction = MiteTrendRising
	case latest.MiteCount < previous:
		trend.Direction = MiteTrendFalling
	default:
		trend.Direction = MiteTrendFlat
	}
	for index := len(sorted) - 1; index > 0; index-- {
		if sorted[index].MiteCount <= sorted[index-1].MiteCount {
			break
		}
		trend.ConsecutiveRises++
	}
	return trend
}

// EscalatesForMite 判断当前趋势是否首次满足螨计数升级条件。
func EscalatesForMite(trend MiteTrend) bool {
	return trend.Direction == MiteTrendRising &&
		trend.ConsecutiveRises >= MiteTrendConsecutiveRises
}

// ResolveAttention 按「最近一次巡检关注项」与「是否已升级」派生关注级别；
// 升级一旦发生即为最高级，后续巡检与补录都不能让它回落。
func ResolveAttention(latestConcerns []string, alreadyEscalated bool) AttentionLevel {
	if alreadyEscalated {
		return AttentionEscalated
	}
	if len(latestConcerns) > 0 {
		return AttentionConcern
	}
	return AttentionNormal
}
