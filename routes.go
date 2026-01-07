package main

import (
	"net/http"
	"time"

	"subtuber-services/handlers"

	"github.com/gin-gonic/gin"
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

	// Authentication routes (send code / verify)
	handlers.RegisterAuthRoutes(r)

	// Twitch monitoring routes
	r.GET("/api/twitch/status", handlers.GetTwitchStatus)
	r.POST("/api/twitch/check-now", handlers.CheckTwitchStatusNow)
	r.GET("/api/twitch/videos", handlers.GetTwitchVideos)

	// Twitch VOD chat download routes
	r.POST("/api/twitch/download-chat", handlers.DownloadVODChat)
	r.POST("/api/twitch/save-chat", handlers.SaveVODChatToFile)

	// Twitch chat analysis routes
	r.GET("/api/twitch/analysis/:videoID", handlers.GetAnalysisResult)
	r.GET("/api/twitch/analysis", handlers.ListAnalysisResults)
	r.GET("/api/twitch/analysis-summary", handlers.GetAnalysisSummary)

	// Streamer query routes
	r.GET("/api/streamers", handlers.ListStreamers)
	r.GET("/api/streamers/:id", handlers.GetStreamerByID)

	// VOD download routes
	r.POST("/api/vod/download", func(c *gin.Context) {
		handlers.HandleVODDownload(c.Writer, c.Request)
	})
	r.GET("/api/vod/info", func(c *gin.Context) {
		handlers.HandleVODInfo(c.Writer, c.Request)
	})

}
