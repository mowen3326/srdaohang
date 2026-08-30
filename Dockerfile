# syntax=docker/dockerfile:1

# ---------- 构建阶段 ----------
FROM golang:1.27-alpine AS build

WORKDIR /src

# 模块代理：默认走国内镜像，海外网络可改为
#   docker build --build-arg GOPROXY=https://proxy.golang.org,direct .
ARG GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct
ENV GOPROXY=${GOPROXY}

# 先只拷贝依赖清单，go.mod / go.sum 没变时复用缓存，不必重新下载模块
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# modernc.org/sqlite 是纯 Go 实现，关闭 CGO 即可得到完全静态的二进制
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/daohang .

# 第三方许可清单转成纯文本，去掉 Markdown 代码围栏，便于容器内直接查看
RUN sed '/^[[:space:]]*```/d' THIRD_PARTY_LICENSES.md > /out/THIRD_PARTY_LICENSES.txt

# ---------- 运行阶段 ----------
FROM alpine:3.21

# tzdata 用于 TZ 生效；不需要可删掉这一行并移除下面的 ENV TZ
RUN apk add --no-cache tzdata \
    && adduser -D -u 10001 -h /app daohang \
    && mkdir -p /app/data/uploads /app/licenses \
    && chown -R daohang:daohang /app

WORKDIR /app

COPY --from=build /out/daohang /app/daohang
COPY LICENSE /app/licenses/LICENSE
COPY --from=build /out/THIRD_PARTY_LICENSES.txt /app/licenses/THIRD_PARTY_LICENSES.txt

# SESSION_SECRET 故意不设默认值：必须由运行时注入，否则服务会回退到内置密钥
ENV TZ=Asia/Shanghai \
    ADDR=:8080 \
    DB_PATH=/app/data/daohang.db

USER daohang

EXPOSE 8080

# 数据库与上传的图片都在这里，务必持久化
VOLUME ["/app/data"]

# 改了 ADDR 的话这里的端口要一起改
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null "http://127.0.0.1:8080/" || exit 1

ENTRYPOINT ["/app/daohang"]
