package handlers

import (
	"fmt"
	"math"
	"sort"
	"time"

	"subtuber-services/models"
)

// VodCommentData 分析结果数据
type VodCommentData struct {
	TimeInterval   string  `json:"time_interval"`
	CommentsScore  float64 `json:"comments_score"`
	OffsetSeconds  float64 `json:"offset_seconds"`
	FormattedTime  string  `json:"formatted_time,omitempty"` // 格式化的时间显示
}

// VodCommentStats 评论统计信息
type VodCommentStats struct {
	Mean  float64 `json:"mean"`
	Sigma float64 `json:"sigma"`
	Count int     `json:"count"`
	sum   float64
	sumSq float64
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

// FindHotCommentsTimelineIQR 使用IQR方法找到热门评论时间段
// IQR = Q3 - Q1
// 高亮时间是评论数 > Q3 + 1.5*IQR 的时间段
func FindHotCommentsTimelineIQR(comments []models.TwitchChatComment, intervalMinutes int) []VodCommentData {
	if len(comments) == 0 {
		return []VodCommentData{}
	}

	if intervalMinutes <= 0 {
		intervalMinutes = 5 // 默认5分钟
	}

	// 按时间分组并计数
	type TimeSlot struct {
		Time        time.Time
		Count       int
		BeginOffset float64
	}

	timeSlotMap := make(map[string]*TimeSlot)

	for _, comment := range comments {
		// 解析创建时间
		createdAt, err := time.Parse(time.RFC3339, comment.CreatedAt)
		if err != nil {
			continue
		}

		// 向下取整到指定分钟间隔
		minute := (createdAt.Minute() / intervalMinutes) * intervalMinutes
		slotTime := time.Date(
			createdAt.Year(),
			createdAt.Month(),
			createdAt.Day(),
			createdAt.Hour(),
			minute,
			0, 0, time.UTC,
		)

		key := slotTime.Format(time.RFC3339)
		if slot, exists := timeSlotMap[key]; exists {
			slot.Count++
		} else {
			timeSlotMap[key] = &TimeSlot{
				Time:        slotTime,
				Count:       1,
				BeginOffset: comment.ContentOffsetSeconds,
			}
		}
	}

	// 转换为切片并排序
	var timelineData []TimeSlot
	for _, slot := range timeSlotMap {
		timelineData = append(timelineData, *slot)
	}

	sort.Slice(timelineData, func(i, j int) bool {
		return timelineData[i].Time.Before(timelineData[j].Time)
	})

	// 按评论数排序以计算分位数
	countSorted := make([]int, len(timelineData))
	for i, slot := range timelineData {
		countSorted[i] = slot.Count
	}
	sort.Ints(countSorted)

	// 计算 Q1, Q3, IQR
	q1Index := int(float64(len(countSorted)) * 0.25)
	q3Index := int(float64(len(countSorted)) * 0.75)
	q1 := float64(countSorted[q1Index])
	q3 := float64(countSorted[q3Index])
	iqr := q3 - q1
	highThreshold := q3 + 1.5*iqr

	// 计算统计信息
	stats := VodCommentStats{}
	for _, slot := range timelineData {
		stats.AddData(VodCommentData{
			CommentsScore: float64(slot.Count),
		})
	}

	// 筛选高于阈值的时间段
	var result []VodCommentData
	for _, slot := range timelineData {
		if float64(slot.Count) > highThreshold {
			result = append(result, VodCommentData{
				TimeInterval:  slot.Time.Format("2006-01-02 15:04"),
				CommentsScore: float64(slot.Count),
				OffsetSeconds: slot.BeginOffset,
				FormattedTime: formatDuration(slot.BeginOffset),
			})
		}
	}

	// 按评论数降序排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].CommentsScore > result[j].CommentsScore
	})

	return result
}

