package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthAndChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := newEngine()

	t.Run("health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d", resp.Code)
		}
		if resp.Body.String() != `{"status":"ok"}` {
			t.Fatalf("body = %s", resp.Body.String())
		}
	})

	t.Run("chat", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(`{"message":"你好"}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
		}
		if resp.Body.String() != `{"message":"你好"}` {
			t.Fatalf("body = %s", resp.Body.String())
		}
	})

	t.Run("empty message", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(`{"message":""}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", resp.Code)
		}
	})

	t.Run("get chat", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chat", nil)
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
		}
	})
}
