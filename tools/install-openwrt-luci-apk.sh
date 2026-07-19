#!/bin/sh
set -eu

PACKAGE=${1:-/tmp/luci-app-iptv-refresh-0.1.0-r18.apk}
I18N_PACKAGE=/tmp/luci-i18n-iptv-refresh-zh-cn-0.1.0-r18.apk
MODE=
case "${2:-}" in
	--no-reload) MODE=--no-reload ;;
	"") ;;
	*) I18N_PACKAGE=$2; MODE=${3:-} ;;
esac
EXPECTED_RELEASE=25.12.5
EXPECTED_ARCH=x86_64

[ "$#" -le 3 ] || {
	echo "Usage: $0 [luci-package.apk] [i18n-package.apk] [--no-reload]" >&2
	exit 2
}
case "$MODE" in
	"") ;;
	--no-reload) ;;
	*) echo "Usage: $0 [luci-package.apk] [i18n-package.apk] [--no-reload]" >&2; exit 2 ;;
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
[ -r "$I18N_PACKAGE" ] || {
	echo "ERROR: i18n package is not readable: $I18N_PACKAGE" >&2
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
checksum_for() {
	package_name="$(basename "$1")"
	awk -v wanted="$package_name" '{ name=$2; sub(/^\.\//, "", name); if (name == wanted) { print $1; exit } }' "$SUMS_FILE"
}

expected_sha256="$(checksum_for "$PACKAGE")"
[ -n "$expected_sha256" ] || {
	echo "ERROR: package is missing from $SUMS_FILE: $PACKAGE" >&2
	exit 1
}
actual_sha256="$(sha256sum "$PACKAGE" | awk '{print $1}')"
[ "$actual_sha256" = "$expected_sha256" ] || {
	echo "ERROR: package SHA256 mismatch" >&2
	echo "expected: $expected_sha256" >&2
	echo "actual:   $actual_sha256" >&2
	exit 1
}
unset actual_sha256 expected_sha256

expected_i18n_sha256="$(checksum_for "$I18N_PACKAGE")"
[ -n "$expected_i18n_sha256" ] || {
	echo "ERROR: package is missing from $SUMS_FILE: $I18N_PACKAGE" >&2
	exit 1
}
actual_i18n_sha256="$(sha256sum "$I18N_PACKAGE" | awk '{print $1}')"
[ "$actual_i18n_sha256" = "$expected_i18n_sha256" ] || {
	echo "ERROR: i18n package SHA256 mismatch" >&2
	echo "expected: $expected_i18n_sha256" >&2
	echo "actual:   $actual_i18n_sha256" >&2
	exit 1
}
unset actual_i18n_sha256 expected_i18n_sha256 package_name SUMS_FILE

apk info -e iptv-refresh >/dev/null 2>&1 || {
	echo "ERROR: install iptv-refresh-0.1.0-r18.apk first" >&2
	exit 1
}

apk add --allow-untrusted "$PACKAGE" "$I18N_PACKAGE"

apk info -e luci-app-iptv-refresh >/dev/null 2>&1 || {
	echo "ERROR: LuCI package was not registered after installation" >&2
	exit 1
}
apk info -e luci-i18n-iptv-refresh-zh-cn >/dev/null 2>&1 || {
	echo "ERROR: Simplified Chinese package was not registered after installation" >&2
	exit 1
}
[ -x /usr/libexec/iptv-refresh-luci ]
[ -x /usr/libexec/iptv-refresh-luci-action ]
[ -r /usr/share/luci/menu.d/luci-app-iptv-refresh.json ]
[ -r /usr/share/rpcd/acl.d/luci-app-iptv-refresh.json ]
[ -r /www/luci-static/resources/view/iptv-refresh/overview.js ]
[ -r /www/luci-static/resources/view/iptv-refresh/settings.js ]
[ -r /www/luci-static/resources/view/iptv-refresh/environment.js ]
[ -s /usr/lib/lua/luci/i18n/iptv-refresh.zh-cn.lmo ]

rm -f /tmp/luci-indexcache
if [ "$MODE" = "--no-reload" ]; then
	echo "LuCI and Simplified Chinese installation verified; rpcd/uhttpd reload skipped."
	exit 0
fi
/etc/init.d/rpcd restart
/etc/init.d/uhttpd reload >/dev/null 2>&1 || true

echo "LuCI and Simplified Chinese installation verified. Open Services -> IPTV Refresh."
