package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"subtuber-services/models"
	"subtuber-services/services"

	"github.com/gin-gonic/gin"
)

var (
	debugMode         = false
	fetchVodCount     = "1" // 每次获取的VOD数量
	twitchMonitor     *TwitchMonitor
	twitchMonitorOnce sync.Once
	defaultPeakParams = PeakDetectionParams{
		WindowsLen:  420, // 7分钟窗口
		Thr:         0.9, // 90百分位阈值
		SearchRange: 210, // 3.5分钟搜索范围
	}
)

// TwitchConfig Twitch配置
type TwitchConfig struct {
	ClientID       string `mapstructure:"client_id"`
	ClientSecret   string `mapstructure:"client_secret"`
	MinInterval    int    `mapstructure:"min_interval_seconds"`    // 最小检查间隔（秒）
	MaxInterval    int    `mapstructure:"max_interval_seconds"`    // 最大检查间隔（秒）
	ReloadInterval int    `mapstructure:"reload_interval_minutes"` // 重新加载主播列表的间隔（分钟）
}

// StreamerStatus 主播状态
type StreamerStatus struct {
	isLive       bool
	latestStatus *models.TwitchStatusResponse
	lastChecked  time.Time
}

// TwitchMonitor Twitch监控服务
type TwitchMonitor struct {
	config         TwitchConfig
	accessToken    string
	tokenExpiry    time.Time
	mu             sync.RWMutex
	streamers      []models.StreamerInfo      // 追踪的主播列表
	streamerStatus map[string]*StreamerStatus // 主播ID -> 状态
	lastReloadTime time.Time                  // 上次重新加载配置的时间
	stopCh         chan struct{}
}

// InitTwitchMonitor 初始化Twitch监控服务
func InitTwitchMonitor(config TwitchConfig) *TwitchMonitor {
	twitchMonitorOnce.Do(func() {
		// 设置默认值
		if config.MinInterval == 0 {
			config.MinInterval = 30 // 默认最小30秒
		}
		if config.MaxInterval == 0 {
			config.MaxInterval = 120 // 默认最大120秒
		}
		if config.ReloadInterval == 0 {
			config.ReloadInterval = 10 // 默认每10分钟重新加载一次
		}

		twitchMonitor = &TwitchMonitor{
			config:         config,
			streamerStatus: make(map[string]*StreamerStatus),
			stopCh:         make(chan struct{}),
		}

		// 初始加载主播列表
		if err := twitchMonitor.loadStreamers(); err != nil {
			log.Printf("警告: 无法加载主播列表: %v", err)
		}
	})
	return twitchMonitor
}

// GetTwitchMonitor 获取Twitch监控实例
func GetTwitchMonitor() *TwitchMonitor {
	return twitchMonitor
}

// LoadStreamers 从配置文件加载主播列表
func (tm *TwitchMonitor) loadStreamers() error {
	trackedStreamers, err := GetTrackedStreamerData()
	if err != nil {
		return fmt.Errorf("读取主播配置文件失败: %w", err)
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.streamers = trackedStreamers.Streamers
	tm.lastReloadTime = time.Now()

	// 初始化新主播的状态
	for _, streamer := range tm.streamers {
		if _, exists := tm.streamerStatus[streamer.ID]; !exists {
			tm.streamerStatus[streamer.ID] = &StreamerStatus{
				isLive:      false,
				lastChecked: time.Time{},
			}
		}
	}

	log.Printf("已加载 %d 个主播", len(tm.streamers))
	return nil
}

// shouldReloadStreamers 检查是否需要重新加载主播列表
func (tm *TwitchMonitor) shouldReloadStreamers() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.lastReloadTime.IsZero() {
		return true
	}

	reloadInterval := time.Duration(tm.config.ReloadInterval) * time.Minute
	return time.Since(tm.lastReloadTime) >= reloadInterval
}

// Start 启动监控服务
func (tm *TwitchMonitor) Start() {
	tm.mu.RLock()
	streamerCount := len(tm.streamers)
	tm.mu.RUnlock()

	log.Printf("启动Twitch监控服务，正在追踪 %d 个主播", streamerCount)
	go tm.monitorLoop()
}

// Stop 停止监控服务
func (tm *TwitchMonitor) Stop() {
	close(tm.stopCh)
	log.Println("Twitch监控服务已停止")
}

// monitorLoop 监控循环
func (tm *TwitchMonitor) monitorLoop() {
	// 初始化时立即检查一次所有主播
	tm.checkAllStreamers()

	for {
		// 检查是否需要重新加载主播列表
		if tm.shouldReloadStreamers() {
			log.Println("重新加载主播列表...")
			if err := tm.loadStreamers(); err != nil {
				log.Printf("重新加载主播列表失败: %v", err)
			}
		}

		// 随机间隔时间
		interval := tm.getRandomInterval()
		log.Printf("下次检查将在 %d 秒后进行", interval)

		select {
		case <-time.After(time.Duration(interval) * time.Second):
			tm.checkAllStreamers()
		case <-tm.stopCh:
			return
		}
	}
}

// getRandomInterval 获取随机检查间隔
func (tm *TwitchMonitor) getRandomInterval() int {
	min := tm.config.MinInterval
	max := tm.config.MaxInterval
	if max <= min {
		return min
	}
	return min + rand.Intn(max-min+1)
}

// checkAllStreamers 检查所有主播的状态
func (tm *TwitchMonitor) checkAllStreamers() {
	// 确保有有效的访问令牌
	if err := tm.ensureValidToken(); err != nil {
		log.Printf("获取访问令牌失败: %v", err)
		return
	}

	tm.mu.RLock()
	streamers := make([]models.StreamerInfo, len(tm.streamers))
	copy(streamers, tm.streamers)
	tm.mu.RUnlock()

	if len(streamers) == 0 {
		log.Println("没有需要监控的主播")
		return
	}

	log.Printf("开始检查 %d 个主播的直播状态...", len(streamers))

	// 逐个检查主播状态
	for _, streamer := range streamers {
		tm.checkStreamerStatus(streamer)
		// 在检查之间添加短暂延迟，避免请求过于频繁
		time.Sleep(time.Duration(1+rand.Intn(3)) * time.Second)
	}
}

