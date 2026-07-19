#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"

fail() {
	echo "Public repository check failed: $*" >&2
	exit 1
}

[ -f LICENSE ] || fail "LICENSE is missing"
[ -f SECURITY.md ] || fail "SECURITY.md is missing"
[ -f THIRD_PARTY_NOTICES.md ] || fail "THIRD_PARTY_NOTICES.md is missing"

grep -q '^PKG_LICENSE:=Apache-2.0$' openwrt/Makefile || fail "backend package license is not Apache-2.0"
grep -q '^PKG_LICENSE:=Apache-2.0$' luci-app-iptv-refresh/Makefile || fail "LuCI package license is not Apache-2.0"
grep -q '^DEFAULT_ALLOW=127\.0\.0\.1$' openwrt/files/iptv-refresh-nginx-config || fail "nginx proxy default is not loopback-only"
grep -q "option iface 'any'" openwrt/files/iptv-refresh.uci || fail "capture interface default is deployment-specific"
grep -q '^PROVIDER_TOKEN_SERVER=auto$' openwrt/files/provider.env || fail "provider endpoint discovery is not automatic"

public_files='README.md CONTRIBUTING.md SECURITY.md THIRD_PARTY_NOTICES.md openwrt/files/provider.env luci-app-iptv-refresh/htdocs/luci-static/resources/view/iptv-refresh/settings.js luci-app-iptv-refresh/po/zh_Hans/iptv-refresh.po'
if grep -n -i -E 'Hubei|湖北|eth3\.3927|121\.60\.255\.|10\.1\.1\.|/mnt/sda1/iptv|hb\.env|HB_[A-Z]' $public_files; then
	fail "deployment-specific names or addresses remain in public documentation/defaults"
fi

token="$(tr -d '\r\n' < openwrt/files/token)"
[ "$token" = change-me ] || fail "tracked API token is not the placeholder"

forbidden_paths="$(git ls-files | grep -E '(^|/)(logs_[^/]*|action-dist|output|config/local)(/|$)|\.creds\.env($|\.)|\.(pcap|pcapng|har|cap|dump|backup)$' || true)"
[ -z "$forbidden_paths" ] || fail "sensitive/runtime paths are tracked:\n$forbidden_paths"

framesets="$(git ls-files | grep -E '(^|/)frameset_builder[^/]*\.jsp$' | grep -v '^internal/playlist/testdata/frameset_builder.jsp$' || true)"
[ -z "$framesets" ] || fail "non-synthetic provider snapshots are tracked:\n$framesets"

credential_assignments="$(
	git grep -I -n -E '^(HB|PROVIDER)_(USER_ID|USER_TOKEN|AUTHENTICATOR|STBID|STBINFO)=[^[:space:]#]+' -- . \
		| grep -v -E '^internal/redact/redact_test\.go:[0-9]+:PROVIDER_AUTHENTICATOR=abcdef$' \
		|| true
)"
if [ -n "$credential_assignments" ]; then
	printf '%s\n' "$credential_assignments" >&2
	fail "a tracked file contains an IPTV credential assignment"
fi

if git grep -I -n -E '/api/webhook/[A-Za-z0-9_-]{24,}' -- . | grep -v 'your-random-id'; then
	fail "a tracked file appears to contain a Home Assistant webhook ID"
fi

if grep -R -n -E 'uses:[[:space:]]+[^[:space:]]+@(main|master|v[0-9]+)([[:space:]]|$)' .github/workflows; then
	fail "a GitHub Action uses a mutable branch or major-version tag"
fi

echo "Public repository safety checks passed"
