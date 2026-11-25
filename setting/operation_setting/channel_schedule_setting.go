package operation_setting

import (
	"fmt"
	"strings"
	"time"
)

// 渠道定时下线的时区口径。
//
// 界面上填写的「定时下线」是墙上时间（例如 2026-12-01 00:00），要落成 Unix 时间戳
// 必须先绑定一个时区。历史上这一步由浏览器本地时区完成，导致同一个日期在不同时区的
// 管理员手里命中不同时刻（差 8 小时）。这里把它固定成可配置的服务端口径。
const (
	// ChannelScheduleTimezoneUTC8 固定 UTC+8（北京时间），默认值
	ChannelScheduleTimezoneUTC8 = "utc+8"
	// ChannelScheduleTimezoneUTC 固定 UTC
	ChannelScheduleTimezoneUTC = "utc"
	// ChannelScheduleTimezoneServer 跟随服务器本地时区
	ChannelScheduleTimezoneServer = "server"

	// ChannelScheduleTimezoneOptionKey 是该项在系统设置中的键名
	ChannelScheduleTimezoneOptionKey = "ChannelScheduleTimezone"
)

// ChannelScheduleTimezone 当前生效的时区口径，由系统设置覆盖。
var ChannelScheduleTimezone = ChannelScheduleTimezoneUTC8

// ChannelScheduleLocation 返回定时下线换算使用的时区。
func ChannelScheduleLocation() *time.Location {
	switch ChannelScheduleTimezone {
	case ChannelScheduleTimezoneUTC:
		return time.UTC
	case ChannelScheduleTimezoneServer:
		return time.Local
	default:
		return time.FixedZone("UTC+8", 8*60*60)
	}
}

// ChannelScheduleUTCOffsetMinutes 返回当前口径相对 UTC 的偏移（分钟）。
// 前端据此把墙上的「年月日时分」换算成时间戳，保证与后端判断一致。
// 跟随服务器本地时区时取当前时刻的偏移，因此夏令时地区会随季节变化。
func ChannelScheduleUTCOffsetMinutes() int {
	_, offsetSeconds := time.Now().In(ChannelScheduleLocation()).Zone()
	return offsetSeconds / 60
}

// NormalizeChannelScheduleTimezone 校验并归一化时区口径的取值。
func NormalizeChannelScheduleTimezone(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ChannelScheduleTimezoneUTC8, "utc8", "utc+08", "utc+08:00", "asia/shanghai", "beijing":
		return ChannelScheduleTimezoneUTC8, nil
	case ChannelScheduleTimezoneUTC, "utc+0", "gmt", "z":
		return ChannelScheduleTimezoneUTC, nil
	case ChannelScheduleTimezoneServer, "local":
		return ChannelScheduleTimezoneServer, nil
	}
	return "", fmt.Errorf("不支持的时区取值：%s（可选 utc+8 / utc / server）", value)
}

// FormatChannelSchedule 按当前口径把时间戳格式化成墙上时间，用于日志与提示。
func FormatChannelSchedule(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	}
	return time.Unix(timestamp, 0).In(ChannelScheduleLocation()).Format("2006-01-02 15:04")
}
