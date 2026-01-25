package models

// UserProfile 表示用户资料信息
type UserProfile struct {
	ID                int    `json:"id"`
	UserHash          string `json:"user_hash"`
	Email             string `json:"email"`
	MaxTrackingLimit  int    `json:"max_tracking_limit"`
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	UserHash         string `json:"user_hash" binding:"required"`
	Email            string `json:"email" binding:"required,email"`
	MaxTrackingLimit int    `json:"max_tracking_limit"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	ID               int    `json:"id" binding:"required"`
	UserHash         string `json:"user_hash"`
	Email            string `json:"email" binding:"email"`
	MaxTrackingLimit int    `json:"max_tracking_limit"`
}

// UserResponse 用户响应
type UserResponse struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	User    *UserProfile `json:"user,omitempty"`
}

// UserListResponse 用户列表响应
type UserListResponse struct {
	Success bool          `json:"success"`
	Users   []UserProfile `json:"users"`
}
