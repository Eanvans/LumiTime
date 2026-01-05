package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	config       TwitchConfig
	accessToken  string
	tokenExpiry  time.Time
	mu           sync.RWMutex
	latestStatus *models.TwitchStatusResponse
	stopCh       chan struct{}
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
	tm.latestStatus = status
	tm.mu.Unlock()

	if stream != nil {
		log.Printf("🔴 %s 正在直播！标题: %s, 观众: %d",
			stream.UserName, stream.Title, stream.ViewerCount)
	} else {
		log.Printf("⚫ %s 当前离线", tm.config.StreamerName)
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
