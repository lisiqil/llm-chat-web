package chat

import (
	"io"
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
	// 声明一个空的请求结构体，等下把 JSON 填进来。
	var req intype.ChatRequest

	// 从 c 里的原始请求体解析 JSON。缺 message、类型不对或是空字符串时 err 不为空。
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数不合法，返回 400，这段 JSON 会显示在前端的错误区域。
		c.JSON(http.StatusBadRequest, gin.H{"detail": "请求参数无效。"})
		// 不再继续调用模型。
		return
	}

	// 把解析出的那句话发给模型。reply 是还没读完的流式回复，err 表示连接是否成功。
	reply, err := chatservice.Open(c.Request.Context(), req.Message)

	// 连不上模型，或者模型直接拒绝了这次请求。
	if err != nil {
		// 返回 502，正文是给用户看的失败说明。
		c.String(http.StatusBadGateway, "模型服务暂时不可用。")
		return
	}
	// 函数结束时关闭模型连接，后面中途 return 也会执行。
	defer reply.Close()

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)

	// 取出能把缓冲区立刻发出去的接口。前面的 _ 丢掉类型断言是否成功，失败时 flusher 为 nil。
	flusher, _ := c.Writer.(http.Flusher)
	// 模型还在往外吐字，就一直循环。
	for {
		// 取下一段正文。流读完时 err 是 io.EOF，读失败时是别的错误。
		piece, err := reply.Next()
		// 模型已经说完，结束函数，这次响应也随之结束。
		if err == io.EOF {
			return
		}
		// 中途读失败，不再继续写。
		if err != nil {
			return
		}
		// 把这一小段追加进响应正文。写入失败就结束。
		if _, err := c.Writer.Write([]byte(piece)); err != nil {
			return
		}
		// 把缓冲区里刚写好的内容马上推给前端，页面可以先看到这一段。
		if flusher != nil {
			flusher.Flush()
		}
	}
}
