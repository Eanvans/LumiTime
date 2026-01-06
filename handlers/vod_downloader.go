package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"subtuber-services/services"
	"sync"
	"time"
)

// VODDownloadRequest 定义 VOD 下载请求的结构
type VODDownloadRequest struct {
	VODID        string  `json:"vod_id"`        // VOD ID (可以是完整URL或纯ID)
	StartTime    float64 `json:"start_time"`    // 开始时间（秒），可选
	EndTime      float64 `json:"end_time"`      // 结束时间（秒），可选
	Quality      string  `json:"quality"`       // 视频质量，如 "1080p60", "720p", "audio_only" 等
	OutputPath   string  `json:"output_path"`   // 输出路径（可选，默认为 downloads 目录）
	ExtractAudio bool    `json:"extract_audio"` // 是否提取音频
}

// VODDownloadResponse 定义下载响应
type VODDownloadResponse struct {
	Success      bool    `json:"success"`
	Message      string  `json:"message"`
	VideoPath    string  `json:"video_path,omitempty"`
	AudioPath    string  `json:"audio_path,omitempty"`
	SubtitlePath string  `json:"subtitle_path,omitempty"`
	Duration     float64 `json:"duration,omitempty"`
	DownloadTime float64 `json:"download_time,omitempty"`
}

// TwitchPlaylist M3U8 播放列表信息
type TwitchPlaylist struct {
	Qualities []QualityOption `json:"qualities"`
}

// QualityOption 质量选项
type QualityOption struct {
	Name       string `json:"name"`
	Resolution string `json:"resolution"`
	URL        string `json:"url"`
	Bandwidth  int    `json:"bandwidth"`
}

// TwitchGQLResponse Twitch GraphQL API 响应
type TwitchGQLResponse struct {
	Data struct {
		Video struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			LengthSeconds int    `json:"lengthSeconds"`
			Owner         struct {
				DisplayName string `json:"displayName"`
			} `json:"owner"`
		} `json:"video"`
		VideoPlaybackAccessToken struct {
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"videoPlaybackAccessToken"`
	} `json:"data"`
}

// VODDownloader VOD 下载器
type VODDownloader struct {
	httpClient *http.Client
	outputDir  string
	mu         sync.Mutex
}

// NewVODDownloader 创建新的 VOD 下载器
func NewVODDownloader(outputDir string) *VODDownloader {
	if outputDir == "" {
		outputDir = "./downloads"
	}

	// 确保输出目录存在
	os.MkdirAll(outputDir, 0755)

	return &VODDownloader{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		outputDir: outputDir,
	}
}

// ExtractVODID 从 URL 或字符串中提取 VOD ID
func (vd *VODDownloader) ExtractVODID(input string) string {
	// 匹配 twitch.tv/videos/123456
	re := regexp.MustCompile(`(?:twitch\.tv/videos/|^)(\d+)`)
	matches := re.FindStringSubmatch(input)
	if len(matches) > 1 {
		return matches[1]
	}
	return input
}

// GetVideoInfo 获取视频信息
func (vd *VODDownloader) GetVideoInfo(vodID string) (*TwitchGQLResponse, error) {
	// Twitch GraphQL API
	gqlQuery := fmt.Sprintf(`{
		"query": "query { video(id: \"%s\") { id title lengthSeconds owner { displayName } } videoPlaybackAccessToken(id: \"%s\", params: { platform: \"web\", playerBackend: \"mediaplayer\", playerType: \"site\" }) { value signature } }"
	}`, vodID, vodID)

	req, err := http.NewRequest("POST", "https://gql.twitch.tv/gql", strings.NewReader(gqlQuery))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Client-ID", "kimne78kx3ncx6brgo4mv6wki5h1ko")
	req.Header.Set("Content-Type", "application/json")

	resp, err := vd.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var gqlResp TwitchGQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, err
	}

	return &gqlResp, nil
}

// GetPlaylistURL 获取播放列表 URL
func (vd *VODDownloader) GetPlaylistURL(vodID string, token, signature string) (string, error) {
	playlistURL := fmt.Sprintf(
		"https://usher.ttvnw.net/vod/%s.m3u8?token=%s&sig=%s&allow_source=true&player=twitchweb",
		vodID, token, signature,
	)
	return playlistURL, nil
}

// ParseM3U8Playlist 解析 M3U8 播放列表获取可用质量
func (vd *VODDownloader) ParseM3U8Playlist(playlistURL string) (*TwitchPlaylist, error) {
	resp, err := vd.httpClient.Get(playlistURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	playlist := &TwitchPlaylist{
		Qualities: []QualityOption{},
	}

	lines := strings.Split(string(body), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "#EXT-X-MEDIA") {
			// 解析质量信息
			nameMatch := regexp.MustCompile(`NAME="([^"]+)"`).FindStringSubmatch(line)
			if len(nameMatch) > 1 {
				// 下一行应该是实际的 URL
				if i+2 < len(lines) && strings.HasPrefix(lines[i+1], "#EXT-X-STREAM-INF") {
					streamInfo := lines[i+1]
					url := strings.TrimSpace(lines[i+2])

					bandwidth := 0
					bandwidthMatch := regexp.MustCompile(`BANDWIDTH=(\d+)`).FindStringSubmatch(streamInfo)
					if len(bandwidthMatch) > 1 {
						fmt.Sscanf(bandwidthMatch[1], "%d", &bandwidth)
					}

					resolution := ""
					resMatch := regexp.MustCompile(`RESOLUTION=(\d+x\d+)`).FindStringSubmatch(streamInfo)
					if len(resMatch) > 1 {
						resolution = resMatch[1]
					}

					playlist.Qualities = append(playlist.Qualities, QualityOption{
						Name:       nameMatch[1],
						Resolution: resolution,
						URL:        url,
						Bandwidth:  bandwidth,
					})
				}
			}
		}
	}

	return playlist, nil
}

