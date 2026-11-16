package setting

import (
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSensitiveTextFoldsCharacters(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain ascii is lowercased", input: "Policy", want: "policy"},
		{name: "full width forms are folded", input: "ｐｏｌｉｃｙ", want: "policy"},
		{name: "zero width space is stripped", input: "po\u200blicy", want: "policy"},
		{name: "format characters are stripped from cjk words", input: "违\u200b禁\u200d词", want: "违禁词"},
		{name: "empty input stays empty", input: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeSensitiveText(tc.input))
		})
	}
}

func TestIsFormatRuneMatchesUnicodeTable(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if unicode.Is(unicode.Cf, r) != isFormatRune(r) {
			t.Fatalf("isFormatRune disagrees with unicode.Cf at %U", r)
		}
	}
}

func TestValidateSensitiveWordsPayloadLimits(t *testing.T) {
	require.NoError(t, ValidateSensitiveWordsPayload("policy\n违禁词"))

	atLimit := strings.Repeat("a\n", SensitiveWordsMaxLines-1) + "a"
	require.NoError(t, ValidateSensitiveWordsPayload(atLimit), "词表正好达到条数上限时应当允许保存")

	require.Error(t, ValidateSensitiveWordsPayload(strings.Repeat("a\n", SensitiveWordsMaxLines)),
		"超过条数上限时应拒绝保存")
	require.Error(t, ValidateSensitiveWordsPayload(strings.Repeat("a", SensitiveWordsMaxBytes+1)),
		"超过字节上限时应拒绝保存")
}
