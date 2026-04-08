.PHONY: build build-dev build-prod swagger run clean docker-build docker-up docker-down docker-logs

APP_DIR := app
BINARY := fusion-search
COMPOSE := docker compose

# 去除调试信息的 ldflags
# -s: 去除符号表
# -w: 去除 DWARF 调试信息（无法使用 go tool pprof / delve 调试）
LDFLAGS_PROD := -s -w

# 开发构建（包含 Swagger，保留调试信息）
build-dev:
	cd $(APP_DIR) && go build -tags dev -o $(BINARY)-dev .

# 生产构建（不包含 Swagger，去除调试信息）
build-prod:
	cd $(APP_DIR) && go build -ldflags "$(LDFLAGS_PROD)" -o $(BINARY) .

# 默认构建（生产）
build: build-prod

# 生成 Swagger 文档
swagger:
	cd $(APP_DIR) && swag init -g main.go --parseDependency --parseInternal

# 运行开发版本
run: swagger build-dev
	cd $(APP_DIR) && ./$(BINARY)-dev

# 清理
clean:
	cd $(APP_DIR) && rm -f $(BINARY) $(BINARY)-dev $(BINARY)-prod

# 构建 Go API 镜像
docker-build:
	$(COMPOSE) build api

# 启动 Go API（含依赖服务）
docker-up:
	$(COMPOSE) up -d api

# 停止所有 compose 服务
docker-down:
	$(COMPOSE) down

# 查看 API 日志
docker-logs:
	$(COMPOSE) logs -f api