// checkStreamerStatus 检查单个主播的状态
func (tm *TwitchMonitor) checkStreamerStatus(streamer models.StreamerInfo) {
	// 从 platforms 中获取 twitch 用户名
	var twitchUsername string
	for _, platform := range streamer.Platforms {
		if platform.Platform == "twitch" {
			// 从 URL 中提取用户名，例如 https://www.twitch.tv/kanekolumi
			parts := strings.Split(platform.URL, "/")
			if len(parts) > 0 {
				twitchUsername = parts[len(parts)-1]
			}
			break
		}
	}

	if twitchUsername == "" {
		log.Printf("主播 %s 没有配置 Twitch 平台", streamer.Name)
		return
	}

	log.Printf("正在检查 %s 的直播状态...", streamer.Name)

	// 获取用户信息并更新头像URL到配置文件
	go func() {
		userInfo, err := tm.getUserInfo(twitchUsername)
		if err != nil {
			log.Printf("获取 %s 用户信息失败: %v", streamer.Name, err)
			// 检查是否是用户不存在的错误
			if strings.Contains(err.Error(), "用户不存在") {
				log.Printf("主播 %s (用户名: %s) 不存在，将从配置中移除", streamer.Name, twitchUsername)
				if removeErr := tm.removeStreamerFromConfig(streamer.ID); removeErr != nil {
					log.Printf("移除主播 %s 失败: %v", streamer.Name, removeErr)
				} else {
					log.Printf("已成功移除主播 %s", streamer.Name)
					// 从内存中移除主播状态
					tm.mu.Lock()
					delete(tm.streamerStatus, streamer.ID)
					tm.mu.Unlock()
				}
			}
		} else if userInfo.ProfileImageURL != "" {
			if err := tm.updateStreamerProfileImage(streamer.ID, twitchUsername, userInfo.ProfileImageURL); err != nil {
				log.Printf("更新 %s 头像URL失败: %v", streamer.Name, err)
			}
		}
	}()

	// 检查直播状态
	stream, err := tm.CheckStreamStatusByUsername(twitchUsername)
	if err != nil {
		log.Printf("检查 %s 直播状态失败: %v", streamer.Name, err)
		return
	}

	// 获取之前的状态
	tm.mu.Lock()
	status, exists := tm.streamerStatus[streamer.ID]
	if !exists {
		status = &StreamerStatus{
			isLive:      false,
			lastChecked: time.Time{},
		}
		tm.streamerStatus[streamer.ID] = status
	}
	previousIsLive := status.isLive

	// 更新状态
	currentIsLive := stream != nil
	status.isLive = currentIsLive
	status.lastChecked = time.Now()
	status.latestStatus = &models.TwitchStatusResponse{
		IsLive:       currentIsLive,
		StreamData:   stream,
		CheckedAt:    time.Now().Format(time.RFC3339),
		StreamerName: streamer.Name,
	}
	tm.mu.Unlock()

	if stream != nil {
		log.Printf("🔴 %s 正在直播！标题: %s, 观众: %d",
			stream.UserName, stream.Title, stream.ViewerCount)
	} else {
		log.Printf("⚫ %s 当前离线", streamer.Name)

		// 检测从直播状态变为离线状态
		if previousIsLive {
			log.Printf("🎬 检测到 %s 的直播结束，开始自动下载聊天记录...", streamer.Name)

			// 检查并下载最近的聊天记录进行分析
			go func(username string) {
				newResults := tm.GetVideoCommentsForStreamer(username)
				if len(newResults) > 0 {
					log.Printf("📊 完成 %s 的 %d 个新视频的分析", username, len(newResults))
					for _, result := range newResults {
						log.Printf("  - VideoID: %s, 热点时刻: %d", result.VideoID, len(result.HotMoments))
					}
				}
			}(twitchUsername)
		}
	}
}

// checkAndUpdate 检查并更新状态（保留用于向后兼容）
func (tm *TwitchMonitor) checkAndUpdate() {
	tm.checkAllStreamers()
}

// ensureValidToken 确保有有效的访问令牌
func (tm *TwitchMonitor) ensureValidToken() error {
	tm.mu.RLock()
	if tm.accessToken != "" && time.Now().Before(tm.tokenExpiry) {
		tm.mu.RUnlock()
		return nil
	}
	tm.mu.RUnlock()

	// 需要获取新令牌
	token, expiresIn, err := tm.getAccessToken()
	if err != nil {
		return err
	}

	tm.mu.Lock()
	tm.accessToken = token
	tm.tokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	tm.mu.Unlock()

	log.Println("成功获取新的访问令牌")
	return nil
}

// getAccessToken 获取OAuth访问令牌
func (tm *TwitchMonitor) getAccessToken() (string, int, error) {
	url := fmt.Sprintf("https://id.twitch.tv/oauth2/token?client_id=%s&client_secret=%s&grant_type=client_credentials",
		tm.config.ClientID, tm.config.ClientSecret)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	var tokenResp models.TwitchTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", 0, err
	}

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}

// checkStreamStatus 检查直播状态（保留用于向后兼容）
func (tm *TwitchMonitor) checkStreamStatus() (*models.TwitchStreamData, error) {
	// 如果有主播列表，检查第一个主播
	tm.mu.RLock()
	if len(tm.streamers) > 0 {
		for _, platform := range tm.streamers[0].Platforms {
			if platform.Platform == "twitch" {
				parts := strings.Split(platform.URL, "/")
				if len(parts) > 0 {
					username := parts[len(parts)-1]
					tm.mu.RUnlock()
					return tm.CheckStreamStatusByUsername(username)
				}
			}
		}
	}
	tm.mu.RUnlock()

	return nil, fmt.Errorf("没有配置主播")
}

