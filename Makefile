.PHONY: proto install-proto-tools clean help

# 生成 protobuf 文件
proto:
	@echo "Generating protobuf files..."
	@export PATH=$$PATH:$$(go env GOPATH)/bin && \
	cd protos && \
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       subtube.proto
	@echo "✅ Protobuf files generated successfully!"

# 安装 protoc 所需的 Go 插件
install-proto-tools:
	@echo "Installing protoc-gen-go..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@echo "Installing protoc-gen-go-grpc..."
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "✅ Protobuf tools installed successfully!"

# 清理生成的文件
clean:
	@echo "Cleaning generated protobuf files..."
	@rm -f protos/*.pb.go
	@echo "✅ Cleaned!"

# 显示帮助信息
help:
	@echo "LumiTime Makefile Commands:"
	@echo "  make proto              - 生成 protobuf Go 文件"
	@echo "  make install-proto-tools - 安装 protobuf 编译工具"
	@echo "  make clean              - 清理生成的 protobuf 文件"
	@echo "  make help               - 显示此帮助信息"
