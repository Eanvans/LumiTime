package services

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "subtuber-services/protos"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// StreamerServiceConfig 主播服务配置
type StreamerServiceConfig struct {
	RPCAddress string // RPC 服务地址，如 "localhost:50051"
	Timeout    time.Duration
}

// StreamerService 主播相关业务服务
type StreamerService struct {
	config          StreamerServiceConfig
	conn            *grpc.ClientConn
	streamerRpc     pb.StreamerRpcClient
	userRpc         pb.UserProfileRpcClient
	subscriptionRpc pb.UserStreamerSubscriptionRpcClient
}

// ChatAnalysisData 聊天分析数据（用于保存）
type ChatAnalysisData struct {
	VideoID        string            `json:"video_id"`
	StreamerName   string            `json:"streamer_name"`
	AnalysisMethod string            `json:"analysis_method"`
	HotMoments     []HotMomentData   `json:"hot_moments"`
	Stats          ChatAnalysisStats `json:"stats"`
	AnalyzedAt     time.Time         `json:"analyzed_at"`
}

// HotMomentData 热门时刻数据
type HotMomentData struct {
	TimeInterval  string  `json:"time_interval"`
	CommentsScore float64 `json:"comments_score"`
	OffsetSeconds float64 `json:"offset_seconds"`
	FormattedTime string  `json:"formatted_time"`
}

// ChatAnalysisStats 分析统计
type ChatAnalysisStats struct {
	TotalComments   int     `json:"total_comments"`
	AnalyzedCount   int     `json:"analyzed_count"`
	HotMomentsCount int     `json:"hot_moments_count"`
	MeanScore       float64 `json:"mean_score,omitempty"`
}

var (
	streamerService     *StreamerService
	streamerServiceOnce = false
)

// InitStreamerService 初始化主播服务
func InitStreamerService(config StreamerServiceConfig) (*StreamerService, error) {
	if streamerServiceOnce {
		return streamerService, nil
	}

	// 设置默认值
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	// 创建 gRPC 连接
	conn, err := grpc.NewClient(
		config.RPCAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 RPC 服务: %w", err)
	}

	service := &StreamerService{
		config:          config,
		conn:            conn,
		streamerRpc:     pb.NewStreamerRpcClient(conn),
		userRpc:         pb.NewUserProfileRpcClient(conn),
		subscriptionRpc: pb.NewUserStreamerSubscriptionRpcClient(conn),
	}

	streamerService = service
	streamerServiceOnce = true

	log.Printf("主播服务已初始化，RPC 地址: %s", config.RPCAddress)
	return service, nil
}

// GetStreamerService 获取主播服务实例
func GetStreamerService() *StreamerService {
	return streamerService
}

// Close 关闭服务
func (s *StreamerService) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// CreateStreamer 创建主播记录
func (s *StreamerService) CreateStreamer(streamerID string,
	streamTitle string, streamPlatform string, duration string, videoId string) (*pb.StreamerResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.Timeout)
	defer cancel()

	req := &pb.CreateStreamerRequest{
		Name:            streamerID,
		Title:           streamTitle,
		Platform:        streamPlatform,
		VideoId:         videoId,
		DurationSeconds: duration,
	}

	resp, err := s.streamerRpc.CreateTubeStreamer(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("创建主播记录失败: %w", err)
	}

	log.Printf("成功创建主播记录: %s", streamerID)
	return resp, nil
}

// 查询主播记录
func (s *StreamerService) ListStreamerVODs(name string) (*pb.StreamerListResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.Timeout)
	defer cancel()

	req := &pb.ListStreamerVODsRequest{
		Name:  name,
		Limit: 10,
	}

	resp, err := s.streamerRpc.ListStreamerVODs(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("查询主播列表: %w", err)
	}

	log.Printf("成功查询主播列表: %s", name)
	return resp, nil
}