// DownloadVOD 下载 VOD
func (vd *VODDownloader) DownloadVOD(ctx context.Context, req *VODDownloadRequest) (*VODDownloadResponse, error) {
	startTime := time.Now()

	// 提取 VOD ID
	vodID := vd.ExtractVODID(req.VODID)

	// 获取视频信息
	videoInfo, err := vd.GetVideoInfo(vodID)
	if err != nil {
		return &VODDownloadResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to get video info: %v", err),
		}, err
	}

	if videoInfo.Data.Video.ID == "" {
		return &VODDownloadResponse{
			Success: false,
			Message: "Video not found or deleted",
		}, fmt.Errorf("video not found")
	}

	// 获取播放列表
	playlistURL, err := vd.GetPlaylistURL(
		vodID,
		videoInfo.Data.VideoPlaybackAccessToken.Value,
		videoInfo.Data.VideoPlaybackAccessToken.Signature,
	)
	if err != nil {
		return &VODDownloadResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to get playlist: %v", err),
		}, err
	}

	// 解析播放列表
	playlist, err := vd.ParseM3U8Playlist(playlistURL)
	if err != nil {
		return &VODDownloadResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to parse playlist: %v", err),
		}, err
	}

	// 选择质量
	selectedQuality := vd.selectQuality(playlist, req.Quality)
	if selectedQuality == nil {
		return &VODDownloadResponse{
			Success: false,
			Message: fmt.Sprintf("Quality '%s' not available", req.Quality),
		}, fmt.Errorf("quality not available")
	}

	// 确定输出路径
	outputDir := req.OutputPath
	if outputDir == "" {
		outputDir = vd.outputDir
	}
	os.MkdirAll(outputDir, 0755)

	// 生成文件名
	safeTitle := sanitizeFilename(videoInfo.Data.Video.Title)
	videoFilename := fmt.Sprintf("%s_%s.mp4", vodID, safeTitle)
	videoPath := filepath.Join(outputDir, videoFilename)

	// 检查 ffmpeg 是否可用
	if err := vd.checkFFmpeg(); err != nil {
		return &VODDownloadResponse{
			Success: false,
			Message: fmt.Sprintf("FFmpeg not found: %v. Please install FFmpeg to download videos.", err),
		}, err
	}

	// 使用 ffmpeg 下载视频
	err = vd.downloadWithFFmpeg(ctx, selectedQuality.URL, videoPath, req.StartTime, req.EndTime)
	if err != nil {
		return &VODDownloadResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to download video: %v", err),
		}, err
	}

	response := &VODDownloadResponse{
		Success:      true,
		Message:      "Video downloaded successfully",
		VideoPath:    videoPath,
		Duration:     float64(videoInfo.Data.Video.LengthSeconds),
		DownloadTime: time.Since(startTime).Seconds(),
	}

	// 如果需要提取音频
	audioFilename := fmt.Sprintf("%s_%s.mp3", vodID, safeTitle)
	audioPath := filepath.Join(outputDir, audioFilename)

	err = vd.extractAudio(ctx, videoPath, audioPath)
	if err != nil {
		response.Message += fmt.Sprintf("; Failed to extract audio: %v", err)
	} else {
		response.AudioPath = audioPath
		response.Message = "Video downloaded and audio extracted successfully"
	}

	// 使用必剪接口提取字幕
	if response.AudioPath != "" {
		subtitleFilename := fmt.Sprintf("%s_%s.srt", vodID, safeTitle)
		subtitlePath := filepath.Join(outputDir, subtitleFilename)

		log.Printf("Starting subtitle extraction for: %s", audioPath)

		// 读取音频文件
		audioFile, err := os.Open(audioPath)
		if err != nil {
			log.Printf("Failed to open audio file: %v", err)
			response.Message += "; Failed to open audio file for subtitle extraction"
		} else {
			audioData, err := io.ReadAll(audioFile)
			audioFile.Close()
			if err != nil {
				log.Printf("Failed to read audio file: %v", err)
				response.Message += "; Failed to read audio file for subtitle extraction"
			} else {
				// 创建必剪ASR实例并运行
				asr := services.NewBcutASR(audioData)
				asrResult, err := asr.Run()
				if err != nil {
					log.Printf("Failed to extract subtitles: %v", err)
					response.Message += fmt.Sprintf("; Failed to extract subtitles: %v", err)
				} else {
					// 转换为SRT格式并保存
					srtContent := vd.convertToSRT(asrResult)
					err = os.WriteFile(subtitlePath, []byte(srtContent), 0644)
					if err != nil {
						log.Printf("Failed to save subtitle file: %v", err)
						response.Message += "; Failed to save subtitle file"
					} else {
						response.SubtitlePath = subtitlePath
						response.Message = "Video downloaded, audio extracted, and subtitles generated successfully"
						log.Printf("Subtitles saved to: %s (segments: %d)", subtitlePath, len(asrResult.Segments))
					}
				}
			}
		}
	}

	return response, nil
}

