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
