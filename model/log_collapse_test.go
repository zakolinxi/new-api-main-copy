package model

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func collapseTestLog(id int, createdAt int64, userId int, username string, modelName string, ip string, other string) *Log {
	return &Log{
		Id:               id,
		CreatedAt:        createdAt,
		Type:             LogTypeConsume,
		UserId:           userId,
		Username:         username,
		ModelName:        modelName,
		PromptTokens:     10,
		CompletionTokens: 5,
		Quota:            100,
		UseTime:          3,
		Ip:               ip,
		Other:            other,
	}
}

func TestCollapseLogsFoldsRepeatedCallsIntoNewestRow(t *testing.T) {
	const base = int64(1_700_000_000)
	logs := make([]*Log, 0, 11)
	for i := 0; i < 11; i++ {
		logs = append(logs, collapseTestLog(200-i, base-int64(i), 7, "alice", "gpt-4o", fmt.Sprintf("10.0.0.%d", i%2), `{"cache_tokens":1000,"cache_creation_tokens_5m":100,"frt":250}`))
	}

	folded := CollapseLogs(logs, LogCollapseThreshold)
	require.Len(t, folded, 1, "11 次调用应折叠成一行")

	row := folded[0]
	assert.Equal(t, LogCollapseThreshold+1, row.CollapsedCount)
	assert.Equal(t, "7|gpt-4o", row.CollapsedKey)
	assert.Equal(t, 110, row.PromptTokens)
	assert.Equal(t, 55, row.CompletionTokens)
	assert.Equal(t, 1100, row.Quota)
	assert.Equal(t, 2, row.CollapsedIpCount, "折叠行记录出现过的 IP 数量")
	assert.Equal(t, "10.0.0.0", row.Ip, "IP 取最新一条")
	assert.Equal(t, base, row.CreatedAt, "时间取最新一条")
	assert.Equal(t, 200, row.Id)

	// 折叠行必须能被现有的 Tokens 列正确渲染：缓存累加、首字为最新一次。
	assert.Contains(t, row.Other, `"cache_tokens":11000`)
	assert.Contains(t, row.Other, `"cache_creation_tokens_5m":1100`)
	assert.Contains(t, row.Other, `"frt":250`)
}

// 折叠行要保留计价字段：详情列用 model_ratio / completion_ratio 现算单价，
// 缺了它们整块会渲染成 $NaN（曾因折叠时只输出缓存字段而出现）。
func TestCollapseLogsKeepsPricingMetadataOfNewestCall(t *testing.T) {
	const base = int64(1_700_000_000)
	logs := make([]*Log, 0, 12)
	for i := 0; i < 12; i++ {
		logs = append(logs, collapseTestLog(
			300-i, base-int64(i), 9, "bob", "gpt-4o", "10.0.0.1",
			`{"model_ratio":1.25,"completion_ratio":4,"group_ratio":1,"cache_ratio":0.5,"cache_tokens":800,"frt":900}`,
		))
	}

	folded := CollapseLogs(logs, LogCollapseThreshold)
	require.Len(t, folded, 1)
	row := folded[0]

	assert.Contains(t, row.Other, `"model_ratio":1.25`)
	assert.Contains(t, row.Other, `"completion_ratio":4`)
	assert.Contains(t, row.Other, `"group_ratio":1`)
	assert.Contains(t, row.Other, `"cache_ratio":0.5`)
	// 可累加的字段仍然被覆盖成合计值，而不是最新一次的 800。
	assert.Contains(t, row.Other, `"cache_tokens":9600`)
}

