package intype

// ChatRequest 是 POST /api/v1/chat 的请求体。
type ChatRequest struct {
	Message string `json:"message" binding:"required,min=1"`
}

// HealthResponse 是健康检查的响应体。
type HealthResponse struct {
	Status string `json:"status"`
}
