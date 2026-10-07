package inspection

import (
	"time"

	"example.com/apiary-colony-service/internal/domain"
	"example.com/apiary-colony-service/internal/store"
)

// Service 接收巡检、完成范围校验并派生关注项与螨计数趋势。
type Service struct {
	store *store.Memory
	clock func() time.Time
}

func New(memory *store.Memory, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: memory, clock: clock}
}

type RecordInput struct {
	ObservedAt     time.Time
	ColonyStrength int
	BroodPattern   string
	Temper         string
	MiteCount      int
	Note           string
}

// RecordResult 是一次巡检提交后可观察到的全部结果：
// 巡检事实、与汇总一致的趋势状态，以及本次是否触发了关注级别升级。
type RecordResult struct {
	Inspection domain.Inspection
	Trend      domain.HiveTrend
	Escalated  bool
	Escalation domain.TrendEscalation
}

func (s *Service) Record(hiveID string, input RecordInput) (RecordResult, error) {
	observedAt := input.ObservedAt
	if observedAt.IsZero() {
		observedAt = s.clock()
	}
	record, err := domain.NewInspection(
		hiveID,
		observedAt,
		input.ColonyStrength,
		domain.BroodPattern(domain.TrimText(input.BroodPattern)),
		domain.Temper(domain.TrimText(input.Temper)),
		input.MiteCount,
		input.Note,
	)
	if err != nil {
		return RecordResult{}, err
	}
	record.Concerns = DeriveConcerns(record)
	saved, err := s.store.AddInspection(hiveID, record, s.clock())
	if err != nil {
		return RecordResult{}, err
	}
	return RecordResult{
		Inspection: saved.Inspection,
		Trend:      saved.Trend,
		Escalated:  saved.Escalated,
		Escalation: saved.Escalation,
	}, nil
}

func (s *Service) List(hiveID string) ([]domain.Inspection, error) {
	return s.store.ListInspections(hiveID)
}