// CheckStreamStatusByUsername 根据用户名检查直播状态
func (tm *TwitchMonitor) CheckStreamStatusByUsername(username string) (*models.TwitchStreamData, error) {
	url := fmt.Sprintf("https://api.twitch.tv/helix/streams?user_login=%s", username)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	tm.mu.RLock()
	accessToken := tm.accessToken
	tm.mu.RUnlock()

	req.Header.Set("Client-ID", tm.config.ClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var streamResp models.TwitchStreamResponse
	if err := json.Unmarshal(body, &streamResp); err != nil {
		return nil, err
	}

	if len(streamResp.Data) > 0 {
		return &streamResp.Data[0], nil
	}

	return nil, nil
}

// GetLatestStatus 获取最新的直播状态（返回所有主播的状态）
func (tm *TwitchMonitor) GetLatestStatus() map[string]*models.TwitchStatusResponse {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make(map[string]*models.TwitchStatusResponse)
	for id, status := range tm.streamerStatus {
		if status.latestStatus != nil {
			result[id] = status.latestStatus
		}
	}
	return result
}

// GetStreamerStatus 获取指定主播的状态
func (tm *TwitchMonitor) GetStreamerStatus(streamerID string) *models.TwitchStatusResponse {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if status, exists := tm.streamerStatus[streamerID]; exists {
		return status.latestStatus
	}
	return nil
}

// === HTTP Handlers ===

// GetTwitchStatus 获取Twitch直播状态的HTTP处理器
func GetTwitchStatus(c *gin.Context) {
	monitor := GetTwitchMonitor()
	if monitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Twitch监控服务未启动",
		})
		return
	}

	// 检查是否指定了主播ID
	streamerID := c.Param("streamer_id")

	if streamerID != "" {
		// 获取指定主播的状态
		status := monitor.GetStreamerStatus(streamerID)
		if status == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "未找到该主播",
			})
			return
		}
		c.JSON(http.StatusOK, status)
	} else {
		// 获取所有主播的状态
		statuses := monitor.GetLatestStatus()
		if len(statuses) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"message": "正在初始化，请稍后再试",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"streamers": statuses,
		})
	}
}

// CheckTwitchStatusNow 立即检查Twitch直播状态的HTTP处理器
func CheckTwitchStatusNow(c *gin.Context) {
	monitor := GetTwitchMonitor()
	if monitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Twitch监控服务未启动",
		})
		return
	}

	// 触发立即检查
	go monitor.checkAndUpdate()

	c.JSON(http.StatusOK, gin.H{
		"message": "已触发检查，请稍后查询结果",
	})
}

// getVideos 获取录像列表
func (tm *TwitchMonitor) getVideos(username, videoType, first, after string) (*models.TwitchVideosListResponse, error) {
	// 首先需要通过用户名获取用户ID
	// 因为这个用户ID是不会改变的，建议通过rpc进行吃持久化
	userID, err := tm.getUserID(username)
	if err != nil {
		return nil, fmt.Errorf("获取用户ID失败: %w", err)
	}

	// 构建URL - 使用user_id而不是user_login
	url := fmt.Sprintf("https://api.twitch.tv/helix/videos?user_id=%s&first=%s", userID, first)

	// 添加录像类型过滤
	if videoType != "all" {
		url += "&type=" + videoType
	}

	// 添加分页游标
	if after != "" {
		url += "&after=" + after
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	tm.mu.RLock()
	accessToken := tm.accessToken
	tm.mu.RUnlock()

	req.Header.Set("Client-ID", tm.config.ClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var videoResp models.TwitchVideoResponse
	if err := json.Unmarshal(body, &videoResp); err != nil {
		return nil, err
	}

	// 构建响应
	response := &models.TwitchVideosListResponse{
		Videos:       videoResp.Data,
		TotalCount:   len(videoResp.Data),
		HasMore:      videoResp.Pagination.Cursor != "",
		Cursor:       videoResp.Pagination.Cursor,
		StreamerName: username,
	}

	log.Printf("获取到 %s 的 %d 个录像", username, len(videoResp.Data))

	return response, nil
}

// getUserID 通过用户名获取用户ID（保留向后兼容）
func (tm *TwitchMonitor) getUserID(username string) (string, error) {
	userInfo, err := tm.getUserInfo(username)
	if err != nil {
		return "", err
	}
	return userInfo.ID, nil
}

// getUserInfo 通过用户名获取完整用户信息
func (tm *TwitchMonitor) getUserInfo(username string) (*models.TwitchUserData, error) {
	url := fmt.Sprintf("https://api.twitch.tv/helix/users?login=%s", username)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	tm.mu.RLock()
	accessToken := tm.accessToken
	tm.mu.RUnlock()

	req.Header.Set("Client-ID", tm.config.ClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userResp models.TwitchUserResponse
	if err := json.Unmarshal(body, &userResp); err != nil {
		return nil, err
	}

	if len(userResp.Data) == 0 {
		return nil, fmt.Errorf("用户不存在: %s", username)
	}

	return &userResp.Data[0], nil
}

// updateStreamerProfileImage 更新主播头像URL到配置文件
func (tm *TwitchMonitor) updateStreamerProfileImage(streamerID, username, imageURL string) error {
	if imageURL == "" {
		return fmt.Errorf("头像URL为空")
	}

	// 读取配置文件
	trackedStreamers, err := GetTrackedStreamerData()
	if err != nil {
		return fmt.Errorf("读取主播配置文件失败: %w", err)
	}

	// 查找并更新主播信息
	updated := false
	for i := range trackedStreamers.Streamers {
		if trackedStreamers.Streamers[i].ID == streamerID {
			// 只在头像URL有变化时更新
			if trackedStreamers.Streamers[i].ProfileImageURL == "" {
				trackedStreamers.Streamers[i].ProfileImageURL = imageURL
				updated = true
				log.Printf("已更新 %s 的头像URL: %s", username, imageURL)
			}
			break
		}
	}

	if !updated {
		return nil // 没有变化，不需要写入
	}

	// 写回配置文件
	UpdateTrackedStreamerData(trackedStreamers)

	return nil
}

// removeStreamerFromConfig 从配置文件中移除主播
func (tm *TwitchMonitor) removeStreamerFromConfig(streamerID string) error {
	// 读取配置文件
	trackedStreamers, err := GetTrackedStreamerData()
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 查找并移除主播
	found := false
	newStreamers := make([]models.StreamerInfo, 0, len(trackedStreamers.Streamers))
	for _, streamer := range trackedStreamers.Streamers {
		if streamer.ID == streamerID {
			found = true
			log.Printf("从配置中移除主播: %s (ID: %s)", streamer.Name, streamer.ID)
			continue
		}
		newStreamers = append(newStreamers, streamer)
	}

	if !found {
		return fmt.Errorf("未找到主播 ID: %s", streamerID)
	}

	trackedStreamers.Streamers = newStreamers

	// 写回配置文件
	err = UpdateTrackedStreamerData(trackedStreamers)
	if err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 更新内存中的主播列表
	tm.mu.Lock()
	newMemoryStreamers := make([]models.StreamerInfo, 0, len(tm.streamers))
	for _, streamer := range tm.streamers {
		if streamer.ID != streamerID {
			newMemoryStreamers = append(newMemoryStreamers, streamer)
		}
	}
	tm.streamers = newMemoryStreamers
	tm.mu.Unlock()

	return nil
}

// DownloadVODChat 下载VOD聊天记录的HTTP处理器
func DownloadVODChat(c *gin.Context) {
	monitor := GetTwitchMonitor()
	if monitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Twitch监控服务未启动",
		})
		return
	}

	var req models.TwitchChatDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 确保有有效的访问令牌
	if err := monitor.ensureValidToken(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取访问令牌失败: " + err.Error(),
		})
		return
	}

	// 下载聊天记录
	response, err := monitor.downloadChatComments(req.VideoID, req.StartTime, req.EndTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "下载聊天记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// SaveVODChatToFile 保存VOD聊天记录到文件
func SaveVODChatToFile(c *gin.Context) {
	monitor := GetTwitchMonitor()
	if monitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Twitch监控服务未启动",
		})
		return
	}

	var req models.TwitchChatDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 确保有有效的访问令牌
	if err := monitor.ensureValidToken(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取访问令牌失败: " + err.Error(),
		})
		return
	}

	// 下载聊天记录
	response, err := monitor.downloadChatComments(req.VideoID, req.StartTime, req.EndTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "下载聊天记录失败: " + err.Error(),
		})
		return
	}

	// 保存到文件
	filename := fmt.Sprintf("chat_%s_%s.json", req.VideoID, time.Now().Format("20060102_150405"))
	filepath := filepath.Join("./chat_logs", filename)

	// 确保目录存在
	if err := os.MkdirAll("./chat_logs", 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建目录失败: " + err.Error(),
		})
		return
	}

	// 将数据序列化为JSON
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "序列化JSON失败: " + err.Error(),
		})
		return
	}

	// 写入文件
	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "写入文件失败: " + err.Error(),
		})
		return
	}

	log.Printf("聊天记录已保存到文件: %s", filepath)

	c.JSON(http.StatusOK, gin.H{
		"message":        "聊天记录已成功保存",
		"filename":       filename,
		"filepath":       filepath,
		"total_comments": response.TotalComments,
		"video_id":       response.VideoID,
	})
}

