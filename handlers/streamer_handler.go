package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
		if strings.EqualFold(streamer.ID, streamerID) {
			return true
		}
	}
	return false
}

// hasPlatform 检查主播是否已有指定平台
func hasPlatform(config *models.TrackedStreamers, streamerID, platform string) bool {
	for _, streamer := range config.Streamers {
		if strings.EqualFold(streamer.ID, streamerID) {
			for _, p := range streamer.Platforms {
				if strings.EqualFold(p.Platform, platform) {
					return true
				}
			}
			return false
		}
	}
	return false
}

// addPlatformToStreamer 为已存在的主播添加新平台
func addPlatformToStreamer(streamerID string, newPlatform models.StreamerPlatform) error {
	config, err := loadOrCreateTrackedStreamers()
	if err != nil {
		return err
	}

	// 找到主播并添加平台
	for i, streamer := range config.Streamers {
		if strings.EqualFold(streamer.ID, streamerID) {
			config.Streamers[i].Platforms = append(config.Streamers[i].Platforms, newPlatform)
			break
		}
	}

	// 保存到文件
	configPath := filepath.Join("App_Data", "tracked_streamers.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
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

// SubscribeStreamer 在主播广场订阅新的主播
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
	// 移除可能存在的 @ 符号，确保 ID 格式统一
	streamerID = strings.TrimPrefix(streamerID, "@")
	platform := req.Platform

	// 准备平台信息
	var newPlatform models.StreamerPlatform
	if strings.ToLower(platform) == "twitch" {
		newPlatform = models.StreamerPlatform{
			Platform: "twitch",
			URL:      "https://www.twitch.tv/" + streamerID,
		}
	} else if strings.ToLower(platform) == "youtube" {
		newPlatform = models.StreamerPlatform{
			Platform: "youtube",
			URL:      "https://www.youtube.com/@" + streamerID,
		}
	} else {
		// 不支持的平台
		c.JSON(http.StatusBadRequest, models.SubscriptionResponse{
			Success: false,
			Message: "暂时不支持的平台: " + platform,
		})
		return
	}

	// 检查主播是否已订阅
	if isStreamerSubscribed(config, streamerID) {
		// 主播已存在，检查是否已有该平台
		if hasPlatform(config, streamerID, platform) {
			c.JSON(http.StatusOK, models.SubscriptionResponse{
				Success: true,
				Message: "该主播的此平台已在订阅列表中",
			})
			return
		}

		// 平台不存在，添加新平台
		if err := addPlatformToStreamer(streamerID, newPlatform); err != nil {
			c.JSON(http.StatusInternalServerError, models.SubscriptionResponse{
				Success: false,
				Message: "添加平台失败: " + err.Error(),
			})
			return
		}
		log.Printf("为主播 %s 添加了新平台: %s", streamerID, platform)
	} else {
		// 主播不存在，添加新主播
		platforms := []models.StreamerPlatform{newPlatform}
		if err := addStreamerToConfig(streamerID, streamerID, platforms); err != nil {
			c.JSON(http.StatusInternalServerError, models.SubscriptionResponse{
				Success: false,
				Message: "添加主播失败: " + err.Error(),
			})
			return
		}
	}

	// 根据平台触发相应的监控服务
	if strings.ToLower(platform) == "twitch" {
		// 触发 TwitchMonitor 重新加载主播列表
		monitor := GetTwitchMonitor()
		if monitor != nil {
			if err := monitor.LoadStreamers(); err != nil {
				c.JSON(http.StatusInternalServerError, models.SubscriptionResponse{
					Success: false,
					Message: "重新加载主播列表失败: " + err.Error(),
				})
				return
			}

			// 异步触发对新主播的聊天记录下载和分析
			go func(username string) {
				// 确保有有效的token
				if err := monitor.ensureValidToken(); err != nil {
					log.Printf("获取token失败，无法检查主播 %s 状态: %v", username, err)
					return
				}

				// 先检查主播是否在直播
				stream, err := monitor.CheckStreamStatusByUsername(username)
				if err != nil {
					log.Printf("检查主播 %s 直播状态失败: %v", username, err)
					return
				}

				if stream != nil {
					// 主播正在直播，不立即下载分析
					log.Printf("🔴 主播 %s 当前正在直播，将在直播结束后自动下载和分析", username)
					return
				}

				// 主播离线，开始下载和分析历史视频
				log.Printf("开始下载和分析主播 %s 的历史视频...", username)
				newResults := monitor.GetVideoCommentsForStreamer(username)
				if len(newResults) > 0 {
					log.Printf("📊 完成新主播 %s 的 %d 个视频的分析", username, len(newResults))
					for _, result := range newResults {
						log.Printf("  - VideoID: %s, 热点时刻: %d", result.VideoID, len(result.HotMoments))
					}
				}
			}(streamerID)
		}
	} else if strings.ToLower(platform) == "youtube" {
		// 触发 YouTubeMonitor 重新加载主播列表
		monitor := GetYouTubeMonitor()
		if monitor != nil {
			if err := monitor.LoadChannels(); err != nil {
				c.JSON(http.StatusInternalServerError, models.SubscriptionResponse{
					Success: false,
					Message: "重新加载频道列表失败: " + err.Error(),
				})
				return
			}

			// 异步触发对新频道的视频下载和分析

		}
	}

	c.JSON(http.StatusOK, models.SubscriptionResponse{
		Success: true,
		Message: "订阅成功，正在后台分析最近的视频，如果正在直播将会在本次直播结束后自动分析。",
	})
}

// GetStreamingStatus 获取主播的跨平台直播状态
// 同时检查 Twitch 和 YouTube 平台，只要任一平台在直播就返回 true
func GetStreamingStatus(c *gin.Context) {
	streamerID := c.Param("streamer_id")
	if streamerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "主播ID不能为空",
		})
		return
	}

	// 移除可能存在的 @ 符号
	streamerID = strings.TrimPrefix(streamerID, "@")

	// 检查 Twitch 状态
	var twitchLive bool
	var twitchStream *models.TwitchStatusResponse
	twitchMonitor := GetTwitchMonitor()
	if twitchMonitor != nil {
		twitchStatus := twitchMonitor.GetStreamerStatus(streamerID)
		if twitchStatus != nil && twitchStatus.IsLive {
			twitchLive = true
			twitchStream = twitchStatus
		}
	}

	// 检查 YouTube 状态
	var youtubeLive bool
	var youtubeStream *models.YouTubeStatusResponse
	youtubeMonitor := GetYouTubeMonitor()
	if youtubeMonitor != nil {
		youtubeStatus := youtubeMonitor.GetChannelStatus(streamerID)
		if youtubeStatus != nil && youtubeStatus.IsLive {
			youtubeLive = true
			youtubeStream = youtubeStatus
		}
	}

	// 判断是否有任一平台在直播
	isLive := twitchLive || youtubeLive

	// 构建响应
	response := gin.H{
		"success":       true,
		"streamer_name": streamerID,
		"is_live":       isLive,
		"platforms":     gin.H{},
	}

	// 添加平台详情
	platforms := gin.H{}
	if twitchMonitor != nil {
		platforms["twitch"] = gin.H{
			"is_live": twitchLive,
			"stream":  twitchStream,
		}
	}
	if youtubeMonitor != nil {
		platforms["youtube"] = gin.H{
			"is_live": youtubeLive,
			"stream":  youtubeStream,
		}
	}
	response["platforms"] = platforms

	c.JSON(http.StatusOK, response)
}
