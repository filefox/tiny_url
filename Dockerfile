# 使用官方Golang基础镜像
FROM golang:latest

# 设置工作目录
WORKDIR /app

# 复制并编译项目
COPY . .
RUN go mod tidy
RUN go build -ldflags "-s -w" -o tiny_url .

# 运行可执行文件
CMD ["./tiny_url"]

