# GeoIP Info API Server

一个基于 Golang + Gin + MaxMind GeoLite2 的 GeoIP 查询服务，支持 IP 国家信息查询、LRU 缓存优化，并集成 request_id 方便调试与日志追踪。

## ✨ 特性

- **IP 地理位置查询**：根据输入的 IP 地址或请求头中的 `X-Forwarded-For`，查询国家、洲际代码、中文国家名称等信息。
- **ASN 信息查询**：提供 IP 对应的自治系统编号（ASN）和组织名称。
- **LRU 缓存**：使用 LRU 缓存减少对 GeoLite2 数据库的重复查询，提高性能。
- **自定义日志**：记录请求的详细信息，包括时间戳、客户端 IP、RequestID、HTTP 方法、路径、状态码、延迟、域名、User-Agent、X-Forwarded-For、X-Real-IP 和远程地址。
- **日志轮转**：使用 `lumberjack` 实现日志文件的自动轮转和压缩。
- **pprof 性能分析**：支持通过环境变量启用 pprof 性能分析端点。
- **RequestID**：为每个请求生成唯一的 RequestID，便于追踪和调试。


## 📦 编译

本服务依赖 Go 1.20+

```bash
git clone https://github.com/beck-8/geoip-server.git
cd geoip-server
go build -o geoip-server main.go
```

**X86_64一键安装**
```bash
curl -sSL https://raw.githubusercontent.com/beck-8/geoip-server/refs/heads/main/install.sh | bash
```

## 🛠️ 参数说明

| 参数             | 类型     | 默认值                      | 描述                        |
|------------------|----------|-----------------------------|-----------------------------|
| `-city-mmdb`  | string   | `GeoLite2-City.mmdb`     | MaxMind 城市数据库路径      |
| `-asn-mmdb`  | string   | `GeoLite2-ASN.mmdb`     | ASN 数据库路径      |
| `-port`          | string   | `:8399`                     | HTTP 监听端口               |
| `-cache`         | int      | `10000`                     | LRU 缓存条目数量            |
| `-log`           | string   | `geo.log`                   | 日志文件路径                |
| `-logsize`       | int      | `10`                        | 单个日志文件最大 MB         |
| `-logbackups`    | int      | `5`                         | 最大保留备份日志数量        |
| `-logage`        | int      | `14`                        | 日志最大保留天数            |


## 🚀 启动方式

```bash
./geoip-server \
  -city-mmdb /path/to/GeoLite2-City.mmdb \
  -asn-mmdb /path/to/GeoLite2-ASN.mmdb \
  -port :8399 \
  -cache 10000 \
  -log geo.log
```


## 🔍 请求示例

### 查询指定 IP

```
GET /api/ipinfo?ip=8.8.8.8
```

### 查询客户端实际 IP（自动提取）

```
GET /api/ipinfo
```

返回结果示例：

```json
{
	"ip": "119.29.29.29",
	"continent_code": "AS",
	"country": "China",
	"country_zh": "中国",
	"country_code": "CN",
	"city": "Guangzhou",
	"city_zh": "广州市",
	"registered_country_code": "CN",
	"asn": 132203,
	"organization": "Tencent Building, Kejizhongyi Avenue",
	"timestamp": 1755592554551,
	"request_id": "523a8da8-2e62-44ad-bd2e-e75411949309"
}
```


## 🧪 环境变量

| 变量名           | 描述                                        |
|------------------|---------------------------------------------|
| `MAXMIND_PPROF`  | 非空时开启 PProf 性能分析，监听端口 :62000 |


## 📓 日志说明

- 输出到 stdout 和 `-log` 指定的文件
- 使用 `lumberjack` 实现日志滚动
- 每行日志包含 `request_id`，便于追踪调试

示例日志：

```
[2025-07-20T23:22:12+08:00] 127.0.0.1 - [bb415f25-cd3b-450d-ab6b-86f153857538] "GET /api/ipinfo?ip=8.8.8.8 HTTP/1.1" 200 0 "127.0.0.1:8399" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36" "" "" "127.0.0.1:10163"
```


## 📥 数据库获取

   - 访问 [MaxMind 官网](https://www.maxmind.com/)，注册并下载 `GeoLite2-Country.mmdb` 和 `GeoLite2-ASN.mmdb` 文件。
   - 或从别的地方[找](https://github.com/P3TERX/GeoLite.mmdb)
   - 将这两个文件放置在项目根目录或指定路径。

## 🧩 性能分析（可选）

设置环境变量后自动启动 pprof：

```bash
export MAXMIND_PPROF=1
./geoip-server ...
```

访问地址：

```
http://localhost:62000/debug/pprof/
```


## 📄 License

MIT