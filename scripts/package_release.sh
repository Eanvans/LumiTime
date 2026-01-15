#!/usr/bin/env bash
set -euo pipefail

# 打包脚本 - 用于Ubuntu服务器部署
# 运行: bash scripts/package_release.sh

RELEASE_DIR="release"
BINARY_NAME="subtuber_services"
VERSION=$(date +"%Y%m%d_%H%M%S")

echo "🚀 开始打包 Subtuber Services for Ubuntu..."
echo "📦 版本: $VERSION"

# 清理并创建release目录
echo "📁 准备release目录..."
rm -rf $RELEASE_DIR
mkdir -p $RELEASE_DIR

# 1. 安装Go依赖
echo "📥 安装Go依赖..."
go mod tidy

# 2. 构建Linux二进制文件
echo "🔨 构建Linux amd64二进制文件..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -buildvcs=false -ldflags='-s -w' -o $RELEASE_DIR/$BINARY_NAME .

# 使其可执行
chmod +x $RELEASE_DIR/$BINARY_NAME

# 3. 复制配置文件
echo "📋 复制配置文件..."
if [ -f "config.yaml" ]; then
    cp config.yaml $RELEASE_DIR/config.yaml.example
    echo "  ✓ config.yaml -> config.yaml.example"
fi

# 4. 复制必要的数据文件
if [ -f "benchlist.json" ]; then
    cp benchlist.json $RELEASE_DIR/
    echo "  ✓ benchlist.json"
fi

if [ -f "data.json" ]; then
    cp data.json $RELEASE_DIR/
    echo "  ✓ data.json"
fi

# 5. 复制proto文件（如果需要）
if [ -d "protos" ]; then
    mkdir -p $RELEASE_DIR/protos
    cp protos/*.proto $RELEASE_DIR/protos/ 2>/dev/null || true
    echo "  ✓ protos/"
fi

# 6. 创建必要的目录结构
echo "📁 创建运行时目录..."
mkdir -p $RELEASE_DIR/{App_Data,downloads,analysis_results,chat_logs}
echo "  ✓ App_Data/"
echo "  ✓ downloads/"
echo "  ✓ analysis_results/"
echo "  ✓ chat_logs/"

# 7. 创建README
cat > $RELEASE_DIR/README.md << 'EOF'
# Subtuber Services 部署包

## 部署步骤

### 1. 上传文件到Ubuntu服务器
```bash
# 在本地执行
scp -r release/* user@your-ubuntu-server:/path/to/deployment/
```

### 2. 在Ubuntu服务器上设置
```bash
# 进入部署目录
cd /path/to/deployment/

# 复制并编辑配置文件
cp config.yaml.example config.yaml
nano config.yaml  # 编辑配置信息

# 确保二进制文件可执行
chmod +x subtuber_services
```

### 3. 运行服务

#### 方式1: 直接运行
```bash
./subtuber_services
```

#### 方式2: 使用nohup后台运行
```bash
nohup ./subtuber_services > app.log 2>&1 &
```

#### 方式3: 使用systemd服务（推荐）
创建服务文件 `/etc/systemd/system/subtuber.service`:
```ini
[Unit]
Description=Subtuber Services
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/deployment
ExecStart=/path/to/deployment/subtuber_services
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

然后启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable subtuber
sudo systemctl start subtuber
sudo systemctl status subtuber
```

### 4. 查看日志
```bash
# 如果使用nohup
tail -f app.log

# 如果使用systemd
sudo journalctl -u subtuber -f
```

### 5. 防火墙设置（如果需要）
```bash
# 开放8080端口（根据实际端口修改）
sudo ufw allow 8080
```

## 配置说明

请在 `config.yaml` 中配置以下信息：
- SMTP邮件服务器设置
- Twitch API凭证
- Google/Alibaba AI API密钥
- RPC服务地址

## 目录结构
- `App_Data/` - 应用数据
- `downloads/` - 下载文件
- `analysis_results/` - 分析结果
- `chat_logs/` - 聊天日志

## 维护

### 停止服务
```bash
# 如果使用systemd
sudo systemctl stop subtuber

# 如果使用nohup，找到进程并kill
ps aux | grep subtuber_services
kill <PID>
```

### 更新服务
1. 停止服务
2. 备份当前数据
3. 上传新的二进制文件
4. 重启服务
EOF

# 8. 创建systemd服务模板
cat > $RELEASE_DIR/subtuber.service << 'EOF'
[Unit]
Description=Subtuber Services
After=network.target

[Service]
Type=simple
User=YOUR_USER
WorkingDirectory=/path/to/deployment
ExecStart=/path/to/deployment/subtuber_services
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# 9. 创建启动脚本
cat > $RELEASE_DIR/start.sh << 'EOF'
#!/bin/bash
# 启动脚本

# 检查config.yaml是否存在
if [ ! -f "config.yaml" ]; then
    echo "❌ 错误: config.yaml 不存在"
    echo "请先复制 config.yaml.example 为 config.yaml 并配置"
    exit 1
fi

echo "🚀 启动 Subtuber Services..."
./subtuber_services
EOF
chmod +x $RELEASE_DIR/start.sh

# 10. 创建停止脚本
cat > $RELEASE_DIR/stop.sh << 'EOF'
#!/bin/bash
# 停止脚本

PID=$(pgrep -f subtuber_services)
if [ -z "$PID" ]; then
    echo "⚠️  Subtuber Services 未在运行"
else
    echo "🛑 停止 Subtuber Services (PID: $PID)..."
    kill $PID
    echo "✅ 已停止"
fi
EOF
chmod +x $RELEASE_DIR/stop.sh

# 11. 打包成tar.gz
echo "📦 创建压缩包..."
ARCHIVE_NAME="subtuber_services_${VERSION}_linux_amd64.tar.gz"
tar -czf $ARCHIVE_NAME -C $RELEASE_DIR .

echo ""
echo "✅ 打包完成！"
echo ""
echo "📦 发布文件:"
echo "  - 目录: ./$RELEASE_DIR/"
echo "  - 压缩包: ./$ARCHIVE_NAME"
echo ""
echo "📤 部署到Ubuntu服务器:"
echo "  1. 上传压缩包:"
echo "     scp $ARCHIVE_NAME user@server:/path/"
echo ""
echo "  2. 在服务器上解压:"
echo "     tar -xzf $ARCHIVE_NAME"
echo "     cp config.yaml.example config.yaml"
echo "     nano config.yaml  # 编辑配置"
echo "     ./start.sh"
echo ""
echo "🎉 部署准备就绪！"
