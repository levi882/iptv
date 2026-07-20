#!/bin/sh
set -eu

PACKAGE=${1:-/tmp/iptv-refresh-0.1.0-r19.apk}
MODE=${2:-}
EXPECTED_RELEASE=25.12.5
EXPECTED_ARCH=x86_64
CONFIG_DIR=/etc/iptv-refresh

usage() {
	echo "Usage: $0 [package.apk] [--no-start]" >&2
}

case "$MODE" in
	"") ;;
	--no-start) ;;
	*) usage; exit 2 ;;
esac

[ -r /etc/openwrt_release ] || {
	echo "ERROR: this installer must run on OpenWrt" >&2
	exit 1
}
. /etc/openwrt_release

[ "${DISTRIB_RELEASE:-}" = "$EXPECTED_RELEASE" ] || {
	echo "ERROR: expected OpenWrt $EXPECTED_RELEASE, found ${DISTRIB_RELEASE:-unknown}" >&2
	exit 1
}
[ "${DISTRIB_ARCH:-}" = "$EXPECTED_ARCH" ] || {
	echo "ERROR: expected architecture $EXPECTED_ARCH, found ${DISTRIB_ARCH:-unknown}" >&2
	exit 1
}
[ -r "$PACKAGE" ] || {
	echo "ERROR: package is not readable: $PACKAGE" >&2
	exit 1
}
command -v apk >/dev/null 2>&1 || {
	echo "ERROR: apk package manager not found" >&2
	exit 1
}

SUMS_FILE="$(dirname "$PACKAGE")/SHA256SUMS"
[ -r "$SUMS_FILE" ] || {
	echo "ERROR: checksum manifest is not readable: $SUMS_FILE" >&2
	exit 1
}
package_name="$(basename "$PACKAGE")"
expected_sha256="$(awk -v wanted="$package_name" '{ name=$2; sub(/^\.\//, "", name); if (name == wanted) { print $1; exit } }' "$SUMS_FILE")"
[ -n "$expected_sha256" ] || {
	echo "ERROR: package is missing from $SUMS_FILE: $package_name" >&2
	exit 1
}
actual_sha256="$(sha256sum "$PACKAGE" | awk '{print $1}')"
[ "$actual_sha256" = "$expected_sha256" ] || {
	echo "ERROR: package SHA256 mismatch" >&2
	echo "expected: $expected_sha256" >&2
	echo "actual:   $actual_sha256" >&2
	exit 1
}

apk add --allow-untrusted "$PACKAGE"

listen_port="$(uci -q get iptv-refresh.main.listen_port 2>/dev/null || true)"
if [ "$listen_port" = "9099" ]; then
	uci set iptv-refresh.main.listen_port='9100'
	uci commit iptv-refresh
	echo "Migrated listen port from 9099 to 9100"
fi
unset listen_port

env_file="$(uci -q get iptv-refresh.main.env_file 2>/dev/null || true)"
creds_file="$(uci -q get iptv-refresh.main.creds_file 2>/dev/null || true)"
[ -n "$env_file" ] || env_file="$CONFIG_DIR/provider.env"
[ -n "$creds_file" ] || creds_file="$CONFIG_DIR/provider.creds.env"
chmod 600 "$env_file" "$CONFIG_DIR/token"
[ ! -e "$creds_file" ] || chmod 600 "$creds_file"

token="$(head -n 1 "$CONFIG_DIR/token" 2>/dev/null || true)"
[ -n "$token" ] && [ "$token" != "change-me" ] || {
	echo "ERROR: package installation did not generate an API token" >&2
	exit 1
}
unset token actual_sha256 expected_sha256 package_name SUMS_FILE

/usr/bin/iptv-refresh version

if [ "$MODE" = "--no-start" ]; then
	echo "Package installation verified; service start skipped."
	exit 0
fi

/etc/init.d/iptv-refresh enable
if /etc/init.d/iptv-refresh running >/dev/null 2>&1; then
	/etc/init.d/iptv-refresh restart
else
	/etc/init.d/iptv-refresh start
fi

listen_host="$(uci -q get iptv-refresh.main.listen_host || printf '%s' 127.0.0.1)"
listen_port="$(uci -q get iptv-refresh.main.listen_port || printf '%s' 9100)"
attempt=0
while [ "$attempt" -lt 20 ]; do
	if health="$(/usr/bin/iptv-refresh control health --host "$listen_host" --port "$listen_port" 2>/dev/null)"; then
		echo "$health"
		echo "iptv-refresh installation and health check succeeded."
		exit 0
	fi
	attempt=$((attempt + 1))
	sleep 1
done

echo "ERROR: iptv-refresh did not become healthy within 20 seconds" >&2
logread -e iptv-refresh >&2 || true
exit 1
