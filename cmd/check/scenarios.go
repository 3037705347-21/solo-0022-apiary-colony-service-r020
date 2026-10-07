package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

const (
	demoApiaryID = "apiary-north"
	demoHiveID   = "hive-01"
)

func seedApiary(c *client, id, name, district, keeper string) error {
	_, err := c.expect("POST", "/apiaries", map[string]any{
		"id":       id,
		"name":     name,
		"district": district,
		"keeper":   keeper,
	}, 201, `"id":"`+id+`"`)
	return err
}

func seedHive(c *client, apiaryID, hiveID, model string) error {
	_, err := c.expect("POST", "/apiaries/"+apiaryID+"/hives", map[string]any{
		"id":    hiveID,
		"model": model,
	}, 201, `"state":"colonized"`)
	return err
}

func seedDefault(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	return seedHive(c, demoApiaryID, demoHiveID, "langstroth")
}

func checkRegisterApiary(c *client) error {
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-north", "name": "北岭蜂场", "district": "北岭片区", "keeper": "周岚",
	}, 201, `"id":"apiary-north"`, `"name":"北岭蜂场"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north", nil, 200,
		`"district":"北岭片区"`, `"keeper":"周岚"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-north", "name": "重复蜂场", "district": "北岭片区", "keeper": "周岚",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-south", "name": "", "district": "南坡片区", "keeper": "李澈",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

func checkHiveInstall(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-01", "model": "langstroth",
	}, 201, `"state":"colonized"`, `"apiaryId":"apiary-north"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"model":"langstroth"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-01", "model": "warre",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-missing/hives", map[string]any{
		"id": "hive-09", "model": "warre",
	}, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-02", "model": "dadant",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

func checkRecordInspection(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	if err := seedHive(c, demoApiaryID, demoHiveID, "langstroth"); err != nil {
		return err
	}
	if err := seedHive(c, demoApiaryID, "hive-02", "warre"); err != nil {
		return err
	}
	if err := seedHive(c, demoApiaryID, "hive-03", "warre"); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 9, "broodPattern": "patchy",
		"temper": "calm", "miteCount": 3, "note": "",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 3, "broodPattern": "patchy",
		"temper": "aggressive", "miteCount": 12, "note": "西侧巢门观察",
	}, 201, `"inspection-000001"`, `"mite_load"`, `"temper_risk"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/inspections", nil, 200, `"note":"西侧巢门观察"`); err != nil {
		return err
	}

	// hive-02：单次螨计数中等（< 9），但连续三次逐次上升，应在第三次升级为 escalated。
	for _, step := range []struct {
		time  string
		mite  int
		frags []string
	}{
		{"2026-04-03T09:00:00Z", 3, []string{`"consecutiveRises":0`, `"direction":"flat"`, `"attentionLevel":"normal"`, `"escalated":false`}},
		{"2026-04-06T09:00:00Z", 5, []string{`"consecutiveRises":1`, `"direction":"rising"`, `"attentionLevel":"normal"`, `"escalated":false`}},
		{"2026-04-09T09:00:00Z", 7, []string{
			`"consecutiveRises":2`, `"direction":"rising"`, `"attentionLevel":"escalated"`,
			`"escalated":true`, `"metric":"mite_load"`, `"triggerInspectionId":"inspection-000004"`,
		}},
	} {
		if _, err := c.expect("POST", "/hives/hive-02/inspections", map[string]any{
			"observedAt": step.time, "colonyStrength": 4, "broodPattern": "solid",
			"temper": "calm", "miteCount": step.mite, "note": "",
		}, 201, step.frags...); err != nil {
			return err
		}
	}

	// 第四次螨计数回落：趋势方向可以回落，但已发生的升级不能被追溯抹掉。
	if _, err := c.expect("POST", "/hives/hive-02/inspections", map[string]any{
		"observedAt": "2026-04-12T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 2, "note": "回落",
	}, 201,
		`"consecutiveRises":0`, `"direction":"falling"`, `"attentionLevel":"escalated"`,
		`"escalated":false`, `"escalationCount":1`,
	); err != nil {
		return err
	}

	// 补录一条更早的巡检：升级事实依旧保留，不重复、不抹除。
	if _, err := c.expect("POST", "/hives/hive-02/inspections", map[string]any{
		"observedAt": "2026-04-01T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 1, "note": "补录",
	}, 201,
		`"consecutiveRises":0`, `"direction":"falling"`, `"attentionLevel":"escalated"`,
		`"escalated":false`, `"escalationCount":1`,
	); err != nil {
		return err
	}

	// 完全重复提交（同一观察时间与全部量测）必须被拒绝，不能让趋势出现伪边或重复升级。
	if _, err := c.expect("POST", "/hives/hive-02/inspections", map[string]any{
		"observedAt": "2026-04-09T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 7, "note": "",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	listed, err := c.expect("GET", "/hives/hive-02/inspections", nil, 200, `"note":"补录"`)
	if err != nil {
		return err
	}
	if strings.Count(listed.text(), `"id":"inspection-`) != 5 {
		return fmt.Errorf("hive-02 应只有 5 条巡检（重复提交被拒）：%s", listed.text())
	}

	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-11T09:00:00Z", "colonyStrength": 3, "broodPattern": "solid",
		"temper": "calm", "miteCount": 1, "note": "",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}

	// 并发提交同一巡检：恰好一次成功、一次冲突，趋势与升级只计算一次。
	concurrent := map[string]any{
		"observedAt": "2026-04-15T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 4, "note": "并发",
	}
	statuses := concurrentPost(c, "hive-03", concurrent)
	var created, conflicts int
	for _, status := range statuses {
		switch status {
		case 201:
			created++
		case 409:
			conflicts++
		}
	}
	if created != 1 || conflicts != 1 {
		return fmt.Errorf("并发重复提交应恰好一次 201、一次 409，实际状态码 %v", statuses)
	}
	return nil
}

