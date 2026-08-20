# PortalT - HomeLab统一门户系统 Makefile
# 环境要求: Go 1.21+, Node 20+, Docker 24+, GNU Make (Windows需配 sh.exe)

.PHONY: version init init-backend init-frontend run run-backend run-frontend \
        test test-unit test-domain test-repo test-virt test-sqlite test-integration test-esxi test-auth \
        test-api test-e2e \
        build build-backend build-frontend \
        docker-build docker-build-backend docker-build-frontend \
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
	@cd $(FRONTEND_DIR) && if [ ! -d node_modules ]; then $(NPM) install --no-fund --no-audit; fi
	@echo "前端依赖下载完成"

# ------------------------------------------------------------------
# 运行
# ------------------------------------------------------------------
run: run-backend

run-backend:
	@cd $(BACKEND_DIR) && $(GO) run ./cmd/server

run-frontend:
	@cd $(FRONTEND_DIR) && $(NPM) run dev

# ------------------------------------------------------------------
# 测试
# ------------------------------------------------------------------
test: test-unit
	@echo "所有测试通过"

# 原生 MinGW gcc（-race 需要）：自动探测 WinGet 安装的 WinLibs
LOCALAPPDATA_FS := $(subst \\,/,$(LOCALAPPDATA))
MINGW_GCC := $(firstword $(wildcard $(LOCALAPPDATA_FS)/Microsoft/WinGet/Packages/BrechtSanders.WinLibs.MCF.UCRT_*/mingw64/bin/gcc.exe))

test-unit: test-domain test-repo test-virt test-api

test-domain:
	@cd $(BACKEND_DIR) && $(GO) test ./internal/domain/... -cover -count=1

test-repo:
	@cd $(BACKEND_DIR) && $(GO) test ./internal/adapters/memory/... ./internal/adapters/gormstore/... -count=1

test-virt:
	@cd $(BACKEND_DIR) && $(GO) test ./internal/adapters/mock/... ./internal/adapters/ -count=1

test-race:
	@cd $(BACKEND_DIR) && CC="$(MINGW_GCC)" $(GO) test -race ./... -count=1

test-sqlite:
	@cd $(BACKEND_DIR) && $(GO) test -tags integration ./internal/adapters/sqlite/... -count=1

test-integration:
	@cd $(BACKEND_DIR) && $(GO) test -tags integration ./internal/adapters/... -count=1

test-esxi:
	@cd $(BACKEND_DIR) && $(GO) test -tags esxi ./internal/adapters/esxi/... -count=1

test-auth:
	@cd $(BACKEND_DIR) && $(GO) test ./internal/adapters/auth/... ./internal/api/... -count=1

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

build-frontend:
	@cd $(FRONTEND_DIR) && $(NPM) run build
	@echo "前端构建完成: $(FRONTEND_DIR)/.output"

# ------------------------------------------------------------------
# Docker 构建与推送（生产部署走 deploy/install.sh 的 systemd + 二进制方案，
# 镜像用于容器化运行与 CI 构建校验；生产 Caddy 为系统服务，不打包镜像）
# ------------------------------------------------------------------
docker-build: docker-build-backend docker-build-frontend
	@echo "Docker 镜像构建完成"

docker-build-backend:
	$(DOCKER) build -t portalt/backend:$(VERSION) $(BACKEND_DIR)

docker-build-frontend:
	$(DOCKER) build -t portalt/frontend:$(VERSION) $(FRONTEND_DIR)

docker-push:
	@echo "docker-push 暂不启用（生产部署走 deploy/update.sh，镜像不推 registry）"

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
