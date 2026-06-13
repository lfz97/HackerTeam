#!/bin/bash
# Linux 构建脚本
set -e

OUTPUT_DIR="release"
LDFLAGS="-s -w"

echo -e "\033[33m构建 linux-x64...\033[0m"

mkdir -p "$OUTPUT_DIR"
go build -ldflags "$LDFLAGS" -o "$OUTPUT_DIR/HackerTeam"

echo -e "\033[32m构建完成: $OUTPUT_DIR/HackerTeam\033[0m"
echo ""
echo "构建产物:"
ls -lh "$OUTPUT_DIR/HackerTeam"
