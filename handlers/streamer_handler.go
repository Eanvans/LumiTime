package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"subtuber-services/models"
	"subtuber-services/services"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	cache "github.com/patrickmn/go-cache"
)

var (
	// 主播数据缓存，60分钟过期，每10分钟清理一次过期项
	streamerCache = cache.New(60*time.Minute, 10*time.Minute)
	// 缓存键
	streamerCacheKey = "tracked_streamers"
	// 用于保护文件写入的互斥锁
	streamerFileMutex sync.Mutex
	// 最后持久化时间
	lastPersistTime time.Time
	// 持久化间隔（5分钟）
	persistInterval = 5 * time.Minute
	// 默认主播配置文件路径
	configPath = filepath.Join("App_Data", "tracked_streamers.json")
	// 初始化标志
	streamerServiceInitialized = false
	// 定期持久化的 ticker
	persistenceTicker *time.Ticker
	// 定期清理无订阅主播的 ticker
	cleanupTicker *time.Ticker
	// 清理间隔（默认24小时）
	cleanupInterval = 24 * time.Hour
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

func InitStreamerCache() error {
	// 预加载数据到缓存
	if _, err := GetTrackedStreamerData(); err != nil {
		log.Printf("警告: 预加载主播数据失败: %v", err)
	}

	// 启动定期持久化
	go startPeriodicPersistence()

	// 启动定期清理无订阅主播
	go startPeriodicCleanup()

	streamerServiceInitialized = true
	log.Printf("主播缓存服务已初始化，配置文件: %s, 持久化间隔: %v, 清理间隔: %v", configPath, persistInterval, cleanupInterval)
	return nil
}

// startPeriodicPersistence 启动定期持久化任务
func startPeriodicPersistence() {
	if persistenceTicker != nil {
		persistenceTicker.Stop()
	}

	persistenceTicker = time.NewTicker(persistInterval)
	defer persistenceTicker.Stop()

	log.Printf("启动主播数据定期持久化任务，间隔: %v", persistInterval)
	for range persistenceTicker.C {
		if err := persistStreamerDataIfNeeded(); err != nil {
			log.Printf("定期持久化主播数据失败: %v", err)
		}
	}
}

// startPeriodicCleanup 启动定期清理无订阅主播任务（每天凌晨2点执行）
func startPeriodicCleanup() {
	log.Println("启动无订阅主播定期清理任务，将在每天凌晨2点执行")

	for {
		// 计算到下一个凌晨2点的时间
		now := time.Now()
		nextCleanup := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())

		// 如果当前时间已经过了今天的2点，则安排到明天2点
		if now.After(nextCleanup) {
			nextCleanup = nextCleanup.Add(24 * time.Hour)
		}

		duration := nextCleanup.Sub(now)
		log.Printf("下次清理时间: %s (距离现在 %v)", nextCleanup.Format("2006-01-02 15:04:05"), duration)

		// 等待到指定时间
		time.Sleep(duration)

		// 执行清理任务
		log.Println("开始执行定时清理任务...")
		if err := cleanupUnsubscribedStreamers(); err != nil {
			log.Printf("定期清理无订阅主播失败: %v", err)
		}
	}
}