// formatSRTTimestamp 格式化时间戳为SRT格式 (HH:MM:SS,mmm)
func (vd *VODDownloader) formatSRTTimestamp(ms int64) string {
	totalSeconds := ms / 1000
	milliseconds := ms % 1000
	seconds := totalSeconds % 60
	minutes := (totalSeconds / 60) % 60
	hours := totalSeconds / 3600
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, milliseconds)
}

// convertToSRT 将ASR结果转换为SRT格式
func (vd *VODDownloader) convertToSRT(result *services.ASRResult) string {
	if result == nil || len(result.Segments) == 0 {
		return ""
	}

	var srt strings.Builder
	for i, segment := range result.Segments {
		// 序号
		srt.WriteString(fmt.Sprintf("%d\n", i+1))
		// 时间戳
		startTime := vd.formatSRTTimestamp(segment.StartTime)
		endTime := vd.formatSRTTimestamp(segment.EndTime)
		srt.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))
		// 文本内容
		srt.WriteString(segment.Text)
		srt.WriteString("\n\n")
	}
	return srt.String()
}

// selectQuality 选择最合适的质量
func (vd *VODDownloader) selectQuality(playlist *TwitchPlaylist, preferredQuality string) *QualityOption {
	if len(playlist.Qualities) == 0 {
		return nil
	}

	// 如果指定了质量，尝试匹配
	if preferredQuality != "" {
		for i := range playlist.Qualities {
			if strings.Contains(strings.ToLower(playlist.Qualities[i].Name), strings.ToLower(preferredQuality)) {
				return &playlist.Qualities[i]
			}
		}
	}

	// 默认返回第一个（通常是最高质量）
	return &playlist.Qualities[0]
}

