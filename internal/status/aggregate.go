package status

import (
	"sort"
	"time"

	"example.com/apiary-colony-service/internal/domain"
	"example.com/apiary-colony-service/internal/store"
)

const InspectionWindow = 14 * 24 * time.Hour

type HiveSummary struct {
	HiveID           string                `json:"hiveId"`
	State            string                `json:"state"`
	LastInspectionAt *time.Time            `json:"lastInspectionAt"`
	LatestConcerns   []string              `json:"latestConcerns"`
	QueenID          string                `json:"queenId"`
	QueenMarkColor   string                `json:"queenMarkColor"`
	AttentionLevel   domain.AttentionLevel `json:"attentionLevel"`
	Trend            domain.HiveTrend      `json:"trend"`
}

type Report struct {
	ApiaryID        string         `json:"apiaryId"`
	Name            string         `json:"name"`
	AsOf            time.Time      `json:"asOf"`
	HiveCount       int            `json:"hiveCount"`
	StateCounts     map[string]int `json:"stateCounts"`
	NeedsInspection []string       `json:"needsInspection"`
	ConcernHives    []string       `json:"concernHives"`
	EscalatedHives  []string       `json:"escalatedHives"`
	LayingQueens    int            `json:"layingQueens"`
	Hives           []HiveSummary  `json:"hives"`
}

// BuildReport 在单份快照上完成聚合，保证计数与逐蜂箱摘要来自同一视图。
// 每个蜂箱的关注级别直接读取提交巡检时固化的同一份趋势状态，
// 因此「趋势」与「汇总中的关注级别」不可能出现分叉。
func BuildReport(snapshot store.Snapshot, apiary domain.Apiary, asOf time.Time) Report {
	asOf = asOf.UTC()
	report := Report{
		ApiaryID:        apiary.ID,
		Name:            apiary.Name,
		AsOf:            asOf,
		StateCounts:     initialStateCounts(),
		NeedsInspection: []string{},
		ConcernHives:    []string{},
		EscalatedHives:  []string{},
		Hives:           []HiveSummary{},
	}
	hives := snapshot.ApiaryHives(apiary.ID)
	report.HiveCount = len(hives)
	for _, hive := range hives {
		report.StateCounts[string(hive.State)]++
		summary := HiveSummary{
			HiveID:           hive.ID,
			State:            string(hive.State),
			LastInspectionAt: hive.LastInspectionAt,
			LatestConcerns:   []string{},
			AttentionLevel:   domain.AttentionNormal,
		}
		summary.Trend = domain.HiveTrend{
			HiveID:    hive.ID,
			Mite:      domain.MiteTrend{Direction: domain.MiteTrendUnknown},
			Attention: domain.AttentionNormal,
		}
		if queen, ok := snapshot.LayingQueen(hive.ID); ok {
			summary.QueenID = queen.ID
			summary.QueenMarkColor = queen.MarkColor
			report.LayingQueens++
		}
		if latest, ok := snapshot.LatestInspection(hive.ID); ok {
			observedAt := latest.ObservedAt
			summary.LastInspectionAt = &observedAt
			if len(latest.Concerns) > 0 {
				summary.LatestConcerns = append([]string{}, latest.Concerns...)
				report.ConcernHives = append(report.ConcernHives, hive.ID)
			}
		}
		if trend, ok := snapshot.HiveTrend(hive.ID); ok {
			summary.Trend = trend
			summary.AttentionLevel = trend.Attention
			if trend.Attention == domain.AttentionEscalated {
				report.EscalatedHives = append(report.EscalatedHives, hive.ID)
			}
		}
		if NeedsInspection(hive, asOf) {
			report.NeedsInspection = append(report.NeedsInspection, hive.ID)
		}
		report.Hives = append(report.Hives, summary)
	}
	sort.Strings(report.NeedsInspection)
	sort.Strings(report.ConcernHives)
	sort.Strings(report.EscalatedHives)
	return report
}

// NeedsInspection 判定蜂箱是否需要巡检：从未巡检，或距 asOf 超过巡检窗口。
func NeedsInspection(hive domain.Hive, asOf time.Time) bool {
	if hive.LastInspectionAt == nil {
		return true
	}
	return asOf.Sub(hive.LastInspectionAt.UTC()) > InspectionWindow
}

func initialStateCounts() map[string]int {
	return map[string]int{
		string(domain.HiveColonized):  0,
		string(domain.HiveProduction): 0,
		string(domain.HiveWintering):  0,
		string(domain.HiveRetired):    0,
	}
}
