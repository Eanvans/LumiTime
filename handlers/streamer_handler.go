package handlers

import (
	"net/http"
	"subtuber-services/services"

	"github.com/gin-gonic/gin"
)

// StreamerInfo 主播信息结构
type StreamerInfo struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Title           string `json:"title"`
	Platform        string `json:"platform"`
	DurationSeconds string `json:"duration_seconds"`
	CreatedAt       string `json:"created_at"`
}

// GetStreamerByID 根据ID查询主播信息
func GetStreamerByID(c *gin.Context) {
	// idStr := c.Param("id")
	// id, err := strconv.Atoi(idStr)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{
	// 		"success": false,
	// 		"message": "无效的主播ID",
	// 	})
	// 	return
	// }

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"streamer": "mocked",
	})
}

// ListStreamers 查询主播列表
func ListStreamers(c *gin.Context) {
	// limitStr := c.DefaultQuery("limit", "10")
	// limit, _ := strconv.Atoi(limitStr)

	streamerService := services.GetStreamerService()

	streamerName := GetTwitchMonitor().config.StreamerName

	streamers, err := streamerService.ListStreamers(streamerName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "查询主播列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"streamers": streamers.Streamers,
		"total":     len(streamers.Streamers),
	})
}