// concurrentPost 让两份完全相同的巡检提交并发到达，返回各自的 HTTP 状态码。
func concurrentPost(c *client, hiveID string, payload map[string]any) []int {
	path := "/hives/" + hiveID + "/inspections"
	statuses := make([]int, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for i := range 2 {
		go func(index int) {
			defer wait.Done()
			reply, err := c.call("POST", path, payload)
			if err != nil {
				statuses[index] = -1
				return
			}
			statuses[index] = reply.status
		}(i)
	}
	wait.Wait()
	return statuses
}

// apiaryStatusPayload 仅解析一致性断言需要的字段。
type apiaryStatusPayload struct {
	Hives []struct {
		HiveID         string `json:"hiveId"`
		AttentionLevel string `json:"attentionLevel"`
		Trend          struct {
			Attention        string `json:"attention"`
			ConsecutiveRises int    `json:"consecutiveRises"`
		} `json:"trend"`
	} `json:"hives"`
	EscalatedHives []string `json:"escalatedHives"`
	ConcernHives   []string `json:"concernHives"`
}

func checkReplaceQueen(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-04-02T08:00:00Z", "reason": "aging",
	}, 201, `"queen-000001"`, `"markColor":"white"`, `"state":"laying"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "cells", "introducedAt": "2026-04-20T08:00:00Z", "reason": "aggression",
	}, 201, `"queen-000002"`, `"markColor":"white"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/queens", nil, 200,
		`"queen-000001"`, `"superseded"`, `"queen-000002"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "imported", "introducedAt": "2026-05-01T08:00:00Z", "reason": "aging",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-05-02T08:00:00Z", "reason": "loss",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}
	return nil
}

func checkAdvanceHiveState(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 200,
		`"from":"colonized"`, `"to":"production"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "colonized"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-missing/state", map[string]any{"target": "production"}, 404,
		`"code":"not_found"`); err != nil {
		return err
	}
	return nil
}

