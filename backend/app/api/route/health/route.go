package health

import (
	"net/http"

	"llm-web-backend/intype"

	"github.com/gin-gonic/gin"
)

// SetupRouter 注册健康检查。
func SetupRouter(r gin.IRouter) {
	r.GET("/health", getHealth)
}

func getHealth(c *gin.Context) {
	c.JSON(http.StatusOK, intype.HealthResponse{Status: "ok"})
}
