#!/usr/bin/env bash
# ChatGo 自建中继一键安装（Linux + systemd + nginx）。
#
# 默认从 GitHub Release 下载 relayd-linux-<arch>，安装为 chatgo-relay
# 服务，并由 nginx 在 8080 端口反向代理。脚本可重复执行：升级二进制、
# 重写服务与 nginx 配置都会保留 relay.db。
#
# 用法：
#   sudo bash scripts/install-relay.sh --port 8080 --public-url http://1.2.3.4:8080
#   curl -fsSL https://raw.githubusercontent.com/tk103331/ChatGo/main/scripts/install-relay.sh \
#     | sudo bash -s -- --port 8080
#
# 使用 --help 查看全部选项。
set -Eeuo pipefail

GITHUB_REPO="${GITHUB_REPO:-tk103331/ChatGo}"
RELAY_USER="${RELAY_USER:-chatgo}"
RELAY_HOME="${RELAY_HOME:-/opt/chatgo-relay}"
RELAY_DATA="${RELAY_DATA:-/var/lib/chatgo-relay/relay.db}"
RELAY_ADDR="${RELAY_ADDR:-127.0.0.1:8787}"
RELAY_PORT="${RELAY_PORT:-8080}"
RELAY_DOMAIN="${RELAY_DOMAIN:-}"
RELAY_PUBLIC_URL="${RELAY_PUBLIC_URL:-}"
RELAY_VERSION="${RELAY_VERSION:-latest}"
RELAY_BINARY="${RELAY_BINARY:-}"
RELAY_BINARY_URL="${RELAY_BINARY_URL:-}"
RELAY_NGINX="${RELAY_NGINX:-1}"
RELAY_TLS_CERT="${RELAY_TLS_CERT:-}"
RELAY_TLS_KEY="${RELAY_TLS_KEY:-}"
APNS_KEY="${APNS_KEY:-}"
APNS_KEY_ID="${APNS_KEY_ID:-}"
APNS_TEAM_ID="${APNS_TEAM_ID:-}"
APNS_TOPIC="${APNS_TOPIC:-}"
APNS_ENV="${APNS_ENV:-prod}"
FCM_ACCOUNT="${FCM_ACCOUNT:-}"

UNIT=/etc/systemd/system/chatgo-relay.service
NGINX_CONF=/etc/nginx/conf.d/chatgo-relay.conf
TMPDIR_INSTALL=""
BACKUP_NGINX=""

usage() {
	cat <<'USAGE'
ChatGo 自建中继一键安装

用法:
  sudo bash scripts/install-relay.sh [选项]

选项:
  --port PORT            nginx 对外端口，默认 8080
  --addr ADDR            relayd 监听地址，默认 127.0.0.1:8787
  --public-url URL       手机与桌面使用的中继地址；默认按端口自动推测
  --domain DOMAIN        nginx server_name；TLS 时通常填写域名
  --tls-cert FILE        nginx TLS 证书（fullchain.pem）
  --tls-key FILE         nginx TLS 私钥（privkey.pem）
  --no-nginx             不安装/配置 nginx；relayd 直接监听 --addr
  --version TAG          GitHub Release 版本，如 v0.18.0；默认 latest
  --binary FILE          使用本地 relayd 二进制，不下载
  --binary-url URL       从指定 URL 下载 relayd 二进制
  --data FILE            relay.db 路径，默认 /var/lib/chatgo-relay/relay.db
  --user USER            systemd 运行用户，默认 chatgo
  --apns-key FILE        APNs .p8 私钥
  --apns-key-id ID       APNs Key ID
  --apns-team-id ID      Apple Team ID
  --apns-topic TOPIC     APNs Topic（iOS Bundle ID）
  --apns-env ENV         APNs 环境 prod 或 dev，默认 prod
  --fcm-account FILE     FCM 服务账号 JSON
  -h, --help             显示本帮助

示例:
  sudo bash scripts/install-relay.sh --port 8080 \
    --public-url http://relay.example.com:8080
  sudo bash scripts/install-relay.sh --port 443 --domain relay.example.com \
    --tls-cert /etc/letsencrypt/live/relay.example.com/fullchain.pem \
    --tls-key /etc/letsencrypt/live/relay.example.com/privkey.pem

说明:
  - 只支持 Linux + systemd，建议 Debian/Ubuntu。
  - nginx 模式下 relayd 只监听回环地址，由 nginx 终止 TLS。
  - 未给 --public-url 时会尝试 https://api.ipify.org 探测公网地址。
  - 升级时保留 relay.db；脚本会覆盖 chatgo-relay 的服务与 nginx 配置。
USAGE
}

