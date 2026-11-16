package setting

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var CheckSensitiveEnabled = true
var CheckSensitiveOnPromptEnabled = true

//var CheckSensitiveOnCompletionEnabled = true

// StopOnSensitiveEnabled 如果检测到敏感词，是否立刻停止生成，否则替换敏感词
var StopOnSensitiveEnabled = true

// StreamCacheQueueLength 流模式缓存队列长度，0表示无缓存
var StreamCacheQueueLength = 0

// 敏感词匹配模式。管理员可以按词表的实际形态选择判定方式。
const (
	// SensitiveWordMatchKeyword 关键词匹配：文本中只要出现任意一个敏感词就拦截。
	SensitiveWordMatchKeyword = "keyword"
	// SensitiveWordMatchFull 全匹配：只有整段文本（忽略首尾空白与大小写）与某个敏感词
	// 完全一致时才拦截，用于规避短词被长文本误命中。
	SensitiveWordMatchFull = "full"
)

// 词表规模上限。词表会在首次匹配时编译成 AC 自动机，放任无上限写入会让
// 单次构建占用过多内存，也会让 option 表出现超大行。
const (
	SensitiveWordsMaxBytes = 1 << 20
	SensitiveWordsMaxLines = 20000
)

// SensitiveCheckMaxTextBytes 限制单次请求送进敏感词过滤的文本长度。
//
// 归一化（NFKC + 去 Cf + 小写）的代价与输入线性相关，实测约 57ms/MB，而
// MAX_REQUEST_BODY_MB 默认允许 128MB 请求体，两者相乘就是单请求数秒的 CPU。
// 超限的文本必须由调用方整包拒绝，不能只扫前 N 字节：截断等于给客户端留了
// "用尾部填充把敏感词挤出扫描窗口"的绕过手段。
//
// 8MiB 的文本约为 200-400 万 token，远高于线上模型的上下文窗口；body 里的
// 图片、音频、文件是 base64，不进入 CombineText，因此正常的多模态请求不会
// 触发这个上限。
const SensitiveCheckMaxTextBytes = 8 << 20

// SensitiveWordMatchMode 当前生效的敏感词匹配模式
var SensitiveWordMatchMode = SensitiveWordMatchKeyword

// ParseSensitiveWordMatchMode 校验并归一化匹配模式，空值回落到关键词匹配。
func ParseSensitiveWordMatchMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", SensitiveWordMatchKeyword:
		return SensitiveWordMatchKeyword, nil
	case SensitiveWordMatchFull:
		return SensitiveWordMatchFull, nil
	}
	return "", fmt.Errorf("invalid sensitive word match mode: %q", value)
}

func SensitiveWordMatchModeToString() string {
	return SensitiveWordMatchMode
}

// ValidateSensitiveWordsPayload 限制词表写入规模。只在写入路径调用，
// 不参与历史数据加载，避免老部署里超限的词表把加载流程打回默认值。
func ValidateSensitiveWordsPayload(value string) error {
	if len(value) > SensitiveWordsMaxBytes {
		return fmt.Errorf("sensitive words list is too large: %d bytes (limit %d bytes)", len(value), SensitiveWordsMaxBytes)
	}
	if lines := strings.Count(value, "\n") + 1; lines > SensitiveWordsMaxLines {
		return fmt.Errorf("sensitive words list has too many entries: %d (limit %d)", lines, SensitiveWordsMaxLines)
	}
	return nil
}

