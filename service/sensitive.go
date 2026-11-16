package service

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
	goahocorasick "github.com/anknown/ahocorasick"
)

// ErrSensitiveTextTooLarge 表示待扫描文本超过了 SensitiveCheckMaxTextBytes。
//
// 它不是"命中"：调用方必须按请求无效拒绝。若当成命中处理，会走敏感词违规
// 分支触发自动封禁与违规扣费，对一条只是过长的正常请求是错误定性；而静默
// 跳过尾部扫描又会给客户端留下"用填充把敏感词挤出扫描范围"的绕过手段。
var ErrSensitiveTextTooLarge = fmt.Errorf("prompt text exceeds the sensitive word inspection limit of %d bytes", setting.SensitiveCheckMaxTextBytes)

func CheckSensitiveMessages(messages []dto.Message) ([]string, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	for _, message := range messages {
		arrayContent := message.ParseContent()
		for _, m := range arrayContent {
			if m.Type == "image_url" {
				// TODO: check image url
				continue
			}
			// 检查 text 是否为空
			if m.Text == "" {
				continue
			}
			ok, words, err := SensitiveWordContains(m.Text)
			if err != nil {
				return nil, err
			}
			if ok {
				return words, errors.New("sensitive words detected")
			}
		}
	}
	return nil, nil
}

func CheckSensitiveText(text string) (bool, []string, error) {
	return SensitiveWordContains(text)
}

// SensitiveWordContains 是否命中敏感词，返回是否命中以及命中的词。
// 返回的词使用词表里的原始写法，便于日志与证据留痕。
//
// 关键词模式下文本包含任意关键词即命中；全匹配模式下仅当整段文本与某个词
// 完全一致才命中。两种模式都在归一化后的文本上比较，见 NormalizeSensitiveText。
func SensitiveWordContains(text string) (bool, []string, error) {
	if len(setting.SensitiveWords) == 0 || len(text) == 0 {
		return false, nil, nil
	}
	// 归一化的代价与输入线性相关，必须在进入之前把长度卡死，见
	// setting.SensitiveCheckMaxTextBytes。
	if len(text) > setting.SensitiveCheckMaxTextBytes {
		return false, nil, ErrSensitiveTextTooLarge
	}
	set := getSensitiveWordSet(setting.SensitiveWords)
	if set == nil {
		return false, nil, nil
	}
	normalized := setting.NormalizeSensitiveText(text)
	if normalized == "" {
		return false, nil, nil
	}
	if setting.SensitiveWordMatchMode == setting.SensitiveWordMatchFull {
		if word, ok := set.wordByKey[strings.TrimSpace(normalized)]; ok {
			return true, []string{word}, nil
		}
		return false, nil, nil
	}
	hits := set.ac.MultiPatternSearch([]rune(normalized), true)
	if len(hits) == 0 {
		return false, nil, nil
	}
	words := make([]string, 0, len(hits))
	for _, hit := range hits {
		key := string(hit.Word)
		if word, ok := set.wordByKey[key]; ok {
			words = append(words, word)
			continue
		}
		words = append(words, key)
	}
	return true, words, nil
}

// sensitiveWordSet 是归一化之后的可匹配词表：AC 自动机负责关键词扫描，
// wordByKey 负责把命中结果还原成词表里的原始写法。
type sensitiveWordSet struct {
	ac        *goahocorasick.Machine
	wordByKey map[string]string
}

var sensitiveWordSetCache = newBoundedCache[*sensitiveWordSet](16)

func getSensitiveWordSet(words []string) *sensitiveWordSet {
	key := sensitiveWordSetKey(words)
	if key == "" {
		return nil
	}
	if cached, ok := sensitiveWordSetCache.Load(key); ok {
		return cached
	}
	set := buildSensitiveWordSet(words)
	if set == nil {
		return nil
	}
	if cached, loaded := sensitiveWordSetCache.LoadOrStore(key, set); loaded {
		return cached
	}
	return set
}

// sensitiveWordSetKey 按词表的原始写法取哈希。不能用 acKey：它会先把词小写，
// 这样管理员只改大小写（policy -> POLICY）时会命中旧缓存，命中证据里就会一直
// 显示旧拼写。
func sensitiveWordSetKey(words []string) string {
	hasher := fnv.New64a()
	hasAny := false
	for _, word := range words {
		trimmed := strings.TrimSpace(word)
		if trimmed == "" {
			continue
		}
		hasAny = true
		hasher.Write([]byte{0})
		hasher.Write([]byte(trimmed))
	}
	if !hasAny {
		return ""
	}
	return fmt.Sprintf("%x", hasher.Sum64())
}

func buildSensitiveWordSet(words []string) *sensitiveWordSet {
	normalized := make([]string, 0, len(words))
	wordByKey := make(map[string]string, len(words))
	for _, word := range words {
		trimmed := strings.TrimSpace(word)
		key := setting.NormalizeSensitiveText(trimmed)
		if key == "" {
			continue
		}
		if _, exists := wordByKey[key]; exists {
			continue
		}
		wordByKey[key] = trimmed
		normalized = append(normalized, key)
	}
	if len(normalized) == 0 {
		return nil
	}
	m := InitAc(normalized)
	if m == nil {
		return nil
	}
	return &sensitiveWordSet{ac: m, wordByKey: wordByKey}
}
