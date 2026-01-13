package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"subtuber-services/models"
)

// YouTubeConfig YouTube配置
type YouTubeConfig struct {
	APIKey                string `mapstructure:"api_key" json:"-"`
	MinIntervalSeconds    int    `mapstructure:"min_interval_seconds" json:"min_interval_seconds"`
	MaxIntervalSeconds    int    `mapstructure:"max_interval_seconds" json:"max_interval_seconds"`
	ReloadIntervalMinutes int    `mapstructure:"reload_interval_minutes" json:"reload_interval_minutes"`
	ChannelsConfigPath    string `mapstructure:"channels_config_path" json:"channels_config_path"`
	Referer               string `mapstructure:"referer" json:"referer"`
}

// YouTubeMonitor YouTube监控服务
type YouTubeMonitor struct {
	config         YouTubeConfig
	channels       []models.StreamerInfo
	channelStatus  map[string]*models.YouTubeStatusResponse
	mu             sync.RWMutex
	stopChan       chan struct{}
	lastReloadTime time.Time
}

var (
	youtubeMonitor     *YouTubeMonitor
	youtubeMonitorOnce sync.Once
)

// InitYouTubeMonitor 初始化YouTube监控服务
func InitYouTubeMonitor(config YouTubeConfig) *YouTubeMonitor {
	youtubeMonitorOnce.Do(func() {
		youtubeMonitor = &YouTubeMonitor{
			config:        config,
			channelStatus: make(map[string]*models.YouTubeStatusResponse),
			stopChan:      make(chan struct{}),
		}

		// 设置默认值
		if youtubeMonitor.config.MinIntervalSeconds == 0 {
			youtubeMonitor.config.MinIntervalSeconds = 30
		}
		if youtubeMonitor.config.MaxIntervalSeconds == 0 {
			youtubeMonitor.config.MaxIntervalSeconds = 120
		}
		if youtubeMonitor.config.ReloadIntervalMinutes == 0 {
			youtubeMonitor.config.ReloadIntervalMinutes = 10
		}
		if youtubeMonitor.config.ChannelsConfigPath == "" {
			youtubeMonitor.config.ChannelsConfigPath = "App_Data/tracked_streamers.json"
		}

		// 加载频道列表
		if err := youtubeMonitor.LoadChannels(); err != nil {
			log.Printf("加载YouTube频道列表失败: %v", err)
		}

		log.Printf("YouTube监控服务初始化完成，监控 %d 个频道", len(youtubeMonitor.channels))
	})

	return youtubeMonitor
}

// GetYouTubeMonitor 获取YouTube监控实例
func GetYouTubeMonitor() *YouTubeMonitor {
	return youtubeMonitor
}