func checkReadApiaryStatus(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	for _, hiveID := range []string{"hive-01", "hive-02", "hive-03"} {
		if err := seedHive(c, demoApiaryID, hiveID, "langstroth"); err != nil {
			return err
		}
	}
	if _, err := c.expect("POST", "/hives/hive-02/state", map[string]any{"target": "production"}, 200,
		`"to":"production"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 12, "note": "",
	}, 201, `"mite_load"`); err != nil {
		return err
	}
	// hive-03 单次螨计数都低于 9，但连续三次上升，第三次触发关注级别升级。
	trendSteps := []struct {
		mite  int
		frags []string
	}{
		{3, []string{`"consecutiveRises":0`, `"direction":"flat"`}},
		{5, []string{`"consecutiveRises":1`, `"direction":"rising"`}},
		{7, []string{
			`"consecutiveRises":2`, `"direction":"rising"`,
			`"attentionLevel":"escalated"`, `"escalated":true`,
		}},
	}
	for _, step := range trendSteps {
		observedAt := fmt.Sprintf("2026-04-%02dT09:00:00Z", 2+step.mite)
		if _, err := c.expect("POST", "/hives/hive-03/inspections", map[string]any{
			"observedAt": observedAt, "colonyStrength": 4, "broodPattern": "solid",
			"temper": "calm", "miteCount": step.mite, "note": "",
		}, 201, step.frags...); err != nil {
			return err
		}
	}
	report, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-12T00:00:00Z", nil, 200,
		`"hiveCount":3`, `"colonized":2`, `"production":1`, `"layingQueens":0`)
	if err != nil {
		return err
	}
	for _, fragment := range []string{
		`"needsInspection":["hive-02"]`,
		`"concernHives":["hive-01"]`,
		`"escalatedHives":["hive-03"]`,
	} {
		if !strings.Contains(report.text(), fragment) {
			return fmt.Errorf("状态汇总缺少 %q：%s", fragment, report.text())
		}
	}
	if err := assertStatusConsistency(report.body); err != nil {
		return err
	}
	if err := report.mustOrder(`"hiveId":"hive-01"`, `"hiveId":"hive-02"`); err != nil {
		return err
	}

	// 补录一条更早、螨计数很低的巡检：已升级事实不能被追溯抹掉，汇总仍应一致。
	if _, err := c.expect("POST", "/hives/hive-03/inspections", map[string]any{
		"observedAt": "2026-04-01T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 0, "note": "补录",
	}, 201, `"attentionLevel":"escalated"`, `"escalationCount":1`); err != nil {
		return err
	}
	report, err = c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-12T00:00:00Z", nil, 200,
		`"escalatedHives":["hive-03"]`)
	if err != nil {
		return err
	}
	if err := assertStatusConsistency(report.body); err != nil {
		return err
	}

	if _, err := c.expect("GET", "/apiaries/apiary-missing/status", nil, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=not-a-time", nil, 400,
		`"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

// assertStatusConsistency 校验同一蜂箱在状态汇总里的关注级别与其趋势状态完全一致，
// 且关注级别为 escalated 的蜂箱恰好出现在 escalatedHives 列表中。
func assertStatusConsistency(body []byte) error {
	var payload apiaryStatusPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("解析状态汇总失败：%w", err)
	}
	escalated := map[string]bool{}
	for _, id := range payload.EscalatedHives {
		escalated[id] = true
	}
	for _, hive := range payload.Hives {
		if hive.AttentionLevel != hive.Trend.Attention {
			return fmt.Errorf("蜂箱 %s 关注级别 %q 与趋势 %q 不一致", hive.HiveID, hive.AttentionLevel, hive.Trend.Attention)
		}
		if (hive.AttentionLevel == "escalated") != escalated[hive.HiveID] {
			return fmt.Errorf("蜂箱 %s 升级标记与 escalatedHives 列表不一致", hive.HiveID)
		}
	}
	return nil
}
