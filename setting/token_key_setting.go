package setting

import (
	"fmt"
	"strings"
	"sync/atomic"
)

const (
	// TokenKeyPrefixOptionKey is the option holding the text users see in front
	// of every generated API key.
	TokenKeyPrefixOptionKey = "TokenKeyPrefix"
	// DefaultTokenKeyPrefix is the historical shape. Keeping it as the default
	// means every existing client, SDK setting and copy-pasted doc keeps
	// working until an administrator deliberately changes it.
	DefaultTokenKeyPrefix = "sk-"
	// MaxTokenKeyPrefixLength bounds the prefix. The random part behind it is a
	// fixed 48 characters, so a long prefix only makes the key harder to paste.
	MaxTokenKeyPrefixLength = 16
)

// tokenKeyPrefix is read on every relay request, so it is stored atomically
// instead of behind a mutex.
var tokenKeyPrefix atomic.Value

func init() {
	tokenKeyPrefix.Store(DefaultTokenKeyPrefix)
}

// GetTokenKeyPrefix returns the prefix shown in front of generated API keys.
func GetTokenKeyPrefix() string {
	if prefix, ok := tokenKeyPrefix.Load().(string); ok && prefix != "" {
		return prefix
	}
	return DefaultTokenKeyPrefix
}

// SetTokenKeyPrefix stores an already normalized prefix.
func SetTokenKeyPrefix(prefix string) {
	if prefix == "" {
		prefix = DefaultTokenKeyPrefix
	}
	tokenKeyPrefix.Store(prefix)
}

// NormalizeTokenKeyPrefix validates an administrator supplied prefix and adds
// the trailing '-' when it is missing, so "sk-mao" and "sk-mao-" are the same
// setting and the random part never fuses into a longer word.
func NormalizeTokenKeyPrefix(value string) (string, error) {
	prefix := strings.TrimSpace(value)
	if prefix == "" {
		return DefaultTokenKeyPrefix, nil
	}
	if len(prefix) > MaxTokenKeyPrefixLength {
		return "", fmt.Errorf("the key prefix must not exceed %d characters", MaxTokenKeyPrefixLength)
	}
	for _, char := range prefix {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case char == '-' || char == '_':
		default:
			return "", fmt.Errorf("the key prefix may only contain letters, digits, '-' and '_'")
		}
	}
	if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}
	return prefix, nil
}

// TrimTokenKeyPrefix strips the configured prefix from key, falling back to the
// historical "sk-". The fallback is what keeps keys issued before the prefix
// was configured working after an administrator changes the setting.
func TrimTokenKeyPrefix(key string) string {
	if prefix := GetTokenKeyPrefix(); strings.HasPrefix(key, prefix) {
		return key[len(prefix):]
	}
	return strings.TrimPrefix(key, "sk-")
}

// TokenKeyPrefixExample is the shape shown next to the setting in the console.
func TokenKeyPrefixExample() string {
	return GetTokenKeyPrefix() + "AbCdEf1234567890AbCdEf1234567890AbCdEf1234"
}
