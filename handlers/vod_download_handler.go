package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

// VODDownloadHandler handles VOD (Video On Demand) download operations
type VODDownloadHandler struct {
	twitchMonitor *TwitchMonitor
	accessToken   string
	tokenExpiry   time.Time
	mu            sync.RWMutex
}

var (
	vodHandler     *VODDownloadHandler
	vodHandlerOnce sync.Once
)

// InitVODDownloadHandler initializes the VOD download handler
func InitVODDownloadHandler(monitor *TwitchMonitor) *VODDownloadHandler {
	vodHandlerOnce.Do(func() {
		vodHandler = &VODDownloadHandler{
			twitchMonitor: monitor,
		}
	})
	return vodHandler
}

// GetVODDownloadHandler returns the VOD download handler instance
func GetVODDownloadHandler() *VODDownloadHandler {
	return vodHandler
}

// ensureValidToken ensures we have a valid OAuth access token
// TODO: 实现获取 OAuth token 的逻辑
func (h *VODDownloadHandler) ensureValidToken() error {
	h.mu.RLock()
	if h.accessToken != "" && time.Now().Before(h.tokenExpiry) {
		h.mu.RUnlock()
		return nil
	}
	h.mu.RUnlock()

	// Get new token from Twitch monitor
	if h.twitchMonitor != nil {
		if err := h.twitchMonitor.ensureValidToken(); err != nil {
			return err
		}

		h.twitchMonitor.mu.RLock()
		token := h.twitchMonitor.accessToken
		expiry := h.twitchMonitor.tokenExpiry
		h.twitchMonitor.mu.RUnlock()

		h.mu.Lock()
		h.accessToken = token
		h.tokenExpiry = expiry
		h.mu.Unlock()

		log.Println("VOD handler: OAuth token refreshed")
		return nil
	}

	return fmt.Errorf("Twitch monitor not initialized")
}

// DownloadVODChat downloads VOD chat records HTTP handler
func DownloadVODChat(c *gin.Context) {
	handler := GetVODDownloadHandler()
	if handler == nil || handler.twitchMonitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "VOD下载服务未启动",
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

	// Ensure valid access token
	if err := handler.ensureValidToken(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取访问令牌失败: " + err.Error(),
		})
		return
	}

	// Download chat records
	response, err := handler.downloadChatComments(req.VideoID, req.StartTime, req.EndTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "下载聊天记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// SaveVODChatToFile saves VOD chat records to file
func SaveVODChatToFile(c *gin.Context) {
	handler := GetVODDownloadHandler()
	if handler == nil || handler.twitchMonitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "VOD下载服务未启动",
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

	// Ensure valid access token
	if err := handler.ensureValidToken(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取访问令牌失败: " + err.Error(),
		})
		return
	}

	// Download chat records
	response, err := handler.downloadChatComments(req.VideoID, req.StartTime, req.EndTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "下载聊天记录失败: " + err.Error(),
		})
		return
	}

	// Save to file
	filename := fmt.Sprintf("chat_%s_%s.json", req.VideoID, time.Now().Format("20060102_150405"))
	filePath := filepath.Join("./chat_logs", filename)

	// Ensure directory exists
	if err := os.MkdirAll("./chat_logs", 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建目录失败: " + err.Error(),
		})
		return
	}

	// Serialize data to JSON
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "序列化JSON失败: " + err.Error(),
		})
		return
	}

	// Write to file
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "写入文件失败: " + err.Error(),
		})
		return
	}

	log.Printf("聊天记录已保存到文件: %s", filePath)

	c.JSON(http.StatusOK, gin.H{
		"message":        "聊天记录已成功保存",
		"filename":       filename,
		"filepath":       filePath,
		"total_comments": response.TotalComments,
		"video_id":       response.VideoID,
	})
}

