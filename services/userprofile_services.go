package services

import (
	"context"
	"fmt"
	"log"
	subtube "subtuber-services/protos"
)

// GetUserByHashFromRPC 通过 RPC 获取用户信息（使用共享连接）
func GetUserByHashFromRPC(userHash string) (*subtube.UserProfile, error) {
	service := GetStreamerService()
	if service == nil {
		return nil, fmt.Errorf("服务未初始化，请先调用 InitStreamerService")
	}

	ctx, cancel := context.WithTimeout(context.Background(), service.config.Timeout)
	defer cancel()

	resp, err := service.userRpc.GetUserByHash(ctx, &subtube.GetUserByHashRequest{
		UserHash: userHash,
	})
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %v", err)
	}

	if !resp.Success || resp.User == nil {
		return nil, fmt.Errorf("用户不存在")
	}

	return resp.User, nil
}

// UpdateUserMaxTrackingLimitRPC 更新用户的 MaxTrackingLimit（使用共享连接）
func UpdateUserMaxTrackingLimitRPC(userID int, userHash, email string, newLimit int32) error {
	service := GetStreamerService()
	if service == nil {
		return fmt.Errorf("服务未初始化，请先调用 InitStreamerService")
	}

	ctx, cancel := context.WithTimeout(context.Background(), service.config.Timeout)
	defer cancel()

	resp, err := service.userRpc.UpdateUser(ctx, &subtube.UpdateUserRequest{
		Id:               int32(userID),
		UserHash:         userHash,
		Email:            email,
		MaxTrackingLimit: newLimit,
	})
	if err != nil {
		return fmt.Errorf("更新用户信息失败: %v", err)
	}

	if !resp.Success {
		return fmt.Errorf("更新用户信息失败: %s", resp.Message)
	}

	log.Printf("成功更新用户 %s 的订阅额度为 %d", userHash, newLimit)
	return nil
}

// ========== 订阅相关服务 ==========

// GetUserSubscriptions 获取用户订阅的所有主播
func GetUserSubscriptions(userHash string) (*subtube.SubscriptionListResponse, error) {
	service := GetStreamerService()
	if service == nil {
		return nil, fmt.Errorf("服务未初始化，请先调用 InitStreamerService")
	}

	ctx, cancel := context.WithTimeout(context.Background(), service.config.Timeout)
	defer cancel()

	resp, err := service.subscriptionRpc.GetUserSubscriptions(ctx, &subtube.GetUserSubscriptionsRequest{
		UserHash: userHash,
	})
	if err != nil {
		return nil, fmt.Errorf("获取用户订阅列表失败: %v", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("获取用户订阅列表失败")
	}

	log.Printf("成功获取用户 %s 的订阅列表，共 %d 个主播", userHash, len(resp.Subscriptions))
	return resp, nil
}

// CreateSubscription 创建用户与主播的订阅关联
func CreateSubscription(userHash, streamerID string) (*subtube.SubscriptionResponse, error) {
	service := GetStreamerService()
	if service == nil {
		return nil, fmt.Errorf("服务未初始化，请先调用 InitStreamerService")
	}

	ctx, cancel := context.WithTimeout(context.Background(), service.config.Timeout)
	defer cancel()

	resp, err := service.subscriptionRpc.CreateSubscription(ctx, &subtube.CreateSubscriptionRequest{
		UserHash:   userHash,
		StreamerId: streamerID,
	})
	if err != nil {
		return nil, fmt.Errorf("创建订阅失败: %v", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("创建订阅失败: %s", resp.Message)
	}

	log.Printf("用户 %s 成功订阅主播 %s", userHash, streamerID)
	return resp, nil
}

// DeleteUserStreamerSubscription 删除用户与主播的订阅关联
func DeleteUserStreamerSubscription(userHash, streamerID string) error {
	service := GetStreamerService()
	if service == nil {
		return fmt.Errorf("服务未初始化，请先调用 InitStreamerService")
	}

	ctx, cancel := context.WithTimeout(context.Background(), service.config.Timeout)
	defer cancel()

	resp, err := service.subscriptionRpc.DeleteUserStreamerSubscription(ctx, &subtube.DeleteUserStreamerSubscriptionRequest{
		UserHash:   userHash,
		StreamerId: streamerID,
	})
	if err != nil {
		return fmt.Errorf("删除订阅失败: %v", err)
	}

	if !resp.Success {
		return fmt.Errorf("删除订阅失败: %s", resp.Message)
	}

	log.Printf("用户 %s 成功取消订阅主播 %s", userHash, streamerID)
	return nil
}

// CheckSubscriptionExists 检查用户是否订阅了某主播
func CheckSubscriptionExists(userHash, streamerID string) (bool, error) {
	service := GetStreamerService()
	if service == nil {
		return false, fmt.Errorf("服务未初始化，请先调用 InitStreamerService")
	}

	ctx, cancel := context.WithTimeout(context.Background(), service.config.Timeout)
	defer cancel()

	resp, err := service.subscriptionRpc.CheckSubscriptionExists(ctx, &subtube.CheckSubscriptionExistsRequest{
		UserHash:   userHash,
		StreamerId: streamerID,
	})
	if err != nil {
		return false, fmt.Errorf("检查订阅状态失败: %v", err)
	}

	return resp.Exists, nil
}

// GetUserSubscriptionCount 获取用户的订阅数量
func GetUserSubscriptionCount(userHash string) (int32, error) {
	service := GetStreamerService()
	if service == nil {
		return 0, fmt.Errorf("服务未初始化，请先调用 InitStreamerService")
	}

	ctx, cancel := context.WithTimeout(context.Background(), service.config.Timeout)
	defer cancel()

	resp, err := service.subscriptionRpc.GetUserSubscriptionCount(ctx, &subtube.GetUserSubscriptionsRequest{
		UserHash: userHash,
	})
	if err != nil {
		return 0, fmt.Errorf("获取订阅数量失败: %v", err)
	}

	return resp.Count, nil
}

// GetStreamerSubscribers 获取某个主播的所有订阅者
func GetStreamerSubscribers(streamerID string) (*subtube.SubscriptionListResponse, error) {
	service := GetStreamerService()
	if service == nil {
		return nil, fmt.Errorf("服务未初始化，请先调用 InitStreamerService")
	}

	ctx, cancel := context.WithTimeout(context.Background(), service.config.Timeout)
	defer cancel()

	resp, err := service.subscriptionRpc.GetStreamerSubscribers(ctx, &subtube.GetStreamerSubscribersRequest{
		StreamerId: streamerID,
	})
	if err != nil {
		return nil, fmt.Errorf("获取主播订阅者列表失败: %v", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("获取主播订阅者列表失败")
	}

	log.Printf("成功获取主播 %s 的订阅者列表，共 %d 个用户", streamerID, len(resp.Subscriptions))
	return resp, nil
}

// GetStreamerSubscriberCount 获取某个主播的订阅者数量
func GetStreamerSubscriberCount(streamerID string) (int, error) {
	resp, err := GetStreamerSubscribers(streamerID)
	if err != nil {
		return 0, err
	}

	return len(resp.Subscriptions), nil
}
