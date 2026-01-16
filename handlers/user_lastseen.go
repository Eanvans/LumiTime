package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const userLastSeenFile = "App_Data/user_lastseen.json"

// UserLastSeen 存储每个用户对每个主播已查看到的最新 VOD 的时间戳 (RFC3339 string)
type UserLastSeen struct {
    // key: userHash -> value: map[streamerID]lastSeenTimestamp
    LastSeen map[string]map[string]string `json:"last_seen"`
}

var (
    lastSeenMutex sync.Mutex
)

// loadUserLastSeen 从文件加载数据，如果文件不存在则返回空结构
func loadUserLastSeen() (*UserLastSeen, error) {
    // ensure dir
    if err := os.MkdirAll(filepath.Dir(userLastSeenFile), 0755); err != nil {
        return nil, err
    }

    data, err := os.ReadFile(userLastSeenFile)
    if err != nil {
        if os.IsNotExist(err) {
            return &UserLastSeen{LastSeen: map[string]map[string]string{}}, nil
        }
        return nil, err
    }

    var s UserLastSeen
    if err := json.Unmarshal(data, &s); err != nil {
        return nil, err
    }
    if s.LastSeen == nil {
        s.LastSeen = map[string]map[string]string{}
    }
    return &s, nil
}

// saveUserLastSeen 将数据写回文件
func saveUserLastSeen(s *UserLastSeen) error {
    // ensure dir
    if err := os.MkdirAll(filepath.Dir(userLastSeenFile), 0755); err != nil {
        return err
    }
    data, err := json.MarshalIndent(s, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(userLastSeenFile, data, 0644)
}

// UpdateUserLastSeen 设置用户对某主播的 lastSeen 时间戳（覆盖或创建）
func UpdateUserLastSeen(userHash, streamerID, lastSeen string) error {
    lastSeenMutex.Lock()
    defer lastSeenMutex.Unlock()

    s, err := loadUserLastSeen()
    if err != nil {
        return err
    }

    m, ok := s.LastSeen[userHash]
    if !ok || m == nil {
        m = map[string]string{}
        s.LastSeen[userHash] = m
    }
    m[streamerID] = lastSeen

    return saveUserLastSeen(s)
}

// GetUserLastSeen 获取用户对某主播的 lastSeen 时间戳，返回 (value, true) 如果存在
func GetUserLastSeen(userHash, streamerID string) (string, bool, error) {
    lastSeenMutex.Lock()
    defer lastSeenMutex.Unlock()

    s, err := loadUserLastSeen()
    if err != nil {
        return "", false, err
    }
    if m, ok := s.LastSeen[userHash]; ok {
        if v, ok2 := m[streamerID]; ok2 {
            return v, true, nil
        }
    }
    return "", false, nil
}
