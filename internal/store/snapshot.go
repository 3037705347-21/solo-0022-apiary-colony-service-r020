package store

import (
	"sort"

	"example.com/apiary-colony-service/internal/domain"
)

// Snapshot 是一次一致性读取的结果，供聚合与校验使用。
type Snapshot struct {
	Apiaries          map[string]domain.Apiary
	Hives             map[string]domain.Hive
	InspectionsByHive map[string][]domain.Inspection
	QueensByHive      map[string][]domain.Queen
	TrendsByHive      map[string]domain.HiveTrend
	EscalationsByHive map[string][]domain.TrendEscalation
}

// ApiaryHives 返回蜂场下的蜂箱，按编号升序稳定排序。
func (s Snapshot) ApiaryHives(apiaryID string) []domain.Hive {
	hives := make([]domain.Hive, 0, len(s.Hives))
	for _, hive := range s.Hives {
		if hive.ApiaryID == apiaryID {
			hives = append(hives, hive)
		}
	}
	sort.Slice(hives, func(i, j int) bool { return hives[i].ID < hives[j].ID })
	return hives
}

// LatestInspection 返回蜂箱最近一次巡检；观察时间相同时按编号取较大者。
func (s Snapshot) LatestInspection(hiveID string) (domain.Inspection, bool) {
	items := s.InspectionsByHive[hiveID]
	if len(items) == 0 {
		return domain.Inspection{}, false
	}
	latest := items[0]
	for _, item := range items[1:] {
		if item.ObservedAt.After(latest.ObservedAt) {
			latest = item
			continue
		}
		if item.ObservedAt.Equal(latest.ObservedAt) && item.ID > latest.ID {
			latest = item
		}
	}
	return latest, true
}

// LayingQueen 返回蜂箱当前处于产卵状态的蜂王。
func (s Snapshot) LayingQueen(hiveID string) (domain.Queen, bool) {
	for _, item := range s.QueensByHive[hiveID] {
		if item.IsLaying() {
			return item, true
		}
	}
	return domain.Queen{}, false
}

func (s Snapshot) Queens(hiveID string) []domain.Queen {
	items := s.QueensByHive[hiveID]
	out := make([]domain.Queen, len(items))
	copy(out, items)
	return out
}

// HiveTrend 返回提交巡检时固化的趋势状态；没有巡检的蜂箱得到 unknown 空趋势。
// 汇总与提交响应都经由这里读取，保证两侧看到的趋势与关注级别一致。
func (s Snapshot) HiveTrend(hiveID string) (domain.HiveTrend, bool) {
	trend, ok := s.TrendsByHive[hiveID]
	if !ok && len(s.InspectionsByHive[hiveID]) == 0 {
		return domain.HiveTrend{HiveID: hiveID, Attention: domain.AttentionNormal}, false
	}
	if !ok {
		latest, _ := s.LatestInspection(hiveID)
		trend = domain.HiveTrend{
			HiveID:    hiveID,
			Mite:      domain.DeriveMiteTrend(s.InspectionsByHive[hiveID]),
			Attention: domain.ResolveAttention(latest.Concerns, false),
		}
	}
	return trend, true
}

// Escalations 返回蜂箱已发生的趋势升级，顺序与追加顺序一致。
func (s Snapshot) Escalations(hiveID string) []domain.TrendEscalation {
	items := s.EscalationsByHive[hiveID]
	out := make([]domain.TrendEscalation, len(items))
	copy(out, items)
	return out
}
