.PHONY: build build-dev build-prod swagger run clean

APP_DIR := app
BINARY := fusion-search

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
