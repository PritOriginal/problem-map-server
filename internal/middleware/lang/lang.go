// Package lang provides a gin middleware that resolves the client's
// language from the Accept-Language header and records it on the request
// context (models.LangFromContext) for the localised dictionaries.
package lang

import (
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/gin-gonic/gin"
)

// Header is the request header the language is negotiated from.
const Header = "Accept-Language"

// New returns the middleware. Unsupported or missing languages resolve to
// models.DefaultLang; the response carries Content-Language with the
// chosen value and Vary: Accept-Language so shared caches keep the
// localised variants apart.
func New() gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := models.ParseAcceptLanguage(c.GetHeader(Header))

		c.Request = c.Request.WithContext(models.ContextWithLang(c.Request.Context(), lang))
		c.Header("Content-Language", string(lang))
		c.Writer.Header().Add("Vary", Header)

		c.Next()
	}
}