// LoadChannels 从配置文件加载频道列表
func (ym *YouTubeMonitor) LoadChannels() error {
	data, err := os.ReadFile(ym.config.ChannelsConfigPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var trackedStreamers models.TrackedStreamers
	if err := json.Unmarshal(data, &trackedStreamers); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	ym.mu.Lock()
	ym.channels = trackedStreamers.Streamers
	ym.lastReloadTime = time.Now()
	ym.mu.Unlock()

	log.Printf("已加载 %d 个主播配置", len(trackedStreamers.Streamers))
	return nil
}

// shouldReloadChannels 检查是否需要重新加载频道列表
func (ym *YouTubeMonitor) shouldReloadChannels() bool {
	ym.mu.RLock()
	defer ym.mu.RUnlock()

	reloadInterval := time.Duration(ym.config.ReloadIntervalMinutes) * time.Minute
	return time.Since(ym.lastReloadTime) >= reloadInterval
}

// Start 启动监控服务
func (ym *YouTubeMonitor) Start() {
	go ym.monitorLoop()
	log.Println("YouTube监控服务已启动")
}

// Stop 停止监控服务
func (ym *YouTubeMonitor) Stop() {
	close(ym.stopChan)
}

// monitorLoop 监控循环
func (ym *YouTubeMonitor) monitorLoop() {
	// 初始化时立即检查一次所有频道
	ym.checkAllChannels()

	ticker := time.NewTicker(time.Duration(ym.getRandomInterval()) * time.Second)
	defer ticker.Stop()

	reloadTicker := time.NewTicker(time.Duration(ym.config.ReloadIntervalMinutes) * time.Minute)
	defer reloadTicker.Stop()

	for {
		select {
		case <-ym.stopChan:
			log.Println("YouTube监控服务已停止")
			return
		case <-ticker.C:
			ym.checkAllChannels()
			// 重置为新的随机间隔
			ticker.Reset(time.Duration(ym.getRandomInterval()) * time.Second)
		case <-reloadTicker.C:
			if ym.shouldReloadChannels() {
				if err := ym.LoadChannels(); err != nil {
					log.Printf("重新加载频道列表失败: %v", err)
				} else {
					log.Println("已重新加载YouTube频道列表")
				}
			}
		}
	}
}

// getRandomInterval 获取随机检查间隔
func (ym *YouTubeMonitor) getRandomInterval() int {
	min := ym.config.MinIntervalSeconds
	max := ym.config.MaxIntervalSeconds
	if min >= max {
		return min
	}
	return min + int(time.Now().UnixNano()%(int64(max-min)))
}

// checkAllChannels 检查所有频道的状态
func (ym *YouTubeMonitor) checkAllChannels() {
	ym.mu.RLock()
	channels := make([]models.StreamerInfo, len(ym.channels))
	copy(channels, ym.channels)
	ym.mu.RUnlock()

	log.Printf("开始检查 %d 个YouTube频道的直播状态", len(channels))

	// 逐个检查频道状态
	for _, channel := range channels {
		ym.checkChannelStatus(channel)
		// 避免请求过快
		time.Sleep(500 * time.Millisecond)
	}
}

// checkChannelStatus 检查单个频道的状态
func (ym *YouTubeMonitor) checkChannelStatus(channel models.StreamerInfo) {
	// 从 platforms 中获取 YouTube 频道ID
	var youtubeChannelID string
	for _, platform := range channel.Platforms {
		if platform.Platform == "youtube" {
			// 从URL中提取频道ID: https://www.youtube.com/@channelname 或 https://www.youtube.com/channel/CHANNEL_ID
			parts := strings.Split(platform.URL, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]
				// 如果是 @username 格式，需要转换为频道ID
				if strings.HasPrefix(lastPart, "@") {
					// 通过用户名获取频道ID
					channelID, err := ym.getChannelIDByUsername(lastPart)
					if err != nil {
						log.Printf("获取频道ID失败 (%s): %v", lastPart, err)
						return
					}
					youtubeChannelID = channelID
				} else {
					youtubeChannelID = lastPart
				}
			}
			break
		}
	}

	if youtubeChannelID == "" {
		log.Printf("主播 %s 没有配置YouTube平台", channel.Name)
		return
	}

	// 检查直播状态
	stream, err := ym.CheckLiveStatusByChannelID(youtubeChannelID)
	if err != nil {
		log.Printf("检查频道 %s 直播状态失败: %v", channel.Name, err)
		return
	}

	// 获取之前的状态
	ym.mu.RLock()
	prevStatus, existed := ym.channelStatus[channel.ID]
	ym.mu.RUnlock()

	// 更新状态
	newStatus := &models.YouTubeStatusResponse{
		IsLive:       stream != nil,
		StreamData:   stream,
		CheckedAt:    time.Now().Format(time.RFC3339),
		ChannelTitle: channel.Name,
	}

	ym.mu.Lock()
	ym.channelStatus[channel.ID] = newStatus
	ym.mu.Unlock()

	if stream != nil {
		log.Printf("✅ %s 正在直播: %s (观众: %s)", channel.Name, stream.Title, stream.ViewerCount)

		// 检测从离线到直播的状态变化
		if !existed || !prevStatus.IsLive {
			log.Printf("🎉 %s 开始直播了！", channel.Name)
			// 这里可以添加通知逻辑
		}
	} else {
		log.Printf("💤 %s 当前未直播", channel.Name)

		// 检测从直播状态变为离线状态
		if existed && prevStatus.IsLive {
			log.Printf("📴 %s 已下播", channel.Name)
			// 这里可以添加直播结束后的处理逻辑
		}
	}
}

