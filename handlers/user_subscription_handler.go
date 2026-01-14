package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"subtuber-services/models"
	"subtuber-services/services"
	"time"

	"github.com/gin-gonic/gin"
)

const userSubscriptionsFile = "App_Data/user_subscriptions.json"

// UserSubscriptions 存储所有用户的订阅数据
type UserSubscriptions struct {
	Subscriptions map[string][]UserSubscription `json:"subscriptions"` // key: userHash
}

// UserSubscription 用户订阅信息
type UserSubscription struct {
	StreamerID   string    `json:"streamer_id"`
	StreamerName string    `json:"streamer_name"`
	Platform     string    `json:"platform"`
	SubscribedAt time.Time `json:"subscribed_at"`
}

// getUserHashFromCookie 从 cookie 中获取用户 hash
func getUserHashFromCookie(c *gin.Context) (string, error) {
	userInfoCookie, err := c.Cookie("UserInfo")
	if err != nil {
		return "", err
	}

	var user struct {
		UserId string `json:"userId"`
	}
	if err := json.Unmarshal([]byte(userInfoCookie), &user); err != nil {
		return "", err
	}

	return user.UserId, nil
}

// GetUserSubscriptions 通过 RPC 获取用户订阅的主播列表
func GetUserSubscriptions(c *gin.Context) {
	// 从 cookie 获取用户信息
	userHash, err := getUserHashFromCookie(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录或登录已过期",
		})
		return
	}

	// 调用 RPC 服务获取订阅列表
	resp, err := services.GetUserSubscriptions(userHash)
	if err != nil {
		log.Printf("获取用户订阅列表失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取订阅列表失败: " + err.Error(),
		})
		return
	}

	streamers, err := GetTrackedStreamerData()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取订阅列表失败: " + err.Error(),
		})
		return
	}

	var subedStreamers []models.StreamerInfo
	for _, sub := range resp.Subscriptions {
		for _, streamer := range streamers.Streamers {
			if streamer.ID == sub.StreamerId {
				subedStreamers = append(subedStreamers, streamer)
				break
			}
		}
	}

	// 返回与 ListStreamers 相同的格式
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"streamers": subedStreamers,
		"total":     len(subedStreamers),
	})
}

// AddUserSubscription 通过 RPC 添加用户订阅
func AddUserSubscription(c *gin.Context) {
	// 从 cookie 获取用户信息
	userHash, err := getUserHashFromCookie(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录或登录已过期",
		})
		return
	}

	// 解析请求
	var req struct {
		StreamerID string `json:"streamer_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 移除可能存在的 @ 符号
	streamerID := strings.TrimPrefix(req.StreamerID, "@")

	// 检查是否已经订阅
	exists, err := services.CheckSubscriptionExists(userHash, streamerID)
	if err != nil {
		log.Printf("检查订阅状态失败: %v", err)
		// 继续执行，尝试创建订阅
	} else if exists {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "该主播已在订阅列表中",
		})
		return
	}

	// 调用 RPC 服务创建订阅
	resp, err := services.CreateSubscription(userHash, streamerID)
	if err != nil {
		log.Printf("创建订阅失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "订阅失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "订阅成功",
		"subscription": resp.Subscription,
	})
}

// RemoveUserSubscription 通过 RPC 删除用户订阅
func RemoveUserSubscription(c *gin.Context) {
	// 从 cookie 获取用户信息
	userHash, err := getUserHashFromCookie(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录或登录已过期",
		})
		return
	}

	// 解析请求
	var req struct {
		StreamerID string `json:"streamer_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 移除可能存在的 @ 符号
	streamerID := strings.TrimPrefix(req.StreamerID, "@")

	// 调用 RPC 服务删除订阅
	err = services.DeleteUserStreamerSubscription(userHash, streamerID)
	if err != nil {
		log.Printf("删除订阅失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "取消订阅失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "取消订阅成功",
	})
}

// GetUserSubscriptionCount 通过 RPC 获取用户的订阅数量
func GetUserSubscriptionCount(c *gin.Context) {
	// 从 cookie 获取用户信息
	userHash, err := getUserHashFromCookie(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录或登录已过期",
		})
		return
	}

	// 调用 RPC 服务获取订阅数量
	count, err := services.GetUserSubscriptionCount(userHash)
	if err != nil {
		log.Printf("获取订阅数量失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取订阅数量失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"count":   count,
	})
}
