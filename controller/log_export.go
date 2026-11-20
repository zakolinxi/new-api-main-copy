package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const (
	// logExportPageSize is how many rows are pulled per query while streaming.
	// Export is a paged loop on purpose: the log table can hold millions of
	// rows, and loading a whole range into memory would be the shortest path to
	// an OOM on a small host.
	logExportPageSize = 500
	// logExportMaxRows caps one export so a single request cannot run for
	// minutes holding the per-user export lock.
	logExportMaxRows = 100000
)

// ExportAllLogs streams every matching log as one JSON object per line.
func ExportAllLogs(c *gin.Context) {
	exportLogs(c, true)
}

// ExportUserLogs streams the caller's own logs; regular users only when the
// deployment allows it.
func ExportUserLogs(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser && !operation_setting.GetLogSetting().UserExportEnabled {
		common.ApiErrorT(c, "Log export is disabled")
		return
	}
	exportLogs(c, false)
}

func exportLogs(c *gin.Context, all bool) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")

	// Clamp the range to the configured maximum so an export cannot walk the
	// whole table by accident.
	maxRange := operation_setting.GetLogSetting().MaxRangeSeconds()
	if endTimestamp == 0 || endTimestamp > common.GetTimestamp() {
		endTimestamp = common.GetTimestamp()
	}
	if startTimestamp == 0 || endTimestamp-startTimestamp > maxRange {
		startTimestamp = endTimestamp - maxRange
	}

	userId := c.GetInt("id")
	release, ok, err := service.AcquireLogExportLock(userId)
	if err != nil {
		common.SysError(common.LogText("failed to lock log export: %s", err.Error()))
		common.ApiErrorT(c, "Failed to export logs")
		return
	}
	if !ok {
		common.ApiErrorT(c, "Another log export is running. Try again after it finishes.")
		return
	}
	defer release()

	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	role := c.GetInt("role")
	count := 0
	failed := ""
	for offset := 0; count < logExportMaxRows; offset += logExportPageSize {
		var logs []*model.Log
		var total int64
		if all {
			logs, total, err = model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, offset, logExportPageSize, channel, group, requestId, upstreamRequestId)
		} else {
			logs, total, err = model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, offset, logExportPageSize, group, requestId, upstreamRequestId)
		}
		if err != nil {
			failed = err.Error()
			break
		}
		if len(logs) == 0 {
			break
		}
		// Same visibility filter the list views apply, so export never hands out
		// fields the UI hides.
		if all {
			if role < common.RoleRootUser {
				model.FormatAdminLogs(logs)
			} else {
				model.FormatRootLogs(logs)
			}
		} else {
			model.FormatUserLogsForExport(logs, offset)
		}
		for _, log := range logs {
			line, marshalErr := common.Marshal(log)
			if marshalErr != nil {
				continue
			}
			if _, writeErr := c.Writer.Write(append(line, '\n')); writeErr != nil {
				// Client went away; stop quietly rather than logging a failure.
				count = logExportMaxRows
				break
			}
			count++
			if count >= logExportMaxRows {
				break
			}
		}
		c.Writer.Flush()
		if count >= logExportMaxRows || int64(offset+len(logs)) >= total {
			break
		}
	}

	tail := gin.H{"success": true, "message": "", "data": gin.H{"count": count}}
	if failed != "" {
		tail = gin.H{"success": false, "message": failed}
	}
	if encoded, marshalErr := common.Marshal(tail); marshalErr == nil {
		_, _ = c.Writer.Write(append(encoded, '\n'))
	}
}