// downloadChatComments downloads VOD chat records using GraphQL API
func (h *VODDownloadHandler) downloadChatComments(videoID string, startTime, endTime *float64) (*models.TwitchChatDownloadResponse, error) {
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

	// Get video information
	videoInfo, err := h.getVideoInfo(videoID)
	if err != nil {
		log.Printf("获取视频信息失败: %v", err)
		// Continue downloading chat even if video info fails
	}

	for hasNextPage {
		var requestBody map[string]interface{}

		if isFirstRequest {
			// First request uses contentOffsetSeconds
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
			// Subsequent requests use cursor for pagination
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

		// Serialize request body
		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("序列化请求失败: %w", err)
		}

		// Create HTTP request
		req, err := http.NewRequest("POST", gqlURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %w", err)
		}

		req.Header.Set("Client-ID", clientID)
		req.Header.Set("Content-Type", "application/json")

		// Send request
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

		// Parse response
		var gqlResp models.TwitchGQLCommentResponse
		if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}

		// Check if there are comment data
		if len(gqlResp.Data.Video.Comments.Edges) == 0 {
			log.Printf("没有更多评论数据，当前游标: %s", cursor)
			break
		}

		// Collect comments
		for _, edge := range gqlResp.Data.Video.Comments.Edges {
			node := edge.Node

			// If end time is specified, check if out of range
			if endTime != nil && float64(node.ContentOffsetSeconds) > *endTime {
				hasNextPage = false
				break
			}

			// If start time is specified, only collect comments after start time
			if startTime != nil && float64(node.ContentOffsetSeconds) < *startTime {
				continue
			}

			// Convert to TwitchChatComment format
			comment := convertGQLNodeToComment(node, videoID)
			allComments = append(allComments, comment)
			cursor = edge.Cursor
		}

		log.Printf("已获取 %d 条评论，总计: %d", len(gqlResp.Data.Video.Comments.Edges), len(allComments))

		// Check if there is next page
		hasNextPage = hasNextPage && gqlResp.Data.Video.Comments.PageInfo.HasNextPage

		// Avoid requesting too fast
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

// getVideoInfo gets video information using OAuth token
func (h *VODDownloadHandler) getVideoInfo(videoID string) (*models.TwitchVideoData, error) {
	if err := h.ensureValidToken(); err != nil {
		return nil, err
	}

	h.mu.RLock()
	token := h.accessToken
	h.mu.RUnlock()

	url := fmt.Sprintf("https://api.twitch.tv/helix/videos?id=%s", videoID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Client-ID", h.twitchMonitor.config.ClientID)
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

// convertGQLNodeToComment converts GraphQL node to TwitchChatComment format
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

	// Convert Commenter
	if node.Commenter != nil {
		comment.Commenter = models.TwitchChatCommenter{
			ID:          node.Commenter.ID,
			DisplayName: node.Commenter.DisplayName,
			Name:        node.Commenter.Login,
		}
	}

	// Convert Message
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

	// Convert UserBadges
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

// AutoDownloadRecentChats automatically downloads recent VOD chat records
func (h *VODDownloadHandler) AutoDownloadRecentChats() {
	log.Println("开始检查并下载未下载的聊天记录...")

	if h.twitchMonitor == nil {
		log.Println("Twitch monitor未初始化")
		return
	}

	// Get recent video list
	videosResp, err := h.twitchMonitor.getVideos(h.twitchMonitor.config.StreamerName, "archive", "20", "")
	if err != nil {
		log.Printf("获取录像列表失败: %v", err)
		return
	}

	if len(videosResp.Videos) == 0 {
		log.Println("没有找到录像")
		return
	}

	log.Printf("找到 %d 个录像，开始检查...", len(videosResp.Videos))

	// Ensure chat log directory exists
	if err := os.MkdirAll("./chat_logs", 0755); err != nil {
		log.Printf("创建聊天日志目录失败: %v", err)
		return
	}

	downloadedCount := 0
	skippedCount := 0

	for _, video := range videosResp.Videos {
		// Check if already downloaded
		if h.isChatAlreadyDownloaded(video.ID) {
			log.Printf("跳过已下载的录像: %s (%s)", video.ID, video.Title)
			skippedCount++
			continue
		}

		log.Printf("开始下载录像 %s 的聊天记录: %s", video.ID, video.Title)

		// Download chat records
		response, err := h.downloadChatComments(video.ID, nil, nil)
		if err != nil {
			log.Printf("下载录像 %s 的聊天记录失败: %v", video.ID, err)
			continue
		}

		// Save to file
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

		// Perform data analysis
		var hotMoments []VodCommentData
		var timeSeriesData []TimeSeriesDataPoint
		var analysisStats VodCommentStats

		analysisResult := FindHotCommentsIntervalSlidingFilter(response.Comments, 5)
		hotMoments = analysisResult.HotMoments
		timeSeriesData = analysisResult.TimeSeriesData
		analysisStats = analysisResult.Stats

		// Save complete analysis results to file
		if err := saveAnalysisResultToFile(video.ID, hotMoments, timeSeriesData,
			video.UserName, analysisStats, &video); err != nil {
			log.Printf("保存分析结果失败: %v", err)
		}

		// Save video info to RPC (if video info available)
		if response.VideoInfo != nil {
			saveStreamerVODInfoToRPC(
				response.VideoInfo.UserName,
				response.VideoInfo.Title,
				"Twitch",
				response.VideoInfo.Duration,
				response.VideoID)
		}

		log.Printf("✅ 成功保存录像 %s 的聊天记录 (%d 条评论) 到: %s",
			video.ID, response.TotalComments, filePath)
		downloadedCount++

		// Avoid requesting too fast
		time.Sleep(2 * time.Second)
	}

	log.Printf("聊天记录下载完成！新下载: %d 个，跳过: %d 个", downloadedCount, skippedCount)
}

// isChatAlreadyDownloaded checks if chat record is already downloaded
func (h *VODDownloadHandler) isChatAlreadyDownloaded(videoID string) bool {
	// Check if file exists in chat_logs directory for this video ID
	pattern := filepath.Join("./chat_logs", fmt.Sprintf("chat_%s_*.json", videoID))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("检查文件失败: %v", err)
		return false
	}
	return len(matches) > 0
}

// loadChatFromFile loads chat records from file
func loadChatFromFile(videoID string) (*models.TwitchChatDownloadResponse, error) {
	pattern := filepath.Join("./chat_logs", fmt.Sprintf("chat_%s_*.json", videoID))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("未找到视频 %s 的聊天记录文件", videoID)
	}

	// Use the latest file
	latestFile := matches[len(matches)-1]
	data, err := os.ReadFile(latestFile)
	if err != nil {
		return nil, err
	}

	var chatData models.TwitchChatDownloadResponse
	if err := json.Unmarshal(data, &chatData); err != nil {
		return nil, err
	}

	return &chatData, nil
}

// AnalysisResult complete analysis results (for saving)
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

// saveAnalysisResultToFile saves analysis results to file
func saveAnalysisResultToFile(videoID string, hotMoments []VodCommentData,
	timeSeriesData []TimeSeriesDataPoint, name string, stats VodCommentStats,
	videoInfo *models.TwitchVideoData) error {

	// Ensure directory exists
	if err := os.MkdirAll("./analysis_results", 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// Build complete analysis result
	result := AnalysisResult{
		VideoID:        videoID,
		StreamerName:   name,
		HotMoments:     hotMoments,
		TimeSeriesData: timeSeriesData,
		Stats:          stats,
		VideoInfo:      *videoInfo,
		AnalyzedAt:     time.Now(),
	}

	// Generate filename
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join("./analysis_results", fmt.Sprintf("analysis_%s_%s.json", videoID, timestamp))

	// Serialize to JSON
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	log.Printf("分析结果已保存到: %s", filename)
	return nil
}

// saveStreamerVODInfoToRPC asynchronously saves stream data to RPC service
func saveStreamerVODInfoToRPC(streamerName string, streamTitle string,
	streamPlatform string, duration string, videoId string) {
	streamerService := services.GetStreamerService()
	if streamerService == nil {
		log.Println("RPC 服务未初始化，跳过保存分析结果")
		return
	}

	// Save to RPC
	if _, err := streamerService.CreateStreamer(streamerName, streamTitle,
		streamPlatform, duration, videoId); err != nil {
		log.Printf("结果保存到 RPC 失败: %v", err)
	} else {
		log.Printf("结果已保存到 RPC: Streamer=%s, Title=%s", streamerName, streamTitle)
	}
}

// GetAnalysisResult gets analysis result
func GetAnalysisResult(c *gin.Context) {
	videoID := c.Param("videoID")
	if videoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少视频ID",
		})
		return
	}

	// Find latest analysis result file
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

	// Use the latest file
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

// ListAnalysisResults lists all analysis results
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

	// Sort by analysis time in descending order
	sort.Slice(results, func(i, j int) bool {
		return results[i].AnalyzedAt.After(results[j].AnalyzedAt)
	})

	c.JSON(http.StatusOK, gin.H{
		"total":   len(results),
		"results": results,
	})
}