// downloadChatComments 下载VOD聊天记录（使用GraphQL API）
func (m *TwitchMonitor) downloadChatComments(videoID string, startTime, endTime *float64) (*models.TwitchChatDownloadResponse, error) {
	const (
		gqlURL    = "https://gql.twitch.tv/gql"
		clientID  = "kd1unb4b3q4t58fwlpcbzcbnm76a8fp"
		operation = "VideoCommentsByOffsetOrCursor"
		sha256    = "b70a3591ff0f4e0313d126c6a1502d79a1c02baebb288227c582044aa76adf6a"
	)

	var allComments []models.TwitchChatComment
	var cursor string
	hasNextPage := true
	isFirstRequest := true

	log.Printf("开始下载 Video ID: %s 的聊天记录", videoID)

	// 获取视频信息
	videoInfo, err := m.getVideoInfo(videoID)
	if err != nil {
		log.Printf("获取视频信息失败: %v", err)
		// 继续下载聊天，即使获取视频信息失败
	}

	for hasNextPage {
		var requestBody map[string]interface{}

		if isFirstRequest {
			// 第一次请求使用 contentOffsetSeconds
			offsetSeconds := 0.0
			if startTime != nil {
				offsetSeconds = *startTime
			}

			requestBody = map[string]interface{}{
				"operationName": operation,
				"variables": map[string]interface{}{
					"videoID":              videoID,
					"contentOffsetSeconds": offsetSeconds,
				},
				"extensions": map[string]interface{}{
					"persistedQuery": map[string]interface{}{
						"version":    1,
						"sha256Hash": sha256,
					},
				},
			}
			isFirstRequest = false
		} else {
			// 后续请求使用 cursor 进行分页
			requestBody = map[string]interface{}{
				"operationName": operation,
				"variables": map[string]interface{}{
					"videoID": videoID,
					"cursor":  cursor,
				},
				"extensions": map[string]interface{}{
					"persistedQuery": map[string]interface{}{
						"version":    1,
						"sha256Hash": sha256,
					},
				},
			}
		}

		// 序列化请求体
		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("序列化请求失败: %w", err)
		}

		// 创建HTTP请求
		req, err := http.NewRequest("POST", gqlURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %w", err)
		}

		req.Header.Set("Client-ID", clientID)
		req.Header.Set("Content-Type", "application/json")

		// 发送请求
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求失败: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API返回错误状态 %d: %s", resp.StatusCode, string(body))
		}

		// 解析响应
		var gqlResp models.TwitchGQLCommentResponse
		if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}

		// 检查是否有评论数据
		if len(gqlResp.Data.Video.Comments.Edges) == 0 {
			log.Printf("没有更多评论数据，当前游标: %s", cursor)
			break
		}

		// 收集评论
		for _, edge := range gqlResp.Data.Video.Comments.Edges {
			node := edge.Node

			// 如果指定了结束时间，检查是否超出范围
			if endTime != nil && float64(node.ContentOffsetSeconds) > *endTime {
				hasNextPage = false
				break
			}

			// 如果指定了开始时间，只收集开始时间之后的评论
			if startTime != nil && float64(node.ContentOffsetSeconds) < *startTime {
				continue
			}

			// 转换为 TwitchChatComment 格式
			comment := convertGQLNodeToComment(node, videoID)
			allComments = append(allComments, comment)
			cursor = edge.Cursor
		}

		log.Printf("已获取 %d 条评论，总计: %d", len(gqlResp.Data.Video.Comments.Edges), len(allComments))

		// 检查是否有下一页
		hasNextPage = hasNextPage && gqlResp.Data.Video.Comments.PageInfo.HasNextPage

		// 避免请求过快
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("下载完成，共获取 %d 条评论", len(allComments))

	return &models.TwitchChatDownloadResponse{
		VideoID:       videoID,
		TotalComments: len(allComments),
		Comments:      allComments,
		VideoInfo:     videoInfo,
		DownloadedAt:  time.Now().Format(time.RFC3339),
	}, nil
}

