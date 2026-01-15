#!/bin/bash
# 启动脚本

# 检查config.yaml是否存在
if [ ! -f "config.yaml" ]; then
    echo "❌ 错误: config.yaml 不存在"
    echo "请先复制 config.yaml.example 为 config.yaml 并配置"
    exit 1
fi

# 检查yt-dlp是否已安装
if ! command -v yt-dlp &> /dev/null; then
    echo "❌ 错误: yt-dlp 未安装"
    echo "请先安装 yt-dlp:"
    echo "  Linux: snap install yt-dlp"
    echo "  或访问: https://github.com/yt-dlp/yt-dlp#installation"
    exit 1
fi

echo "✅ yt-dlp 已安装: $(yt-dlp --version)"
echo "🚀 启动 Subtuber Services..."
./subtuber_services
