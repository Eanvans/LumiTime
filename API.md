# LumiTime API 文档

## 基础信息

- **Base URL**: `http://localhost:8080/api`
- **Content-Type**: `application/json`

## 用户管理 API

### 1. 创建用户（注册）

**POST** `/api/users`

创建一个新用户账户。

**请求体：**
```json
{
  "user_hash": "unique_user_hash",
  "email": "user@example.com",
  "max_tracking_limit": 10
}
```

**响应：**
```json
{
  "success": true,
  "message": "用户创建成功",
  "user": {
    "id": 1,
    "user_hash": "unique_user_hash",
    "email": "user@example.com",
    "max_tracking_limit": 10
  }
}
```

### 2. 获取所有用户

**GET** `/api/users`

获取系统中的所有用户列表。

**响应：**
```json
{
  "success": true,
  "users": [
    {
      "id": 1,
      "user_hash": "hash1",
      "email": "user1@example.com",
      "max_tracking_limit": 10
    }
  ]
}
```

### 3. 根据ID获取用户

**GET** `/api/users/:id`

通过用户ID获取用户详细信息。

**路径参数：**
- `id` (int): 用户ID

**响应：**
```json
{
  "success": true,
  "message": "获取用户成功",
  "user": {
    "id": 1,
    "user_hash": "sample_hash",
    "email": "user@example.com",
    "max_tracking_limit": 10
  }
}
```

### 4. 根据Hash获取用户

**GET** `/api/users/hash/:hash`

通过用户Hash获取用户详细信息。

**路径参数：**
- `hash` (string): 用户Hash值

**响应：** 同上

### 5. 更新用户信息

**PUT** `/api/users`

更新指定用户的信息。

**请求体：**
```json
{
  "id": 1,
  "user_hash": "updated_hash",
  "email": "newemail@example.com",
  "max_tracking_limit": 20
}
```

**响应：**
```json
{
  "success": true,
  "message": "用户更新成功",
  "user": {
    "id": 1,
    "user_hash": "updated_hash",
    "email": "newemail@example.com",
    "max_tracking_limit": 20
  }
}
```

### 6. 删除用户

**DELETE** `/api/users/:id`

删除指定ID的用户。

**路径参数：**
- `id` (int): 用户ID

**响应：**
```json
{
  "success": true,
  "message": "用户删除成功"
}
```

---

## 追踪管理 API

### 1. 添加追踪项目

**POST** `/api/tracks`

为用户添加一个新的追踪项目。

**请求体：**
```json
{
  "user_id": "user123",
  "code": "TRACK001"
}
```

**响应：**
```json
{
  "success": true,
  "message": "追踪添加成功",
  "item": {
    "code": "TRACK001",
    "timestamp": "2025-12-29T10:00:00Z",
    "found": false
  }
}
```

### 2. 获取追踪列表

**GET** `/api/tracks?user_id=user123&limit=10`

获取指定用户的所有追踪项目。

**查询参数：**
- `user_id` (string, 必填): 用户ID
- `limit` (int, 可选): 限制返回数量

**响应：**
```json
{
  "success": true,
  "items": [
    {
      "code": "TRACK001",
      "timestamp": "2025-12-29T10:00:00Z",
      "found": true,
      "result_url": "https://example.com/result"
    }
  ]
}
```

### 3. 删除追踪项目

**DELETE** `/api/tracks`

删除用户的指定追踪项目。

**请求体：**
```json
{
  "user_id": "user123",
  "code": "TRACK001"
}
```

**响应：**
```json
{
  "success": true,
  "message": "追踪删除成功"
}
```

### 4. 检查用户是否存在

**GET** `/api/tracks/check?nickname=username`

根据昵称检查用户是否存在。

**查询参数：**
- `nickname` (string, 必填): 用户昵称

**响应：**
```json
{
  "exists": true
}
```

---

## 区块链管理 API

### 1. 添加区块

**POST** `/api/blockchain/blocks`

向用户的区块链添加新区块。

**请求体：**
```json
{
  "user_id": "user123",
  "block_type": "transaction",
  "data": "block data content"
}
```

**响应：**
```json
{
  "success": true,
  "message": "区块添加成功",
  "blockchain": {
    "blocks": [
      {
        "index": 0,
        "timestamp": "2025-12-29T10:00:00Z",
        "data": "Genesis Block",
        "previous_hash": "",
        "hash": "genesis_hash"
      },
      {
        "index": 1,
        "timestamp": "2025-12-29T10:01:00Z",
        "data": "block data content",
        "previous_hash": "genesis_hash",
        "hash": "block1_hash"
      }
    ],
    "root_hash": "sample_root_hash"
  }
}
```

### 2. 加载区块链

**GET** `/api/blockchain?user_id=user123`

加载指定用户的区块链数据。

**查询参数：**
- `user_id` (string, 必填): 用户ID

**响应：**
```json
{
  "success": true,
  "message": "区块链加载成功",
  "blockchain": {
    "blocks": [...],
    "root_hash": "root_hash_123"
  }
}
```

### 3. 验证区块链

**POST** `/api/blockchain/validate`

验证区块链数据的完整性和有效性。

**请求体：**
```json
{
  "blocks": [
    {
      "index": 0,
      "timestamp": "2025-12-29T10:00:00Z",
      "data": "Genesis Block",
      "previous_hash": "",
      "hash": "genesis_hash"
    }
  ],
  "root_hash": "root_hash_123"
}
```

**响应：**
```json
{
  "is_valid": true,
  "error_message": ""
}
```

### 4. 与服务器验证

**GET** `/api/blockchain/verify?user_id=user123&root_hash=abc123`

向服务器验证用户区块链的根哈希。

**查询参数：**
- `user_id` (string, 必填): 用户ID
- `root_hash` (string, 必填): 根哈希值

**响应：**
```json
{
  "is_valid": true,
  "message": "验证成功"
}
```

---

## 错误响应

所有错误响应遵循以下格式：

```json
{
  "success": false,
  "message": "错误描述信息"
}
```

**HTTP 状态码：**
- `200 OK`: 请求成功
- `400 Bad Request`: 请求参数错误
- `404 Not Found`: 资源不存在
- `500 Internal Server Error`: 服务器内部错误

---

## 注意事项

1. 所有时间戳使用 RFC3339 格式（ISO 8601）
2. 当前所有接口返回模拟数据，需要实现 gRPC 客户端来连接实际的数据服务
3. 建议为 API 添加认证中间件（JWT 等）
4. 建议添加请求速率限制
5. 生产环境需要完善错误处理和日志记录
