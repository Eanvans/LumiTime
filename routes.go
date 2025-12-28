package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

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

	// Search Twitch channels
	r.GET("/api/search/twitch", func(c *gin.Context) {
		query := c.Query("q")
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "查询参数不能为空"})
			return
		}

		// 调用Twitch API搜索频道
		results, err := searchTwitchChannels(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "查询失败",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, results)
	})
}

// TwitchChannel represents a Twitch channel search result
type TwitchChannel struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	Description     string `json:"description"`
	ProfileImageURL string `json:"profile_image_url"`
	ViewCount       int    `json:"view_count"`
	FollowerCount   int    `json:"follower_count,omitempty"`
}

// TwitchSearchResponse represents Twitch API search response
type TwitchSearchResponse struct {
	Data []TwitchChannel `json:"data"`
}

// searchTwitchChannels searches for Twitch channels
func searchTwitchChannels(query string) ([]TwitchChannel, error) {
	// 注意：这里需要Twitch API的Client ID和Access Token
	// 你需要在 https://dev.twitch.tv/ 注册应用获取
	clientID := "qgjdb6lpqtvo67bsisvojzpz9zmcan"
	accessToken := "n0i4mc6zvorjv4i8gjkydimaozhkks"

	// 如果没有配置Twitch API凭证，返回模拟数据
	if clientID == "your_twitch_client_id" {
		return getMockTwitchResults(query), nil
	}

	apiURL := fmt.Sprintf("https://api.twitch.tv/helix/search/channels?query=%s", url.QueryEscape(query))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Client-ID", clientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Twitch API error: %s, body: %s", resp.Status, string(body))
	}

	var searchResp TwitchSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	return searchResp.Data, nil
}

// getMockTwitchResults returns mock data for testing
func getMockTwitchResults(query string) []TwitchChannel {
	mockChannels := []TwitchChannel{
		{
			ID:              "1",
			Login:           "kanekolumi",
			DisplayName:     "Kaneko Lumi",
			Description:     "Phase Connect VTuber - Strategy games, variety content, and cozy streams",
			ProfileImageURL: "https://static-cdn.jtvnw.net/jtv_user_pictures/kaneko-lumi-profile_image.png",
			ViewCount:       500000,
			FollowerCount:   50000,
		},
		{
			ID:              "2",
			Login:           query + "_stream",
			DisplayName:     query + " Stream",
			Description:     fmt.Sprintf("搜索结果：%s 的直播频道", query),
			ProfileImageURL: "https://api.dicebear.com/7.x/avataaars/svg?seed=" + query + "1",
			ViewCount:       150000,
			FollowerCount:   15000,
		},
		{
			ID:              "3",
			Login:           query + "_gaming",
			DisplayName:     query + " Gaming",
			Description:     fmt.Sprintf("%s 的游戏直播", query),
			ProfileImageURL: "https://api.dicebear.com/7.x/bottts/svg?seed=" + query + "2",
			ViewCount:       80000,
			FollowerCount:   8000,
		},
	}

	return mockChannels
}
