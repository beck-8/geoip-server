#!/bin/bash

# Exit on any error
set -e

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    echo "jq 未安装。请先安装 jq（例如：sudo apt install jq 或 sudo yum install jq）。"
    exit 1
fi

# Define variables
INSTALL_DIR="/opt/geoip-server"
SERVICE_FILE="/etc/systemd/system/geoip-server.service"
LOG_FILE="/var/log/geoip-server.log"

# Fetch the latest release version and URL from GitHub API
echo "正在获取最新的 geoip-server release 版本..."
RELEASE_INFO=$(curl -s https://api.github.com/repos/beck-8/geoip-server/releases/latest)
GEOIP_SERVER_VERSION=$(echo "$RELEASE_INFO" | jq -r '.tag_name' | sed 's/^v//')  # 去掉前缀 'v'
GEOIP_SERVER_URL=$(echo "$RELEASE_INFO" | jq -r '.assets[] | select(.name | contains("Linux_x86_64.tar.gz")) | .browser_download_url')

if [ -z "$GEOIP_SERVER_VERSION" ] || [ -z "$GEOIP_SERVER_URL" ]; then
    echo "无法获取最新的 release 信息，请检查网络连接或 GitHub API 限制。"
    exit 1
fi

echo "最新的 geoip-server 版本: v${GEOIP_SERVER_VERSION}"
echo "下载 URL: ${GEOIP_SERVER_URL}"

# Fetch the latest GeoLite2 database URLs from GitHub API
echo "正在获取最新的 GeoLite2 数据库 release 版本..."
MMDB_RELEASE_INFO=$(curl -s https://api.github.com/repos/P3TERX/GeoLite.mmdb/releases/latest)
CITY_MMDB_URL=$(echo "$MMDB_RELEASE_INFO" | jq -r '.assets[] | select(.name == "GeoLite2-City.mmdb") | .browser_download_url')
ASN_MMDB_URL=$(echo "$MMDB_RELEASE_INFO" | jq -r '.assets[] | select(.name == "GeoLite2-ASN.mmdb") | .browser_download_url')

if [ -z "$CITY_MMDB_URL" ] || [ -z "$ASN_MMDB_URL" ]; then
    echo "无法获取最新的 GeoLite2 数据库链接，请检查网络连接或 GitHub API 限制。"
    exit 1
fi

echo "GeoLite2-City.mmdb 下载 URL: ${CITY_MMDB_URL}"
echo "GeoLite2-ASN.mmdb 下载 URL: ${ASN_MMDB_URL}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "此脚本需要以 root 权限运行。请使用 sudo。"
    exit 1
fi

# Create installation directory
echo "创建安装目录: ${INSTALL_DIR}"
mkdir -p "${INSTALL_DIR}"

# Download and extract geoip-server
echo "正在下载 geoip-server v${GEOIP_SERVER_VERSION}..."
curl -L -o geoip-server.tar.gz "${GEOIP_SERVER_URL}"
tar -xzf geoip-server.tar.gz -C "${INSTALL_DIR}"
rm geoip-server.tar.gz

# Download GeoLite2 databases
echo "正在下载 GeoLite2-City.mmdb..."
wget -O "${INSTALL_DIR}/GeoLite2-City.mmdb" "${CITY_MMDB_URL}"
echo "正在下载 GeoLite2-ASN.mmdb..."
wget -O "${INSTALL_DIR}/GeoLite2-ASN.mmdb" "${ASN_MMDB_URL}"

# Ensure geoip-server binary is executable
chmod +x "${INSTALL_DIR}/geoip-server"

# Create log file and set permissions
echo "创建日志文件: ${LOG_FILE}"
touch "${LOG_FILE}"
chmod 644 "${LOG_FILE}"

# Create systemd service file
echo "创建 systemd 服务文件: ${SERVICE_FILE}"
cat > "${SERVICE_FILE}" << EOL
[Unit]
Description=geoip-server
After=network.target syslog.target
Wants=network.target

[Service]
Type=simple
StandardOutput=null
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/geoip-server -port :8399 -asn-mmdb GeoLite2-ASN.mmdb -city-mmdb GeoLite2-City.mmdb -log ${LOG_FILE}
Restart=always

[Install]
WantedBy=multi-user.target
EOL

# Reload systemd, enable and start the service
echo "重新加载 systemd 守护进程..."
systemctl daemon-reload
echo "启用 geoip-server 服务..."
systemctl enable geoip-server.service
echo "启动 geoip-server 服务..."
systemctl restart geoip-server.service

# Verify service status
echo "检查 geoip-server 服务状态..."
systemctl status geoip-server.service --no-pager

echo "安装完成！geoip-server 正在端口 8399 上运行。"