// getVideoInfo 获取视频信息
func (m *TwitchMonitor) getVideoInfo(videoID string) (*models.TwitchVideoData, error) {
	if err := m.ensureValidToken(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	token := m.accessToken
	m.mu.RUnlock()

	url := fmt.Sprintf("https://api.twitch.tv/helix/videos?id=%s", videoID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Client-ID", m.config.ClientID)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取视频信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var videoResp models.TwitchVideoResponse
	if err := json.NewDecoder(resp.Body).Decode(&videoResp); err != nil {
		return nil, err
	}

	if len(videoResp.Data) == 0 {
		return nil, fmt.Errorf("未找到视频 ID: %s", videoID)
	}

	return &videoResp.Data[0], nil
}

// convertGQLNodeToComment 将 GraphQL 节点转换为 TwitchChatComment 格式
func convertGQLNodeToComment(node struct {
	ID                   string    `json:"id"`
	CreatedAt            time.Time `json:"createdAt"`
	ContentOffsetSeconds int       `json:"contentOffsetSeconds"`
	Commenter            *struct {
		ID          string `json:"id"`
		Login       string `json:"login"`
		DisplayName string `json:"displayName"`
	} `json:"commenter"`
	Message struct {
		Fragments []struct {
			Text  string `json:"text"`
			Emote *struct {
				EmoteID string `json:"emoteID"`
			} `json:"emote"`
		} `json:"fragments"`
		UserBadges []struct {
			ID      string `json:"id"`
			SetID   string `json:"setID"`
			Version string `json:"version"`
		} `json:"userBadges"`
		UserColor string `json:"userColor"`
	} `json:"message"`
}, videoID string) models.TwitchChatComment {

	comment := models.TwitchChatComment{
		ID:                   node.ID,
		CreatedAt:            node.CreatedAt.Format(time.RFC3339),
		ContentOffsetSeconds: float64(node.ContentOffsetSeconds),
		ContentType:          "video",
		ContentID:            videoID,
	}

	// 转换 Commenter
	if node.Commenter != nil {
		comment.Commenter = models.TwitchChatCommenter{
			ID:          node.Commenter.ID,
			DisplayName: node.Commenter.DisplayName,
			Name:        node.Commenter.Login,
		}
	}

	// 转换 Message
	var messageBody strings.Builder
	var fragments []models.TwitchChatMessageFragment
	var emoticons []models.TwitchChatEmoticon

	for i, frag := range node.Message.Fragments {
		messageBody.WriteString(frag.Text)

		fragment := models.TwitchChatMessageFragment{
			Text: frag.Text,
		}

		if frag.Emote != nil {
			emoticon := models.TwitchChatEmoticon{
				EmoticonID: frag.Emote.EmoteID,
				Begin:      i,
				End:        i + len(frag.Text),
			}
			fragment.Emoticon = &emoticon
			emoticons = append(emoticons, emoticon)
		}

		fragments = append(fragments, fragment)
	}

	// 转换 UserBadges
	var badges []models.TwitchChatBadge
	for _, badge := range node.Message.UserBadges {
		badges = append(badges, models.TwitchChatBadge{
			ID:      badge.SetID,
			Version: badge.Version,
		})
	}

	comment.Message = models.TwitchChatMessage{
		Body:       messageBody.String(),
		Fragments:  fragments,
		UserColor:  node.Message.UserColor,
		UserBadges: badges,
		Emoticons:  emoticons,
	}

	return comment
}

// GetVideoCommentsForStreamer 下载并分析指定主播的视频评论，返回新完成的分析结果
func (m *TwitchMonitor) GetVideoCommentsForStreamer(twitchUsername string) []AnalysisResult {
	log.Printf("开始检查并下载 %s 的未下载聊天记录...", twitchUsername)

	// 获取最近的录像列表
	videosResp, err := m.getVideos(twitchUsername, "archive", fetchVodCount, "")
	if err != nil {
		log.Printf("获取 %s 的录像列表失败: %v", twitchUsername, err)
		return nil
	}

	if len(videosResp.Videos) == 0 {
		log.Printf("%s 没有找到录像", twitchUsername)
		return nil
	}

	log.Printf("找到 %s 的 %d 个录像，开始检查...", twitchUsername, len(videosResp.Videos))

	// 确保聊天日志目录存在
	if err := os.MkdirAll("./chat_logs", 0755); err != nil {
		log.Printf("创建聊天日志目录失败: %v", err)
		return nil
	}

	downloadedCount := 0
	skippedCount := 0
	var newAnalysisResults []AnalysisResult

	for _, video := range videosResp.Videos {
		// 检查是否已经下载过
		if m.isChatAlreadyDownloaded(video.ID) {
			log.Printf("跳过已下载的录像: %s (%s)", video.ID, video.Title)
			skippedCount++
			continue
		}

		log.Printf("开始下载录像 %s 的聊天记录: %s", video.ID, video.Title)

		// 下载聊天记录
		response, err := m.downloadChatComments(video.ID, nil, nil)
		if err != nil {
			log.Printf("下载录像 %s 的聊天记录失败: %v", video.ID, err)
			continue
		}

		// 保存到文件
		filename := fmt.Sprintf("chat_%s_%s.json", video.ID, time.Now().Format("20060102_150405"))
		filePath := filepath.Join("./chat_logs", filename)

		jsonData, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			log.Printf("序列化JSON失败: %v", err)
			continue
		}

		if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
			log.Printf("写入文件失败: %v", err)
			continue
		}

		// 进行数据分析
		var hotMoments []VodCommentData
		var timeSeriesData []TimeSeriesDataPoint
		var analysisStats VodCommentStats

		// 使用默认参数进行分析
		params := defaultPeakParams
		analysisResult := FindHotCommentsWithParamsTwitch(response.Comments, 5, params)
		hotMoments = analysisResult.HotMoments
		timeSeriesData = analysisResult.TimeSeriesData
		analysisStats = analysisResult.Stats

		// 保存完整的分析结果到文件（包含params参数）
		if err := saveAnalysisResultToFile(video.ID, hotMoments, timeSeriesData,
			video.UserName, analysisStats, &video, params); err != nil {
			log.Printf("保存分析结果失败: %v", err)
		}

		// 保存录像信息到 RPC（如果有视频信息）
		if response.VideoInfo != nil {
			saveStreamerVODInfoToRPC(
				response.VideoInfo.UserLogin,
				response.VideoInfo.Title,
				"Twitch",
				response.VideoInfo.Duration,
				response.VideoID)
		}

		// 收集新完成的分析结果
		newResult := AnalysisResult{
			VideoID:        video.ID,
			StreamerName:   video.UserName,
			HotMoments:     hotMoments,
			TimeSeriesData: timeSeriesData,
			Stats:          analysisStats,
			VideoInfo:      video,
			AnalyzedAt:     time.Now(),
		}
		newAnalysisResults = append(newAnalysisResults, newResult)

		log.Printf("✅ 成功保存 %s 的录像 %s 聊天记录 (%d 条评论) 到: %s",
			twitchUsername, video.ID, response.TotalComments, filePath)

		downloadedCount++

		// 避免请求过快
		time.Sleep(2 * time.Second)
	}

	log.Printf("%s 的聊天记录下载完成！新下载: %d 个，跳过: %d 个", twitchUsername, downloadedCount, skippedCount)

	// 下载热点片段
	for _, v := range newAnalysisResults {
		m.downloadHotMomentClips(v.VideoID, v.HotMoments, 420)
	}

	return newAnalysisResults
}

