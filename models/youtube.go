package models

// YouTubeStreamData YouTube直播流数据
type YouTubeStreamData struct {
	ID                   string `json:"id"`
	ChannelID            string `json:"channel_id"`
	ChannelTitle         string `json:"channel_title"`
	Title                string `json:"title"`
	Description          string `json:"description"`
	ThumbnailURL         string `json:"thumbnail_url"`
	ViewerCount          string `json:"viewer_count"`
	ScheduledStart       string `json:"scheduled_start_time"`
	ActualStart          string `json:"actual_start_time"`
	LiveStreamingDetails struct {
		ActualStartTime    string `json:"actualStartTime"`
		ScheduledStartTime string `json:"scheduledStartTime"`
		ConcurrentViewers  string `json:"concurrentViewers"`
	} `json:"liveStreamingDetails"`
}

// YouTubeSearchResponse YouTube搜索API响应
type YouTubeSearchResponse struct {
	Items []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
		Snippet struct {
			ChannelID    string `json:"channelId"`
			ChannelTitle string `json:"channelTitle"`
			Title        string `json:"title"`
			Description  string `json:"description"`
			Thumbnails   struct {
				High struct {
					URL string `json:"url"`
				} `json:"high"`
			} `json:"thumbnails"`
		} `json:"snippet"`
	} `json:"items"`
}

// YouTubeVideoResponse YouTube视频详情API响应
type YouTubeVideoResponse struct {
	Items []YouTubeVideoItem `json:"items"`
}

// YouTubeVideoItem YouTube视频条目
type YouTubeVideoItem struct {
	ID      string `json:"id"`
	Snippet struct {
		ChannelID    string `json:"channelId"`
		ChannelTitle string `json:"channelTitle"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Thumbnails   struct {
			High struct {
				URL string `json:"url"`
			} `json:"high"`
		} `json:"thumbnails"`
	} `json:"snippet"`
	LiveStreamingDetails *struct {
		ActualStartTime    string `json:"actualStartTime"`
		ScheduledStartTime string `json:"scheduledStartTime"`
		ConcurrentViewers  string `json:"concurrentViewers"`
	} `json:"liveStreamingDetails,omitempty"`
	ContentDetails *struct {
		Duration string `json:"duration"`
	} `json:"contentDetails,omitempty"`
}

// YouTubeChannelData YouTube频道数据
type YouTubeChannelData struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	URL      string `json:"url"`
}

// YouTubeStatusResponse YouTube直播状态响应
type YouTubeStatusResponse struct {
	IsLive       bool               `json:"is_live"`
	StreamData   *YouTubeStreamData `json:"stream_data,omitempty"`
	CheckedAt    string             `json:"checked_at"`
	ChannelTitle string             `json:"channel_title"`
}
