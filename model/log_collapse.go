package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

// Collapsing repeats answers one question on the usage log page: "this account
// has called this model 300 times today, why do I have to scroll past all of
// them?" A group of consume logs that share the same user and the same model is
// replaced by a single row that keeps accumulating tokens, cache reads/writes
// and quota, while IP / first-token latency / timestamp show the newest call.
// Nothing is deleted: the row expands back into the individual records on demand.
//
// Folding shapes the rows the API already loaded instead of aggregating in SQL
// because cache tokens and first-token latency only exist inside the `other`
// JSON blob, which cannot be summed portably across SQLite, MySQL, PostgreSQL
// and ClickHouse. The read therefore stays a single indexed ORDER BY ... LIMIT.
const (
	// LogCollapseThreshold is the number of calls above which a group folds.
	LogCollapseThreshold = 10
	// LogCollapseDefaultScanLimit bounds how many recent rows one collapsed page
	// folds. Folding is a view over a window, not a global aggregate, so a group
	// only folds when it repeats often inside that window.
	LogCollapseDefaultScanLimit = 1000
	LogCollapseMinScanLimit     = 100
	LogCollapseMaxScanLimit     = 5000
)

// NormalizeLogCollapseScanLimit clamps a client supplied window size.
func NormalizeLogCollapseScanLimit(value int) int {
	if value <= 0 {
		return LogCollapseDefaultScanLimit
	}
	if value < LogCollapseMinScanLimit {
		return LogCollapseMinScanLimit
	}
	if value > LogCollapseMaxScanLimit {
		return LogCollapseMaxScanLimit
	}
	return value
}

type logCollapseKey struct {
	userId int
	model  string
}

// collapseAccumulator collects one group while the log list is scanned. The
// first row seen for a group is its newest one, because callers pass a
// created_at descending list.
type collapseAccumulator struct {
	template         Log
	count            int
	cacheRead        int
	cacheWrite5m     int
	cacheWrite1h     int
	cacheWriteLegacy int
	firstResponse    float64
	ips              map[string]struct{}
}

// CollapseLogs folds repeated consume logs of the same user and model into one
// row. Groups at or below threshold stay as individual rows. The folded row
// takes the position of the group's newest call, so the list keeps its
// chronological shape.
func CollapseLogs(logs []*Log, threshold int) []*Log {
	if threshold <= 0 || len(logs) <= threshold {
		return logs
	}

	collapseKeyOf := func(log *Log) (logCollapseKey, bool) {
		if log == nil || log.Type != LogTypeConsume || log.ModelName == "" || log.UserId == 0 {
			return logCollapseKey{}, false
		}
		return logCollapseKey{userId: log.UserId, model: log.ModelName}, true
	}

	accumulators := make(map[logCollapseKey]*collapseAccumulator)
	for _, log := range logs {
		key, eligible := collapseKeyOf(log)
		if !eligible {
			continue
		}
		accumulator, exists := accumulators[key]
		if !exists {
			accumulator = &collapseAccumulator{
				template: *log,
				ips:      make(map[string]struct{}),
			}
			// The template keeps the newest call's identity fields (IP, latency,
			// timestamp, channel, token name) but starts from zero for every
			// summed field, otherwise add would count the newest call twice.
			accumulator.template.PromptTokens = 0
			accumulator.template.CompletionTokens = 0
			accumulator.template.Quota = 0
			accumulators[key] = accumulator
		}
		accumulator.add(log)
	}

	folded := make([]*Log, 0, len(logs))
	emitted := make(map[logCollapseKey]struct{}, len(accumulators))
	for _, log := range logs {
		key, eligible := collapseKeyOf(log)
		if !eligible {
			folded = append(folded, log)
			continue
		}
		accumulator := accumulators[key]
		if accumulator.count <= threshold {
			folded = append(folded, log)
			continue
		}
		if _, done := emitted[key]; done {
			continue
		}
		emitted[key] = struct{}{}
		folded = append(folded, accumulator.fold(key))
	}
	return folded
}

func (accumulator *collapseAccumulator) add(log *Log) {
	accumulator.count++
	accumulator.template.PromptTokens += log.PromptTokens
	accumulator.template.CompletionTokens += log.CompletionTokens
	accumulator.template.Quota += log.Quota
	if log.Ip != "" {
		accumulator.ips[log.Ip] = struct{}{}
	}

	// The newest call is the template, so its latency is already the latest one.
	if accumulator.count == 1 {
		accumulator.firstResponse = logFirstResponseMillis(log.Other)
	}

	var cache struct {
		Read     int `json:"cache_tokens"`
		Creation int `json:"cache_creation_tokens"`
		FiveMin  int `json:"cache_creation_tokens_5m"`
		OneHour  int `json:"cache_creation_tokens_1h"`
	}
	if log.Other != "" && common.UnmarshalJsonStr(log.Other, &cache) == nil {
		accumulator.cacheRead += cache.Read
		if cache.FiveMin > 0 || cache.OneHour > 0 {
			accumulator.cacheWrite5m += cache.FiveMin
			accumulator.cacheWrite1h += cache.OneHour
		} else {
			accumulator.cacheWriteLegacy += cache.Creation
		}
	}
}

