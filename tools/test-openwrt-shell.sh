#!/bin/sh
set -eu

PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/mingw64/bin
export PATH

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM

sh -n "$ROOT/openwrt/files/iptv-refresh.init"
sh -n "$ROOT/openwrt/files/iptv-refresh-nginx-config"
sh -n "$ROOT/openwrt/files/iptv-refresh-scheduler"
sh -n "$ROOT/openwrt/files/install-bundle.sh"
sh -n "$ROOT/tools/install-openwrt-apk.sh"
sh -n "$ROOT/tools/install-openwrt-luci-apk.sh"
sh -n "$ROOT/tools/check-public-repo.sh"
sh -n "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci"
sh -n "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- '--provider-iface "$provider_iface"' "$ROOT/openwrt/files/iptv-refresh.init"
grep -q -- '--log-max-size "$log_max_size"' "$ROOT/openwrt/files/iptv-refresh.init"
grep -q -- '--ha-webhook-timeout "$ha_webhook_timeout"' "$ROOT/openwrt/files/iptv-refresh.init"
grep -q -- 'IPTV_REFRESH_HA_WEBHOOK_URL="$ha_webhook_url"' "$ROOT/openwrt/files/iptv-refresh.init"
grep -q -- 'iptv-refresh-scheduler sync' "$ROOT/openwrt/files/iptv-refresh.init"
grep -Fq -- 'cp "$ROOT/go.mod" "$ROOT/go.sum" "$PACKAGE_DIR/src/"' "$ROOT/tools/build-openwrt-package.sh"
grep -Fq -- 'cp "$ROOT/LICENSE" "$PACKAGE_DIR/src/LICENSE"' "$ROOT/tools/build-openwrt-package.sh"
grep -Fq -- 'cp "$ROOT/LICENSE" "$LUCI_PACKAGE_DIR/LICENSE"' "$ROOT/tools/build-openwrt-package.sh"
grep -q -- 'iptv-refresh-nginx-config' "$ROOT/openwrt/files/install-bundle.sh"
grep -q -- 'iptv-refresh-scheduler' "$ROOT/openwrt/files/install-bundle.sh"
grep -q -- 'iptv-refresh-nginx-config' "$ROOT/tools/build-openwrt-bundle.ps1"
grep -q -- 'iptv-refresh-scheduler' "$ROOT/tools/build-openwrt-bundle.ps1"
grep -q -- 'iptv-refresh-scheduler' "$ROOT/openwrt/Makefile"
grep -q -- 'openwrt\\files\\provider.env' "$ROOT/tools/build-openwrt-bundle.ps1"
grep -q -- "option env_file '/etc/iptv-refresh/provider.env'" "$ROOT/openwrt/files/iptv-refresh.uci"
grep -q -- "option iface 'any'" "$ROOT/openwrt/files/iptv-refresh.uci"
grep -q -- "option provider_iface 'none'" "$ROOT/openwrt/files/iptv-refresh.uci"
grep -q -- "option capture_schedule_enabled '0'" "$ROOT/openwrt/files/iptv-refresh.uci"
grep -q -- "option capture_schedule '30 7 \* \* \*'" "$ROOT/openwrt/files/iptv-refresh.uci"
grep -q -- '^PROVIDER_TOKEN_SERVER=auto$' "$ROOT/openwrt/files/provider.env"
grep -q -- 'IPKG_INSTROOT="${IPKG_INSTROOT:-}"' "$ROOT/openwrt/files/iptv-refresh-nginx-config"
grep -q -- '^DEFAULT_ALLOW=127\.0\.0\.1$' "$ROOT/openwrt/files/iptv-refresh-nginx-config"
grep -q -- 'clear-log)' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- 'set-log-max-size)' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- 'set-log-max-size NUMBER K|M>' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- 'between 1 KB and 100 MB' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- '/etc/iptv-refresh/provider.env' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- 'tail -n 200 "$log_file"' "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci"

scheduler="$ROOT/openwrt/files/iptv-refresh-scheduler"
sh "$scheduler" validate '30 7 * * *'
sh "$scheduler" validate '*/15 0-23/2 1,15 * 1-5'
if sh "$scheduler" validate '60 7 * * *' >/dev/null 2>&1; then
	echo "Out-of-range cron expression was accepted" >&2
	exit 1
fi
if sh "$scheduler" validate '30 7 * * *;reboot' >/dev/null 2>&1; then
	echo "Unsafe cron expression was accepted" >&2
	exit 1
fi

functions_fixture="$TEST_DIR/functions.sh"
printf '%s\n' \
	'config_load() { :; }' \
	'config_get_bool() {' \
	' case "$1" in' \
	'  service_enabled) service_enabled="${TEST_SERVICE_ENABLED:-1}" ;;' \
	'  schedule_enabled) schedule_enabled="${TEST_SCHEDULE_ENABLED:-0}" ;;' \
	' esac' \
	'}' \
	'config_get() {' \
	' case "$1" in' \
	'  schedule_expression) schedule_expression="${TEST_SCHEDULE_EXPRESSION:-30 7 * * *}" ;;' \
	' esac' \
	'}' > "$functions_fixture"
cron_fixture="$TEST_DIR/root.crontab"
printf '%s\n' '5 6 * * * /usr/bin/example' > "$cron_fixture"
export IPTV_REFRESH_FUNCTIONS_FILE="$functions_fixture"
export IPTV_REFRESH_CRONTAB_FILE="$cron_fixture"
export IPTV_REFRESH_CRON_INIT="$TEST_DIR/missing-cron-init"
export IPTV_REFRESH_SCHEDULER_PATH=/usr/libexec/iptv-refresh-scheduler
export TEST_SERVICE_ENABLED=1
export TEST_SCHEDULE_ENABLED=1
export TEST_SCHEDULE_EXPRESSION='30 7 * * *'
sh "$scheduler" sync
grep -Fq '5 6 * * * /usr/bin/example' "$cron_fixture"
grep -Fq '30 7 * * * /usr/libexec/iptv-refresh-scheduler run >/dev/null 2>&1 # iptv-refresh scheduled capture' "$cron_fixture"
export TEST_SCHEDULE_EXPRESSION='60 7 * * *'
if sh "$scheduler" sync >/dev/null 2>&1; then
	echo "Invalid configured schedule was accepted" >&2
	exit 1
fi
if grep -Fq '# iptv-refresh scheduled capture' "$cron_fixture"; then
	echo "Invalid configured schedule remained in crontab" >&2
	exit 1
fi
export TEST_SCHEDULE_EXPRESSION='30 7 * * *'
export TEST_SCHEDULE_ENABLED=0
sh "$scheduler" sync
grep -Fq '5 6 * * * /usr/bin/example' "$cron_fixture"
if grep -Fq '# iptv-refresh scheduled capture' "$cron_fixture"; then
	echo "Disabled schedule remained in crontab" >&2
	exit 1
fi

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
sh "$nginx_helper" render-locations 127.0.0.1 9100 192.0.2.50 2001:db8::/64 > "$locations"
[ "$(grep -Fc 'allow 192.0.2.50;' "$locations")" -eq 2 ]
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
if sh "$nginx_helper" render-locations 127.0.0.1 9100 '192.0.2.0/24;' >/dev/null 2>&1; then
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
