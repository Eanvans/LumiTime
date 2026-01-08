# Subtuber Services - VTuber 内容管理与分析平台

这是一个现代化的 VTuber 内容管理与分析平台，提供直播监控、VOD 下载、聊天分析、AI 摘要等功能。使用 **Go + Gin** 作为后端 API 服务，集成了 Google AI 和阿里云 AI。

## 🏗️ 项目架构

```
subtuber_services/
├── handlers/             # 业务逻辑处理器
│   ├── ai_service.go     # AI 服务接口
│   ├── aliyunai_handler.go   # 阿里云 AI 集成
│   ├── googleai_handler.go   # Google AI 集成
│   ├── auth_handler.go       # 用户认证
│   ├── twitch_handler.go     # Twitch 集成
│   ├── chat_analyze.go       # 聊天数据分析
│   ├── streamer_handler.go   # 主播管理
│   └── vod_downloader.go     # VOD 下载
├── models/               # 数据模型
│   ├── user.go
│   ├── twitch.go
│   ├── tracking.go
│   └── blockchain.go
├── protos/               # Protocol Buffers 定义
│   ├── subtube.proto     # gRPC 服务定义
│   ├── subtube.pb.go     # 生成的 protobuf 代码
│   └── subtube_grpc.pb.go # 生成的 gRPC 代码
├── analysis_results/     # AI 分析结果存储
├── App_Data/            # 用户数据存储
├── main.go              # Go 后端主文件
├── routes.go            # API 路由定义
├── config.yaml          # 配置文件
├── Makefile             # 自动化构建脚本
└── README.md
```

## 🚀 快速开始

### 环境要求

- Go 1.19+
- Node.js 16+
- Protocol Buffers Compiler (protoc)
- ffmpeg

### 配置文件

创建 `config.yaml` 文件并配置 API 密钥：

```yaml
google_api:
  api_key: "your-google-api-key"

alibaba_api:
  api_key: "your-dashscope-api-key"

twitch:
  client_id: "your-twitch-client-id"
  client_secret: "your-twitch-client-secret"
```
### 后端开发

#### 安装 Go 依赖

```bash
go mod tidy
```

#### 首次设置（安装 protobuf 工具）

```bash
make install-proto-tools
```

#### 生成 protobuf 文件

```bash
make proto
```

#### 运行后端服务

```bash
go run main.go routes.go
```

后端 API 服务将运行在 `http://localhost:8080`

### VS Code 快速启动

项目已配置 VS Code 任务，可通过以下方式快速启动：

- **启动前端开发服务器**: `Cmd+Shift+P` → `Tasks: Run Task` → `启动前端开发服务器`
- **启动 Go 后端**: `Cmd+Shift+P` → `Tasks: Run Task` → `启动 Go 后端`
- **安装依赖**: 使用 `安装前端依赖` 和 `安装 Go 依赖` 任务

## 📡 API 接口

### 基础接口
- `GET /` - 健康检查
- `GET /api/time` - 获取服务器时间

### 认证接口
- `POST /api/auth/send-code` - 发送验证码
- `POST /api/auth/verify-code` - 验证登录

### Twitch 监控接口
- `GET /api/twitch/status` - 获取 Twitch 直播状态
- `POST /api/twitch/check-now` - 立即检查直播状态
- `GET /api/twitch/videos` - 获取历史视频列表

### VOD 下载接口
- `POST /api/twitch/download-chat` - 下载 VOD 聊天记录
- `POST /api/twitch/save-chat` - 保存聊天记录到文件
- `POST /api/vod/download` - 下载 VOD 视频
- `GET /api/vod/info` - 获取 VOD 信息

### 聊天分析接口
- `GET /api/twitch/analysis/:videoID` - 获取视频分析结果
- `GET /api/twitch/analysis` - 列出所有分析结果
- `GET /api/twitch/analysis-summary?video_id={id}&offset_seconds={seconds}` - 获取特定时间点的 AI 摘要

### 主播管理接口
- `GET /api/streamers` - 获取主播列表
- `GET /api/streamers/:id` - 获取主播详细信息

## 💡 功能特性

### 🎥 Twitch 直播监控
- 实时监控主播直播状态
- 自动记录直播时长和观看人数
- 历史视频查询和管理

### 💬 聊天数据分析
- 下载和解析 Twitch VOD 聊天记录
- 智能识别聊天高潮时刻（热点时刻）
- 时间序列数据可视化支持
- 基于统计学的峰值检测算法

### 🤖 AI 内容摘要
- 集成 Google Gemini AI 和阿里云通义千问
- 自动生成视频内容摘要
- SRT 字幕文件解析和分段摘要
- 支持自定义 AI 模型选择
- 统一的 AI 服务接口，方便切换不同 AI 提供商