// autoDownloadRecentChats 自动下载最近录像的聊天记录，返回新完成分析的结果（保留用于向后兼容）
func (m *TwitchMonitor) autoDownloadRecentChats() []AnalysisResult {
	log.Println("开始检查并下载未下载的聊天记录...")

	// 获取第一个主播的用户名
	m.mu.RLock()
	var twitchUsername string
	if len(m.streamers) > 0 {
		for _, platform := range m.streamers[0].Platforms {
			if platform.Platform == "twitch" {
				parts := strings.Split(platform.URL, "/")
				if len(parts) > 0 {
					twitchUsername = parts[len(parts)-1]
				}
				break
			}
		}
	}
	m.mu.RUnlock()

	if twitchUsername == "" {
		log.Println("没有配置主播")
		return nil
	}

	return m.GetVideoCommentsForStreamer(twitchUsername)
}

// isChatAlreadyDownloaded 检查聊天记录是否已经下载过
func (m *TwitchMonitor) isChatAlreadyDownloaded(videoID string) bool {
	// 检查 chat_logs 目录下是否存在该视频ID的文件
	pattern := filepath.Join("./chat_logs", fmt.Sprintf("chat_%s_*.json", videoID))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("检查文件失败: %v", err)
		return false
	}
	return len(matches) > 0
}

// downloadHotMomentClips 根据热点时刻下载 VOD 片段
func (m *TwitchMonitor) downloadHotMomentClips(videoID string, hotMoments []VodCommentData, interval float64) {
	log.Printf("开始下载视频 %s 的热点片段，共 %d 个热点", videoID, len(hotMoments))

	// 创建 VOD 下载器
	downloader := NewVODDownloader("./downloads/hot_clips")

	// 确保输出目录存在
	outputDir := filepath.Join("./downloads/hot_clips", videoID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("创建输出目录失败: %v", err)
		return
	}

	// 遍历每个热点时刻
	for i, hotMoment := range hotMoments {
		// 计算下载的时间范围：向前推 interval 的一半，向后推 interval 的一半
		halfInterval := interval / 2.0
		startTime := hotMoment.OffsetSeconds - halfInterval
		endTime := interval

		// 确保开始时间不小于0
		if startTime < 0 {
			startTime = 0
		}

		log.Printf("下载热点 #%d: 偏移 %.2f 秒, 时间范围 %.2f - %.2f 秒",
			i+1, hotMoment.OffsetSeconds, startTime, endTime)

		// 构建下载请求
		req := &VODDownloadRequest{
			VODID:      videoID,
			StartTime:  startTime,
			EndTime:    endTime,
			Quality:    "720p", // 使用 720p 质量以节省空间和时间
			OutputPath: outputDir,
		}

		// 执行下载
		ctx := context.Background()
		resp, err := downloader.DownloadVOD(ctx, req)
		if err != nil {
			log.Printf("下载热点 #%d 失败: %v", i+1, err)
			continue
		}

		if resp.Success {
			log.Printf("成功下载热点 #%d 到: %s (用时 %.2f 秒)",
				i+1, resp.VideoPath, resp.DownloadTime)

			// 下载完成后执行AI总结
			if resp.SubtitlePath != "" {
				log.Printf("开始对热点 #%d 的字幕进行AI总结...", i+1)

				// 从配置读取AI服务提供商
				aiConfig := GetAIConfig()
				aiService := NewAIService(aiConfig.Provider, "")
				if aiService == nil {
					log.Println("AI 服务未初始化，跳过AI总结")
				} else {
					// 执行字幕总结
					ctx := context.Background()
					file, err := os.Open(resp.SubtitlePath)
					if err != nil {
						log.Printf("打开字幕文件失败: %v", err)
						continue
					}
					defer file.Close()

					srtContext, err := io.ReadAll(file)
					if err != nil {
						log.Printf("读取字幕文件失败: %v", err)
						continue
					}

					summary, _, err := aiService.SummarizeSRT(ctx, string(srtContext), 10000)

					if err != nil {
						log.Printf("AI总结失败: %v", err)
					} else {
						// 保存总结到analysis_results文件夹，避免被清理
						analysisDir := filepath.Join("./analysis_results", videoID)
						if err := os.MkdirAll(analysisDir, 0755); err != nil {
							log.Printf("创建分析目录失败: %v", err)
						} else {
							// 使用原始字幕文件名，但保存到analysis_results目录
							summaryPath := filepath.Join(analysisDir, fmt.Sprintf("%f", hotMoment.OffsetSeconds))
							if err := aiService.SaveSummaryToFile(summaryPath, summary); err != nil {
								log.Printf("保存总结失败: %v", err)
							} else {
								log.Printf("热点 #%d AI总结完成并已保存到: %s", i+1, summaryPath)
							}
						}
					}
				}
			}
		} else {
			log.Printf("下载热点 #%d 失败: %s", i+1, resp.Message)
		}

		// 清理downloads文件夹中的临时文件
		if err := cleanTempFiles(outputDir); err != nil {
			log.Printf("清理临时文件失败: %v", err)
		}

		// 避免请求过快
		time.Sleep(10 * time.Second)
	}

	log.Printf("视频 %s 的所有热点片段下载完成", videoID)
}