// cleanupUnsubscribedStreamers 清理没有任何订阅者的主播
func cleanupUnsubscribedStreamers() error {
	log.Println("开始检查并清理无订阅主播...")

	// 检查 RPC 服务是否可用
	streamerService := services.GetStreamerService()
	if streamerService == nil {
		log.Println("RPC 服务未初始化，跳过本次清理")
		return nil
	}

	// 获取所有追踪的主播
	config, err := GetTrackedStreamerData()
	if err != nil {
		return fmt.Errorf("获取主播列表失败: %w", err)
	}

	if len(config.Streamers) == 0 {
		log.Println("当前没有追踪的主播，无需清理")
		return nil
	}

	// 统计信息
	totalStreamers := len(config.Streamers)
	removedCount := 0
	errorCount := 0

	// 遍历所有主播，检查订阅者数量
	newStreamers := make([]models.StreamerInfo, 0, len(config.Streamers))
	for _, streamer := range config.Streamers {
		subscriberCount, err := services.GetStreamerSubscriberCount(streamer.ID)
		if err != nil {
			log.Printf("警告: 获取主播 %s (ID: %s) 的订阅者数量失败: %v", streamer.Name, streamer.ID, err)
			// 出错时保留该主播，避免误删
			newStreamers = append(newStreamers, streamer)
			errorCount++
			continue
		}

		// 如果有订阅者，保留该主播
		if subscriberCount > 0 {
			newStreamers = append(newStreamers, streamer)
			log.Printf("主播 %s (ID: %s) 有 %d 个订阅者，保留", streamer.Name, streamer.ID, subscriberCount)
		} else {
			// 没有订阅者，移除该主播
			log.Printf("主播 %s (ID: %s) 没有订阅者，从广场移除", streamer.Name, streamer.ID)
			removedCount++
		}
	}

	// 如果有主播被移除，更新配置
	if removedCount > 0 {
		config.Streamers = newStreamers
		if err := UpdateTrackedStreamerData(config); err != nil {
			return fmt.Errorf("更新主播配置失败: %w", err)
		}
		log.Printf("清理完成: 共检查 %d 个主播，移除 %d 个无订阅主播，%d 个检查失败",
			totalStreamers, removedCount, errorCount)
	} else {
		log.Printf("清理完成: 共检查 %d 个主播，没有需要移除的主播，%d 个检查失败",
			totalStreamers, errorCount)
	}

	return nil
}

// RemoveStreamerFromSquare 从广场移除指定主播（公开方法，可供其他模块调用）
func RemoveStreamerFromSquare(streamerID string) error {
	config, err := GetTrackedStreamerData()
	if err != nil {
		return fmt.Errorf("获取主播列表失败: %w", err)
	}

	// 查找并移除主播
	found := false
	newStreamers := make([]models.StreamerInfo, 0, len(config.Streamers))
	for _, streamer := range config.Streamers {
		if streamer.ID == streamerID {
			found = true
			log.Printf("从广场移除主播: %s (ID: %s)", streamer.Name, streamer.ID)
			continue
		}
		newStreamers = append(newStreamers, streamer)
	}

	if !found {
		return fmt.Errorf("未找到主播 ID: %s", streamerID)
	}

	config.Streamers = newStreamers

	// 更新配置
	err = UpdateTrackedStreamerData(config)
	if err != nil {
		return fmt.Errorf("更新主播配置失败: %w", err)
	}

	return nil
}

// StopStreamerCache 停止主播缓存服务（优雅关闭）
func StopStreamerCache() error {
	if !streamerServiceInitialized {
		return nil
	}

	log.Println("正在停止主播缓存服务...")

	// 停止定期持久化
	if persistenceTicker != nil {
		persistenceTicker.Stop()
	}

	// 停止定期清理
	if cleanupTicker != nil {
		cleanupTicker.Stop()
	}

	// 最后一次持久化
	if err := persistStreamerDataIfNeeded(); err != nil {
		log.Printf("最终持久化失败: %v", err)
		return err
	}

	streamerServiceInitialized = false
	log.Println("主播缓存服务已停止")
	return nil
}

// persistStreamerDataIfNeeded 如果缓存有变化则持久化
func persistStreamerDataIfNeeded() error {
	data, found := streamerCache.Get(streamerCacheKey)
	if !found {
		return nil // 缓存中没有数据，无需持久化
	}

	config, ok := data.(*models.TrackedStreamers)
	if !ok {
		return nil
	}

	return persistStreamerData(config)
}

// persistStreamerData 持久化主播数据到文件
func persistStreamerData(config *models.TrackedStreamers) error {
	streamerFileMutex.Lock()
	defer streamerFileMutex.Unlock()

	// 确保目录存在
	if err := os.MkdirAll("App_Data", 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return err
	}

	lastPersistTime = time.Now()
	log.Printf("主播数据已持久化到文件，共 %d 个主播", len(config.Streamers))
	return nil
}

