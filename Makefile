# 最小化 Makefile - 本地编译
OUTPUT_DIR := release
GO := go
LDFLAGS := -s -w

.PHONY: all clean

all:
	mkdir -p $(OUTPUT_DIR)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/HackerTeam

clean:
	rm -rf $(OUTPUT_DIR)/*
