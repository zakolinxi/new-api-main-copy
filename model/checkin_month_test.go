package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckinMonthRangeRejectsInvalidInput 确保月份参数被真正校验，
// 而不是靠字符串拼接把任意输入带进查询。
func TestCheckinMonthRangeRejectsInvalidInput(t *testing.T) {
	invalid := []string{
		"",
		"   ",
		"2026",
		"2026-1",
		"2026-13",
		"2026-00",
		"9999-99",
		"2026-01-01",
		"2026/01",
		"january",
		"2026-01' OR '1'='1",
		"2026-01;DROP TABLE checkins",
	}
	for _, month := range invalid {
		t.Run(month, func(t *testing.T) {
			_, _, err := checkinMonthRange(month)
			assert.Error(t, err, "month %q must be rejected", month)
		})
	}
}

// TestCheckinMonthRangeReturnsRealMonthEnd 验证短月份使用真实月末，
// 而不是依赖 "-31" 的字符串序巧合。
func TestCheckinMonthRangeReturnsRealMonthEnd(t *testing.T) {
	tests := []struct {
		month    string
		wantFrom string
		wantTo   string
	}{
		{month: "2026-01", wantFrom: "2026-01-01", wantTo: "2026-01-31"},
		{month: "2026-02", wantFrom: "2026-02-01", wantTo: "2026-02-28"},
		{month: "2024-02", wantFrom: "2024-02-01", wantTo: "2024-02-29"}, // 闰年
		{month: "2026-04", wantFrom: "2026-04-01", wantTo: "2026-04-30"},
		{month: "2026-06", wantFrom: "2026-06-01", wantTo: "2026-06-30"},
		{month: "2026-09", wantFrom: "2026-09-01", wantTo: "2026-09-30"},
		{month: "2026-11", wantFrom: "2026-11-01", wantTo: "2026-11-30"},
		{month: "2026-12", wantFrom: "2026-12-01", wantTo: "2026-12-31"},
		{month: " 2026-03 ", wantFrom: "2026-03-01", wantTo: "2026-03-31"}, // 首尾空白被裁剪
	}
	for _, test := range tests {
		t.Run(test.month, func(t *testing.T) {
			from, to, err := checkinMonthRange(test.month)
			require.NoError(t, err)
			assert.Equal(t, test.wantFrom, from)
			assert.Equal(t, test.wantTo, to)
		})
	}
}

// TestCheckinMonthRangeCoversAllDaysOfMonth 保证区间端点本身是合法日期，
// 且不会出现上界早于下界的退化区间。
func TestCheckinMonthRangeCoversAllDaysOfMonth(t *testing.T) {
	for monthNum := 1; monthNum <= 12; monthNum++ {
		month := "2026-" + []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12"}[monthNum-1]
		from, to, err := checkinMonthRange(month)
		require.NoError(t, err)
		assert.LessOrEqual(t, from, to, "range for %s must not be inverted", month)
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, from)
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, to)
	}
}
