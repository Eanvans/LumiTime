package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"subtuber-services/models"
)

// YouTubeConfig YouTube配置
type YouTubeConfig struct {
	APIKeys               []string `mapstructure:"api_keys" json:"-"`
	MinIntervalSeconds    int      `mapstructure:"min_interval_seconds" json:"min_interval_seconds"`
	MaxIntervalSeconds    int      `mapstructure:"max_interval_seconds" json:"max_interval_seconds"`
	ReloadIntervalMinutes int      `mapstructure:"reload_interval_minutes" json:"reload_interval_minutes"`
	ChannelsConfigPath    string   `mapstructure:"channels_config_path" json:"channels_config_path"`
	Referer               string   `mapstructure:"referer" json:"referer"`
}

// YouTubeMonitor YouTube监控服务
type YouTubeMonitor struct {
	config          YouTubeConfig
	channels        []models.StreamerInfo
	channelStatus   map[string]*models.YouTubeStatusResponse
	mu              sync.RWMutex
	stopChan        chan struct{}
	lastReloadTime  time.Time
	currentKeyIndex int        // 当前使用的API Key索引
	apiKeyMu        sync.Mutex // API Key索引的互斥锁
}

var (
	youtubeMonitor     *YouTubeMonitor
	youtubeMonitorOnce sync.Once
)