// cleanTempFiles 清理指定目录下的临时文件
func cleanTempFiles(dir string) error {
	log.Printf("开始清理目录中的临时文件: %s", dir)

	// 临时文件的扩展名模式
	tempExtensions := []string{".ts", ".tmp", ".part", ".download", ".mp4", ".mp3"}

	var deletedCount int
	var deletedSize int64

	// 遍历目录
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 检查是否是临时文件
		for _, ext := range tempExtensions {
			if strings.HasSuffix(strings.ToLower(info.Name()), ext) {
				// 删除临时文件
				if err := os.Remove(path); err != nil {
					log.Printf("删除临时文件失败 %s: %v", path, err)
					return nil // 继续处理其他文件
				}
				deletedCount++
				deletedSize += info.Size()
				log.Printf("已删除临时文件: %s (%.2f MB)", info.Name(), float64(info.Size())/1024/1024)
				break
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("清理临时文件时出错: %w", err)
	}

	if deletedCount > 0 {
		log.Printf("清理完成: 删除了 %d 个临时文件，释放了 %.2f MB 空间",
			deletedCount, float64(deletedSize)/1024/1024)
	} else {
		log.Printf("没有找到需要清理的临时文件")
	}

	return nil
}

// saveChatAnalysisToRPC 异步保存一个直播数据到 RPC 服务
func saveStreamerVODInfoToRPC(streamerName string, streamTitle string,
	streamPlatform string, duration string, videoId string) {
	streamerService := services.GetStreamerService()
	if streamerService == nil {
		log.Println("RPC 服务未初始化，跳过保存分析结果")
		return
	}

	// 保存到 RPC
	if _, err := streamerService.CreateStreamer(streamerName, streamTitle,
		streamPlatform, duration, videoId); err != nil {
		log.Printf("结果保存到 RPC 失败: %v", err)
	} else {
		log.Printf("结果已保存到 RPC: Streamer=%s, Title=%s", streamerName, streamTitle)
	}
}

// AnalysisResult 完整的分析结果（用于保存）
type AnalysisResult struct {
	VideoID        string                 `json:"video_id"`
	StreamerName   string                 `json:"streamer_name"`
	Method         string                 `json:"method"`
	HotMoments     []VodCommentData       `json:"hot_moments"`
	TimeSeriesData []TimeSeriesDataPoint  `json:"time_series_data"`
	Stats          VodCommentStats        `json:"stats"`
	VideoInfo      models.TwitchVideoData `json:"video_info"`
	AnalyzedAt     time.Time              `json:"analyzed_at"`
}

// saveAnalysisResultToFile 保存分析结果到文件
func saveAnalysisResultToFile(videoID string, hotMoments []VodCommentData,
	timeSeriesData []TimeSeriesDataPoint, name string, stats VodCommentStats,
	videoInfo *models.TwitchVideoData, params PeakDetectionParams) error {

	// 按videoID创建目录
	videoDir := filepath.Join("./analysis_results", videoID)
	if err := os.MkdirAll(videoDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 构建完整的分析结果
	result := AnalysisResult{
		VideoID:        videoID,
		StreamerName:   name,
		HotMoments:     hotMoments,
		TimeSeriesData: timeSeriesData,
		Stats:          stats,
		VideoInfo:      *videoInfo,
		AnalyzedAt:     time.Now(),
	}

	// 使用参数生成文件名：analysis_{windowsLen}_{thr}_{searchRange}.json
	filename := filepath.Join(videoDir, fmt.Sprintf("analysis_%d_%.2f_%d.json",
		params.WindowsLen, params.Thr, params.SearchRange))

	// 序列化为JSON
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	log.Printf("分析结果已保存到: %s", filename)
	return nil
}

// GetAnalysisResult 获取分析结果
func GetAnalysisResult(c *gin.Context) {
	videoID := c.Param("videoID")
	if videoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少视频ID",
		})
		return
	}

	// 获取可选的查询参数
	windowsLen := c.DefaultQuery("windows_len", "420")
	thr := c.DefaultQuery("thr", "0.90")
	searchRange := c.DefaultQuery("search_range", "210")

	// 查找分析结果文件
	videoDir := filepath.Join("./analysis_results", videoID)
	var targetFile string

	// 如果提供了参数，查找特定的文件
	if windowsLen != "" || thr != "" || searchRange != "" {
		// 转换参数为正确的类型以格式化文件名
		var params PeakDetectionParams
		params.WindowsLen, _ = strconv.Atoi(windowsLen)
		params.Thr, _ = strconv.ParseFloat(thr, 64)
		params.SearchRange, _ = strconv.Atoi(searchRange)

		filename := fmt.Sprintf("analysis_%d_%.2f_%d.json", params.WindowsLen, params.Thr, params.SearchRange)
		targetFile = filepath.Join(videoDir, filename)
		if _, err := os.Stat(targetFile); os.IsNotExist(err) {
			// 如果指定参数的文件不存在，执行分析并保存结果
			// 查找聊天记录文件
			chatPattern := filepath.Join("./chat_logs", fmt.Sprintf("chat_%s_*.json", videoID))
			chatFiles, err := filepath.Glob(chatPattern)
			if err != nil || len(chatFiles) == 0 {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "未找到该视频的聊天记录，请先下载聊天记录",
				})
				return
			}

			// 读取聊天记录
			chatData, err := os.ReadFile(chatFiles[0])
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "读取聊天记录失败: " + err.Error(),
				})
				return
			}

			var chatResponse models.TwitchChatDownloadResponse
			if err := json.Unmarshal(chatData, &chatResponse); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "解析聊天记录失败: " + err.Error(),
				})
				return
			}

			// 执行分析
			analysisResult := FindHotCommentsWithParamsTwitch(chatResponse.Comments, 5, params)

			// 保存分析结果
			if chatResponse.VideoInfo != nil {
				if err := saveAnalysisResultToFile(
					videoID,
					analysisResult.HotMoments,
					analysisResult.TimeSeriesData,
					chatResponse.VideoInfo.UserName,
					analysisResult.Stats,
					chatResponse.VideoInfo,
					params,
				); err != nil {
					log.Printf("保存分析结果失败: %v", err)
				}
			}
		}
	} else {
		// 查找目录下的所有分析文件
		pattern := filepath.Join(videoDir, "analysis_*.json")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "查询分析结果失败: " + err.Error(),
			})
			return
		}

		if len(matches) == 0 {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "未找到该视频的分析结果",
			})
			return
		}

		// 使用第一个文件（如果有多个，用户应该指定参数）
		targetFile = matches[0]
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "读取分析结果失败: " + err.Error(),
		})
		return
	}

	var result AnalysisResult
	if err := json.Unmarshal(data, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "解析分析结果失败: " + err.Error(),
		})
		return
	}

	// 读取默认参数的hotmoments数据
	defaultFilename := fmt.Sprintf("analysis_%d_%.2f_%d.json",
		defaultPeakParams.WindowsLen, defaultPeakParams.Thr, defaultPeakParams.SearchRange)
	defaultFile := filepath.Join(videoDir, defaultFilename)

	// 如果默认参数文件存在且不是当前文件，则从默认文件读取HotMoments
	if defaultFile != targetFile {
		if defaultData, err := os.ReadFile(defaultFile); err == nil {
			var defaultResult AnalysisResult
			if err := json.Unmarshal(defaultData, &defaultResult); err == nil {
				// 用默认参数的HotMoments替换当前结果的HotMoments
				result.HotMoments = defaultResult.HotMoments
				log.Printf("已从默认参数文件读取HotMoments: %s", defaultFilename)
			} else {
				log.Printf("解析默认参数文件失败: %v", err)
			}
		} else {
			log.Printf("默认参数文件不存在或读取失败: %s, 使用当前文件的HotMoments", defaultFilename)
		}
	}

	c.JSON(http.StatusOK, result)
}

