package handlers

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"subtuber-services/models"

	"github.com/gin-gonic/gin"
)

// VodCommentData 分析结果数据
type VodCommentData struct {
	TimeInterval  string  `json:"time_interval"`
	CommentsScore float64 `json:"comments_score"`
	OffsetSeconds float64 `json:"offset_seconds"`
	FormattedTime string  `json:"formatted_time,omitempty"` // 格式化的时间显示
}

// TimeSeriesDataPoint 时间序列数据点
type TimeSeriesDataPoint struct {
	OffsetSeconds float64 `json:"offset_seconds"`
	FormattedTime string  `json:"formatted_time"`
	Score         float64 `json:"score"`
	IsPeak        bool    `json:"is_peak"` // 是否为峰值点
}

// AnalysisResultWithTimeSeries 包含时间序列的完整分析结果
type AnalysisResultWithTimeSeries struct {
	HotMoments     []VodCommentData      `json:"hot_moments"`
	TimeSeriesData []TimeSeriesDataPoint `json:"time_series_data"`
	Stats          VodCommentStats       `json:"stats"`
}

// VodCommentStats 评论统计信息
type VodCommentStats struct {
	Mean  float64 `json:"mean"`
	Sigma float64 `json:"sigma"`
	Count int     `json:"count"`
	sum   float64
	sumSq float64
}

// PeakDetectionParams 峰值检测参数
type PeakDetectionParams struct {
	WindowsLen  int     // 滑动窗口长度（秒），用于计算评论密度，默认120
	Thr         float64 // 阈值百分位（0-1），只考虑超过该百分位的密度值，默认0.9
	SearchRange int     // 搜索范围（秒），在此范围内查找局部最大值，默认60
}

// AddData 添加数据点
func (s *VodCommentStats) AddData(data VodCommentData) {
	s.Count++
	s.sum += data.CommentsScore
	s.sumSq += data.CommentsScore * data.CommentsScore
	s.Mean = s.sum / float64(s.Count)
	if s.Count > 1 {
		variance := (s.sumSq - s.sum*s.sum/float64(s.Count)) / float64(s.Count-1)
		s.Sigma = math.Sqrt(variance)
	}
}

// FindHotCommentsWithParams 使用自定义参数的峰值检测
func FindHotCommentsWithParams(comments []models.TwitchChatComment, secondsDt int,
	params PeakDetectionParams) AnalysisResultWithTimeSeries {
	if len(comments) == 0 {
		return AnalysisResultWithTimeSeries{
			HotMoments:     []VodCommentData{},
			TimeSeriesData: []TimeSeriesDataPoint{},
		}
	}

	if secondsDt <= 0 {
		secondsDt = 5 // 默认5秒间隔
	}

	// 设置默认参数
	if params.WindowsLen <= 0 {
		params.WindowsLen = 120
	}
	if params.Thr <= 0 || params.Thr > 1 {
		params.Thr = 0.9
	}
	if params.SearchRange <= 0 {
		params.SearchRange = 60
	}

	// 提取所有时间偏移并找到时间范围
	var offsetSeconds []float64
	for _, comment := range comments {
		offsetSeconds = append(offsetSeconds, comment.ContentOffsetSeconds)
	}

	if len(offsetSeconds) == 0 {
		return AnalysisResultWithTimeSeries{
			HotMoments:     []VodCommentData{},
			TimeSeriesData: []TimeSeriesDataPoint{},
		}
	}

	// 找到最大时间偏移
	maxOffset := 0.0
	for _, offset := range offsetSeconds {
		if offset > maxOffset {
			maxOffset = offset
		}
	}

	// 构建按秒计数的评论数组（从0到最大时间）
	totalSeconds := int(math.Ceil(maxOffset)) + 1
	commentCountPerSecond := make([]float64, totalSeconds)

	// 统计每秒的评论数
	for _, offset := range offsetSeconds {
		timeIndex := int(math.Floor(offset))
		if timeIndex >= 0 && timeIndex < totalSeconds {
			commentCountPerSecond[timeIndex]++
		}
	}

	// 使用新算法检测峰值
	isPeak, commentDensity := findPeakWithParams(commentCountPerSecond, params)

	// 构建时间序列数据
	var timeSeriesData []TimeSeriesDataPoint
	for i := 0; i < len(commentDensity); i++ {
		timeSeriesData = append(timeSeriesData, TimeSeriesDataPoint{
			OffsetSeconds: float64(i),
			FormattedTime: formatDuration(float64(i)),
			Score:         commentDensity[i],
			IsPeak:        isPeak[i],
		})
	}

	// 提取峰值点作为热点时刻
	var hotMoments []VodCommentData
	for i := 0; i < len(isPeak); i++ {
		if isPeak[i] {
			hotMoments = append(hotMoments, VodCommentData{
				TimeInterval:  fmt.Sprintf("%ds", params.WindowsLen),
				CommentsScore: commentDensity[i],
				OffsetSeconds: float64(i),
				FormattedTime: formatDuration(float64(i)),
			})
		}
	}

	// 根据searchRange合并接近的热点时刻
	hotMoments = mergeCloseHotMoments(hotMoments, params.SearchRange)

	// 计算统计信息
	stats := VodCommentStats{}
	for _, point := range timeSeriesData {
		stats.Count++
		stats.sum += point.Score
		stats.sumSq += point.Score * point.Score
	}
	if stats.Count > 0 {
		stats.Mean = stats.sum / float64(stats.Count)
		if stats.Count > 1 {
			variance := (stats.sumSq - stats.sum*stats.sum/float64(stats.Count)) / float64(stats.Count-1)
			stats.Sigma = math.Sqrt(variance)
		}
	}

	return AnalysisResultWithTimeSeries{
		HotMoments:     hotMoments,
		TimeSeriesData: timeSeriesData,
		Stats:          stats,
	}
}