// InitYouTubeMonitor 初始化YouTube监控服务
func InitYouTubeMonitor(config YouTubeConfig) *YouTubeMonitor {
	youtubeMonitorOnce.Do(func() {
		youtubeMonitor = &YouTubeMonitor{
			config:          config,
			channelStatus:   make(map[string]*models.YouTubeStatusResponse),
			stopChan:        make(chan struct{}),
			currentKeyIndex: 0,
		}

		// 验证API Keys
		if len(youtubeMonitor.config.APIKeys) == 0 {
			log.Println("警告：未配置YouTube API Keys")
		} else {
			log.Printf("YouTube监控服务已配置 %d 个API Keys", len(youtubeMonitor.config.APIKeys))
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

// getCurrentAPIKey 获取当前使用的API Key
func (ym *YouTubeMonitor) getCurrentAPIKey() string {
	ym.apiKeyMu.Lock()
	defer ym.apiKeyMu.Unlock()

	if len(ym.config.APIKeys) == 0 {
		return ""
	}

	return ym.config.APIKeys[ym.currentKeyIndex]
}

// rotateAPIKey 轮换到下一个API Key
func (ym *YouTubeMonitor) rotateAPIKey() string {
	ym.apiKeyMu.Lock()
	defer ym.apiKeyMu.Unlock()

	if len(ym.config.APIKeys) == 0 {
		return ""
	}

	// 切换到下一个Key
	ym.currentKeyIndex = (ym.currentKeyIndex + 1) % len(ym.config.APIKeys)
	newKey := ym.config.APIKeys[ym.currentKeyIndex]

	log.Printf("YouTube API Key已轮换到第 %d 个Key (共%d个)", ym.currentKeyIndex+1, len(ym.config.APIKeys))

	return newKey
}

// makeRequestWithRetry 使用API Key重试机制发送请求
func (ym *YouTubeMonitor) makeRequestWithRetry(url string) (*http.Response, error) {
	maxRetries := len(ym.config.APIKeys)
	if maxRetries == 0 {
		return nil, fmt.Errorf("未配置API Keys")
	}

	var lastErr error

	for i := 0; i < maxRetries; i++ {
		apiKey := ym.getCurrentAPIKey()
		if apiKey == "" {
			return nil, fmt.Errorf("无可用的API Key")
		}

		// 在URL中添加API Key
		fullURL := url
		if strings.Contains(url, "?") {
			fullURL = fmt.Sprintf("%s&key=%s", url, apiKey)
		} else {
			fullURL = fmt.Sprintf("%s?key=%s", url, apiKey)
		}

		req, err := http.NewRequest("GET", fullURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Referer", ym.config.Referer)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			ym.rotateAPIKey()
			continue
		}

		// 检查响应状态
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		// 如果是配额错误，尝试下一个Key
		if resp.StatusCode == 403 || resp.StatusCode == 429 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("API Key配额可能已用尽 (状态码: %d)，尝试下一个Key", resp.StatusCode)
			lastErr = fmt.Errorf("API返回错误状态 %d: %s", resp.StatusCode, string(body))
			ym.rotateAPIKey()
			time.Sleep(500 * time.Millisecond) // 短暂延迟
			continue
		}

		// 其他错误直接返回
		return resp, nil
	}

	return nil, fmt.Errorf("所有API Keys都失败了: %v", lastErr)
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
			// 优先使用已缓存的YouTube频道ID
			if channel.YouTubeChannelID != "" && strings.HasPrefix(channel.YouTubeChannelID, "UC") {
				youtubeChannelID = channel.YouTubeChannelID
				log.Printf("使用缓存的YouTube频道ID: %s -> %s", channel.Name, youtubeChannelID)
				break
			}

			// 从URL中提取频道ID或用户名
			parts := strings.Split(platform.URL, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]

				// 如果是 @username 格式或不是UC开头的频道ID格式
				if strings.HasPrefix(lastPart, "@") {
					// 通过用户名获取频道ID并保存
					channelID, err := ym.getChannelIDByUsernameAndCache(channel.ID, lastPart)
					if err != nil {
						log.Printf("获取频道ID失败 (%s): %v", lastPart, err)
						return
					}
					youtubeChannelID = channelID
				} else {
					// 已经是频道ID格式
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

	// 获取频道信息并更新头像URL到配置文件
	go func() {
		channelInfo, err := ym.getChannelInfo(youtubeChannelID)
		if err != nil {
			log.Printf("获取 %s 频道信息失败: %v", channel.Name, err)
		} else if channelInfo.ProfileImageURL != "" {
			if err := ym.updateChannelProfileImage(channel.ID, channel.Name, channelInfo.ProfileImageURL); err != nil {
				log.Printf("更新 %s 头像URL失败: %v", channel.Name, err)
			}
		}
	}()

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
			// 主播下播后，自动下载最近的VOD
			go func() {
				log.Printf("开始处理 %s 的最近VOD...", channel.Name)
				ym.ProcessRecentVOD(youtubeChannelID, channel.Name)
			}()
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
	searchURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/search?part=snippet&q=%s&type=channel",
		username)

	resp, err := ym.makeRequestWithRetry(searchURL)
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

// getChannelIDByUsernameAndCache 获取频道ID并缓存到配置文件
func (ym *YouTubeMonitor) getChannelIDByUsernameAndCache(currentID, username string) (string, error) {
	// 调用原方法获取频道ID
	channelID, err := ym.getChannelIDByUsername(username)
	if err != nil {
		return "", err
	}

	// 如果获取成功，保存到配置文件
	if channelID != "" && channelID != currentID {
		if err := ym.updateStreamerChannelID(currentID, channelID, username); err != nil {
			log.Printf("保存频道ID到配置文件失败: %v", err)
			// 不影响主流程，继续返回频道ID
		} else {
			log.Printf("✅ 已缓存频道ID: %s -> %s", username, channelID)
		}
	}

	return channelID, nil
}

// updateStreamerChannelID 更新主播的YouTube频道ID到配置文件
func (ym *YouTubeMonitor) updateStreamerChannelID(streamerID, newChannelID, username string) error {
	// 读取配置文件
	data, err := os.ReadFile(ym.config.ChannelsConfigPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var trackedStreamers models.TrackedStreamers
	if err := json.Unmarshal(data, &trackedStreamers); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 查找并更新主播的YouTubeChannelID字段
	updated := false
	for i := range trackedStreamers.Streamers {
		// 通过当前ID或用户名匹配
		if trackedStreamers.Streamers[i].ID == streamerID ||
			strings.Contains(trackedStreamers.Streamers[i].Name, strings.TrimPrefix(username, "@")) {
			// 更新YouTubeChannelID字段（不修改ID）
			if trackedStreamers.Streamers[i].YouTubeChannelID != newChannelID {
				trackedStreamers.Streamers[i].YouTubeChannelID = newChannelID
				updated = true
				log.Printf("更新YouTube频道ID: %s (%s) -> %s",
					trackedStreamers.Streamers[i].Name, streamerID, newChannelID)
			}
			break
		}
	}

	if !updated {
		return nil // 没有变化，不需要写入
	}

	// 写回配置文件
	newData, err := json.MarshalIndent(trackedStreamers, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(ym.config.ChannelsConfigPath, newData, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 重新加载配置到内存
	ym.LoadChannels()

	return nil
}

// CheckLiveStatusByChannelID 根据频道ID检查直播状态
func (ym *YouTubeMonitor) CheckLiveStatusByChannelID(channelID string) (*models.YouTubeStreamData, error) {
	// 搜索该频道的直播视频
	searchURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/search?part=snippet&channelId=%s&eventType=live&type=video",
		channelID)

	resp, err := ym.makeRequestWithRetry(searchURL)
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
	videoURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/videos?part=snippet,liveStreamingDetails&id=%s",
		videoID)

	videoResp, err := ym.makeRequestWithRetry(videoURL)
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

	// 检查LiveStreamingDetails是否存在
	if item.LiveStreamingDetails == nil {
		return nil, nil
	}

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

// getChannelInfo 获取频道详细信息
func (ym *YouTubeMonitor) getChannelInfo(channelID string) (*struct {
	ID              string
	Title           string
	ProfileImageURL string
}, error) {
	url := fmt.Sprintf("https://www.googleapis.com/youtube/v3/channels?part=snippet&id=%s",
		channelID)

	resp, err := ym.makeRequestWithRetry(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API返回错误状态 %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title      string `json:"title"`
				Thumbnails struct {
					High struct {
						URL string `json:"url"`
					} `json:"high"`
					Medium struct {
						URL string `json:"url"`
					} `json:"medium"`
					Default struct {
						URL string `json:"url"`
					} `json:"default"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("未找到频道: %s", channelID)
	}

	item := result.Items[0]
	// 优先使用 high 质量的头像，如果不存在则使用 medium 或 default
	profileImageURL := item.Snippet.Thumbnails.High.URL
	if profileImageURL == "" {
		profileImageURL = item.Snippet.Thumbnails.Medium.URL
	}
	if profileImageURL == "" {
		profileImageURL = item.Snippet.Thumbnails.Default.URL
	}

	return &struct {
		ID              string
		Title           string
		ProfileImageURL string
	}{
		ID:              item.ID,
		Title:           item.Snippet.Title,
		ProfileImageURL: profileImageURL,
	}, nil
}

// updateChannelProfileImage 更新频道头像URL到配置文件
func (ym *YouTubeMonitor) updateChannelProfileImage(channelID, channelName, imageURL string) error {
	if imageURL == "" {
		return fmt.Errorf("头像URL为空")
	}

	// 读取配置文件
	data, err := os.ReadFile(ym.config.ChannelsConfigPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var trackedStreamers models.TrackedStreamers
	if err := json.Unmarshal(data, &trackedStreamers); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 查找并更新频道信息
	updated := false
	for i := range trackedStreamers.Streamers {
		if trackedStreamers.Streamers[i].ID == channelID {
			// 只在头像URL有变化时更新
			if trackedStreamers.Streamers[i].ProfileImageURL == "" {
				trackedStreamers.Streamers[i].ProfileImageURL = imageURL
				updated = true
				log.Printf("已更新 %s 的头像URL: %s", channelName, imageURL)
			}
			break
		}
	}

	if !updated {
		return nil // 没有变化，不需要写入
	}

	// 写回配置文件
	newData, err := json.MarshalIndent(trackedStreamers, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(ym.config.ChannelsConfigPath, newData, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// getVideos 获取频道的视频列表（VOD）
func (ym *YouTubeMonitor) getVideos(channelID string, maxResults int) ([]models.YouTubeVideoItem, error) {
	if maxResults <= 0 {
		maxResults = 1 // 默认获取1个视频
	}

	// 搜索该频道最近的视频，按发布时间倒序排列
	searchURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/search?part=snippet&channelId=%s&order=date&type=video&maxResults=%d",
		channelID, maxResults)

	resp, err := ym.makeRequestWithRetry(searchURL)
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

	if len(searchResp.Items) == 0 {
		return nil, fmt.Errorf("未找到视频")
	}

	// 获取视频的详细信息
	videoIDs := make([]string, 0, len(searchResp.Items))
	for _, item := range searchResp.Items {
		videoIDs = append(videoIDs, item.ID.VideoID)
	}

	videoURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/videos?part=snippet,liveStreamingDetails,contentDetails&id=%s",
		strings.Join(videoIDs, ","))

	videoResp, err := ym.makeRequestWithRetry(videoURL)
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

	return videoData.Items, nil
}

// isVODAlreadyProcessed 检查VOD是否已经处理过
func (ym *YouTubeMonitor) isVODAlreadyProcessed(videoID string) bool {
	// 检查 chat_logs 目录下是否存在该视频ID的文件
	files, err := os.ReadDir("./chat_logs")
	if err != nil {
		return false
	}

	for _, file := range files {
		if strings.Contains(file.Name(), videoID) {
			return true
		}
	}
	return false
}

// ProcessRecentVOD 处理最近的VOD
func (ym *YouTubeMonitor) ProcessRecentVOD(channelID, channelName string) {
	log.Printf("开始获取 %s 的最近视频...", channelName)

	// 获取最近的5个视频
	videos, err := ym.getVideos(channelID, 1)
	if err != nil {
		log.Printf("获取 %s 视频列表失败: %v", channelName, err)
		return
	}

	// 查找最近的一个直播VOD（有 liveStreamingDetails 的视频）
	var latestLiveVOD *models.YouTubeVideoItem
	for i := range videos {
		video := &videos[i]
		// 检查是否是直播录像（有actualStartTime表示这是个直播过的视频）
		if video.LiveStreamingDetails != nil && video.LiveStreamingDetails.ActualStartTime != "" {
			latestLiveVOD = video
			break
		}
	}

	if latestLiveVOD == nil {
		log.Printf("未找到 %s 的直播VOD", channelName)
		return
	}

	// 检查是否已经处理过
	if ym.isVODAlreadyProcessed(latestLiveVOD.ID) {
		log.Printf("视频 %s 已经处理过，跳过", latestLiveVOD.ID)
		return
	}

	log.Printf("找到最近的直播VOD: %s (%s)", latestLiveVOD.Snippet.Title, latestLiveVOD.ID)

	// 下载聊天记录
	if err := ym.downloadYouTubeLiveChat(latestLiveVOD, channelName); err != nil {
		log.Printf("下载YouTube聊天记录失败: %v", err)
		return
	}

	log.Printf("成功处理 %s 的VOD: %s", channelName, latestLiveVOD.Snippet.Title)
}

// downloadYouTubeLiveChat 下载YouTube直播聊天记录
func (ym *YouTubeMonitor) downloadYouTubeLiveChat(video *models.YouTubeVideoItem, channelName string) error {
	// 确保聊天日志目录存在
	if err := os.MkdirAll("./chat_logs", 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 构建文件名
	filename := fmt.Sprintf("chat_youtube_%s_%s.json", video.ID, time.Now().Format("20060102_150405"))
	filepath := filepath.Join("./chat_logs", filename)

	// 构建聊天数据结构
	chatData := struct {
		VideoID      string `json:"video_id"`
		ChannelName  string `json:"channel_name"`
		VideoTitle   string `json:"video_title"`
		VideoURL     string `json:"video_url"`
		StartTime    string `json:"start_time"`
		DownloadedAt string `json:"downloaded_at"`
		Note         string `json:"note"`
	}{
		VideoID:      video.ID,
		ChannelName:  channelName,
		VideoTitle:   video.Snippet.Title,
		VideoURL:     fmt.Sprintf("https://www.youtube.com/watch?v=%s", video.ID),
		StartTime:    video.LiveStreamingDetails.ActualStartTime,
		DownloadedAt: time.Now().Format(time.RFC3339),
		Note:         "YouTube聊天记录需要使用第三方工具（如yt-dlp）下载，此文件仅记录视频信息",
	}

	// 序列化为JSON
	jsonData, err := json.MarshalIndent(chatData, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	log.Printf("YouTube VOD信息已保存到: %s", filepath)

	// 提示：实际的聊天下载可以使用yt-dlp等工具
	log.Printf("提示：要下载实际的聊天记录，可以使用命令: yt-dlp --write-subs --sub-lang live_chat %s", chatData.VideoURL)

	return nil
}