// ListAnalysisResults 列出所有分析结果
func ListAnalysisResults(c *gin.Context) {
	analysisDir := "./analysis_results"

	// 读取所有视频ID目录
	dirs, err := os.ReadDir(analysisDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询分析结果失败: " + err.Error(),
		})
		return
	}

	type AnalysisListItem struct {
		VideoID      string    `json:"video_id"`
		StreamerName string    `json:"streamer_name"`
		Title        string    `json:"title"`
		Method       string    `json:"method"`
		AnalyzedAt   time.Time `json:"analyzed_at"`
		HotMoments   int       `json:"hot_moments_count"`
		Params       string    `json:"params"` // 参数信息
	}

	var results []AnalysisListItem

	// 遍历每个视频ID目录
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}

		videoID := dir.Name()
		videoDir := filepath.Join(analysisDir, videoID)

		// 查找该视频的所有分析文件
		pattern := filepath.Join(videoDir, "analysis_*.json")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, file := range matches {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}

			var result AnalysisResult
			if err := json.Unmarshal(data, &result); err != nil {
				continue
			}

			// 从文件名中提取参数信息
			filename := filepath.Base(file)
			params := strings.TrimPrefix(filename, "analysis_")
			params = strings.TrimSuffix(params, ".json")

			results = append(results, AnalysisListItem{
				VideoID:      result.VideoID,
				StreamerName: result.StreamerName,
				Title:        result.VideoInfo.Title,
				Method:       result.Method,
				AnalyzedAt:   result.AnalyzedAt,
				HotMoments:   len(result.HotMoments),
				Params:       params,
			})
		}
	}

	// 按分析时间倒序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].AnalyzedAt.After(results[j].AnalyzedAt)
	})

	c.JSON(http.StatusOK, gin.H{
		"total":   len(results),
		"results": results,
	})
}

// GetVideoCommentsAndAnalysis 下载并分析视频评论，返回新完成的分析结果
func GetVideoCommentsAndAnalysis(tm *TwitchMonitor) []AnalysisResult {
	// 下载与分析
	ars := tm.autoDownloadRecentChats()

	for _, v := range ars {
		// 调用下载 VOD 片段的方法
		tm.downloadHotMomentClips(v.VideoID, v.HotMoments, 420)
	}

	return ars
}
