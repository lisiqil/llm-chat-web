package main

import (
	"log"
	"net/http"

	"llm-web-backend/app/api/routesetup"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := newEngine().Run("127.0.0.1:8000"); err != nil {
		log.Fatal(err)
	}
}

func newEngine() *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.Use(gin.Logger(), gin.Recovery())
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Not Found"})
	})
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"detail": "Method Not Allowed"})
	})

	api := r.Group("/api/v1")
	routesetup.RegisterAllRoutes(api)
	return r
}