log() { printf '\033[1;32m[chatgo-relay]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[chatgo-relay]\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31m[chatgo-relay]\033[0m %s\n' "$*" >&2; exit 1; }

cleanup() {
	[ -n "$TMPDIR_INSTALL" ] && rm -rf "$TMPDIR_INSTALL"
}
trap cleanup EXIT

need_value() {
	[ $# -ge 2 ] && [ -n "$2" ] || die "选项 $1 需要一个值"
}

while [ $# -gt 0 ]; do
	case "$1" in
	--port) need_value "$1" "${2:-}"; RELAY_PORT="$2"; shift 2 ;;
	--port=*) RELAY_PORT="${1#*=}"; shift ;;
	--addr) need_value "$1" "${2:-}"; RELAY_ADDR="$2"; shift 2 ;;
	--addr=*) RELAY_ADDR="${1#*=}"; shift ;;
	--public-url) need_value "$1" "${2:-}"; RELAY_PUBLIC_URL="$2"; shift 2 ;;
	--public-url=*) RELAY_PUBLIC_URL="${1#*=}"; shift ;;
	--domain) need_value "$1" "${2:-}"; RELAY_DOMAIN="$2"; shift 2 ;;
	--domain=*) RELAY_DOMAIN="${1#*=}"; shift ;;
	--tls-cert) need_value "$1" "${2:-}"; RELAY_TLS_CERT="$2"; shift 2 ;;
	--tls-cert=*) RELAY_TLS_CERT="${1#*=}"; shift ;;
	--tls-key) need_value "$1" "${2:-}"; RELAY_TLS_KEY="$2"; shift 2 ;;
	--tls-key=*) RELAY_TLS_KEY="${1#*=}"; shift ;;
	--no-nginx) RELAY_NGINX=0; shift ;;
	--nginx) RELAY_NGINX=1; shift ;;
	--version) need_value "$1" "${2:-}"; RELAY_VERSION="$2"; shift 2 ;;
	--version=*) RELAY_VERSION="${1#*=}"; shift ;;
	--binary) need_value "$1" "${2:-}"; RELAY_BINARY="$2"; shift 2 ;;
	--binary=*) RELAY_BINARY="${1#*=}"; shift ;;
	--binary-url) need_value "$1" "${2:-}"; RELAY_BINARY_URL="$2"; shift 2 ;;
	--binary-url=*) RELAY_BINARY_URL="${1#*=}"; shift ;;
	--data) need_value "$1" "${2:-}"; RELAY_DATA="$2"; shift 2 ;;
	--data=*) RELAY_DATA="${1#*=}"; shift ;;
	--user) need_value "$1" "${2:-}"; RELAY_USER="$2"; shift 2 ;;
	--user=*) RELAY_USER="${1#*=}"; shift ;;
	--apns-key) need_value "$1" "${2:-}"; APNS_KEY="$2"; shift 2 ;;
	--apns-key=*) APNS_KEY="${1#*=}"; shift ;;
	--apns-key-id) need_value "$1" "${2:-}"; APNS_KEY_ID="$2"; shift 2 ;;
	--apns-key-id=*) APNS_KEY_ID="${1#*=}"; shift ;;
	--apns-team-id) need_value "$1" "${2:-}"; APNS_TEAM_ID="$2"; shift 2 ;;
	--apns-team-id=*) APNS_TEAM_ID="${1#*=}"; shift ;;
	--apns-topic) need_value "$1" "${2:-}"; APNS_TOPIC="$2"; shift 2 ;;
	--apns-topic=*) APNS_TOPIC="${1#*=}"; shift ;;
	--apns-env) need_value "$1" "${2:-}"; APNS_ENV="$2"; shift 2 ;;
	--apns-env=*) APNS_ENV="${1#*=}"; shift ;;
	--fcm-account) need_value "$1" "${2:-}"; FCM_ACCOUNT="$2"; shift 2 ;;
	--fcm-account=*) FCM_ACCOUNT="${1#*=}"; shift ;;
	-h|--help) usage; exit 0 ;;
	*) die "未知选项: $1（使用 --help 查看）" ;;
	esac
