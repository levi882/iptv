#!/bin/sh
set -eu

GO_VERSION_MAJOR_MINOR=1.26
GO_VERSION_PATCH=5
GO_SOURCE_SHA256=495be4bc87176ac567392e5b4116abd98466d33d7b49d41e764ccc6976b2dc42

if [ "$#" -ne 1 ]; then
	echo "Usage: $0 /path/to/openwrt-sdk" >&2
	exit 2
fi

SDK="$1"
GO_MAKEFILE="$SDK/feeds/packages/lang/golang/golang1.26/Makefile"

[ -f "$SDK/rules.mk" ] || {
	echo "ERROR: not an OpenWrt SDK: $SDK" >&2
	exit 1
}
[ -f "$GO_MAKEFILE" ] || {
	echo "ERROR: Go 1.26 feed is missing: $GO_MAKEFILE" >&2
	exit 1
}

grep -q "^GO_VERSION_MAJOR_MINOR:=$GO_VERSION_MAJOR_MINOR$" "$GO_MAKEFILE" || {
	echo "ERROR: unsupported OpenWrt Go feed version" >&2
	exit 1
}

if grep -q "^GO_VERSION_PATCH:=$GO_VERSION_PATCH$" "$GO_MAKEFILE" && \
	grep -q "^PKG_HASH:=$GO_SOURCE_SHA256$" "$GO_MAKEFILE"; then
	echo "OpenWrt host toolchain is already pinned to Go $GO_VERSION_MAJOR_MINOR.$GO_VERSION_PATCH"
	exit 0
fi

sed -i \
	-e "s/^GO_VERSION_PATCH:=.*/GO_VERSION_PATCH:=$GO_VERSION_PATCH/" \
	-e "s/^PKG_HASH:=.*/PKG_HASH:=$GO_SOURCE_SHA256/" \
	"$GO_MAKEFILE"

grep -q "^GO_VERSION_PATCH:=$GO_VERSION_PATCH$" "$GO_MAKEFILE"
grep -q "^PKG_HASH:=$GO_SOURCE_SHA256$" "$GO_MAKEFILE"

echo "Pinned OpenWrt host toolchain to Go $GO_VERSION_MAJOR_MINOR.$GO_VERSION_PATCH"
