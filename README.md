# IPTV Refresh

IPTV Refresh is an OpenWrt-oriented playlist refresh tool. It captures or reuses credentials from an authorized set-top box login, connects to the provider portal, and generates channel data for players on the local network.

It can:

- Generate M3U playlists with channel ordering, grouping, and name matching.
- Publish XMLTV guides and switch to ordered fallback sources when the primary guide is unavailable or expired.
- Match and cache channel logos locally.
- Generate rtp2httpd-compatible live and catch-up URLs.
- Provide LuCI configuration, manual refresh, scheduling, and status pages.

OpenWrt backend, LuCI, and Simplified Chinese packages are available from [Releases](https://github.com/levi882/iptv/releases). Verify downloads with the published `SHA256SUMS`, then configure the service in LuCI for your own network and IPTV subscription.

## Responsible use

Use this project only with IPTV subscriptions, networks, and equipment you are authorized to access. Follow applicable laws, provider terms, and content licensing requirements. Never share account credentials, tokens, packet captures, or provider responses containing subscriber information.

This project does not provide, host, store, or sell television content or stream sources. It is not affiliated with any operator, equipment vendor, broadcaster, EPG provider, or logo provider. Users are responsible for the third-party sources they configure and for how they use this software.

## License

Code and documentation are licensed under the [Apache License 2.0](LICENSE). See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and [SECURITY.md](SECURITY.md) for third-party notices and security guidance.
