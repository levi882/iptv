#!/bin/sh
set -eu

SELF_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

if [ "$(id -u)" -ne 0 ]; then
	echo "ERROR: run this installer as root" >&2
	exit 1
fi

[ -f "$SELF_DIR/iptv-refresh" ] || {
	echo "ERROR: iptv-refresh binary is missing from $SELF_DIR" >&2
	exit 1
}
[ -f "$SELF_DIR/iptv-refresh-nginx-config" ] || {
	echo "ERROR: nginx configuration helper is missing from $SELF_DIR" >&2
	exit 1
}

was_running=0
if [ -x /etc/init.d/iptv-refresh ] && /etc/init.d/iptv-refresh running >/dev/null 2>&1; then
	was_running=1
	/etc/init.d/iptv-refresh stop
fi

mkdir -p /usr/bin /usr/libexec /etc/init.d /etc/config /etc/iptv-refresh
cp "$SELF_DIR/iptv-refresh" /usr/bin/.iptv-refresh.new
chmod 0755 /usr/bin/.iptv-refresh.new
mv -f /usr/bin/.iptv-refresh.new /usr/bin/iptv-refresh

cp "$SELF_DIR/iptv-refresh.init" /etc/init.d/.iptv-refresh.new
chmod 0755 /etc/init.d/.iptv-refresh.new
mv -f /etc/init.d/.iptv-refresh.new /etc/init.d/iptv-refresh

cp "$SELF_DIR/iptv-refresh-nginx-config" /usr/libexec/.iptv-refresh-nginx-config.new
chmod 0755 /usr/libexec/.iptv-refresh-nginx-config.new
mv -f /usr/libexec/.iptv-refresh-nginx-config.new /usr/libexec/iptv-refresh-nginx-config

if [ ! -e /etc/config/iptv-refresh ]; then
	cp "$SELF_DIR/iptv-refresh.uci" /etc/config/iptv-refresh
	chmod 0600 /etc/config/iptv-refresh
fi

env_file="$(uci -q get iptv-refresh.main.env_file 2>/dev/null || true)"
if [ "$env_file" = /etc/iptv-refresh/hb.env ]; then
	if [ -r "$env_file" ]; then
		cp -p "$env_file" /etc/iptv-refresh/provider.env
	else
		cp "$SELF_DIR/provider.env" /etc/iptv-refresh/provider.env
	fi
	sed -i \
		-e 's/^HB_/PROVIDER_/' \
		-e 's#^CREDS_FILE=/etc/iptv-refresh/hb\.creds\.env$#CREDS_FILE=/etc/iptv-refresh/provider.creds.env#' \
		/etc/iptv-refresh/provider.env
	chmod 0600 /etc/iptv-refresh/provider.env
	uci set iptv-refresh.main.env_file='/etc/iptv-refresh/provider.env'
	uci commit iptv-refresh
elif [ ! -e /etc/iptv-refresh/provider.env ]; then
	cp "$SELF_DIR/provider.env" /etc/iptv-refresh/provider.env
	chmod 0600 /etc/iptv-refresh/provider.env
fi

creds_file="$(uci -q get iptv-refresh.main.creds_file 2>/dev/null || true)"
if [ "$creds_file" = /etc/iptv-refresh/hb.creds.env ]; then
	if [ -r "$creds_file" ]; then
		cp -p "$creds_file" /etc/iptv-refresh/provider.creds.env
		sed -i 's/^HB_/PROVIDER_/' /etc/iptv-refresh/provider.creds.env
		chmod 0600 /etc/iptv-refresh/provider.creds.env
	fi
	uci set iptv-refresh.main.creds_file='/etc/iptv-refresh/provider.creds.env'
	uci commit iptv-refresh
fi

token=""
if [ -r /etc/iptv-refresh/token ]; then
	IFS= read -r token < /etc/iptv-refresh/token || true
fi
if [ -z "$token" ] || [ "$token" = "change-me" ]; then
	random_file=/etc/iptv-refresh/token.random.$$
	if ! dd if=/dev/urandom of="$random_file" bs=32 count=1 2>/dev/null; then
		rm -f "$random_file"
		echo "ERROR: unable to generate API token" >&2
		exit 1
	fi
	token="$(sha256sum "$random_file")"
	rm -f "$random_file"
	token="${token%% *}"
	[ "${#token}" -eq 64 ] || {
		echo "ERROR: generated API token has an invalid length" >&2
		exit 1
	}
	printf '%s\n' "$token" > /etc/iptv-refresh/token
fi
unset token random_file
chmod 0600 /etc/iptv-refresh/token

if [ "$was_running" -eq 1 ]; then
	/etc/init.d/iptv-refresh start
fi

echo "Installed $(/usr/bin/iptv-refresh version)"
echo "Configuration: /etc/config/iptv-refresh"
env_file="$(uci -q get iptv-refresh.main.env_file 2>/dev/null || true)"
[ -n "$env_file" ] || env_file=/etc/iptv-refresh/provider.env
echo "Environment:   $env_file"
if [ "$was_running" -eq 0 ]; then
	echo "After checking the configuration, run:"
	echo "  /etc/init.d/iptv-refresh enable"
	echo "  /etc/init.d/iptv-refresh start"
fi
