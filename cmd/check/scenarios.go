package main

import (
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
	if err := seedDefault(c); err != nil {
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
	}, 201, `"inspection-000001"`, `"mite_load"`, `"temper_risk"`, `"watchLevel":"watch"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/inspections", nil, 200, `"note":"西侧巢门观察"`); err != nil {
		return err
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
	return nil
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
	report, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-12T00:00:00Z", nil, 200,
		`"hiveCount":3`, `"colonized":2`, `"production":1`, `"layingQueens":0`)
	if err != nil {
		return err
	}
	for _, fragment := range []string{
		`"needsInspection":["hive-02","hive-03"]`,
		`"concernHives":["hive-01"]`,
		`"escalatedHives":[]`,
	} {
		if !strings.Contains(report.text(), fragment) {
			return fmt.Errorf("状态汇总缺少 %q：%s", fragment, report.text())
		}
	}
	if err := report.mustOrder(`"hiveId":"hive-01"`, `"hiveId":"hive-02"`); err != nil {
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

// checkInspectionWatchTrend 校验螨计数连续上升派生趋势并提升关注级别的完整链路：
// 单次中等螨计数不升级，连续上升达到条件后升级并对协调人可见；
// 重复或并发提交不打断趋势，补录更早的巡检不抹掉已经发生的升级。
func checkInspectionWatchTrend(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	inspect := func(observedAt string, miteCount int) map[string]any {
		return map[string]any{
			"observedAt": observedAt, "colonyStrength": 4, "broodPattern": "solid",
			"temper": "calm", "miteCount": miteCount, "note": "",
		}
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections",
		inspect("2026-04-10T09:00:00Z", 61), 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	// 单次中等螨计数：无关注项，未形成趋势。
	if _, err := c.expect("POST", "/hives/hive-01/inspections",
		inspect("2026-04-10T09:00:00Z", 4), 201, `"watchLevel":"none"`, `"trends":[]`); err != nil {
		return err
	}
	// 连续第二次上升：趋势出现，关注级别提升为 watch。
	if _, err := c.expect("POST", "/hives/hive-01/inspections",
		inspect("2026-04-11T09:00:00Z", 5), 201, `"watchLevel":"watch"`, `"mite_rising"`); err != nil {
		return err
	}
	// 连续第三次上升且螨计数处于中等区间：升级为 escalated。
	if _, err := c.expect("POST", "/hives/hive-01/inspections",
		inspect("2026-04-12T09:00:00Z", 6), 201, `"watchLevel":"escalated"`); err != nil {
		return err
	}
	// 并发重复提交同一次观察：趋势不断裂、不重复计算，仍为 escalated。
	var wg sync.WaitGroup
	failures := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.expect("POST", "/hives/hive-01/inspections",
				inspect("2026-04-12T09:00:00Z", 6), 201, `"watchLevel":"escalated"`); err != nil {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			return err
		}
	}
	// 状态汇总与蜂箱趋势一致：升级后的蜂箱对协调人可见。
	if _, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-13T00:00:00Z", nil, 200,
		`"escalatedHives":["hive-01"]`, `"concernHives":["hive-01"]`,
		`"watchLevel":"escalated"`, `"mite_rising"`); err != nil {
		return err
	}
	// 补录更早的观察：该记录自身按当时已知事实派生，不抹掉已经发生的升级。
	if _, err := c.expect("POST", "/hives/hive-01/inspections",
		inspect("2026-04-08T09:00:00Z", 2), 201, `"watchLevel":"none"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-13T00:00:00Z", nil, 200,
		`"escalatedHives":["hive-01"]`, `"concernHives":["hive-01"]`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-missing/inspections",
		inspect("2026-04-12T09:00:00Z", 6), 404, `"code":"not_found"`); err != nil {
		return err
	}
	return nil
}
