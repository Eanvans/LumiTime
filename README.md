# LumiTime - 前后端分离项目

这是一个现代化的前后端分离项目，使用 **Vite 3 + Vue 3** 作为前端，**Go + Gin** 作为后端 API 服务。

## 🏗️ 项目架构

```
LumiTime/
├── frontend/              # 前端项目 (Vite + Vue 3)
│   ├── src/
│   │   ├── views/        # 页面组件
│   │   ├── api/          # API 封装
│   │   └── styles/       # 样式文件
│   └── package.json
├── main.go               # Go 后端主文件
├── routes.go             # API 路由定义
└── README.md
```

## 🚀 快速开始

### 前端开发

```bash
cd frontend
npm install
npm run dev
```

前端将运行在 `http://localhost:3000`

### 后端开发

```bash
go mod tidy
go run main.go routes.go
```

后端 API 服务将运行在 `http://localhost:8080`

### VS Code 调试

按 `F5` 选择 **"🚀 启动前端+后端"** 即可同时启动前后端项目。

## 📡 API 接口

- `GET /` - 健康检查
- `GET /api/time` - 获取服务器时间
- `GET /api/benchlist` - 获取主播列表
- `GET /api/names` - 获取主播详细信息
- `GET /img/proxy?url=<url>` - 图片代理（避免CORS问题）

## 💡 功能特性

### 主播订阅页面 (/)
- 🔍 搜索主播
- 📋 查看主播列表
- ⭐ 订阅功能

### 直播日程页面 (/schedule)
- 📅 查看 Lumi 的直播日程
- 🎮 支持多平台（Twitch, YouTube, Discord）
- 🌙 美观的深色主题

## 🛠️ 技术栈

**前端：**
- Vite 4.4.9
- Vue 3.3.4
- Vue Router 4.2.4
- Axios 1.5.0

**后端：**
- Go 1.x
- Gin Web Framework

## 📦 生产构建

### 构建前端

```bash
cd frontend
npm run build
```

构建产物将输出到 `frontend/dist` 目录。

### 构建后端

```bash
go build -o lumitime main.go routes.go
```

## 🎯 关于 LumiTime

A time tracking website for VTubers, welcome to join this project!    
    ....
    if you find this idea interesting, welcome to file a issue ticket and share your ideas~

* Plan & work to do
	[ ] Record the Lumi time history
	[ ] Easily transfrom raw schedule data to Lumi time JSON
	
    

