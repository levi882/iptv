# IPTV Refresh

This project provides a self-contained Go service with a native LuCI JavaScript
frontend backed by narrowly scoped rpcd ACLs. It captures STB credentials,
authenticates with the provider portal, and generates playlist, EPG, logo,
catch-up, and rtp2httpd-compatible outputs.

## Commands

```text
iptv-refresh refresh [options]  Capture credentials and regenerate outputs
iptv-refresh capture [options]  Capture only the dynamic STB credentials
iptv-refresh serve [options]    Run the local HTTP service
iptv-refresh control ACTION     Call the local service without exposing its token
iptv-refresh version            Print the build version
```

Existing `hb.env` keys and output locations remain supported. A refresh can be
tested without waiting for a new STB login when a valid credentials file
already exists:

```sh
/usr/bin/iptv-refresh refresh \
  --repo-root /mnt/sda1/iptv \
  --env-file /etc/iptv-refresh/hb.env \
  --creds-file /etc/iptv-refresh/hb.creds.env \
  --iface eth3.3927 \
  --skip-capture
```

## HTTP API

The OpenWrt service listens on `127.0.0.1:9100` by default:

```text
GET  /healthz
GET  /status
GET  /refresh?token=TOKEN&iface=eth3.3927
POST /refresh?token=TOKEN&iface=eth3.3927
POST /refresh?token=TOKEN&iface=eth3.3927&capture=1
GET  /playlist?token=TOKEN
```

`Authorization: Bearer TOKEN` can be used instead of putting the token in the
URL. Refresh runs in the background; `/status` reports its last result. The
normal `/refresh` route reuses the saved credentials, so Home Assistant and
LuCI can refresh without waiting for the STB. Add `capture=1` only when the
provider credentials have expired and the STB is powered on. A failed refresh
keeps the previously generated playlist and the last successful report.

The existing nginx routes for `/iptv/refresh` and `/iptv/healthz` continue to
work. Optional routes for status and the generated playlist are:

```nginx
location = /iptv/status {
    proxy_pass http://127.0.0.1:9100/status;
}

location = /iptv/playlist {
    proxy_pass http://127.0.0.1:9100/playlist$is_args$args;
}
```

## Build locally

The module targets Go 1.26 and recommends the Go 1.26.5 security release. Go
is needed only on the build computer, not on the router:

```sh
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -trimpath -ldflags "-s -w -X main.version=dev" \
  -o dist/iptv-refresh ./cmd/iptv-refresh
```

Use the router's real architecture. MIPS devices commonly also require
`GOMIPS=softfloat`; 32-bit ARM devices require the matching `GOARM` value.

## Build an OpenWrt package

Download the SDK that exactly matches the router release, target, and
subtarget. Copy the same target directory's `config.buildinfo` to `.config` in
the SDK and run `make defconfig`. Ensure the SDK-locked `packages` feed,
including `golang/host`, is installed. Install the SDK-locked `base` feed
definitions for `tcpdump` and `ca-certificates` so runtime dependencies are
resolved, then run:

```sh
./tools/build-openwrt-package.sh /path/to/openwrt-sdk
```

The script stages this source tree inside the SDK and builds either an `.apk`
or `.ipk`, depending on the selected OpenWrt release. The package declares
`tcpdump` and `ca-bundle` as runtime dependencies; it does not install the Go
compiler on the router. On a live router, the package replaces the placeholder
API token with a random 256-bit token during its first installation.

Before compiling, the build script pins the SDK's Go 1.26 feed to Go 1.26.5
and its official source SHA-256. This intentionally carries the latest Go
1.26 security fixes while retaining the OpenWrt release's existing Go package
integration.

For an immediate SDK-free deployment to an `x86/64` router, Windows can build
a self-contained bundle instead:

```powershell
powershell -ExecutionPolicy Bypass -File tools/build-openwrt-bundle.ps1
```

Copy the resulting `dist/iptv-refresh-*-x86_64.tar.gz` to the router, then:

```sh
tar -xzf iptv-refresh-*-x86_64.tar.gz
cd iptv-refresh-*-x86_64
sh install.sh
```

The bundle installer preserves existing UCI and `hb.env` files and generates a
local API token when one has not been configured. Go is not installed on the
router. Unlike the SDK-built `.apk`, the bundle does not resolve runtime
dependencies, so install `tcpdump` and `ca-bundle` with `apk` if they are
missing.

