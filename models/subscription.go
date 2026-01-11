package models

// SubscriptionRequest 订阅主播请求
type SubscriptionRequest struct {
	Streamer_Id string `json:"streamer_id" binding:"required"`
	Platform    string `json:"platform" binding:"required"`
}

// Subscription 订阅信息
type Subscription struct {
	ID           int    `json:"id"`
	UserHash     string `json:"user_hash"`
	StreamerID   int    `json:"streamer_id"`
	StreamerName string `json:"streamer_name"`
	Platform     string `json:"platform"`
	SubscribedAt string `json:"subscribed_at"`
}

// SubscriptionResponse 订阅响应
type SubscriptionResponse struct {
	Success      bool          `json:"success"`
	Message      string        `json:"message"`
	Subscription *Subscription `json:"subscription,omitempty"`
}

// SubscriptionListResponse 订阅列表响应
type SubscriptionListResponse struct {
	Success       bool           `json:"success"`
	Subscriptions []Subscription `json:"subscriptions"`
	Total         int            `json:"total"`
}

// UnsubscribeRequest 取消订阅请求
type UnsubscribeRequest struct {
	UserHash   string `json:"user_hash" binding:"required"`
	StreamerID int    `json:"streamer_id" binding:"required"`
}
