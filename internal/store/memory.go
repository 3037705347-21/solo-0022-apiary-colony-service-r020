package store

import (
	"fmt"
	"sync"
	"time"

	"example.com/apiary-colony-service/internal/domain"
)

// Memory 是并发安全的内存仓储。所有写入都在写锁内完成状态校验，
// 读取则通过 Snapshot 得到一致视图。
type Memory struct {
	mu sync.RWMutex

	apiaries    map[string]domain.Apiary
	hives       map[string]domain.Hive
	inspections map[string][]domain.Inspection
	queens      map[string][]domain.Queen
	trends      map[string]domain.HiveTrend
	escalations map[string][]domain.TrendEscalation

	inspectionSeq int
	escalationSeq int
	queenSeq      int
}

// InspectionRecord 是一次巡检提交在同一把写锁内固化出的全部结果，
// 保证调用方读到的巡检事实与趋势状态来自同一个提交时刻。
type InspectionRecord struct {
	Inspection domain.Inspection
	Trend      domain.HiveTrend
	Escalated  bool
	Escalation domain.TrendEscalation
}

func NewMemory() *Memory {
	return &Memory{
		apiaries:    make(map[string]domain.Apiary),
		hives:       make(map[string]domain.Hive),
		inspections: make(map[string][]domain.Inspection),
		queens:      make(map[string][]domain.Queen),
		trends:      make(map[string]domain.HiveTrend),
		escalations: make(map[string][]domain.TrendEscalation),
	}
}

func (m *Memory) CreateApiary(apiary domain.Apiary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apiaries[apiary.ID]; ok {
		return domain.Conflict("蜂场 %q 已存在", apiary.ID)
	}
	m.apiaries[apiary.ID] = apiary
	return nil
}

func (m *Memory) GetApiary(id string) (domain.Apiary, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	apiary, ok := m.apiaries[id]
	return apiary, ok
}

func (m *Memory) CreateHive(hive domain.Hive) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apiaries[hive.ApiaryID]; !ok {
		return domain.NotFound("蜂场", hive.ApiaryID)
	}
	if _, ok := m.hives[hive.ID]; ok {
		return domain.Conflict("蜂箱 %q 已存在", hive.ID)
	}
	m.hives[hive.ID] = hive
	return nil
}

func (m *Memory) GetHive(id string) (domain.Hive, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hive, ok := m.hives[id]
	return hive, ok
}

// AdvanceHive 在同一把写锁内完成状态读取、迁移校验与写入，避免并发迁移产生交错结果。
func (m *Memory) AdvanceHive(hiveID string, target domain.HiveState, _ time.Time) (domain.Transition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Transition{}, domain.NotFound("蜂箱", hiveID)
	}
	if hive.State == target {
		return domain.Transition{}, domain.InvalidState("蜂箱 %q 已经处于状态 %q", hiveID, target)
	}
	if !domain.CanTransitionHive(hive.State, target) {
		return domain.Transition{}, domain.InvalidState(
			"蜂箱 %q 不允许从 %q 迁移到 %q", hiveID, hive.State, target)
	}
	from := hive.State
	hive.State = target
	m.hives[hiveID] = hive
	return domain.Transition{HiveID: hiveID, From: from, To: target}, nil
}

