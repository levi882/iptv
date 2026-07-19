#!/bin/sh
set -eu

PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/mingw64/bin
export PATH

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM

sh -n "$ROOT/openwrt/files/iptv-refresh.init"
sh -n "$ROOT/openwrt/files/iptv-refresh-nginx-config"
sh -n "$ROOT/openwrt/files/install-bundle.sh"
sh -n "$ROOT/tools/install-openwrt-apk.sh"
sh -n "$ROOT/tools/install-openwrt-luci-apk.sh"
sh -n "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci"
sh -n "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- '--provider-iface "$provider_iface"' "$ROOT/openwrt/files/iptv-refresh.init"
grep -q -- '--log-max-size "$log_max_size"' "$ROOT/openwrt/files/iptv-refresh.init"
grep -q -- '--ha-webhook-timeout "$ha_webhook_timeout"' "$ROOT/openwrt/files/iptv-refresh.init"
grep -q -- 'IPTV_REFRESH_HA_WEBHOOK_URL="$ha_webhook_url"' "$ROOT/openwrt/files/iptv-refresh.init"
grep -Fq -- 'cp "$ROOT/go.mod" "$ROOT/go.sum" "$PACKAGE_DIR/src/"' "$ROOT/tools/build-openwrt-package.sh"
grep -q -- 'iptv-refresh-nginx-config' "$ROOT/openwrt/files/install-bundle.sh"
grep -q -- 'iptv-refresh-nginx-config' "$ROOT/tools/build-openwrt-bundle.ps1"
grep -q -- 'IPKG_INSTROOT="${IPKG_INSTROOT:-}"' "$ROOT/openwrt/files/iptv-refresh-nginx-config"
grep -q -- 'clear-log)' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- 'set-log-max-size)' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- 'set-log-max-size NUMBER K|M>' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- 'between 1 KB and 100 MB' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- 'tail -n 200 "$log_file"' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci"

nginx_helper="$ROOT/openwrt/files/iptv-refresh-nginx-config"
auth="$(sh "$nginx_helper" render-auth 0123456789abcdef)"
[ "$auth" = 'proxy_set_header Authorization "Bearer 0123456789abcdef";' ] || {
	echo "Unexpected nginx Authorization header" >&2
	exit 1
}
if sh "$nginx_helper" render-auth 'bad"token' >/dev/null 2>&1; then
	echo "Unsafe nginx token was accepted" >&2
	exit 1
fi

locations="$TEST_DIR/iptv-refresh.locations"
sh "$nginx_helper" render-locations 127.0.0.1 9100 10.1.1.50 2001:db8::/64 > "$locations"
[ "$(grep -Fc 'allow 10.1.1.50;' "$locations")" -eq 2 ]
[ "$(grep -Fc 'allow 2001:db8::/64;' "$locations")" -eq 2 ]
grep -Fq 'include /etc/iptv-refresh/nginx.d/*.conf;' "$locations"
grep -Fq 'proxy_method POST;' "$locations"
grep -Fq 'proxy_pass_request_body off;' "$locations"
grep -Fq 'proxy_pass http://127.0.0.1:9100/refresh?;' "$locations"
if grep -Eq '\$is_args|\$args|iface=' "$locations"; then
	echo "Generated nginx route preserves legacy query parameters" >&2
	exit 1
fi
if sh "$nginx_helper" render-locations '127.0.0.1;return' 9100 >/dev/null 2>&1; then
	echo "Unsafe nginx upstream address was accepted" >&2
	exit 1
fi
if sh "$nginx_helper" render-locations 127.0.0.1 9100 '10.1.1.0/24;' >/dev/null 2>&1; then
	echo "Unsafe nginx allow address was accepted" >&2
	exit 1
fi

for helper in iptv-refresh-luci iptv-refresh-luci-action; do
	mode="$(git -C "$ROOT" ls-files -s -- "luci-app-iptv-refresh/root/usr/libexec/$helper" | awk '{print $1}')"
	[ "$mode" = 100755 ] || {
		echo "LuCI helper is not executable in Git: $helper ($mode)" >&2
		exit 1
	}
done

. "$ROOT/openwrt/files/iptv-refresh.init"

token_file="$TEST_DIR/token"
printf '%s\n' change-me > "$token_file"
ensure_token "$token_file"
IFS= read -r token < "$token_file"

[ "${#token}" -eq 64 ] || {
	echo "Generated token length is not 64" >&2
	exit 1
}
case "$token" in
	*[!0-9a-f]*)
		echo "Generated token is not lowercase hexadecimal" >&2
		exit 1
		;;
esac
[ "$(wc -l < "$token_file")" -eq 1 ] || {
	echo "Generated token file must contain exactly one line" >&2
	exit 1
}

echo "OpenWrt shell checks passed"
