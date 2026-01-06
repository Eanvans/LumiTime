package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"subtuber-services/models"

	"github.com/gin-gonic/gin"
)

// TwitchConfig Twitch配置
type TwitchConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	StreamerName string `mapstructure:"streamer_name"`
	MinInterval  int    `mapstructure:"min_interval_seconds"` // 最小检查间隔（秒）
	MaxInterval  int    `mapstructure:"max_interval_seconds"` // 最大检查间隔（秒）
}

// TwitchMonitor Twitch监控服务
type TwitchMonitor struct {
	config         TwitchConfig
	accessToken    string
	tokenExpiry    time.Time
	mu             sync.RWMutex
	latestStatus   *models.TwitchStatusResponse
	previousIsLive bool // 上一次的直播状态
	stopCh         chan struct{}
}

var (
	twitchMonitor     *TwitchMonitor
	twitchMonitorOnce sync.Once
)

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

		twitchMonitor = &TwitchMonitor{
			config: config,
			stopCh: make(chan struct{}),
		}
	})
	return twitchMonitor
}

// GetTwitchMonitor 获取Twitch监控实例
func GetTwitchMonitor() *TwitchMonitor {
	return twitchMonitor
}

// Start 启动监控服务
func (tm *TwitchMonitor) Start() {
	log.Printf("启动Twitch监控服务，主播: %s", tm.config.StreamerName)
	go tm.monitorLoop()
}

// Stop 停止监控服务
func (tm *TwitchMonitor) Stop() {
	close(tm.stopCh)
	log.Println("Twitch监控服务已停止")
}

// monitorLoop 监控循环
func (tm *TwitchMonitor) monitorLoop() {
	// 初始化时立即检查一次
	tm.checkAndUpdate()

	for {
		// 随机间隔时间
		interval := tm.getRandomInterval()
		log.Printf("下次检查将在 %d 秒后进行", interval)

		select {
		case <-time.After(time.Duration(interval) * time.Second):
			tm.checkAndUpdate()
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

// checkAndUpdate 检查并更新状态
func (tm *TwitchMonitor) checkAndUpdate() {
	log.Printf("正在检查 %s 的直播状态...", tm.config.StreamerName)

	// 确保有有效的访问令牌
	if err := tm.ensureValidToken(); err != nil {
		log.Printf("获取访问令牌失败: %v", err)
		return
	}

	// 检查直播状态
	stream, err := tm.checkStreamStatus()
	if err != nil {
		log.Printf("检查直播状态失败: %v", err)
		return
	}

	// 更新状态
	status := &models.TwitchStatusResponse{
		IsLive:       stream != nil,
		StreamData:   stream,
		CheckedAt:    time.Now().Format(time.RFC3339),
		StreamerName: tm.config.StreamerName,
	}

	tm.mu.Lock()
	previousIsLive := tm.previousIsLive
	tm.latestStatus = status
	tm.previousIsLive = stream != nil
	tm.mu.Unlock()

	// 测试自动下载最近聊天记录功能
	//tm.autoDownloadRecentChats()

	if stream != nil {
		log.Printf("🔴 %s 正在直播！标题: %s, 观众: %d",
			stream.UserName, stream.Title, stream.ViewerCount)
	} else {
		log.Printf("⚫ %s 当前离线", tm.config.StreamerName)

		// 检测从直播状态变为离线状态
		if previousIsLive {
			log.Printf("🎬 检测到直播结束，开始自动下载聊天记录...")
			vodHandler := GetVODDownloadHandler()
			if vodHandler != nil {
				go vodHandler.AutoDownloadRecentChats()
			}
		}
	}
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

// checkStreamStatus 检查直播状态
func (tm *TwitchMonitor) checkStreamStatus() (*models.TwitchStreamData, error) {
	url := fmt.Sprintf("https://api.twitch.tv/helix/streams?user_login=%s", tm.config.StreamerName)

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

// GetLatestStatus 获取最新的直播状态
func (tm *TwitchMonitor) GetLatestStatus() *models.TwitchStatusResponse {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.latestStatus
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

	status := monitor.GetLatestStatus()
	if status == nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "正在初始化，请稍后再试",
		})
		return
	}

	c.JSON(http.StatusOK, status)
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

// GetTwitchVideos 获取Twitch主播的录像列表
func GetTwitchVideos(c *gin.Context) {
	monitor := GetTwitchMonitor()
	if monitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Twitch监控服务未启动",
		})
		return
	}

	// 获取查询参数
	username := c.DefaultQuery("username", monitor.config.StreamerName)
	videoType := c.DefaultQuery("type", "archive") // archive, highlight, upload, all
	first := c.DefaultQuery("first", "20")         // 每页数量，最大100
	after := c.Query("after")                      // 分页游标

	// 确保有有效的访问令牌
	if err := monitor.ensureValidToken(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取访问令牌失败: " + err.Error(),
		})
		return
	}

	// 获取录像列表
	videos, err := monitor.getVideos(username, videoType, first, after)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取录像列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, videos)
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

// getUserID 通过用户名获取用户ID
func (tm *TwitchMonitor) getUserID(username string) (string, error) {
	url := fmt.Sprintf("https://api.twitch.tv/helix/users?login=%s", username)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	tm.mu.RLock()
	accessToken := tm.accessToken
	tm.mu.RUnlock()

	req.Header.Set("Client-ID", tm.config.ClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var userResp models.TwitchUserResponse
	if err := json.Unmarshal(body, &userResp); err != nil {
		return "", err
	}

	if len(userResp.Data) == 0 {
		return "", fmt.Errorf("用户不存在: %s", username)
	}

	return userResp.Data[0].ID, nil
}

// DownloadVODChat is now handled by vod_download_handler.go
// Keeping this function for backwards compatibility, but it delegates to the new handler

// SaveVODChatToFile is now handled by vod_download_handler.go
// Keeping this function for backwards compatibility, but it delegates to the new handler


