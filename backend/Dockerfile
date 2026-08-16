# PortalT 后端镜像（多阶段构建，纯 Go 无 cgo）
# 生产部署默认走 deploy/install.sh（systemd + 二进制）；本镜像用于容器化运行与 CI 构建校验。
# 运行依赖按需注入环境变量（完整清单见仓库 .env.example）：
#   GUACD_URL=host:4822（远程桌面）、PLUGINS_DIR、JWT_SECRET、ADMIN_PASSWORD 等；
#   postgres 模式设 DB_DRIVER=postgres + DB_DSN。
FROM golang:1.26-alpine AS builder
WORKDIR /src
# 先拷依赖清单再拷源码，利用镜像层缓存加速重复构建
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/portalt-server ./cmd/server

FROM alpine:3
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/portalt-server ./portalt-server
COPY migrations ./migrations
# 容器内必须监听 0.0.0.0 才能被端口映射访问；sqlite 数据放 /data 卷持久化
ENV PORT=0.0.0.0:8080 \
    DB_DRIVER=sqlite \
    DB_DSN=/data/portalt.db \
    DB_MIGRATIONS_DIR=migrations
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["./portalt-server"]