// checkFFmpeg 检查 ffmpeg 是否可用
func (vd *VODDownloader) checkFFmpeg() error {
	cmd := exec.Command("ffmpeg", "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH")
	}
	return nil
}

// downloadWithFFmpeg 使用 ffmpeg 下载视频
func (vd *VODDownloader) downloadWithFFmpeg(ctx context.Context, m3u8URL, outputPath string, startTime, endTime float64) error {
	args := []string{
		"-i", m3u8URL,
		"-c", "copy",
		"-bsf:a", "aac_adtstoasc",
	}

	// 添加时间裁剪参数
	if startTime > 0 {
		args = append([]string{"-ss", fmt.Sprintf("%.2f", startTime)}, args...)
	}
	if endTime > 0 {
		args = append(args, "-to", fmt.Sprintf("%.2f", endTime))
	}

	args = append(args, "-y", outputPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// extractAudio 从视频中提取音频
func (vd *VODDownloader) extractAudio(ctx context.Context, videoPath, audioPath string) error {
	args := []string{
		"-i", videoPath,
		"-vn",                   // 不包含视频
		"-acodec", "libmp3lame", // 使用 MP3 编码
		"-ab", "192k", // 音频比特率
		"-ar", "44100", // 采样率
		"-y", // 覆盖输出文件
		audioPath,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// sanitizeFilename 清理文件名中的非法字符
func sanitizeFilename(filename string) string {
	// 移除或替换非法字符
	reg := regexp.MustCompile(`[<>:"/\\|?*]`)
	filename = reg.ReplaceAllString(filename, "_")

	// 限制长度
	if len(filename) > 100 {
		filename = filename[:100]
	}

	return filename
}

// HandleVODDownload HTTP 处理器
func HandleVODDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req VODDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// 创建下载器
	downloader := NewVODDownloader("./downloads")

	// 下载 VOD
	ctx := r.Context()
	resp, err := downloader.DownloadVOD(ctx, &req)

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	json.NewEncoder(w).Encode(resp)
}

// HandleVODInfo 获取 VOD 信息的处理器
func HandleVODInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vodID := r.URL.Query().Get("vod_id")
	if vodID == "" {
		http.Error(w, "vod_id parameter required", http.StatusBadRequest)
		return
	}

	downloader := NewVODDownloader("")
	vodID = downloader.ExtractVODID(vodID)

	// 获取视频信息
	videoInfo, err := downloader.GetVideoInfo(vodID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get video info: %v", err), http.StatusInternalServerError)
		return
	}

	// 获取播放列表
	playlistURL, err := downloader.GetPlaylistURL(
		vodID,
		videoInfo.Data.VideoPlaybackAccessToken.Value,
		videoInfo.Data.VideoPlaybackAccessToken.Signature,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get playlist: %v", err), http.StatusInternalServerError)
		return
	}

	// 解析播放列表
	playlist, err := downloader.ParseM3U8Playlist(playlistURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse playlist: %v", err), http.StatusInternalServerError)
		return
	}

	// 构建响应
	response := map[string]interface{}{
		"vod_id":    vodID,
		"title":     videoInfo.Data.Video.Title,
		"duration":  videoInfo.Data.Video.LengthSeconds,
		"streamer":  videoInfo.Data.Video.Owner.DisplayName,
		"qualities": playlist.Qualities,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
