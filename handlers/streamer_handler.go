package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"subtuber-services/models"
	"subtuber-services/services"

	"github.com/gin-gonic/gin"
)

// StreamerInfo 主播信息结构
type StreamerInfo struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Title           string `json:"title"`
	Platform        string `json:"platform"`
	DurationSeconds string `json:"duration_seconds"`
	CreatedAt       string `json:"created_at"`
}

// GetStreamerByID 根据ID查询主播信息
func GetStreamerVODsByStreamerID(c *gin.Context) {
	// 从 URL 参数获取主播 ID (string 类型)
	streamerID := c.Param("id")
	if streamerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "主播ID不能为空",
		})
		return
	}

	// 获取 streamer service
	streamerService := services.GetStreamerService()
	if streamerService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "主播服务未初始化",
		})
		return
	}

	// 调用服务层查询主播信息
	streamer, err := streamerService.ListStreamerVODs(streamerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "查询主播信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"vods":    streamer.Streamers,
	})
}

// ListStreamers 查询主播列表
func ListStreamers(c *gin.Context) {
	// 读取跟踪主播配置文件
	configPath := filepath.Join("App_Data", "tracked_streamers.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "读取主播配置文件失败: " + err.Error(),
		})
		return
	}

	var config models.TrackedStreamers
	if err := json.Unmarshal(data, &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "解析主播配置文件失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"streamers": config.Streamers,
		"total":     len(config.Streamers),
	})
}

// 临时存储订阅信息（实际项目中应使用数据库）
var subscriptions = make(map[string][]models.Subscription)
var subscriptionIDCounter = 1

// loadOrCreateTrackedStreamers 加载或创建主播配置文件
func loadOrCreateTrackedStreamers() (*models.TrackedStreamers, error) {
	configPath := filepath.Join("App_Data", "tracked_streamers.json")

	// 确保目录存在
	if err := os.MkdirAll("App_Data", 0755); err != nil {
		return nil, err
	}

	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// 文件不存在，创建新的配置
		config := &models.TrackedStreamers{
			Streamers: []models.StreamerInfo{},
		}
		// 写入文件
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return nil, err
		}
		return config, nil
	}

	// 文件存在，读取并解析
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config models.TrackedStreamers
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// isStreamerSubscribed 检查主播是否已订阅
func isStreamerSubscribed(config *models.TrackedStreamers, streamerID string) bool {
	for _, streamer := range config.Streamers {
		if streamer.ID == streamerID {
			return true
		}
	}
	return false
}

// addStreamerToConfig 添加主播到配置文件
func addStreamerToConfig(streamerID, streamerName string, platforms []models.StreamerPlatform) error {
	config, err := loadOrCreateTrackedStreamers()
	if err != nil {
		return err
	}

	// 检查是否已存在
	if isStreamerSubscribed(config, streamerID) {
		return nil // 已存在，不需要重复添加
	}

	// 添加新主播
	newStreamer := models.StreamerInfo{
		ID:        streamerID,
		Name:      streamerName,
		Platforms: platforms,
	}
	config.Streamers = append(config.Streamers, newStreamer)

	// 保存到文件
	configPath := filepath.Join("App_Data", "tracked_streamers.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// SubscribeStreamer 订阅新的主播
func SubscribeStreamer(c *gin.Context) {
	var req models.SubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.SubscriptionResponse{
			Success: false,
			Message: "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 加载或创建配置文件
	config, err := loadOrCreateTrackedStreamers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SubscriptionResponse{
			Success: false,
			Message: "加载配置文件失败: " + err.Error(),
		})
		return
	}

	// 使用 streamer 字段作为主播ID
	streamerID := req.Streamer_Id

	// 检查主播是否已订阅
	if isStreamerSubscribed(config, streamerID) {
		c.JSON(http.StatusOK, models.SubscriptionResponse{
			Success: true,
			Message: "该主播已在订阅列表中",
		})
		return
	}

	// 添加主播到配置文件
	// 默认添加 Twitch 平台（可根据需要扩展）
	platforms := []models.StreamerPlatform{
		{
			Platform: "twitch",
			URL:      "https://www.twitch.tv/" + streamerID,
		},
	}

	if err := addStreamerToConfig(streamerID, streamerID, platforms); err != nil {
		c.JSON(http.StatusInternalServerError, models.SubscriptionResponse{
			Success: false,
			Message: "添加主播失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.SubscriptionResponse{
		Success: true,
		Message: "订阅成功",
	})
}