// AddInspection 分配巡检编号、写入记录、推进蜂箱的最近巡检时间，
// 并在同一把锁内基于已经保存的巡检事实派生趋势与关注级别。
// 蜂箱存在与可接受巡检的判定与写入处于同一把锁，避免退役与巡检之间的竞态；
// 去重、趋势派生与升级追加也在锁内完成，使重复或并发提交既不会断裂趋势，
// 也不会重复计入升级。
func (m *Memory) AddInspection(hiveID string, inspection domain.Inspection, now time.Time) (InspectionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return InspectionRecord{}, domain.NotFound("蜂箱", hiveID)
	}
	if !hive.AcceptsInspection() {
		return InspectionRecord{}, domain.InvalidState("蜂箱 %q 处于 %q 状态，不接受巡检", hiveID, hive.State)
	}
	if m.isDuplicateInspectionLocked(hiveID, inspection) {
		return InspectionRecord{}, domain.Conflict("蜂箱 %q 已存在相同观察时间与量测结果的巡检", hiveID)
	}

	m.inspectionSeq++
	inspection.ID = fmt.Sprintf("inspection-%06d", m.inspectionSeq)
	inspection.HiveID = hiveID
	m.inspections[hiveID] = append(m.inspections[hiveID], inspection)

	observedAt := inspection.ObservedAt
	if hive.LastInspectionAt == nil || observedAt.After(*hive.LastInspectionAt) {
		hive.LastInspectionAt = &observedAt
	}
	m.hives[hiveID] = hive

	items := m.inspections[hiveID]
	miteTrend := domain.DeriveMiteTrend(items)
	latest, _ := latestInspectionLocked(items)

	trend := m.trends[hiveID]
	trend.HiveID = hiveID
	trend.Mite = miteTrend
	record := InspectionRecord{Inspection: inspection, Trend: trend}

	// 升级事实只追加：已经升级过的蜂箱不再重复计入，补录较早巡检也无法改写它。
	if trend.EscalationCount == 0 && domain.EscalatesForMite(miteTrend) {
		m.escalationSeq++
		escalatedAt := now.UTC()
		escalation := domain.TrendEscalation{
			ID:                fmt.Sprintf("escalation-%06d", m.escalationSeq),
			HiveID:            hiveID,
			Metric:            domain.MetricMiteLoad,
			TriggerInspection: latest.ID,
			ObservedAt:        latest.ObservedAt,
			EscalatedAt:       escalatedAt,
		}
		m.escalations[hiveID] = append(m.escalations[hiveID], escalation)

		firstAt := escalation.ObservedAt
		lastAt := escalation.EscalatedAt
		trend.EscalationCount++
		trend.EscalationMetric = escalation.Metric
		trend.FirstEscalatedAt = &firstAt
		trend.LastEscalationID = escalation.ID
		trend.LastEscalationAt = &lastAt
		record.Escalated = true
		record.Escalation = escalation
	}

	// 关注级别直接消费上面同一份趋势事实，确保「趋势」与「汇总关注级别」永远一致。
	trend.Attention = domain.ResolveAttention(latest.Concerns, trend.EscalationCount > 0)
	m.trends[hiveID] = trend
	record.Trend = trend
	return record, nil
}

// isDuplicateInspectionLocked 判定同一蜂箱是否已存在观察时间与全部量测、
// 备注完全一致的巡检，防止重复提交让连续上升趋势出现伪边。
func (m *Memory) isDuplicateInspectionLocked(hiveID string, candidate domain.Inspection) bool {
	for _, existing := range m.inspections[hiveID] {
		if existing.ObservedAt.Equal(candidate.ObservedAt) &&
			existing.ColonyStrength == candidate.ColonyStrength &&
			existing.BroodPattern == candidate.BroodPattern &&
			existing.Temper == candidate.Temper &&
			existing.MiteCount == candidate.MiteCount &&
			existing.Note == candidate.Note {
			return true
		}
	}
	return false
}

func latestInspectionLocked(items []domain.Inspection) (domain.Inspection, bool) {
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

// AddQueen 关闭原有产卵蜂王并写入新蜂王，两件事在同一把锁内完成，
// 保证任意时刻同一蜂箱至多只有一只 laying 蜂王。
func (m *Memory) AddQueen(
	hiveID string,
	queen domain.Queen,
	closeQueenID string,
	closeState domain.QueenState,
	now time.Time,
) (domain.Queen, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Queen{}, domain.NotFound("蜂箱", hiveID)
	}
	if !hive.AcceptsQueenChange() {
		return domain.Queen{}, domain.InvalidState("蜂箱 %q 处于 %q 状态，不接受换王", hiveID, hive.State)
	}
	if closeQueenID != "" {
		if err := m.closeQueenLocked(hiveID, closeQueenID, closeState, now); err != nil {
			return domain.Queen{}, err
		}
	}
	m.queenSeq++
	queen.ID = fmt.Sprintf("queen-%06d", m.queenSeq)
	queen.HiveID = hiveID
	queen.State = domain.QueenLaying
	m.queens[hiveID] = append(m.queens[hiveID], queen)

	hive.ActiveQueenID = queen.ID
	m.hives[hiveID] = hive
	return queen, nil
}

