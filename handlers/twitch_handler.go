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
	"strings"
	"sync"
	"time"

	"subtuber-services/models"
	"subtuber-services/services"

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
	fetchVodCount     = "5"
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
	//GetVideoCommentsAndAnalysis(tm)

	if stream != nil {
		log.Printf("🔴 %s 正在直播！标题: %s, 观众: %d",
			stream.UserName, stream.Title, stream.ViewerCount)
	} else {
		log.Printf("⚫ %s 当前离线", tm.config.StreamerName)

		// 检测从直播状态变为离线状态
		if previousIsLive {
			log.Printf("🎬 检测到直播结束，开始自动下载聊天记录...")

			// 检查并下载最近的聊天记录进行分析
			go func() {
				newResults := GetVideoCommentsAndAnalysis(tm)
				if len(newResults) > 0 {
					log.Printf("📊 完成 %d 个新视频的分析", len(newResults))
					for _, result := range newResults {
						log.Printf("  - VideoID: %s, 热点时刻: %d", result.VideoID, len(result.HotMoments))
					}
				}
			}()
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

// autoDownloadRecentChats 自动下载最近录像的聊天记录，返回新完成分析的结果
func (m *TwitchMonitor) autoDownloadRecentChats() []AnalysisResult {
	log.Println("开始检查并下载未下载的聊天记录...")

	// 获取最近的录像列表（使用 getVideos 的正确签名）
	videosResp, err := m.getVideos(m.config.StreamerName, "archive", fetchVodCount, "")
	if err != nil {
		log.Printf("获取录像列表失败: %v", err)
		return nil
	}

	if len(videosResp.Videos) == 0 {
		log.Println("没有找到录像")
		return nil
	}

	log.Printf("找到 %d 个录像，开始检查...", len(videosResp.Videos))

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
		// 根据方法选择分析算法
		var hotMoments []VodCommentData
		var timeSeriesData []TimeSeriesDataPoint
		var analysisStats VodCommentStats

		analysisResult := FindHotCommentsIntervalSlidingFilter(response.Comments, 5)
		hotMoments = analysisResult.HotMoments
		timeSeriesData = analysisResult.TimeSeriesData
		analysisStats = analysisResult.Stats

		// 保存完整的分析结果到文件
		if err := saveAnalysisResultToFile(video.ID, hotMoments, timeSeriesData,
			video.UserName, analysisStats, &video); err != nil {
			log.Printf("保存分析结果失败: %v", err)
		}

		// 保存录像信息到 RPC（如果有视频信息）
		if response.VideoInfo != nil {
			saveStreamerVODInfoToRPC(
				response.VideoInfo.UserName,
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

		log.Printf("✅ 成功保存录像 %s 的聊天记录 (%d 条评论) 到: %s",
			video.ID, response.TotalComments, filePath)

		downloadedCount++

		// 避免请求过快
		time.Sleep(2 * time.Second)
	}

	log.Printf("聊天记录下载完成！新下载: %d 个，跳过: %d 个", downloadedCount, skippedCount)
	return newAnalysisResults
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

				aiService := NewAIService("aliyun", "")
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
	tempExtensions := []string{".ts", ".tmp", ".part", ".download"}

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
	videoInfo *models.TwitchVideoData) error {

	// 确保目录存在
	if err := os.MkdirAll("./analysis_results", 0755); err != nil {
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

	// 生成文件名
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join("./analysis_results", fmt.Sprintf("analysis_%s_%s.json", videoID, timestamp))

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

	// 查找最新的分析结果文件
	pattern := filepath.Join("./analysis_results", fmt.Sprintf("analysis_%s_*.json", videoID))
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

	// 使用最新的文件
	latestFile := matches[len(matches)-1]
	data, err := os.ReadFile(latestFile)
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

	c.JSON(http.StatusOK, result)
}

// ListAnalysisResults 列出所有分析结果
func ListAnalysisResults(c *gin.Context) {
	pattern := filepath.Join("./analysis_results", "analysis_*.json")
	matches, err := filepath.Glob(pattern)
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
	}

	var results []AnalysisListItem
	for _, file := range matches {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var result AnalysisResult
		if err := json.Unmarshal(data, &result); err != nil {
			continue
		}

		results = append(results, AnalysisListItem{
			VideoID:      result.VideoID,
			StreamerName: result.StreamerName,
			Title:        result.VideoInfo.Title,
			Method:       result.Method,
			AnalyzedAt:   result.AnalyzedAt,
			HotMoments:   len(result.HotMoments),
		})
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
		// TODO 这里默认了420的间隔也就是7min，后续可以修改为可配置的
		// 调用下载 VOD 片段的方法
		tm.downloadHotMomentClips(v.VideoID, v.HotMoments, 420)

	}

	return ars
}
