# 构建阶段
FROM golang:1.21-alpine AS builder

WORKDIR /build

# 复制 go mod 文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# 运行阶段
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /build/main .
COPY --from=builder /build/templates ./templates
COPY --from=builder /build/static ./static

# 复制配置文件（如果存在）
COPY benchlist.json data.json ./

# 暴露端口
EXPOSE 8080

# 运行应用
CMD ["./main"]
