package models

// TrackItem 追踪项目信息
type TrackItem struct {
	Code      string `json:"code"`
	Timestamp string `json:"timestamp"`
	Found     bool   `json:"found"`
	ResultURL string `json:"result_url,omitempty"`
}

// AddTrackRequest 添加追踪请求
type AddTrackRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Code   string `json:"code" binding:"required"`
}

// GetTracksRequest 获取追踪列表请求
type GetTracksRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Limit  int    `json:"limit"`
}

// DeleteTrackRequest 删除追踪请求
type DeleteTrackRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Code   string `json:"code" binding:"required"`
}

// TrackResponse 追踪响应
type TrackResponse struct {
	Success bool      `json:"success"`
	Message string    `json:"message"`
	Item    *TrackItem `json:"item,omitempty"`
}

// TrackListResponse 追踪列表响应
type TrackListResponse struct {
	Success bool        `json:"success"`
	Items   []TrackItem `json:"items"`
}
