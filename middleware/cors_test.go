package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCORSTest(t *testing.T) {
	t.Helper()
	previous := common.SessionCookieTrustedURLs
	t.Cleanup(func() { common.SessionCookieTrustedURLs = previous })
	gin.SetMode(gin.TestMode)
}

func newCORSRouter() *gin.Engine {
	router := gin.New()
	router.Use(CORS())
	router.GET("/api/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	return router
}

func preflight(t *testing.T, router *gin.Engine, origin, method string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodOptions, "/api/status", nil)
	request.Header.Set("Origin", origin)
	request.Header.Set("Access-Control-Request-Method", method)
	request.Header.Set("Access-Control-Request-Headers", "Authorization,New-Api-User,Content-Type")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// TestCORSDefaultModeNeverCombinesWildcardWithCredentials 锁定核心不变量：
// 默认（未配置可信来源）模式下绝不能同时下发 `*` 和
// `Access-Control-Allow-Credentials: true`，否则浏览器之外的客户端或
// 未来的来源反射改动会立刻退化为跨域凭据泄露。
func TestCORSDefaultModeNeverCombinesWildcardWithCredentials(t *testing.T) {
	setupCORSTest(t)
	common.SessionCookieTrustedURLs = nil

	response := preflight(t, newCORSRouter(), "https://evil.example", http.MethodGet)

	allowOrigin := response.Header().Get("Access-Control-Allow-Origin")
	allowCredentials := response.Header().Get("Access-Control-Allow-Credentials")
	assert.Equal(t, "*", allowOrigin)
	assert.NotEqual(t, "true", allowCredentials,
		"wildcard origin must never be paired with credentialed CORS")
}

// TestCORSDefaultModeDoesNotReflectArbitraryOrigin 确保默认模式不回显 Origin。
func TestCORSDefaultModeDoesNotReflectArbitraryOrigin(t *testing.T) {
	setupCORSTest(t)
	common.SessionCookieTrustedURLs = nil

	origin := "https://attacker-controlled.example"
	response := preflight(t, newCORSRouter(), origin, http.MethodGet)

	assert.NotEqual(t, origin, response.Header().Get("Access-Control-Allow-Origin"))
}

// TestCORSTrustedModeAllowsOnlyConfiguredOrigins 验证配置可信来源后，
// 只有精确匹配的来源获得凭据许可。
func TestCORSTrustedModeAllowsOnlyConfiguredOrigins(t *testing.T) {
	setupCORSTest(t)
	common.SessionCookieTrustedURLs = []string{"https://console.example.com"}

	allowed := preflight(t, newCORSRouter(), "https://console.example.com", http.MethodPost)
	assert.Equal(t, "https://console.example.com", allowed.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", allowed.Header().Get("Access-Control-Allow-Credentials"))

	denied := preflight(t, newCORSRouter(), "https://evil.example", http.MethodPost)
	assert.Empty(t, denied.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEqual(t, "true", denied.Header().Get("Access-Control-Allow-Credentials"))
}

// TestCORSTrustedModeRejectsOriginSuffixTricks 防前缀/后缀绕过。
func TestCORSTrustedModeRejectsOriginSuffixTricks(t *testing.T) {
	setupCORSTest(t)
	common.SessionCookieTrustedURLs = []string{"https://console.example.com"}

	for _, origin := range []string{
		"https://console.example.com.evil.example",
		"https://evil-console.example.com",
		"http://console.example.com",
	} {
		t.Run(origin, func(t *testing.T) {
			response := preflight(t, newCORSRouter(), origin, http.MethodPost)
			assert.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}

// TestCORSTrustedModeMatchesCanonicalEquivalentOrigin 验证等价来源写法被接受。
func TestCORSTrustedModeMatchesCanonicalEquivalentOrigin(t *testing.T) {
	setupCORSTest(t)
	common.SessionCookieTrustedURLs = []string{"https://console.example.com:443"}

	response := preflight(t, newCORSRouter(), "https://console.example.com", http.MethodGet)
	require.NotEmpty(t, response.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", response.Header().Get("Access-Control-Allow-Credentials"))
}