// getChannelIDByUsername 通过用户名/Handle获取频道ID
func (ym *YouTubeMonitor) getChannelIDByUsername(username string) (string, error) {
	// 保留 @ 符号用于 search 接口
	if !strings.HasPrefix(username, "@") {
		username = "@" + username
	}

	// 方法 A: 使用 search 接口通过 Handle 查询频道
	// 这是目前推荐的方法，因为 forUsername 只适用于旧版
	searchURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/search?part=snippet&q=%s&type=channel&key=%s",
		username, ym.config.APIKey)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", ym.config.Referer)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API返回错误状态 %d: %s", resp.StatusCode, string(body))
	}

	var searchResult struct {
		Items []struct {
			ID struct {
				ChannelID string `json:"channelId"`
			} `json:"id"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return "", err
	}

	if len(searchResult.Items) == 0 {
		return "", fmt.Errorf("未找到频道: %s", username)
	}

	// 获取真正的频道 ID
	channelID := searchResult.Items[0].ID.ChannelID
	if channelID == "" {
		return "", fmt.Errorf("频道ID为空: %s", username)
	}

	log.Printf("通过 Handle %s 找到频道ID: %s", username, channelID)
	return channelID, nil
}

// CheckLiveStatusByChannelID 根据频道ID检查直播状态
func (ym *YouTubeMonitor) CheckLiveStatusByChannelID(channelID string) (*models.YouTubeStreamData, error) {
	// 搜索该频道的直播视频
	searchURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/search?part=snippet&channelId=%s&eventType=live&type=video&key=%s",
		channelID, ym.config.APIKey)

	searchReq, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	searchReq.Header.Set("Referer", ym.config.Referer)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(searchReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API返回错误状态 %d: %s", resp.StatusCode, string(body))
	}

	var searchResp models.YouTubeSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	// 如果没有直播，返回nil
	if len(searchResp.Items) == 0 {
		return nil, nil
	}

	// 获取第一个直播视频的详细信息
	videoID := searchResp.Items[0].ID.VideoID
	videoURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/videos?part=snippet,liveStreamingDetails&id=%s&key=%s",
		videoID, ym.config.APIKey)

	videoReq, err := http.NewRequest("GET", videoURL, nil)
	if err != nil {
		return nil, err
	}
	videoReq.Header.Set("Referer", ym.config.Referer)

	videoResp, err := client.Do(videoReq)
	if err != nil {
		return nil, err
	}
	defer videoResp.Body.Close()

	if videoResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(videoResp.Body)
		return nil, fmt.Errorf("API返回错误状态 %d: %s", videoResp.StatusCode, string(body))
	}

	var videoData models.YouTubeVideoResponse
	if err := json.NewDecoder(videoResp.Body).Decode(&videoData); err != nil {
		return nil, err
	}

	if len(videoData.Items) == 0 {
		return nil, nil
	}

	item := videoData.Items[0]
	stream := &models.YouTubeStreamData{
		ID:             item.ID,
		ChannelID:      item.Snippet.ChannelID,
		ChannelTitle:   item.Snippet.ChannelTitle,
		Title:          item.Snippet.Title,
		Description:    item.Snippet.Description,
		ThumbnailURL:   item.Snippet.Thumbnails.High.URL,
		ViewerCount:    item.LiveStreamingDetails.ConcurrentViewers,
		ActualStart:    item.LiveStreamingDetails.ActualStartTime,
		ScheduledStart: item.LiveStreamingDetails.ScheduledStartTime,
	}

	return stream, nil
}

// GetLatestStatus 获取最新的直播状态（返回所有频道的状态）
func (ym *YouTubeMonitor) GetLatestStatus() map[string]*models.YouTubeStatusResponse {
	ym.mu.RLock()
	defer ym.mu.RUnlock()

	result := make(map[string]*models.YouTubeStatusResponse)
	for id, status := range ym.channelStatus {
		result[id] = status
	}
	return result
}

// GetChannelStatus 获取指定频道的状态
func (ym *YouTubeMonitor) GetChannelStatus(channelID string) *models.YouTubeStatusResponse {
	ym.mu.RLock()
	defer ym.mu.RUnlock()

	if status, ok := ym.channelStatus[channelID]; ok {
		return status
	}
	return nil
}