func TestCollapseLogsKeepsGroupsAtOrBelowThreshold(t *testing.T) {
	const base = int64(1_700_000_000)
	logs := make([]*Log, 0, 10)
	for i := 0; i < LogCollapseThreshold; i++ {
		logs = append(logs, collapseTestLog(100-i, base-int64(i), 7, "alice", "gpt-4o", "10.0.0.1", ""))
	}

	folded := CollapseLogs(logs, LogCollapseThreshold)
	assert.Len(t, folded, LogCollapseThreshold, "只到阈值不折叠，第 11 次才开始折叠")
	for _, log := range folded {
		assert.Zero(t, log.CollapsedCount)
	}

	// 补上第 11 次后同一组立刻折叠。
	logs = append(logs, collapseTestLog(101, base-10, 7, "alice", "gpt-4o", "10.0.0.1", ""))
	assert.Len(t, CollapseLogs(logs, LogCollapseThreshold), 1)
}

func TestCollapseLogsSeparatesUsersAndModels(t *testing.T) {
	const base = int64(1_700_000_000)
	type spec struct {
		userId   int
		username string
		model    string
	}
	specs := make([]spec, 0, 16)
	for i := 0; i < 2; i++ {
		specs = append(specs, spec{8, "bob", "gpt-4o"})
	}
	for i := 0; i < 3; i++ {
		specs = append(specs, spec{7, "alice", "claude-3"})
	}
	for i := 0; i < 11; i++ {
		specs = append(specs, spec{7, "alice", "gpt-4o"})
	}
	logs := make([]*Log, 0, len(specs))
	for i, item := range specs {
		logs = append(logs, collapseTestLog(400-i, base-int64(i), item.userId, item.username, item.model, "10.0.0.1", ""))
	}

	folded := CollapseLogs(logs, LogCollapseThreshold)
	foldedGroups := make(map[string]*Log, len(folded))
	rawRows := make(map[string]int, len(folded))
	for _, log := range folded {
		if log.CollapsedCount > 0 {
			foldedGroups[log.CollapsedKey] = log
			continue
		}
		rawRows[fmt.Sprintf("%d|%s", log.UserId, log.ModelName)]++
	}
	require.Contains(t, foldedGroups, "7|gpt-4o")
	assert.Equal(t, 11, foldedGroups["7|gpt-4o"].CollapsedCount)
	assert.Equal(t, 1, foldedGroups["7|gpt-4o"].CollapsedIpCount)
	assert.NotContains(t, foldedGroups, "7|claude-3", "同一用户的另一个模型单独统计")
	assert.Equal(t, 3, rawRows["7|claude-3"])
	assert.Equal(t, 2, rawRows["8|gpt-4o"], "不同用户的同名模型不合并")
	assert.NotContains(t, foldedGroups, "8|gpt-4o")
}

func TestCollapseLogsKeepsFoldRowAtNewestPosition(t *testing.T) {
	const base = int64(1_700_000_000)
	logs := []*Log{
		{Id: 500, CreatedAt: base, Type: LogTypeTopup, UserId: 7, Username: "alice"},
		collapseTestLog(499, base-1, 7, "alice", "gpt-4o", "10.0.0.1", ""),
		{Id: 498, CreatedAt: base - 2, Type: LogTypeManage, UserId: 7, Username: "alice"},
	}
	for i := 2; i < 12; i++ {
		logs = append(logs, collapseTestLog(499-i, base-int64(i+1), 7, "alice", "gpt-4o", "10.0.0.1", ""))
	}

	folded := CollapseLogs(logs, LogCollapseThreshold)
	require.Len(t, folded, 3, "充值 + 管理 + 折叠行，非消费日志不参与折叠")
	assert.Equal(t, LogTypeTopup, folded[0].Type)
	assert.Equal(t, LogCollapseThreshold+1, folded[1].CollapsedCount, "折叠行占据该组最新一次调用的位置")
	assert.Equal(t, 499, folded[1].Id)
	assert.Equal(t, LogTypeManage, folded[2].Type)
}

func setupCollapseLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousLogDB := LOG_DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = db
	t.Cleanup(func() { LOG_DB = previousLogDB })
	require.NoError(t, db.AutoMigrate(&Log{}))
	return db
}

