package main

import (
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	// dataStore holds persistedData per vmid
	dataMu          sync.RWMutex
	_googleAiApiKey = "AIzaSyBuz5ddmuj7ykpSdIjjHtDJea1Y2M5p7yQ"
)

func main() {
	r := gin.Default()

	// CORS middleware for frontend development
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// register API routes
	registerAPIs(r)

	//testGenaiAPI(_googleAiApiKey)

	// Listen on :8080
	r.Run(":8080")
}
