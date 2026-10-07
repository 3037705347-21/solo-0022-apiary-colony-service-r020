package httpapi

import (
	"net/http"

	"example.com/apiary-colony-service/internal/domain"
	"example.com/apiary-colony-service/internal/inspection"
)

type recordInspectionRequest struct {
	ObservedAt     string `json:"observedAt"`
	ColonyStrength int    `json:"colonyStrength"`
	BroodPattern   string `json:"broodPattern"`
	Temper         string `json:"temper"`
	MiteCount      int    `json:"miteCount"`
	Note           string `json:"note"`
}

// recordInspectionResponse 以平铺方式返回巡检事实与同一次提交派生的趋势，
// 既有巡检字段保持不变，新增 trend 与 attentionLevel 与状态汇总严格一致。
type recordInspectionResponse struct {
	domain.Inspection
	Trend          domain.HiveTrend        `json:"trend"`
	AttentionLevel domain.AttentionLevel   `json:"attentionLevel"`
	Escalated      bool                    `json:"escalated"`
	Escalation     *domain.TrendEscalation `json:"escalation,omitempty"`
}

func (s *Server) handleRecordInspection(w http.ResponseWriter, r *http.Request) {
	var request recordInspectionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err)
		return
	}
	observedAt, err := parseOptionalTime("observedAt", request.ObservedAt)
	if err != nil {
		writeError(w, err)
		return
	}
	input := inspection.RecordInput{
		ColonyStrength: request.ColonyStrength,
		BroodPattern:   request.BroodPattern,
		Temper:         request.Temper,
		MiteCount:      request.MiteCount,
		Note:           request.Note,
	}
	if observedAt != nil {
		input.ObservedAt = *observedAt
	}
	result, err := s.deps.Inspection.Record(r.PathValue("hiveId"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	response := recordInspectionResponse{
		Inspection:     result.Inspection,
		Trend:          result.Trend,
		AttentionLevel: result.Trend.Attention,
		Escalated:      result.Escalated,
	}
	if result.Escalated {
		escalation := result.Escalation
		response.Escalation = &escalation
	}
	writeJSON(w, http.StatusCreated, response)
}

func (s *Server) handleListInspections(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Inspection.List(r.PathValue("hiveId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if items == nil {
		items = []domain.Inspection{}
	}
	writeJSON(w, http.StatusOK, items)
}
