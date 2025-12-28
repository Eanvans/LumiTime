# LumiTime - 前后端分离项目

这是一个使用 Vite 3 + Vue 3 的前后端分离项目，后端使用 Go + Gin。

## 项目结构

```
LumiTime/
├── frontend/                 # 前端项目 (Vite + Vue 3)
│   ├── src/
│   │   ├── views/           # 页面组件
│   │   │   ├── Home.vue     # 主播订阅页面
│   │   │   └── Schedule.vue # 直播日程页面
│   │   ├── styles/          # 全局样式
│   │   ├── App.vue          # 根组件
│   │   └── main.js          # 入口文件
│   ├── index.html
│   ├── vite.config.js       # Vite 配置
│   └── package.json
├── main.go                   # Go 后端主文件
├── routes.go                 # API 路由
└── README.md
```

## 开发指南

### 前端开发

1. 安装依赖：
```bash
cd frontend
npm install
```

2. 启动开发服务器：
```bash
npm run dev
```

前端将运行在 `http://localhost:3000`

### 后端开发

1. 确保已安装 Go 1.16+

2. 安装依赖：
```bash
go mod tidy
```

3. 启动后端服务器：
```bash
go run main.go routes.go
```

后端将运行在 `http://localhost:8080`

## API 接口

- `GET /api/time` - 获取服务器时间
- `GET /api/benchlist` - 获取主播列表
- `GET /api/names` - 获取主播详细信息
- `GET /img/proxy?url=<url>` - 图片代理（避免CORS问题）

## 功能特性

### 主播订阅页面 (/)
- 搜索主播
- 查看主播列表
- 订阅功能（演示）

### 直播日程页面 (/schedule)
- 查看 Lumi 的直播日程
- 支持多平台（Twitch, YouTube, Discord）
- 美观的深色主题

## 技术栈

**前端：**
- Vite 4.4.9
- Vue 3.3.4
- Vue Router 4.2.4
- Axios 1.5.0

**后端：**
- Go 1.x
- Gin Web Framework

## 生产构建

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

## 部署

1. 构建前端资源
2. 将前端构建产物部署到 CDN 或静态服务器
3. 部署 Go 后端服务
4. 配置 Nginx 反向代理（可选）

## 开发注意事项

- 前端开发时使用 Vite 的代理功能访问后端 API
- 生产环境需配置实际的 API 地址
- 后端已配置 CORS，允许前端跨域访问

## License

MIT
