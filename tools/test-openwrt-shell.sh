#!/bin/sh
set -eu

PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/mingw64/bin
export PATH

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM

sh -n "$ROOT/openwrt/files/iptv-refresh.init"
sh -n "$ROOT/openwrt/files/install-bundle.sh"
sh -n "$ROOT/tools/install-openwrt-apk.sh"
sh -n "$ROOT/tools/install-openwrt-luci-apk.sh"
sh -n "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci"
sh -n "$ROOT/luci-app-iptv-refresh/root/usr/libexec/iptv-refresh-luci-action"
grep -q -- '--provider-iface "$provider_iface"' "$ROOT/openwrt/files/iptv-refresh.init"

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
