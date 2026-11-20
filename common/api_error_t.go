package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ApiErrorT answers with the English source text as `message`, and also carries
// `message_key` / `message_params` so a frontend that knows how to translate
// can render it in the user's language.
//
// Upstream added this together with its i18n message keys. The classic console
// here only reads `message`, so the two extra fields are inert for it; keeping
// them means API clients written against upstream still get what they expect.
func ApiErrorT(c *gin.Context, key string, params ...map[string]any) {
	message := NewMessage(key, params...)
	body := gin.H{
		"success": false,
		"message": message.Error(),
	}
	for name, value := range message.Fields() {
		body[name] = value
	}
	c.JSON(http.StatusOK, body)
}