// fold materialises the aggregated row. The template's own token and cache
// numbers were already summed into it by add, so only the fields that must show
// the newest call stay untouched.
func (accumulator *collapseAccumulator) fold(key logCollapseKey) *Log {
	row := accumulator.template
	row.CollapsedCount = accumulator.count
	row.CollapsedIpCount = len(accumulator.ips)
	row.CollapsedKey = fmt.Sprintf("%d|%s", key.userId, key.model)
	row.Other = accumulator.foldedOtherJSON()
	return &row
}

// foldedOtherJSON builds the metadata of the folded row.
//
// It starts from the newest call's metadata instead of emitting only the fields
// that were summed: price rendering reads model_ratio, completion_ratio,
// group_ratio and friends straight out of `other`, so dropping them made every
// priced line on the folded row render as $NaN. Summed fields are overwritten
// afterwards, and everything else keeps describing the newest call — which is
// what the row already shows for timestamp, IP and latency.
func (accumulator *collapseAccumulator) foldedOtherJSON() string {
	payload := make(map[string]any)
	if accumulator.template.Other != "" {
		if err := common.UnmarshalJsonStr(accumulator.template.Other, &payload); err != nil {
			payload = make(map[string]any)
		}
	}
	for _, key := range []string{
		"cache_tokens",
		"cache_creation_tokens",
		"cache_creation_tokens_5m",
		"cache_creation_tokens_1h",
		"frt",
	} {
		delete(payload, key)
	}

	if accumulator.cacheRead > 0 {
		payload["cache_tokens"] = accumulator.cacheRead
	}
	if accumulator.cacheWrite5m > 0 {
		payload["cache_creation_tokens_5m"] = accumulator.cacheWrite5m
	}
	if accumulator.cacheWrite1h > 0 {
		payload["cache_creation_tokens_1h"] = accumulator.cacheWrite1h
	}
	if accumulator.cacheWrite5m == 0 && accumulator.cacheWrite1h == 0 && accumulator.cacheWriteLegacy > 0 {
		payload["cache_creation_tokens"] = accumulator.cacheWriteLegacy
	}
	if accumulator.firstResponse > 0 {
		payload["frt"] = accumulator.firstResponse
	}
	if len(payload) == 0 {
		return ""
	}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// logFirstResponseMillis returns other.frt as written by
// GenerateTextOtherInfo. Both sides keep milliseconds, so the folded row can be
// rendered by the same timing column as a single record.
func logFirstResponseMillis(other string) float64 {
	if other == "" {
		return 0
	}
	var payload struct {
		FirstResponseTime float64 `json:"frt"`
	}
	if err := common.UnmarshalJsonStr(other, &payload); err != nil {
		return 0
	}
	if payload.FirstResponseTime <= 0 {
		return 0
	}
	return payload.FirstResponseTime
}

func paginateLogs(logs []*Log, startIdx int, num int) []*Log {
	if startIdx <= 0 && (num <= 0 || num >= len(logs)) {
		return logs
	}
	if startIdx >= len(logs) {
		return []*Log{}
	}
	end := len(logs)
	if num > 0 && startIdx+num < end {
		end = startIdx + num
	}
	return logs[startIdx:end]
}

// GetAllLogsCollapsed folds the most recent scanLimit rows that match the same
// filters as GetAllLogs, then paginates the folded result in memory.
func GetAllLogsCollapsed(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string, scanLimit int) (logs []*Log, total int64, err error) {
	tx, err := buildAllLogsQuery(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, requestId, upstreamRequestId)
	if err != nil {
		return nil, 0, err
	}
	var scanned []*Log
	if err = tx.Order(logQueryOrder()).Limit(NormalizeLogCollapseScanLimit(scanLimit)).Find(&scanned).Error; err != nil {
		return nil, 0, err
	}
	if err = attachChannelNames(scanned); err != nil {
		return nil, 0, err
	}
	folded := CollapseLogs(scanned, LogCollapseThreshold)
	page := paginateLogs(folded, startIdx, num)
	// ClickHouse rows carry no primary key, so the display id has to be
	// synthesised. Assign it after folding: the folded list is shorter than the
	// scanned window, and numbering the window instead would shift every id on
	// page 2+ by the number of rows folding removed.
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		assignDisplayLogIds(page, startIdx)
	}
	return page, int64(len(folded)), nil
}

// GetUserLogsCollapsed is the self-service counterpart of GetAllLogsCollapsed.
func GetUserLogsCollapsed(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string, upstreamRequestId string, scanLimit int) (logs []*Log, total int64, err error) {
	tx, err := buildUserLogsQuery(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, group, requestId, upstreamRequestId)
	if err != nil {
		return nil, 0, err
	}
	var scanned []*Log
	if err = tx.Order(logQueryOrder()).Limit(NormalizeLogCollapseScanLimit(scanLimit)).Find(&scanned).Error; err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, err
	}
	folded := CollapseLogs(scanned, LogCollapseThreshold)
	page := paginateLogs(folded, startIdx, num)
	// Same visibility filter as the plain self-service listing, otherwise the
	// template row of a folded group would expose admin-only diagnostics.
	formatUserLogs(page, startIdx)
	return page, int64(len(folded)), nil
}
