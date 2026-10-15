package routesetup

import (
	"llm-web-backend/app/api/route/chat"
	"llm-web-backend/app/api/route/health"

	"github.com/gin-gonic/gin"
)

// RegisterAllRoutes 把业务路由挂到 /api/v1 分组上。
func RegisterAllRoutes(api gin.IRouter) {
	health.SetupRouter(api)
	chat.SetupRouter(api)
}