func TestGetAllLogsCollapsedPaginatesFoldedRows(t *testing.T) {
	db := setupCollapseLogTestDB(t)

	const base = int64(1_700_000_000)
	seq := 0
	insert := func(userId int, modelName string, count int) {
		for i := 0; i < count; i++ {
			seq++
			require.NoError(t, db.Create(collapseTestLog(seq, base-int64(seq), userId, fmt.Sprintf("user-%d", userId), modelName, "10.0.0.1", `{"cache_tokens":10}`)).Error)
		}
	}
	insert(1, "gpt-4o", 12)    // 折叠
	insert(1, "claude-3", 3)   // 逐条
	insert(2, "gpt-4o", 2)     // 逐条

	firstPage, total, err := GetAllLogsCollapsed(LogTypeUnknown, 0, 0, "", "", "", 0, 3, 0, "", "", "", 1000)
	require.NoError(t, err)
	assert.Equal(t, int64(6), total, "折叠后共 1 + 3 + 2 行")
	require.Len(t, firstPage, 3)
	assert.Equal(t, 12, firstPage[0].CollapsedCount, "最新的折叠行排在最前")

	secondPage, total, err := GetAllLogsCollapsed(LogTypeUnknown, 0, 0, "", "", "", 3, 3, 0, "", "", "", 1000)
	require.NoError(t, err)
	assert.Equal(t, int64(6), total)
	require.Len(t, secondPage, 3)
	assert.NotEqual(t, firstPage[0].Id, secondPage[0].Id)

	// 关掉折叠时总量回到原始条数，说明折叠只是视图。
	_, rawTotal, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 100, 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(17), rawTotal)
}

func TestGetAllLogsCollapsedOnlyFoldsInsideScanWindow(t *testing.T) {
	db := setupCollapseLogTestDB(t)

	const base = int64(1_700_000_000)
	// 12 条重复调用，被一批更新的、互不相同的模型调用挡在窗口之外。
	for i := 0; i < 12; i++ {
		require.NoError(t, db.Create(collapseTestLog(i+1, base-int64(i), 1, "u1", "gpt-4o", "10.0.0.1", "")).Error)
	}
	for i := 0; i < 120; i++ {
		require.NoError(t, db.Create(collapseTestLog(1000+i, base+int64(i)+1, 2, "u2", fmt.Sprintf("model-%d", i), "10.0.0.2", "")).Error)
	}

	// 窗口只覆盖最近的 100 条（全是 u2 的目标外模型），重复调用落在窗口外，不折叠。
	_, total, err := GetAllLogsCollapsed(LogTypeUnknown, 0, 0, "", "", "", 0, 1000, 0, "", "", "", LogCollapseMinScanLimit)
	require.NoError(t, err)
	assert.Equal(t, int64(LogCollapseMinScanLimit), total)

	// 放大窗口后，12 条重复调用合并成 1 行。
	_, total, err = GetAllLogsCollapsed(LogTypeUnknown, 0, 0, "", "", "", 0, 1000, 0, "", "", "", LogCollapseDefaultScanLimit)
	require.NoError(t, err)
	assert.Equal(t, int64(121), total, "120 条逐条 + 1 行折叠")
}

func TestNormalizeLogCollapseScanLimit(t *testing.T) {
	assert.Equal(t, LogCollapseDefaultScanLimit, NormalizeLogCollapseScanLimit(0))
	assert.Equal(t, LogCollapseDefaultScanLimit, NormalizeLogCollapseScanLimit(-5))
	assert.Equal(t, LogCollapseMinScanLimit, NormalizeLogCollapseScanLimit(1))
	assert.Equal(t, 500, NormalizeLogCollapseScanLimit(500))
	assert.Equal(t, LogCollapseMaxScanLimit, NormalizeLogCollapseScanLimit(LogCollapseMaxScanLimit*10))
}
