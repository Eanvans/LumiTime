package models

// TwitchStreamData Twitch直播流数据
type TwitchStreamData struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	UserLogin    string `json:"user_login"`
	UserName     string `json:"user_name"`
	GameID       string `json:"game_id"`
	GameName     string `json:"game_name"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	ViewerCount  int    `json:"viewer_count"`
	StartedAt    string `json:"started_at"`
	Language     string `json:"language"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// TwitchStreamResponse Twitch API响应
type TwitchStreamResponse struct {
	Data []TwitchStreamData `json:"data"`
}

// TwitchTokenResponse OAuth Token响应
type TwitchTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// TwitchStatusResponse 直播状态响应
type TwitchStatusResponse struct {
	IsLive       bool              `json:"is_live"`
	StreamData   *TwitchStreamData `json:"stream_data,omitempty"`
	CheckedAt    string            `json:"checked_at"`
	StreamerName string            `json:"streamer_name"`
}

// TwitchVideoData Twitch录像数据
type TwitchVideoData struct {
	ID            string `json:"id"`
	StreamID      string `json:"stream_id"`
	UserID        string `json:"user_id"`
	UserLogin     string `json:"user_login"`
	UserName      string `json:"user_name"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	CreatedAt     string `json:"created_at"`
	PublishedAt   string `json:"published_at"`
	URL           string `json:"url"`
	ThumbnailURL  string `json:"thumbnail_url"`
	Viewable      string `json:"viewable"`
	ViewCount     int    `json:"view_count"`
	Language      string `json:"language"`
	Type          string `json:"type"` // "archive", "highlight", "upload"
	Duration      string `json:"duration"`
	MutedSegments []struct {
		Duration int `json:"duration"`
		Offset   int `json:"offset"`
	} `json:"muted_segments"`
}

// TwitchVideoResponse Twitch录像API响应
type TwitchVideoResponse struct {
	Data       []TwitchVideoData `json:"data"`
	Pagination struct {
		Cursor string `json:"cursor,omitempty"`
	} `json:"pagination"`
}

// TwitchVideosListResponse 录像列表响应
type TwitchVideosListResponse struct {
	Videos       []TwitchVideoData `json:"videos"`
	TotalCount   int               `json:"total_count"`
	HasMore      bool              `json:"has_more"`
	Cursor       string            `json:"cursor,omitempty"`
	StreamerName string            `json:"streamer_name"`
}

// TwitchUserData Twitch用户数据
type TwitchUserData struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	Type            string `json:"type"`
	BroadcasterType string `json:"broadcaster_type"`
	Description     string `json:"description"`
	ProfileImageURL string `json:"profile_image_url"`
	OfflineImageURL string `json:"offline_image_url"`
	ViewCount       int    `json:"view_count"`
	CreatedAt       string `json:"created_at"`
}

// TwitchUserResponse Twitch用户API响应
type TwitchUserResponse struct {
	Data []TwitchUserData `json:"data"`
}
