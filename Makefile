# 最小化交叉编译 Makefile
# 支持: linux-arm64, linux-x64, macos-arm64, macos-x64, windows-x64

# 定义目标平台和架构
TARGETS := linux-arm64 linux-x64 macos-arm64 macos-x64 windows-x64

# 编译输出目录
OUTPUT_DIR := release

# Go 构建参数
GO := go
LDFLAGS := -s -w

.PHONY: all clean $(TARGETS)

# 默认目标：构建所有平台
all: $(TARGETS)

# 单独平台构建
linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/linux-arm64/HyperBot

linux-x64:
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/linux-x64/HyperBot

macos-arm64:
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/macos-arm64/HyperBot

macos-x64:
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/macos-x64/HyperBot

windows-x64:
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/windows-x64/HyperBot.exe

# 清理构建产物
clean:
	rm -rf $(OUTPUT_DIR)/*
