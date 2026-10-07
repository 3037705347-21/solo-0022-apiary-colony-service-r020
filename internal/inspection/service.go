package inspection

import (
	"time"

	"example.com/apiary-colony-service/internal/domain"
	"example.com/apiary-colony-service/internal/store"
)

// Service 接收巡检、完成范围校验并派生关注项。
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

func (s *Service) Record(hiveID string, input RecordInput) (domain.Inspection, error) {
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
		return domain.Inspection{}, err
	}
	record.Concerns = DeriveConcerns(record)
	saved, err := s.store.AddInspection(hiveID, record, deriveTrend)
	if err != nil {
		return domain.Inspection{}, err
	}
	return saved, nil
}

// deriveTrend 在仓储写锁内基于该蜂箱已保存的巡检历史派生趋势与关注级别，
// 派生结果随记录一起固化，后续补录不会追溯改写已保存的记录。
func deriveTrend(history []domain.Inspection, current domain.Inspection) domain.Inspection {
	streak := MiteRiseStreak(history, current)
	current.Trends = DeriveTrends(streak)
	current.WatchLevel = DeriveWatchLevel(current, streak)
	return current
}

func (s *Service) List(hiveID string) ([]domain.Inspection, error) {
	return s.store.ListInspections(hiveID)
}
