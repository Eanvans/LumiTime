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
