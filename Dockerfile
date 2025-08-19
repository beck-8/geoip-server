# 构建阶段
FROM golang:alpine AS builder
WORKDIR /app
COPY . .
ARG GITHUB_SHA
ARG VERSION
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
# 根据TARGETVARIANT设置GOARM（仅对arm/v7有用）
ENV GOARM=${TARGETVARIANT}
# 使用交叉编译，动态设置目标平台
RUN echo "Building commit: ${GITHUB_SHA:0:7}" && \
    go mod tidy && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w -X main.Version=${VERSION} -X main.CurrentCommit=${GITHUB_SHA:0:7}" -trimpath -o geoip-server .

# 运行阶段
FROM alpine
ENV TZ=Asia/Shanghai
RUN apk add --no-cache alpine-conf ca-certificates && \
    /usr/sbin/setup-timezone -z Asia/Shanghai && \
    apk del alpine-conf && \
    rm -rf /var/cache/apk/*
COPY --from=builder /app/geoip-server /app/geoip-server
WORKDIR /app
CMD /app/geoip-server -asn-mmdb GeoLite2-ASN.mmdb -city-mmdb GeoLite2-City.mmdb
EXPOSE 8399