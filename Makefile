# PortalT - HomeLab统一门户系统 Makefile
# 环境要求: Go 1.21+, Node 20+, Docker 24+, GNU Make (Windows需配 sh.exe)

.PHONY: version init init-backend init-frontend run run-backend \
        test test-unit test-domain test-repo test-integration test-esxi test-auth \
        test-api test-e2e \
        build build-backend \
        docker-build docker-build-backend docker-build-caddy docker-build-frontend \
        docker-push up down logs clean

GO       ?= go
NPM      ?= npm
DOCKER   ?= docker
VERSION  ?= v0.1

# Windows (MSYS/MinGW) 下 npm 是 .cmd 包装，sh 无法直接执行
UNAME_S := $(shell uname -s)
ifneq ($(findstring MINGW,$(UNAME_S)),)
NPM := npm.cmd
endif
ifneq ($(findstring MSYS,$(UNAME_S)),)
NPM := npm.cmd
endif

BACKEND_DIR   := backend
FRONTEND_DIR  := frontend
BIN_DIR       := bin
BIN_NAME      := portalt
ifeq ($(OS),Windows_NT)
BIN_NAME := portalt.exe
endif

# ------------------------------------------------------------------
# 工具链信息
# ------------------------------------------------------------------
version:
	@echo "=== PortalT 工具链版本 (要求: Go 1.21+, Node 20+, Docker 24+) ==="
	@echo "Go:              $$($(GO) version 2>&1)"
	@echo "Node:            $$($(NPM) --version 2>&1)"
	@echo "Docker:          $$($(DOCKER) --version 2>&1)"
	@echo "Docker Compose:  $$($(DOCKER) compose version 2>&1)"

# ------------------------------------------------------------------
# 初始化: 创建目录结构, 下载依赖
# ------------------------------------------------------------------
init: init-backend init-frontend
	@echo "PortalT 初始化完成"

init-backend:
	@mkdir -p $(BACKEND_DIR)/cmd/server \
		$(BACKEND_DIR)/internal/domain/services \
		$(BACKEND_DIR)/internal/ports \
		$(BACKEND_DIR)/internal/adapters/postgres \
		$(BACKEND_DIR)/internal/adapters/esxi \
		$(BACKEND_DIR)/internal/adapters/memory \
		$(BACKEND_DIR)/internal/adapters/auth \
		$(BACKEND_DIR)/internal/api/middleware \
		$(BACKEND_DIR)/internal/api/v1 \
		$(BACKEND_DIR)/internal/config \
		$(BACKEND_DIR)/migrations \
		$(BACKEND_DIR)/pkg/crypt \
		$(BACKEND_DIR)/pkg/validator
	@cd $(BACKEND_DIR) && if [ ! -f go.mod ]; then $(GO) mod init portalt; fi
	@cd $(BACKEND_DIR) && $(GO) mod tidy
	@echo "后端依赖下载完成"

init-frontend:
	@mkdir -p $(FRONTEND_DIR)/pages/vms $(FRONTEND_DIR)/pages/plugins \
		$(FRONTEND_DIR)/components/layout $(FRONTEND_DIR)/components/menu \
		$(FRONTEND_DIR)/components/cards \
		$(FRONTEND_DIR)/composables $(FRONTEND_DIR)/stores \
		$(FRONTEND_DIR)/server/api/proxy
	@echo "前端目录创建完成 (npm install 将在 Phase 7 执行)"

# ------------------------------------------------------------------
# 运行
# ------------------------------------------------------------------
run: run-backend

run-backend:
	@cd $(BACKEND_DIR) && $(GO) run ./cmd/server

# ------------------------------------------------------------------
# 测试
# ------------------------------------------------------------------
test: test-unit
	@echo "所有测试通过"

test-unit: test-domain test-repo test-api

test-domain:
	@cd $(BACKEND_DIR) && $(GO) test ./internal/domain/... -cover -count=1

test-repo:
	@cd $(BACKEND_DIR) && $(GO) test ./internal/adapters/memory/... -count=1

test-integration:
	@cd $(BACKEND_DIR) && $(GO) test -tags integration ./internal/adapters/... -count=1

test-esxi:
	@cd $(BACKEND_DIR) && $(GO) test -tags esxi ./internal/adapters/esxi/... -count=1

test-auth:
	@cd $(BACKEND_DIR) && $(GO) test ./internal/adapters/auth/... ./internal/api/v1/... -count=1

test-api:
	@cd $(BACKEND_DIR) && $(GO) test ./internal/api/... -count=1

test-e2e:
	@echo "E2E 测试 (Docker Compose) 将在 Phase 10 提供"

# ------------------------------------------------------------------
# 构建
# ------------------------------------------------------------------
build: build-backend
	@echo "构建完成"

build-backend:
	@cd $(BACKEND_DIR) && CGO_ENABLED=0 $(GO) build -o $(BIN_DIR)/$(BIN_NAME) ./cmd/server
	@echo "后端构建完成: $(BACKEND_DIR)/$(BIN_DIR)/$(BIN_NAME)"

# ------------------------------------------------------------------
# Docker 构建与推送 (镜像推送在 Phase 10 启用)
# ------------------------------------------------------------------
docker-build: docker-build-backend docker-build-caddy
	@echo "Docker 镜像构建完成"

docker-build-backend:
	$(DOCKER) build -t portalt/backend:$(VERSION) -f $(BACKEND_DIR)/Dockerfile $(BACKEND_DIR)

docker-build-caddy:
	$(DOCKER) build -t portalt/caddy:$(VERSION) -f caddy/Dockerfile caddy

docker-build-frontend:
	$(DOCKER) build -t portalt/frontend:$(VERSION) -f $(FRONTEND_DIR)/Dockerfile $(FRONTEND_DIR)

docker-push:
	@echo "docker-push 将在 Phase 10 (CI/CD) 启用"

# ------------------------------------------------------------------
# Docker Compose 便捷命令
# ------------------------------------------------------------------
up:
	$(DOCKER) compose up -d

down:
	$(DOCKER) compose down

logs:
	$(DOCKER) compose logs -f

# ------------------------------------------------------------------
# 清理
# ------------------------------------------------------------------
clean:
	@rm -rf $(BACKEND_DIR)/$(BIN_DIR) $(BACKEND_DIR)/vendor
	@echo "清理完成"