### 📥 VOD 下载管理
- 支持多平台 VOD 下载（Twitch、YouTube 等）
- 视频信息获取和元数据管理
- 批量下载支持

### 👤 用户系统
- 邮箱验证码登录
- 用户数据持久化存储
- JWT 会话管理

### 🎯 主播追踪
- 多主播管理
- 主播信息查询
- 直播历史记录

## 🛠️ 技术栈

**后端：**
- Go 1.19+
- Gin Web Framework
- gRPC & Protocol Buffers
- Google Generative AI SDK
- Alibaba Cloud DashScope API (通义千问)

**数据存储：**
- 本地文件存储 (JSON)
- 分析结果持久化

**AI 集成：**
- Google Gemini 2.5 Flash Lite
- 阿里云通义千问 (Qwen-Plus/Turbo/Max)
- 统一 AI 服务接口

## 🔧 开发工具

### Makefile 命令

- `make proto` - 生成 protobuf Go 文件
- `make install-proto-tools` - 安装 protobuf 编译工具（protoc-gen-go, protoc-gen-go-grpc）
- `make clean` - 清理生成的 protobuf 文件
- `make help` - 显示所有可用命令

### VS Code 任务

项目已配置以下 VS Code 任务（通过 `Cmd+Shift+P` → `Tasks: Run Task` 调用）：

- **安装前端依赖** - 在 frontend 目录安装 npm 包
- **构建前端** - 构建前端生产版本
- **启动前端开发服务器** - 启动 Vite 开发服务器
- **启动 Go 后端** - 启动 Go 后端服务
- **安装 Go 依赖** - 运行 `go mod tidy`

## 📦 生产构建

### 构建前端

```bash
cd frontend
npm run build
```

构建产物将输出到 `frontend/dist` 目录。

### 构建后端

#### Linux AMD64 (服务器部署)

```bash
./scripts/build_linux_amd64.sh
```

#### 本地构建

```bash
go build -o subtuber-services main.go routes.go
```

### 运行生产版本

```bash
./subtuber-services
```

## 🔐 配置说明

### config.yaml 配置示例

```yaml
# Google AI 配置
google_api:
  api_key: "your-google-gemini-api-key"
  model: "gemini-2.5-flash-lite"

# 阿里云 AI 配置
alibaba_api:
  api_key: "your-dashscope-api-key"
  model: "qwen-plus"  # 可选: qwen-plus, qwen-turbo, qwen-max

# Twitch API 配置
twitch:
  client_id: "your-twitch-client-id"
  client_secret: "your-twitch-client-secret"
  streamer_username: "target-streamer-username"

# 服务器配置
server:
  port: 8080
  mode: "release"  # debug, release, test
```

### 环境变量（可选）

可以通过环境变量覆盖配置文件：

```bash
export DASHSCOPE_API_KEY="your-api-key"
export GOOGLE_API_KEY="your-api-key"
export TWITCH_CLIENT_ID="your-client-id"
export TWITCH_CLIENT_SECRET="your-client-secret"
```

## 📊 AI 服务使用示例

### 使用统一接口

```go
import "subtuber-services/handlers"

// 使用 Google AI
aiService := handlers.NewAIService("google", "")
summary, chunks, err := aiService.SummarizeSRT(ctx, srtContent, 10000)

// 切换到阿里云 AI
aiService = handlers.NewAIService("aliyun", "")
summary, chunks, err = aiService.SummarizeSRT(ctx, srtContent, 10000)
```

### 直接使用特定服务

```go
// Google AI
googleAI := handlers.NewGoogleAIService("")
text, err := googleAI.GenerateContent(ctx, prompt, 600)

// 阿里云 AI
aliyunAI := handlers.NewAliyunAIService("")
text, err := aliyunAI.GenerateContent(ctx, prompt, 600)
```

## 🎯 关于 Subtuber Services

一个用于 VTuber 内容管理和分析的综合平台，旨在帮助粉丝和内容创作者更好地追踪、分析和管理直播内容。

### 主要目标

- ✅ 实时监控 Twitch 直播状态
- ✅ 下载和分析 VOD 聊天数据
- ✅ AI 驱动的内容摘要和高光时刻检测
- ✅ 多平台支持（Twitch, YouTube 等）
- 🚧 区块链集成用于数据存证
- 🚧 用户订阅和推送通知
- 🚧 更多 AI 分析功能

### 贡献

欢迎提交 Issue 和 Pull Request！如果你有任何想法或建议，欢迎在 Issues 中讨论。

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 🔗 相关链接

- [Gin Web Framework](https://gin-gonic.com/)
- [Google Generative AI](https://ai.google.dev/)
- [阿里云通义千问](https://dashscope.aliyun.com/)
- [Twitch API](https://dev.twitch.tv/)
- [Protocol Buffers](https://protobuf.dev/)