func (m *Memory) closeQueenLocked(hiveID, queenID string, state domain.QueenState, now time.Time) error {
	items := m.queens[hiveID]
	for index, item := range items {
		if item.ID != queenID {
			continue
		}
		if !item.IsLaying() {
			return domain.Conflict("蜂王 %q 已经关闭", queenID)
		}
		closedAt := now.UTC()
		item.State = state
		item.ClosedAt = &closedAt
		items[index] = item
		m.queens[hiveID] = items
		return nil
	}
	return domain.Storage("蜂箱 %q 上的蜂王 %q 记录缺失", hiveID, queenID)
}

func (m *Memory) ListInspections(hiveID string) ([]domain.Inspection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.inspections[hiveID]
	out := make([]domain.Inspection, len(items))
	copy(out, items)
	return out, nil
}

func (m *Memory) ListQueens(hiveID string) ([]domain.Queen, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.queens[hiveID]
	out := make([]domain.Queen, len(items))
	copy(out, items)
	return out, nil
}

// Snapshot 返回深拷贝的一致视图，供汇总类读取使用。
func (m *Memory) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := Snapshot{
		Apiaries:          make(map[string]domain.Apiary, len(m.apiaries)),
		Hives:             make(map[string]domain.Hive, len(m.hives)),
		InspectionsByHive: make(map[string][]domain.Inspection, len(m.inspections)),
		QueensByHive:      make(map[string][]domain.Queen, len(m.queens)),
		TrendsByHive:      make(map[string]domain.HiveTrend, len(m.trends)),
		EscalationsByHive: make(map[string][]domain.TrendEscalation, len(m.escalations)),
	}
	for id, apiary := range m.apiaries {
		snapshot.Apiaries[id] = apiary
	}
	for id, hive := range m.hives {
		snapshot.Hives[id] = cloneHive(hive)
	}
	for id, items := range m.inspections {
		copied := make([]domain.Inspection, len(items))
		copy(copied, items)
		for index := range copied {
			copied[index].Concerns = append([]string{}, items[index].Concerns...)
		}
		snapshot.InspectionsByHive[id] = copied
	}
	for id, items := range m.queens {
		copied := make([]domain.Queen, len(items))
		copy(copied, items)
		for index := range copied {
			if items[index].ClosedAt != nil {
				closedAt := *items[index].ClosedAt
				copied[index].ClosedAt = &closedAt
			}
		}
		snapshot.QueensByHive[id] = copied
	}
	for id, trend := range m.trends {
		snapshot.TrendsByHive[id] = cloneTrend(trend)
	}
	for id, items := range m.escalations {
		copied := make([]domain.TrendEscalation, len(items))
		copy(copied, items)
		snapshot.EscalationsByHive[id] = copied
	}
	return snapshot
}

func cloneHive(hive domain.Hive) domain.Hive {
	if hive.LastInspectionAt != nil {
		observedAt := *hive.LastInspectionAt
		hive.LastInspectionAt = &observedAt
	}
	return hive
}

func cloneTrend(trend domain.HiveTrend) domain.HiveTrend {
	if trend.FirstEscalatedAt != nil {
		firstAt := *trend.FirstEscalatedAt
		trend.FirstEscalatedAt = &firstAt
	}
	if trend.LastEscalationAt != nil {
		lastAt := *trend.LastEscalationAt
		trend.LastEscalationAt = &lastAt
	}
	return trend
}
