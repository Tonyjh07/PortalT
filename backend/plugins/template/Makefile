# PortalT 插件管理
.PHONY: build install clean

# 插件 ID（从 manifest.json 读取）
ID := $(shell sed -n 's/.*"id": *"\(.*\)".*/\1/p' manifest.json)

# 投放目录（默认 dev 的 PLUGINS_DIR，部署时由 install.sh/update.sh 决定）
PLUGINS_DIR ?= $(or $(PLUGINS_DIR),../../../plugins)

## build — 编译插件二进制
build:
	CGO_ENABLED=0 go build -o plugin ./cmd/plugin

## install — 投放编译产物到 PLUGINS_DIR
install: build
	mkdir -p $(PLUGINS_DIR)/$(ID)
	cp plugin manifest.json $(PLUGINS_DIR)/$(ID)/
	@test -d static && cp -r static $(PLUGINS_DIR)/$(ID)/ && echo "静态资源已同步" || true

## clean — 清理本地编译产物
clean:
	rm -f plugin