package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/requestctx"
)

func TestRequireRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name, role string
		want       int
	}{
		{name: "system administrator", role: auth.SystemAdmin, want: http.StatusNoContent},
		{name: "viewer denied", role: auth.Viewer, want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				metadata := requestctx.Metadata{RequestID: "test", Roles: []string{tt.role}}
				c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
				c.Next()
			})
			router.POST("/admin", requireRoles(auth.SystemAdmin), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/admin", nil))
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}
}