done

[ "$(id -u)" -eq 0 ] || die "请用 root 运行：sudo bash $0 ..."
[ "$(uname -s)" = "Linux" ] || die "只支持 Linux"
command -v systemctl >/dev/null 2>&1 || die "需要 systemd"

case "$RELAY_PORT" in
''|*[!0-9]*) die "--port 必须是数字" ;;
esac
[ "$RELAY_PORT" -ge 1 ] && [ "$RELAY_PORT" -le 65535 ] || die "--port 超出范围"
case "$RELAY_DATA" in
/*) ;;
*) die "--data 必须是绝对路径" ;;
esac
case "$RELAY_ADDR" in
*:*) ;;
*) die "--addr 需要 host:port" ;;
esac

if [ "$RELAY_NGINX" -eq 0 ] && [ "$RELAY_ADDR" = "127.0.0.1:8787" ]; then
	# 没有 nginx 时回环地址无法被手机访问；直接监听公网端口。
	RELAY_ADDR="0.0.0.0:${RELAY_PORT}"
	warn "--no-nginx：relayd 将直接监听 ${RELAY_ADDR}"
fi
if [ "$RELAY_NGINX" -eq 0 ] && [ -z "$RELAY_TLS_CERT" ]; then
	warn "--no-nginx 且未配置 TLS：手机与桌面会以明文 HTTP/WS 连接，生产环境不建议这样使用"
fi

if { [ -n "$RELAY_TLS_CERT" ] && [ -z "$RELAY_TLS_KEY" ]; } || { [ -z "$RELAY_TLS_CERT" ] && [ -n "$RELAY_TLS_KEY" ]; }; then
	die "--tls-cert 与 --tls-key 必须同时提供"
fi
if [ -n "$RELAY_TLS_CERT" ]; then
	[ -r "$RELAY_TLS_CERT" ] || die "TLS 证书不可读: $RELAY_TLS_CERT"
	[ -r "$RELAY_TLS_KEY" ] || die "TLS 私钥不可读: $RELAY_TLS_KEY"
fi

apns_fields=0
for v in "$APNS_KEY" "$APNS_KEY_ID" "$APNS_TEAM_ID" "$APNS_TOPIC"; do
	[ -n "$v" ] && apns_fields=$((apns_fields + 1))
done
[ "$apns_fields" -eq 0 ] || [ "$apns_fields" -eq 4 ] || die "APNs 需要同时提供 --apns-key --apns-key-id --apns-team-id --apns-topic"
case "$APNS_ENV" in prod|dev) ;; *) die "--apns-env 只能是 prod 或 dev" ;; esac
[ -z "$APNS_KEY" ] || [ -r "$APNS_KEY" ] || die "APNs 私钥不可读: $APNS_KEY"
[ -z "$FCM_ACCOUNT" ] || [ -r "$FCM_ACCOUNT" ] || die "FCM 账号文件不可读: $FCM_ACCOUNT"

# 安装最小的依赖。nginx 只在需要反向代理时安装。
install_packages() {
	if command -v apt-get >/dev/null 2>&1; then
		export DEBIAN_FRONTEND=noninteractive
		apt-get update -qq
		apt-get install -y -qq "$@"
	elif command -v dnf >/dev/null 2>&1; then
		dnf install -y -q "$@"
	elif command -v yum >/dev/null 2>&1; then
		yum install -y -q "$@"
	else
		die "找不到 apt-get/dnf/yum，请手动安装: $*"
	fi
}

packages=()
command -v curl >/dev/null 2>&1 || packages+=(curl)
if [ ! -e /etc/ssl/certs/ca-certificates.crt ] && [ ! -e /etc/pki/tls/certs/ca-bundle.crt ]; then
	packages+=(ca-certificates)
fi
if [ "$RELAY_NGINX" -eq 1 ] && ! command -v nginx >/dev/null 2>&1; then
	packages+=(nginx)
fi
if [ ${#packages[@]} -gt 0 ]; then
	log "安装依赖: ${packages[*]}"
	install_packages "${packages[@]}"
fi

case "$(uname -m)" in
x86_64|amd64) RELAY_ARCH=amd64 ;;
aarch64|arm64) RELAY_ARCH=arm64 ;;
*) die "不支持的架构: $(uname -m)（可用 --binary 指定 relayd）" ;;
esac

if ! id -u "$RELAY_USER" >/dev/null 2>&1; then
	nologin=$(command -v nologin || echo /bin/false)
	useradd --system --home "${RELAY_DATA%/*}" --shell "$nologin" "$RELAY_USER"
	log "创建服务用户 $RELAY_USER"
fi
install -d -o root -g root -m 0755 "$RELAY_HOME"
install -d -o root -g root -m 0755 /etc/chatgo-relay
data_dir=$(dirname "$RELAY_DATA")
install -d -o "$RELAY_USER" -g "$RELAY_USER" -m 0750 "$data_dir"
if [ -f "$RELAY_DATA" ]; then
	chown "$RELAY_USER:$RELAY_USER" "$RELAY_DATA"
	chmod 0600 "$RELAY_DATA"
fi

TMPDIR_INSTALL=$(mktemp -d)
download() {
	url=$1
	out=$2
	log "下载 $url"
	curl -fL --retry 3 --retry-delay 2 --connect-timeout 20 --max-time 300 -o "$out" "$url"
}

# build_from_source 构建 relayd：用于 Release 尚未附带 relayd 资产时。
build_from_source() {
	command -v go >/dev/null 2>&1 || return 1
	archive="$TMPDIR_INSTALL/source.tar.gz"
	src="$TMPDIR_INSTALL/source"
	mkdir -p "$src"
	if [ "$RELAY_VERSION" = "latest" ]; then
		source_url="https://github.com/${GITHUB_REPO}/archive/refs/heads/main.tar.gz"
	else
		source_url="https://github.com/${GITHUB_REPO}/archive/refs/tags/${RELAY_VERSION}.tar.gz"
	fi
	log "Release 中没有 relayd 二进制，改用源码构建（$source_url）"
	download "$source_url" "$archive" || return 1
	tar -xzf "$archive" -C "$src" || return 1
	gomod=$(find "$src" -name go.mod -type f | head -1)
	[ -n "$gomod" ] || return 1
	(cd "$(dirname "$gomod")" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$relayd_tmp" ./cmd/relayd)
}

relayd_tmp="$TMPDIR_INSTALL/relayd"
if [ -n "$RELAY_BINARY" ]; then
	[ -f "$RELAY_BINARY" ] || die "找不到 --binary 文件: $RELAY_BINARY"
	cp "$RELAY_BINARY" "$relayd_tmp"
elif [ -n "$RELAY_BINARY_URL" ]; then
	download "$RELAY_BINARY_URL" "$relayd_tmp" ||
		die "下载失败: $RELAY_BINARY_URL"
else
	if [ "$RELAY_VERSION" = "latest" ]; then
		relayd_url="https://github.com/${GITHUB_REPO}/releases/latest/download/relayd-linux-${RELAY_ARCH}"
	else
		relayd_url="https://github.com/${GITHUB_REPO}/releases/download/${RELAY_VERSION}/relayd-linux-${RELAY_ARCH}"
	fi
	if ! download "$relayd_url" "$relayd_tmp"; then
		build_from_source ||
			die "没有可用的 relayd Release，也没有 Go 工具链；请加 --binary /path/to/relayd 或 --binary-url URL"
	fi
fi
[ -s "$relayd_tmp" ] || die "relayd 二进制为空"
install -m 0755 -o root -g root "$relayd_tmp" "$RELAY_HOME/relayd"
log "安装 relayd 到 $RELAY_HOME/relayd"

# 推送凭据放进 /etc/chatgo-relay，避免引用 root 家目录里的文件。
install_secret() {
	src=$1
	dst=$2
	[ -n "$src" ] || return 0
	install -m 0640 -o root -g "$RELAY_USER" "$src" "$dst"
}
if [ -n "$APNS_KEY" ]; then
	install_secret "$APNS_KEY" "/etc/chatgo-relay/apns.p8"
fi
if [ -n "$FCM_ACCOUNT" ]; then
	install_secret "$FCM_ACCOUNT" "/etc/chatgo-relay/fcm.json"
fi

if [ -z "$RELAY_PUBLIC_URL" ]; then
	if [ -n "$RELAY_DOMAIN" ]; then
		scheme=http
		[ -n "$RELAY_TLS_CERT" ] && scheme=https
		if [ "$RELAY_PORT" = "443" ]; then
			RELAY_PUBLIC_URL="${scheme}://${RELAY_DOMAIN}"
		else
			RELAY_PUBLIC_URL="${scheme}://${RELAY_DOMAIN}:${RELAY_PORT}"
		fi
	else
		public_ip=$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)
		if [ -n "$public_ip" ]; then
			scheme=http
			[ -n "$RELAY_TLS_CERT" ] && scheme=https
			RELAY_PUBLIC_URL="${scheme}://${public_ip}:${RELAY_PORT}"
		fi
	fi
fi

# 服务参数。
relay_flags=(-addr "$RELAY_ADDR" -data "$RELAY_DATA")
[ -n "$RELAY_PUBLIC_URL" ] && relay_flags+=(-public-url "$RELAY_PUBLIC_URL")
if [ "$RELAY_NGINX" -eq 0 ] && [ -n "$RELAY_TLS_CERT" ]; then
	relay_flags+=(-tls-cert "$RELAY_TLS_CERT" -tls-key "$RELAY_TLS_KEY")
fi
if [ -n "$APNS_KEY" ]; then
	relay_flags+=(
		-apns-key /etc/chatgo-relay/apns.p8
		-apns-key-id "$APNS_KEY_ID"
		-apns-team-id "$APNS_TEAM_ID"
		-apns-topic "$APNS_TOPIC"
		-apns-env "$APNS_ENV"
	)
fi
[ -n "$FCM_ACCOUNT" ] && relay_flags+=(-fcm-service-account /etc/chatgo-relay/fcm.json)

quote_arg() {
	printf '"%s"' "$(printf '%s' "$1" | sed 's/[\\"]/\\&/g')"
}
exec_start="$RELAY_HOME/relayd"
for arg in "${relay_flags[@]}"; do
	exec_start+=" $(quote_arg "$arg")"
done

log "写入 systemd 服务 $UNIT"
cat > "$UNIT" <<UNIT_EOF
[Unit]
Description=ChatGo Relay
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$RELAY_USER
Group=$RELAY_USER
ExecStart=$exec_start
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$(dirname "$RELAY_DATA")

[Install]
WantedBy=multi-user.target
UNIT_EOF

systemctl daemon-reload
systemctl enable chatgo-relay.service >/dev/null 2>&1 || true
systemctl restart chatgo-relay.service

if [ "$RELAY_NGINX" -eq 1 ]; then
	command -v nginx >/dev/null 2>&1 || die "nginx 未安装"
	proxy_addr="$RELAY_ADDR"
	case "$proxy_addr" in
	0.0.0.0:*) proxy_addr="127.0.0.1:${proxy_addr#*:}" ;;
	"[::]:"*) proxy_addr="127.0.0.1:${proxy_addr#*:}" ;;
	esac
	if [ -f "$NGINX_CONF" ]; then
		BACKUP_NGINX="$NGINX_CONF.bak.$(date +%s)"
		cp "$NGINX_CONF" "$BACKUP_NGINX"
	fi

	log "写入 nginx 配置 $NGINX_CONF"
	{
		cat <<NGINX_HEAD
map \$http_upgrade \$chatgo_connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen ${RELAY_PORT}$([ -n "$RELAY_TLS_CERT" ] && printf ' ssl');
    server_name ${RELAY_DOMAIN:-_};
    client_max_body_size 16m;
NGINX_HEAD
		if [ -n "$RELAY_TLS_CERT" ]; then
			cat <<NGINX_TLS
    ssl_certificate ${RELAY_TLS_CERT};
    ssl_certificate_key ${RELAY_TLS_KEY};
    ssl_protocols TLSv1.2 TLSv1.3;
NGINX_TLS
		fi
		cat <<NGINX_BODY

    location / {
        proxy_pass http://${proxy_addr};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$chatgo_connection_upgrade;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering off;
    }
}
NGINX_BODY
	} > "$NGINX_CONF"

	if ! nginx -t >/dev/null 2>&1; then
		nginx -t || true
		if [ -n "$BACKUP_NGINX" ]; then
			mv "$BACKUP_NGINX" "$NGINX_CONF"
			warn "nginx 配置无效，已恢复原配置"
		else
			rm -f "$NGINX_CONF"
		fi
		die "nginx 配置检查失败，请检查端口 $RELAY_PORT 是否被其他站点占用"
	fi
	systemctl enable nginx >/dev/null 2>&1 || true
	systemctl reload nginx 2>/dev/null || systemctl restart nginx
fi

healthy=0
health_scheme=http
health_curl=(-fsS)
if [ "$RELAY_NGINX" -eq 0 ] && [ -n "$RELAY_TLS_CERT" ]; then
	health_scheme=https
	health_curl+=(-k)
fi
for _ in $(seq 1 30); do
	if curl "${health_curl[@]}" --max-time 2 "${health_scheme}://127.0.0.1:${RELAY_ADDR##*:}/healthz" >/dev/null 2>&1; then
		healthy=1
		break
	fi
	sleep 1
done
if [ "$healthy" -ne 1 ]; then
	systemctl status chatgo-relay.service --no-pager || true
	die "relayd 未通过健康检查，查看 journalctl -u chatgo-relay -n 50"
fi

if [ "$RELAY_NGINX" -eq 1 ]; then
	if [ -n "$RELAY_TLS_CERT" ]; then
		curl -kfsS --max-time 5 "https://127.0.0.1:${RELAY_PORT}/healthz" >/dev/null 2>&1 || true
	else
		curl -fsS --max-time 5 -H "Host: ${RELAY_DOMAIN:-127.0.0.1}" \
			"http://127.0.0.1:${RELAY_PORT}/healthz" >/dev/null 2>&1 || true
	fi
fi

cat <<DONE

ChatGo 中继已安装并启动。

  服务      chatgo-relay.service
  二进制    $RELAY_HOME/relayd
  数据库    $RELAY_DATA
  内部地址  http://$RELAY_ADDR
  对外地址  ${RELAY_PUBLIC_URL:-（未设置，请用 --public-url 指定）}

下一步：
  1. 在桌面 ChatGo 的 设置 → 手机 中启用中继，填写：
       ${RELAY_PUBLIC_URL:-wss://你的域名或地址}
  2. 打开「允许新设备配对」，生成配对码，用手机扫码。
  3. 如果服务器有防火墙，放行 TCP $RELAY_PORT。

TLS：生产环境请在 nginx 上配置证书（--tls-cert/--tls-key + --domain）。
查看日志： journalctl -u chatgo-relay -f
DONE