// GetTrackedStreamerData 获取主播广场的所有主播数据（使用缓存）
// 注意：返回的是指向缓存数据的指针，直接修改会影响缓存
// 如果需要修改数据，请使用 UpdateTrackedStreamerData 方法确保数据一致性
func GetTrackedStreamerData() (*models.TrackedStreamers, error) {
	// 先从缓存获取
	if cached, found := streamerCache.Get(streamerCacheKey); found {
		if config, ok := cached.(*models.TrackedStreamers); ok {
			log.Printf("从缓存获取主播数据，共 %d 个主播", len(config.Streamers))
			return config, nil
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// 文件不存在时，创建新的空配置
		if os.IsNotExist(err) {
			config := &models.TrackedStreamers{
				Streamers: []models.StreamerInfo{},
			}
			// 存入缓存
			streamerCache.Set(streamerCacheKey, config, cache.DefaultExpiration)
			log.Printf("创建新的主播配置文件")
			return config, nil
		}
		return nil, err
	}

	var config models.TrackedStreamers
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// 存入缓存
	streamerCache.Set(streamerCacheKey, &config, cache.DefaultExpiration)
	log.Printf("从文件加载主播数据到缓存，共 %d 个主播", len(config.Streamers))

	return &config, nil
}

// UpdateTrackedStreamerData 更新主播数据到缓存并持久化
// 使用此方法确保缓存和文件的数据一致性
func UpdateTrackedStreamerData(config *models.TrackedStreamers) error {
	if config == nil {
		return fmt.Errorf("配置数据不能为空")
	}

	// 更新缓存
	streamerCache.Set(streamerCacheKey, config, cache.DefaultExpiration)

	// 立即持久化到文件
	return persistStreamerData(config)
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
	config, err := GetTrackedStreamerData()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取主播广场列表失败: " + err.Error(),
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
	config, err := GetTrackedStreamerData()
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

	// 更新缓存并持久化
	return UpdateTrackedStreamerData(config)
}

// addStreamerToConfig 添加主播到配置文件
func addStreamerToConfig(rawStreamerID, streamerName string, platforms []models.StreamerPlatform) error {
	// 保障 ID 统一小写
	streamerID := strings.ToLower(rawStreamerID)

	config, err := GetTrackedStreamerData()
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

	// 更新缓存并持久化
	return UpdateTrackedStreamerData(config)
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

	// 从 cookie 获取用户信息
	userHash, err := getUserHashFromCookie(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录或登录已过期",
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
	rawStreamerID := req.Streamer_Id
	streamerID := strings.ToLower(req.Streamer_Id)
	// 移除可能存在的 @ 符号，确保 ID 格式统一
	streamerID = strings.TrimPrefix(streamerID, "@")
	// 如果主播不在总体追踪列表中添加到追踪列表
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

	// 如果主播在总体追踪列表中，直接创建订阅
	if isStreamerSubscribed(config, streamerID) {
		// 主播已存在，检查是否已有该平台
		if hasPlatform(config, streamerID, platform) {
			// 平台已存在，直接创建订阅
			err := checkAndSubscribeStreamer(userHash, streamerID)
			if err != nil {
				log.Printf("创建订阅失败: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": "订阅失败: " + err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, models.SubscriptionResponse{
				Success: true,
				Message: "订阅成功",
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

		// 创建订阅
		err := checkAndSubscribeStreamer(userHash, streamerID)
		if err != nil {
			log.Printf("创建订阅失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "订阅失败: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, models.SubscriptionResponse{
			Success: true,
			Message: "订阅成功",
		})
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

		// 调用 RPC 服务创建订阅
		_, err := services.CreateSubscription(userHash, streamerID)
		if err != nil {
			log.Printf("创建订阅失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "订阅失败: " + err.Error(),
			})
			return
		}
	}

	// 根据平台触发相应的监控服务
	if strings.ToLower(platform) == "twitch" {
		// 触发 TwitchMonitor 重新加载主播列表
		monitor := GetTwitchMonitor()
		if monitor != nil {
			// 异步触发对新主播的聊天记录下载和分析
			go func(username string) {
				// 确保有有效的token
				if err := monitor.ensureValidToken(); err != nil {
					log.Printf("获取token失败，无法检查主播 %s 状态: %v", username, err)
					return
				}

				userInfo, err := monitor.getUserInfo(username)
				if err != nil {
					log.Printf("获取 %s 用户信息失败: %v", username, err)
					// 检查是否是用户不存在的错误
					if strings.Contains(err.Error(), "用户不存在") {
						log.Printf("主播 %s (用户名: %s) 不存在", username, username)
						if removeErr := monitor.removeStreamerFromConfig(username); removeErr != nil {
							log.Printf("移除主播 %s 失败: %v", username, removeErr)
						} else {
							log.Printf("已成功移除主播 %s", username)
							// 从内存中移除主播状态
							monitor.mu.Lock()
							delete(monitor.streamerStatus, username)
							monitor.mu.Unlock()
						}
					}
				} else if userInfo.ProfileImageURL != "" {
					if err := monitor.updateStreamerProfileImage(userInfo.Login, username, userInfo.ProfileImageURL); err != nil {
						log.Printf("更新 %s 头像URL失败: %v", username, err)
					}
				}

				// 检查主播是否在直播
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
			// 异步触发对新频道的视频下载和分析
			go func(username string) {
				log.Printf("开始处理YouTube频道 %s ...", username)

				// 首先尝试通过用户名获取频道ID
				var channelID string
				var err error

				// 如果用户名以@开头，需要通过API获取频道ID
				if strings.HasPrefix(username, "@") || !strings.HasPrefix(username, "UC") {
					// 使用带缓存的方法获取频道ID
					channelID, err = monitor.getChannelIDByUsernameAndCache(username, username)
					if err != nil {
						log.Printf("获取频道ID失败 (%s): %v", username, err)
						return
					}

					// 获取并更新头像
					channelInfo, err := monitor.getChannelInfo(channelID)
					if err != nil {
						log.Printf("获取 %s 频道信息失败: %v", username, err)
					} else if channelInfo.ProfileImageURL != "" {
						if err := monitor.updateChannelProfileImage(channelInfo.ID, username, channelInfo.ProfileImageURL); err != nil {
							log.Printf("更新 %s 头像URL失败: %v", username, err)
						}
					}
				} else {
					// 已经是频道ID格式
					channelID = username
				}

				log.Printf("频道 %s 的ID为: %s", username, channelID)

				// 检查频道是否在直播
				stream, err := monitor.CheckLiveStatusByChannelID(channelID)
				if err != nil {
					log.Printf("检查YouTube频道 %s 直播状态失败: %v", username, err)
					return
				}

				if stream != nil {
					// 频道正在直播，不立即下载分析
					log.Printf("🔴 YouTube频道 %s 当前正在直播，将在直播结束后自动下载和分析", username)
					return
				}

				// 频道离线，开始处理最近的VOD
				log.Printf("开始处理YouTube频道 %s 的最近VOD...", username)
				monitor.ProcessRecentVOD(channelID, username)
				log.Printf("✅ 完成YouTube频道 %s 的VOD处理", username)
			}(rawStreamerID)
		}
	}

	c.JSON(http.StatusOK, models.SubscriptionResponse{
		Success: true,
		Message: "订阅成功，正在后台分析最近的视频，如果正在直播将会在本次直播结束后自动分析。",
	})
}

func checkAndSubscribeStreamer(userHash, streamerID string) error {
	// 检查是否已经订阅
	exists, err := services.CheckSubscriptionExists(userHash, streamerID)
	if err != nil {
		log.Printf("检查订阅状态失败: %v", err)
		// 继续执行，尝试创建订阅
	} else if exists {
		return nil // 已经订阅，无需重复订阅
	}

	// 调用 RPC 服务创建订阅
	_, err = services.CreateSubscription(userHash, streamerID)
	if err != nil {
		log.Printf("创建订阅失败: %v", err)
		return fmt.Errorf("订阅失败: " + err.Error())
	}
	return nil
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
