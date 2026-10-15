package chat

import (
	"net/http"

	"llm-web-backend/intype"
	chatservice "llm-web-backend/service/chat"

	"github.com/gin-gonic/gin"
)

// SetupRouter 注册聊天接口。
func SetupRouter(r gin.IRouter) {
	r.POST("/chat", postChat)
}

func postChat(c *gin.Context) {
	var req intype.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "请求参数无效。"})
		return
	}
	c.JSON(http.StatusOK, intype.ChatResponse{Message: chatservice.Echo(req.Message)})
}
