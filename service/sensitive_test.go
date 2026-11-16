package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func TestSensitiveWordMatchModes(t *testing.T) {
	previousWords := setting.SensitiveWords
	previousMode := setting.SensitiveWordMatchMode
	t.Cleanup(func() {
		setting.SensitiveWords = previousWords
		setting.SensitiveWordMatchMode = previousMode
	})

	setting.SensitiveWordsFromString("  违禁词  \nPOLICY\npolicy\n\n")
	require.Equal(t, []string{"违禁词", "POLICY"}, setting.SensitiveWords,
		"词表应去除首尾空白、跳过空行，并按大小写不敏感去重")

	tests := []struct {
		name         string
		mode         string
		text         string
		wantContains bool
		wantWords    []string
	}{
		{
			name:         "keyword mode blocks on substring",
			mode:         setting.SensitiveWordMatchKeyword,
			text:         "请帮我写一段包含 policy 的说明",
			wantContains: true,
			wantWords:    []string{"POLICY"},
		},
		{
			name:         "keyword mode matches case-insensitively",
			mode:         setting.SensitiveWordMatchKeyword,
			text:         "Policy violation",
			wantContains: true,
			wantWords:    []string{"POLICY"},
		},
		{
			name:         "keyword mode blocks zero width split",
			mode:         setting.SensitiveWordMatchKeyword,
			text:         "po\u200blicy is bad",
			wantContains: true,
			wantWords:    []string{"POLICY"},
		},
		{
			name:         "keyword mode blocks full width forms",
			mode:         setting.SensitiveWordMatchKeyword,
			text:         "这是ｐｏｌｉｃｙ",
			wantContains: true,
			wantWords:    []string{"POLICY"},
		},
		{
			name:         "keyword mode blocks zero width split in chinese words",
			mode:         setting.SensitiveWordMatchKeyword,
			text:         "违\u200b禁词",
			wantContains: true,
			wantWords:    []string{"违禁词"},
		},
		{
			name:         "keyword mode ignores clean text",
			mode:         setting.SensitiveWordMatchKeyword,
			text:         "今天天气不错",
			wantContains: false,
		},
		{
			name:         "keyword mode does not match unrelated words",
			mode:         setting.SensitiveWordMatchKeyword,
			text:         "The politician spoke",
			wantContains: false,
		},
		{
			name:         "full mode blocks on whole text match ignoring case and spaces",
			mode:         setting.SensitiveWordMatchFull,
			text:         "  policy \n",
			wantContains: true,
			wantWords:    []string{"POLICY"},
		},
		{
			name:         "full mode blocks on whole text match for non-latin words",
			mode:         setting.SensitiveWordMatchFull,
			text:         "违禁词",
			wantContains: true,
			wantWords:    []string{"违禁词"},
		},
		{
			name:         "full mode blocks full width whole text",
			mode:         setting.SensitiveWordMatchFull,
			text:         "ｐｏｌｉｃｙ",
			wantContains: true,
			wantWords:    []string{"POLICY"},
		},
		{
			name:         "full mode ignores words embedded in longer text",
			mode:         setting.SensitiveWordMatchFull,
			text:         "请帮我写一段包含 policy 的说明",
			wantContains: false,
		},
		{
			name:         "full mode ignores empty text",
			mode:         setting.SensitiveWordMatchFull,
			text:         "   ",
			wantContains: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setting.SensitiveWordMatchMode = tc.mode
			contains, words, err := SensitiveWordContains(tc.text)
			require.NoError(t, err)
			require.Equal(t, tc.wantContains, contains)
			require.Equal(t, tc.wantWords, words)
		})
	}
}

// 词表只改大小写时也必须重建匹配集，否则命中证据会一直沿用旧拼写。
func TestSensitiveWordSetFollowsCaseOnlyEdits(t *testing.T) {
	previousWords := setting.SensitiveWords
	previousMode := setting.SensitiveWordMatchMode
	t.Cleanup(func() {
		setting.SensitiveWords = previousWords
		setting.SensitiveWordMatchMode = previousMode
	})
	setting.SensitiveWordMatchMode = setting.SensitiveWordMatchKeyword

	setting.SensitiveWords = []string{"policy"}
	_, words, err := SensitiveWordContains("policy")
	require.NoError(t, err)
	require.Equal(t, []string{"policy"}, words)

	setting.SensitiveWords = []string{"POLICY"}
	_, words, err = SensitiveWordContains("policy")
	require.NoError(t, err)
	require.Equal(t, []string{"POLICY"}, words)
}

// 超预算的文本必须整包拒绝：既不能当成命中（会触发违规扣费与封禁），也不能
// 只扫前面一部分（客户端用填充把敏感词挤出窗口就能绕过）。
func TestSensitiveWordContainsRejectsOversizedText(t *testing.T) {
	previousWords := setting.SensitiveWords
	previousMode := setting.SensitiveWordMatchMode
	t.Cleanup(func() {
		setting.SensitiveWords = previousWords
		setting.SensitiveWordMatchMode = previousMode
	})
	setting.SensitiveWords = []string{"policy"}
	setting.SensitiveWordMatchMode = setting.SensitiveWordMatchKeyword

	oversized := strings.Repeat("a", setting.SensitiveCheckMaxTextBytes) + "policy"
	contains, words, err := SensitiveWordContains(oversized)
	require.ErrorIs(t, err, ErrSensitiveTextTooLarge)
	require.False(t, contains, "超预算不是命中")
	require.Nil(t, words)

	// 一个字节的余量内仍然正常扫描，说明边界是"大于上限才拒绝"。
	atLimit := strings.Repeat("a", setting.SensitiveCheckMaxTextBytes-6) + "policy"
	contains, words, err = SensitiveWordContains(atLimit)
	require.NoError(t, err)
	require.True(t, contains)
	require.Equal(t, []string{"policy"}, words)

	// 没有配置词表时不存在扫描成本，超长文本也不需要拒绝。
	setting.SensitiveWords = nil
	contains, _, err = SensitiveWordContains(oversized)
	require.NoError(t, err)
	require.False(t, contains)
}