// NormalizeSensitiveText 归一化用于匹配的文本：NFKC 折叠全角与兼容字符，
// 剔除零宽空格、双向控制符等格式字符，最后统一转为小写。
//
// 词表与待检文本必须走同一个函数，否则「ｐｏｌｉｃｙ」「po\u200blicy」这类
// 字符级变形可以直接绕过关键词匹配。纯 ASCII 文本无法用这类变形规避，
// 因此跳过 NFKC 只做小写转换，避免在每个请求上多付一次归一化开销。
func NormalizeSensitiveText(text string) string {
	if text == "" {
		return ""
	}
	if isASCII(text) {
		return strings.ToLower(text)
	}
	// 用 NFKC 折叠全角（ｐｏｌｉｃｙ）与兼容字符。实测 norm.IsNormalString
	// 的探测开销与 NFKC.String 相当，拿它做前置判断反而更慢，因此直接折叠。
	folded := norm.NFKC.String(text)
	// 命中 Cf 裁剪的分支要多一次 builder 复制，先做纯扫描跳过它。
	if !containsFormatRune(folded) {
		return strings.ToLower(folded)
	}
	var builder strings.Builder
	builder.Grow(len(folded))
	for _, r := range folded {
		if isFormatRune(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return strings.ToLower(builder.String())
}

// containsFormatRune 先做一次纯扫描判断是否需要裁剪，避免在绝大多数正常文本上
// 走 strings.Map 加闭包（每百万字节约多花 20ms）。扫描用 range 循环而不是
// strings.IndexFunc，同样是为了省掉逐字符的函数值调用。
func containsFormatRune(text string) bool {
	for _, r := range text {
		if isFormatRune(r) {
			return true
		}
	}
	return false
}

// isFormatRune 判断 rune 是否属于 Unicode Cf（格式字符）类别，即零宽空格、
// 零宽连字、双向控制符、软连字符一类不显示但会切断关键词匹配的字符。
//
// 这里用显式区间代替 unicode.Is(unicode.Cf, r)：后者走通用区间表查找，
// 在长中文文本上每百万字节会多出约 30ms，属于请求热路径不能接受的成本。
// 区间与 unicode.Cf 完全等价，由 TestIsFormatRuneMatchesUnicodeTable 保证。
func isFormatRune(r rune) bool {
	if r < 0x00AD {
		return false
	}
	// 汉字覆盖了绝大多数真实请求文本，且都不在 Cf 类别内。
	if r >= 0x4E00 && r <= 0x9FFF {
		return false
	}
	switch r {
	case 0x00AD, 0x0600, 0x061C, 0x06DD, 0x070F, 0x0890, 0x0891, 0x08E2,
		0x180E, 0x200B, 0xFEFF, 0x110BD, 0x110CD, 0xE0001:
		return true
	}
	switch {
	case r >= 0x0601 && r <= 0x0605,
		r >= 0x200C && r <= 0x200F,
		r >= 0x202A && r <= 0x202E,
		r >= 0x2060 && r <= 0x2064,
		r >= 0x2066 && r <= 0x206F,
		r >= 0xFFF9 && r <= 0xFFFB,
		r >= 0x13430 && r <= 0x1343F,
		r >= 0x1BCA0 && r <= 0x1BCA3,
		r >= 0x1D173 && r <= 0x1D17A,
		r >= 0xE0020 && r <= 0xE007F:
		return true
	}
	return false
}

func isASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// SensitiveWords 敏感词
// var SensitiveWords []string
var SensitiveWords = []string{
	"test_sensitive",
}

func SensitiveWordsToString() string {
	return strings.Join(SensitiveWords, "\n")
}

// SensitiveWordsFromString 解析词表：按行切分，去掉首尾空白，跳过空行，
// 并忽略大小写重复的条目，避免同一关键词在词表里重复占位。
func SensitiveWordsFromString(s string) {
	words := make([]string, 0)
	seen := make(map[string]struct{})
	sw := strings.SplitSeq(s, "\n")
	for line := range sw {
		word := strings.TrimSpace(line)
		if word == "" {
			continue
		}
		key := strings.ToLower(word)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		words = append(words, word)
	}
	SensitiveWords = words
}

func ShouldCheckPromptSensitive() bool {
	return CheckSensitiveEnabled && CheckSensitiveOnPromptEnabled
}

//func ShouldCheckCompletionSensitive() bool {
//	return CheckSensitiveEnabled && CheckSensitiveOnCompletionEnabled
//}