// FindHotCommentsIntervalSlidingFilter 使用滑动滤波方式过滤峰值
func FindHotCommentsIntervalSlidingFilter(comments []models.TwitchChatComment, secondsDt int) []VodCommentData {
	if len(comments) == 0 {
		return []VodCommentData{}
	}

	if secondsDt <= 0 {
		secondsDt = 5 // 默认5秒间隔
	}

	// 提取所有时间偏移
	var offsetSeconds []float64
	for _, comment := range comments {
		offsetSeconds = append(offsetSeconds, comment.ContentOffsetSeconds)
	}

	// 排序
	sort.Float64s(offsetSeconds)

	startSecond := offsetSeconds[0]
	endSecond := offsetSeconds[len(offsetSeconds)-1]
	maxTime := endSecond - startSecond + float64(secondsDt)

	// 构建时间区间
	intervalLen := int(maxTime/float64(secondsDt)) + 1
	T := make([]float64, intervalLen)
	commentCountByDt := make([]float64, intervalLen)

	for i := 0; i < intervalLen; i++ {
		T[i] = float64(i * secondsDt)
	}

	// 分配数据到区间内
	for _, offset := range offsetSeconds {
		timeOffset := offset - startSecond
		k := int(math.Floor(timeOffset / float64(secondsDt)))
		if k >= 0 && k < intervalLen {
			commentCountByDt[k]++
		}
	}

	// 滑动窗口长度（默认10分钟）
	windowLength := (10 * 60) / secondsDt

	// 应用均值滤波
	filteredCount := meanFilter(commentCountByDt, windowLength+1)

	// 缩放结果
	scale := float64(windowLength + 1)
	for i := range filteredCount {
		filteredCount[i] *= scale
	}

	// 截取中间部分
	startIdx := windowLength / 2
	endIdx := len(T) - windowLength/2 - 1
	resultLength := endIdx - startIdx + 1
	T1 := make([]float64, resultLength)
	copy(T1, T[startIdx:endIdx+1])

	// 检测峰值
	peakIndex, peak, _ := detectPeaks(filteredCount, windowLength)

	// 过滤真实峰值
	peakIndexTrue, peakTrue := filterTruePeaks(peakIndex, peak, windowLength)

	// 构建结果
	var result []VodCommentData
	for i, index := range peakIndexTrue {
		if index < len(T1) {
			result = append(result, VodCommentData{
				TimeInterval:  "7min",
				CommentsScore: peakTrue[i],
				OffsetSeconds: T1[index] + startSecond,
				FormattedTime: formatDuration(T1[index] + startSecond),
			})
		}
	}

	return result
}

// meanFilter 均值滤波
func meanFilter(data []float64, windowSize int) []float64 {
	n := len(data)
	result := make([]float64, n)

	for i := 0; i < n; i++ {
		sum := 0.0
		count := 0
		halfWindow := windowSize / 2

		for j := i - halfWindow; j <= i+halfWindow; j++ {
			if j >= 0 && j < n {
				sum += data[j]
				count++
			}
		}

		if count > 0 {
			result[i] = sum / float64(count)
		}
	}

	return result
}

// detectPeaks 检测峰值
func detectPeaks(count []float64, windowLength int) ([]int, []float64, float64) {
	// 计算平均值
	sum := 0.0
	for _, val := range count {
		sum += val
	}
	meanVal := sum / float64(len(count))
	threshold := 1.3 * meanVal

	var peakIndex []int
	var peak []float64

	for i := 0; i <= len(count)-windowLength-1; i++ {
		// 提取窗口数据
		windowData := count[i : i+windowLength+1]

		// 找最大值
		maxVal := windowData[0]
		maxIdx := 0
		for j, val := range windowData {
			if val > maxVal {
				maxVal = val
				maxIdx = j
			}
		}

		// 跳过未超过阈值的情况
		if maxVal < threshold {
			continue
		}

		// 排除窗口边缘的极值点
		if maxIdx == 0 || maxIdx == len(windowData)-1 {
			continue
		}

		// 全局索引
		globalIndex := i + maxIdx

		// 判断是否已经记录了这个峰值
		if len(peakIndex) == 0 {
			peakIndex = append(peakIndex, globalIndex)
			peak = append(peak, maxVal)
		} else if globalIndex != peakIndex[len(peakIndex)-1] {
			peakIndex = append(peakIndex, globalIndex)
			peak = append(peak, maxVal)
		}
	}

	return peakIndex, peak, meanVal
}

// filterTruePeaks 过滤真实峰值
func filterTruePeaks(peakIndex []int, peak []float64, windowLength int) ([]int, []float64) {
	var peakIndexTrue []int
	var peakTrue []float64

	for i := 0; i < len(peak); i++ {
		nowIndex := peakIndex[i]
		nowPeak := peak[i]
		isTrue := true

		for j := 0; j < len(peak); j++ {
			if i == j {
				continue
			}

			distance := abs(peakIndex[j] - nowIndex)
			if distance > windowLength {
				continue
			}

			if peak[j] > nowPeak {
				isTrue = false
				break
			}
		}

		if isTrue {
			peakIndexTrue = append(peakIndexTrue, nowIndex)
			peakTrue = append(peakTrue, nowPeak)
		}
	}

	return peakIndexTrue, peakTrue
}

// abs 绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
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
