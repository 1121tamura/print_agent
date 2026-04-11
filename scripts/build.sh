#!/bin/bash
# build.sh — Linux/Mac 環境から Windows 向けにクロスコンパイルするスクリプト
# 実行: bash scripts/build.sh

set -e

OUTPUT_DIR="dist"
BINARY_NAME="print-agent.exe"

# バージョン（git tag があれば使用）
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")

echo "Building print-agent $VERSION ..."

mkdir -p "$OUTPUT_DIR"

GOOS=windows GOARCH=amd64 go build \
  -ldflags "-X main.version=$VERSION" \
  -o "$OUTPUT_DIR/$BINARY_NAME" \
  .

echo "Built: $OUTPUT_DIR/$BINARY_NAME"
