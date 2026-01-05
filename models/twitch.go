package models

import "time"

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

// TwitchChatComment VOD聊天评论
type TwitchChatComment struct {
	ID                   string              `json:"_id"`
	CreatedAt            string              `json:"created_at"`
	UpdatedAt            string              `json:"updated_at"`
	ChannelID            string              `json:"channel_id"`
	ContentType          string              `json:"content_type"`
	ContentID            string              `json:"content_id"`
	ContentOffsetSeconds float64             `json:"content_offset_seconds"`
	Commenter            TwitchChatCommenter `json:"commenter"`
	Source               string              `json:"source"`
	State                string              `json:"state"`
	Message              TwitchChatMessage   `json:"message"`
	MoreReplies          bool                `json:"more_replies"`
}

// TwitchChatCommenter 评论者信息
type TwitchChatCommenter struct {
	ID          string `json:"_id"`
	Bio         string `json:"bio,omitempty"`
	CreatedAt   string `json:"created_at"`
	DisplayName string `json:"display_name"`
	Logo        string `json:"logo,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	UpdatedAt   string `json:"updated_at"`
}

// TwitchChatMessage 聊天消息
type TwitchChatMessage struct {
	Body             string                      `json:"body"`
	BitsSpent        int                         `json:"bits_spent,omitempty"`
	Fragments        []TwitchChatMessageFragment `json:"fragments"`
	IsAction         bool                        `json:"is_action"`
	UserBadges       []TwitchChatBadge           `json:"user_badges,omitempty"`
	UserColor        string                      `json:"user_color,omitempty"`
	UserNoticeParams map[string]string           `json:"user_notice_params,omitempty"`
	Emoticons        []TwitchChatEmoticon        `json:"emoticons,omitempty"`
}

// TwitchChatMessageFragment 消息片段
type TwitchChatMessageFragment struct {
	Text     string              `json:"text"`
	Emoticon *TwitchChatEmoticon `json:"emoticon,omitempty"`
}

// TwitchChatEmoticon 表情信息
type TwitchChatEmoticon struct {
	ID            string `json:"_id"`
	Begin         int    `json:"begin"`
	End           int    `json:"end"`
	EmoticonID    string `json:"emoticon_id,omitempty"`
	EmoticonSetID string `json:"emoticon_set_id,omitempty"`
}

// TwitchChatBadge 徽章信息
type TwitchChatBadge struct {
	ID      string `json:"_id"`
	Version string `json:"version"`
}

// TwitchChatDownloadRequest 下载聊天记录请求
type TwitchChatDownloadRequest struct {
	VideoID   string   `json:"video_id" binding:"required"`
	StartTime *float64 `json:"start_time,omitempty"` // 可选：开始时间（秒）
	EndTime   *float64 `json:"end_time,omitempty"`   // 可选：结束时间（秒）
}

// TwitchChatDownloadResponse 下载聊天记录响应
type TwitchChatDownloadResponse struct {
	VideoID       string              `json:"video_id"`
	TotalComments int                 `json:"total_comments"`
	Comments      []TwitchChatComment `json:"comments"`
	VideoInfo     *TwitchVideoData    `json:"video_info,omitempty"`
	DownloadedAt  string              `json:"downloaded_at"`
}

// TwitchGQLCommentResponse GraphQL评论响应
type TwitchGQLCommentResponse struct {
	Data struct {
		Video struct {
			ID       string `json:"id"`
			Comments struct {
				Edges []struct {
					Node struct {
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
					} `json:"node"`
					Cursor string `json:"cursor"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage     bool `json:"hasNextPage"`
					HasPreviousPage bool `json:"hasPreviousPage"`
				} `json:"pageInfo"`
			} `json:"comments"`
		} `json:"video"`
	} `json:"data"`
}

// TwitchGQLRequest GraphQL请求
type TwitchGQLRequest struct {
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	Extensions    map[string]interface{} `json:"extensions"`
}

// ChatAnalyzeRequest 聊天分析请求
type ChatAnalyzeRequest struct {
	VideoID         string `json:"video_id" binding:"required"`
	Method          string `json:"method"`           // "iqr" 或 "sliding", 默认 "sliding"
	IntervalMinutes int    `json:"interval_minutes"` // IQR方法的时间间隔（分钟），默认5
	IntervalSeconds int    `json:"interval_seconds"` // 滑动滤波方法的时间间隔（秒），默认5
}

// ChatAnalyzeResponse 聊天分析响应
type ChatAnalyzeResponse struct {
	VideoID    string                 `json:"video_id"`
	Method     string                 `json:"method"`
	HotMoments []ChatAnalyzeHotMoment `json:"hot_moments"`
	Stats      ChatAnalyzeStats       `json:"stats"`
	VideoInfo  *TwitchVideoData       `json:"video_info,omitempty"`
}

// ChatAnalyzeHotMoment 热门时刻
type ChatAnalyzeHotMoment struct {
	TimeInterval  string  `json:"time_interval"`
	CommentsScore float64 `json:"comments_score"`
	OffsetSeconds float64 `json:"offset_seconds"`
	FormattedTime string  `json:"formatted_time"`
}

// ChatAnalyzeStats 分析统计信息
type ChatAnalyzeStats struct {
	TotalComments   int     `json:"total_comments"`
	AnalyzedCount   int     `json:"analyzed_count"`
	HotMomentsCount int     `json:"hot_moments_count"`
	MeanScore       float64 `json:"mean_score,omitempty"`
}
