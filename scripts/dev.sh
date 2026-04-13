#!/bin/bash
# dev.sh — Dev Container 内での開発用起動・停止スクリプト
# 使い方:
#   bash scripts/dev.sh start   # 起動
#   bash scripts/dev.sh stop    # 停止
#   bash scripts/dev.sh restart # 再起動
#   bash scripts/dev.sh status  # 状態確認

set -e

BINARY=/tmp/print-agent-bin
CONFIG=/workspaces/print_agent/configs/config.dev.yaml
LOCAL_API=http://127.0.0.1:18181

build() {
    echo "Building..."
    go build -o "$BINARY" .
    echo "Built: $BINARY"
}

start() {
    if pgrep -f print-agent-bin > /dev/null 2>&1; then
        echo "Already running. Use 'restart' to restart."
        exit 1
    fi
    build
    mkdir -p /tmp/print-agent/logs
    PRINT_AGENT_CONFIG="$CONFIG" "$BINARY" &
    echo "Started (PID: $!)"
}

stop() {
    if pkill -f print-agent-bin 2>/dev/null; then
        echo "Stopped."
    else
        echo "Not running."
    fi
}

status() {
    if pgrep -f print-agent-bin > /dev/null 2>&1; then
        echo "Running"
        curl -s "$LOCAL_API/health"
        echo ""
        curl -s "$LOCAL_API/info"
        echo ""
    else
        echo "Not running."
    fi
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; sleep 1; start ;;
    status)  status ;;
    *)
        echo "Usage: bash scripts/dev.sh {start|stop|restart|status}"
        exit 1
        ;;
esac