Install the resulting package with the package manager used by the router:

```sh
apk add --allow-untrusted ./iptv-refresh-*.apk
# or: opkg install ./iptv-refresh_*.ipk
```

The SDK build also produces `luci-app-iptv-refresh`, an architecture-independent
LuCI package, plus `luci-i18n-iptv-refresh-zh-cn` for Simplified Chinese. It
provides status polling, service controls, manual refresh,
playlist download, recent logs, UCI settings, and a structured Environment page.
The Environment page follows the tabbed configuration style used by
`luci-app-rtp2httpd`: output, rtp2httpd, EPG/logo, and provider/capture fields
use typed controls, while comments and unknown `hb.env` variables are preserved.
The raw preview masks `R2H_TOKEN`.
The LuCI browser code calls a narrowly permitted local helper; the API token is
read by the Go process and is never returned to the browser.

For the verified OpenWrt 25.12.5 `x86_64` artifacts in `dist/`, copy all three APKs
and their guarded installers to the router and run:

```powershell
scp .\dist\iptv-refresh-0.1.0-r7.apk `
  .\dist\luci-app-iptv-refresh-0.1.0-r8.apk `
  .\dist\luci-i18n-iptv-refresh-zh-cn-0.1.0-r8.apk `
  .\tools\install-openwrt-apk.sh `
  .\tools\install-openwrt-luci-apk.sh root@10.1.1.1:/tmp/
ssh root@10.1.1.1 "sh /tmp/install-openwrt-apk.sh"
ssh root@10.1.1.1 "sh /tmp/install-openwrt-luci-apk.sh"
```

The installer verifies the release, architecture, and SHA-256 before changing
the router. APK conffile handling preserves the installed configuration on
upgrades. It then enables the service and checks `/healthz`.
When upgrading from the previous default, an unchanged listen port of `9099`
is migrated to `9100`; any other custom port is preserved.
After the LuCI packages are installed, Simplified Chinese is registered as
`zh-cn`. Select it under **System -> System -> Language and Style** if LuCI is
not already using Chinese. Sign in again if the new menu is not immediately
visible, then open **Services -> IPTV Refresh**.

Before starting the service, open **Services -> IPTV Refresh -> Environment**
and check the output paths, rtp2httpd address/token, EPG, logo, and provider
settings. The same values can be edited directly in
`/etc/iptv-refresh/hb.env`. Check `/etc/config/iptv-refresh`, then enable and
start the service:

```sh
chmod 600 /etc/iptv-refresh/hb.env /etc/iptv-refresh/token
[ ! -f /etc/iptv-refresh/hb.creds.env ] || \
  chmod 600 /etc/iptv-refresh/hb.creds.env
/etc/init.d/iptv-refresh enable
/etc/init.d/iptv-refresh start
curl http://127.0.0.1:9100/healthz
test -s /etc/iptv-refresh/token && echo "API token is present"
```

## Runtime dependencies

- `tcpdump` captures the STB authentication exchange.
- `ca-bundle` validates HTTPS EPG and logo sources.
- nginx is optional for the Go process itself but remains the recommended
  reverse proxy for Home Assistant access.

## GitHub and automated builds

Do not commit router credentials or captured provider responses. The repository
`.gitignore` excludes `config/local/`, `scripts/`, runtime output, caches,
build artifacts, packet captures, and local `frameset_builder*.jsp` snapshots.
The parser tests use the synthetic fixture under
`internal/playlist/testdata/`, so CI never needs subscriber data.

Before the first push, inspect both normal and ignored files:

```sh
git status --short
git status --short --ignored
```

`.github/workflows/ci.yml` runs on every push and pull request. It checks Go
formatting, runs `go vet`, race-enabled Go tests, LuCI JavaScript tests, and
uploads a Linux x86-64 binary. It reads Go 1.26.5 from `go.mod`.

`.github/workflows/openwrt.yml` is intentionally limited to manual dispatches
and `v*` tags because an SDK build is much heavier. It downloads and verifies
the exact OpenWrt 25.12.5 x86-64 SDK, restores the release-pinned feeds, builds
the backend, LuCI app, and Simplified Chinese package, then uploads the APKs and
`SHA256SUMS` as a workflow artifact. No GitHub repository secrets are required.

If a real `frameset_builder` snapshot or `config/local/*.env` was ever
committed, adding it to `.gitignore` is not enough: remove it from Git history
before publishing and rotate the exposed IPTV/API credentials.
