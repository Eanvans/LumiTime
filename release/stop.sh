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
