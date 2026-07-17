#!/bin/sh
set -eu

# WSL appends the Windows PATH by default. Entries such as "Program Files
# (x86)" break the unquoted PATH assignment emitted by OpenWrt's Go helper.
# SDK host tools and build prerequisites live in these standard Linux paths.
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export PATH

if [ "$#" -ne 1 ]; then
	echo "Usage: $0 /path/to/openwrt-sdk" >&2
	exit 2
fi

SDK="$1"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
PACKAGE_DIR="$SDK/package/iptv-refresh"
LUCI_PACKAGE_DIR="$SDK/package/luci-app-iptv-refresh"

case "$PACKAGE_DIR" in
	"$SDK"/package/iptv-refresh) ;;
	*) echo "Unsafe package staging path: $PACKAGE_DIR" >&2; exit 1 ;;
esac
case "$LUCI_PACKAGE_DIR" in
	"$SDK"/package/luci-app-iptv-refresh) ;;
	*) echo "Unsafe LuCI package staging path: $LUCI_PACKAGE_DIR" >&2; exit 1 ;;
esac

[ -f "$SDK/rules.mk" ] || { echo "Not an OpenWrt SDK: $SDK" >&2; exit 1; }
[ -f "$SDK/.config" ] || {
	echo "SDK .config is missing; copy the target's exact config.buildinfo to $SDK/.config" >&2
	exit 1
}
[ -f "$SDK/feeds/packages/lang/golang/golang-package.mk" ] || {
	echo "Go package feed is missing; run ./scripts/feeds update packages && ./scripts/feeds install golang" >&2
	exit 1
}
[ -f "$SDK/feeds/luci/modules/luci-base/Makefile" ] || {
	echo "LuCI feed is missing; run ./scripts/feeds update luci && ./scripts/feeds install -p luci luci-base" >&2
	exit 1
}

"$ROOT/tools/pin-openwrt-go.sh" "$SDK"

rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR/src"
cp "$ROOT/openwrt/Makefile" "$PACKAGE_DIR/Makefile"
cp -R "$ROOT/openwrt/files" "$PACKAGE_DIR/files"
cp "$ROOT/go.mod" "$PACKAGE_DIR/src/go.mod"
cp -R "$ROOT/cmd" "$PACKAGE_DIR/src/cmd"
cp -R "$ROOT/internal" "$PACKAGE_DIR/src/internal"

rm -rf "$LUCI_PACKAGE_DIR"
mkdir -p "$LUCI_PACKAGE_DIR"
cp "$ROOT/luci-app-iptv-refresh/Makefile" "$LUCI_PACKAGE_DIR/Makefile"
cp -R "$ROOT/luci-app-iptv-refresh/htdocs" "$LUCI_PACKAGE_DIR/htdocs"
cp -R "$ROOT/luci-app-iptv-refresh/po" "$LUCI_PACKAGE_DIR/po"
cp -R "$ROOT/luci-app-iptv-refresh/root" "$LUCI_PACKAGE_DIR/root"

# The SDK package index does not notice a newly added package directory solely
# from the child Makefile timestamp. Refresh the generated package metadata
# after staging so luci.mk's main and translation targets are both discovered.
rm -f "$SDK/tmp/.packageinfo" "$SDK/tmp/.packageauxvars"
rm -f "$SDK/tmp/.packagedeps" "$SDK/tmp/.packageusergroup"
rm -f "$SDK/tmp/.config-package.in"
rm -f "$SDK/tmp/info/.packageinfo-luci-app-iptv-refresh"
rm -f "$SDK/tmp/info/.files-packageinfo.mk"
rm -f "$SDK/tmp/info/.files-packageinfo.stamp"

(cd "$SDK" && ./scripts/feeds install -p luci luci-base)

make -C "$SDK" defconfig
make -C "$SDK" package/iptv-refresh/compile V=s
make -C "$SDK" package/luci-app-iptv-refresh/compile V=s
find "$SDK/bin/packages" -type f \( -name 'iptv-refresh*.apk' -o -name 'iptv-refresh*.ipk' -o -name 'luci-app-iptv-refresh*.apk' -o -name 'luci-app-iptv-refresh*.ipk' -o -name 'luci-i18n-iptv-refresh*.apk' -o -name 'luci-i18n-iptv-refresh*.ipk' \) -print
