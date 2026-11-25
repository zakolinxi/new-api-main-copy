package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeChannelScheduleTimezone(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"utc+8", ChannelScheduleTimezoneUTC8},
		{"UTC+08:00", ChannelScheduleTimezoneUTC8},
		{" Asia/Shanghai ", ChannelScheduleTimezoneUTC8},
		{"utc", ChannelScheduleTimezoneUTC},
		{"GMT", ChannelScheduleTimezoneUTC},
		{"local", ChannelScheduleTimezoneServer},
	}
	for _, tc := range cases {
		got, err := NormalizeChannelScheduleTimezone(tc.input)
		require.NoError(t, err, "input=%q", tc.input)
		assert.Equal(t, tc.want, got, "input=%q", tc.input)
	}

	if _, err := NormalizeChannelScheduleTimezone("utc+3"); err == nil {
		t.Fatal("unsupported timezone must be rejected, otherwise the setting silently falls back")
	}
}

// 同一个时间戳在不同口径下应当呈现为不同的墙上时间；前端换算与后端判断依赖这一对齐，
// 否则管理员填的「零点」会在另一时区变成前一天 16:00。
func TestFormatChannelScheduleFollowsConfiguredTimezone(t *testing.T) {
	const midnightUTC8 = int64(1796054400) // 2026-12-01 00:00 UTC+8

	original := ChannelScheduleTimezone
	t.Cleanup(func() { ChannelScheduleTimezone = original })

	ChannelScheduleTimezone = ChannelScheduleTimezoneUTC8
	assert.Equal(t, "2026-12-01 00:00", FormatChannelSchedule(midnightUTC8))
	assert.Equal(t, 480, ChannelScheduleUTCOffsetMinutes())

	ChannelScheduleTimezone = ChannelScheduleTimezoneUTC
	assert.Equal(t, "2026-11-30 16:00", FormatChannelSchedule(midnightUTC8))
	assert.Equal(t, 0, ChannelScheduleUTCOffsetMinutes())

	assert.Equal(t, "", FormatChannelSchedule(0))
}