// findPeakWithParams 基于MATLAB算法的峰值检测
// 参数:
//
//	comment: 每秒的评论数数组
//	params: 峰值检测参数
//
// 返回:
//
//	isPeak: 标识每个时间点是否为峰值的布尔数组
//	commentDensity: 计算得到的评论密度数组
func findPeakWithParams(comment []float64, params PeakDetectionParams) ([]bool, []float64) {
	n := len(comment)
	if n == 0 {
		return []bool{}, []float64{}
	}

	// 创建卷积核（长度为windowsLen+1的全1数组）
	kernel := make([]float64, params.WindowsLen+1)
	for i := range kernel {
		kernel[i] = 1.0
	}

	// 使用卷积计算评论密度（same模式）
	commentDensity := convSame(comment, kernel)

	// 计算阈值密度（使用百分位）
	sortedDensity := make([]float64, len(commentDensity))
	copy(sortedDensity, commentDensity)
	sort.Float64s(sortedDensity)

	thrIndex := int(math.Floor(float64(len(commentDensity)) * params.Thr))
	if thrIndex >= len(sortedDensity) {
		thrIndex = len(sortedDensity) - 1
	}
	thrDensity := sortedDensity[thrIndex]

	// 对commentDensity进行padding，方便搜索
	commentDenPad := make([]float64, len(commentDensity)+2*params.SearchRange)
	// 前面填充searchRange个0
	for i := 0; i < params.SearchRange; i++ {
		commentDenPad[i] = 0
	}
	// 复制原始数据
	copy(commentDenPad[params.SearchRange:], commentDensity)
	// 后面填充searchRange个0
	for i := len(commentDensity) + params.SearchRange; i < len(commentDenPad); i++ {
		commentDenPad[i] = 0
	}

	// 检测峰值
	isPeak := make([]bool, n)
	for i := 0; i < len(commentDensity); i++ {
		isPeak[i] = false

		// 如果小于阈值，跳过
		if commentDensity[i] < thrDensity {
			continue
		}

		// 在搜索范围内查找最大值
		ind := i + params.SearchRange
		tmpData := commentDenPad[ind-params.SearchRange : ind+params.SearchRange+1]

		// 找到最大值
		maxVal := tmpData[0]
		for _, val := range tmpData {
			if val > maxVal {
				maxVal = val
			}
		}

		// 如果当前值等于最大值，则为峰值
		if commentDensity[i] == maxVal {
			isPeak[i] = true
		}
	}

	return isPeak, commentDensity
}

