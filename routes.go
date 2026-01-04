package main

import (
	"net/http"
	"time"

	"subtuber-services/handlers"

	"github.com/gin-gonic/gin"
)

var (
	_twitchClientId = "2ksfb55qayddhrq25m9e16a51r3yp9"
	_twitchToken    = "uiwphdyvd96s1ntbcfm20l1mautj38"
)

// registerAPIs registers HTTP handlers on the provided gin Engine.
// This is a pure API server for the frontend application.
func registerAPIs(r *gin.Engine) {
	// Health check endpoint
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "oshivtuber API Server",
			"version": "1.0.0",
		})
	})

	// API endpoints for frontend
	r.GET("/api/time", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"time": time.Now().Format(time.RFC3339)})
	})

	// Summarize transcript using Google Generative AI
	r.POST("/api/summarize", func(c *gin.Context) {
		// handler in googleai.go
		summarizeHandler(c)
	})

	// Authentication routes (send code / verify)
	handlers.RegisterAuthRoutes(r)

	// Twitch monitoring routes
	r.GET("/api/twitch/status", handlers.GetTwitchStatus)
	r.POST("/api/twitch/check-now", handlers.CheckTwitchStatusNow)
	r.GET("/api/twitch/videos", handlers.GetTwitchVideos)

}