// mergeCloseHotMoments 合并接近的热点时刻
// 在searchRange范围内，如果有多个接近的热点时刻，只保留得分最高的一个
func mergeCloseHotMoments(hotMoments []VodCommentData, searchRange int) []VodCommentData {
	if len(hotMoments) <= 1 {
		return hotMoments
	}

	// 按offset_seconds排序
	sort.Slice(hotMoments, func(i, j int) bool {
		return hotMoments[i].OffsetSeconds < hotMoments[j].OffsetSeconds
	})

	var merged []VodCommentData
	i := 0

	for i < len(hotMoments) {
		// 找出与当前热点时刻在searchRange范围内的所有热点
		group := []VodCommentData{hotMoments[i]}
		j := i + 1

		for j < len(hotMoments) {
			// 计算时间差
			timeDiff := hotMoments[j].OffsetSeconds - hotMoments[i].OffsetSeconds

			// 如果在searchRange范围内，加入到group中
			if timeDiff <= float64(searchRange) {
				group = append(group, hotMoments[j])
				j++
			} else {
				break
			}
		}

		// 在group中找到得分最高的热点时刻
		maxScoreIndex := 0
		maxScore := group[0].CommentsScore
		for k := 1; k < len(group); k++ {
			if group[k].CommentsScore > maxScore {
				maxScore = group[k].CommentsScore
				maxScoreIndex = k
			}
		}

		// 将得分最高的热点时刻添加到结果中
		merged = append(merged, group[maxScoreIndex])

		// 移动到下一个未处理的热点时刻
		i = j
	}

	return merged
}

// convSame 卷积运算（same模式）
// 实现MATLAB中的conv(x, kernel, 'same')
func convSame(signal []float64, kernel []float64) []float64 {
	n := len(signal)
	m := len(kernel)
	result := make([]float64, n)

	// 计算偏移量以实现'same'模式
	offset := (m - 1) / 2

	for i := 0; i < n; i++ {
		sum := 0.0
		for j := 0; j < m; j++ {
			signalIndex := i - offset + j
			if signalIndex >= 0 && signalIndex < n {
				sum += signal[signalIndex] * kernel[j]
			}
		}
		result[i] = sum
	}

	return result
}

// formatDuration 格式化时长为可读格式
func formatDuration(seconds float64) string {
	duration := time.Duration(seconds) * time.Second
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	secs := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

// GetAnalysisSummary 根据videoID和offset_seconds获取对应的分析摘要
// 查找analysis_results/{videoID}_{provider}/目录下最接近offset_seconds的summary文件
func GetAnalysisSummary(c *gin.Context) {
	videoID := c.Query("video_id")
	offsetSecondsStr := c.Query("offset_seconds")

	if videoID == "" || offsetSecondsStr == "" {
		c.JSON(400, gin.H{
			"error": "video_id and offset_seconds are required",
		})
		return
	}

	// 转换offset_seconds为float64
	offsetSeconds, err := strconv.ParseFloat(offsetSecondsStr, 64)
	if err != nil {
		c.JSON(400, gin.H{
			"error": "invalid offset_seconds format",
		})
		return
	}

	// 查找analysis_results目录
	analysisDir := "analysis_results"
	entries, err := os.ReadDir(analysisDir)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "failed to read analysis_results directory",
		})
		return
	}

	// 查找匹配videoID的文件夹
	var targetDir string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), videoID) {
			targetDir = filepath.Join(analysisDir, entry.Name())
			break
		}
	}

	if targetDir == "" {
		c.JSON(404, gin.H{
			"error": "no analysis results found for video_id: " + videoID,
		})
		return
	}

	// 读取目标目录下的所有summary文件
	summaryFiles, err := filepath.Glob(filepath.Join(targetDir, "*_summary.txt"))
	if err != nil || len(summaryFiles) == 0 {
		c.JSON(404, gin.H{
			"error": "no summary files found",
		})
		return
	}

	// 找到最接近offset_seconds的文件
	var closestFile string
	minDiff := math.MaxFloat64

	for _, file := range summaryFiles {
		// 从文件名中提取数字
		basename := filepath.Base(file)
		parts := strings.Split(basename, "_")
		if len(parts) < 2 {
			continue
		}

		fileOffset, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			continue
		}

		diff := math.Abs(fileOffset - offsetSeconds)
		if diff < minDiff {
			minDiff = diff
			closestFile = file
		}
	}

	if closestFile == "" {
		c.JSON(404, gin.H{
			"error": "no matching summary file found",
		})
		return
	}

	// 读取文件内容
	content, err := os.ReadFile(closestFile)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "failed to read summary file",
		})
		return
	}

	// 从文件名中提取offset
	basename := filepath.Base(closestFile)
	parts := strings.Split(basename, "_")
	actualOffset := ""
	if len(parts) >= 2 {
		actualOffset = parts[1]
	}

	c.JSON(200, gin.H{
		"actual_offset": actualOffset,
		"summary":       string(content),
	})
}
