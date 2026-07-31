# IPTV Refresh

IPTV Refresh is an OpenWrt-oriented playlist refresh tool. It captures or reuses credentials from an authorized set-top box login, connects to the provider portal, and generates channel data for players on the local network.

It can:

- Generate M3U playlists with channel ordering, grouping, and name matching.
- Fetch the operator's own channel schedules, retain catch-up history, and publish a sorted XMLTV guide.
- Match and cache channel logos locally.
- Generate rtp2httpd-compatible live URLs, short rolling timeshift, and programme-level operator TVOD catch-up.
- Provide LuCI configuration, manual refresh, scheduling, and status pages.

OpenWrt backend, LuCI, and Simplified Chinese packages are available from [Releases](https://github.com/levi882/iptv/releases). Verify downloads with the published `SHA256SUMS`, then configure the service in LuCI for your own network and IPTV subscription.

Operator catch-up uses the `prevuecode` stored in the generated XMLTV to request a fresh TVOD URL only when a historical programme is selected. With `PROVIDER_CATCHUP_URL=auto`, add each playback device IP (or its trusted LAN CIDR) to LuCI's **Proxy source addresses** list so nginx can forward `/iptv/catchup` without exposing the router API token in the playlist.

## Responsible use

Use this project only with IPTV subscriptions, networks, and equipment you are authorized to access. Follow applicable laws, provider terms, and content licensing requirements. Never share account credentials, tokens, packet captures, or provider responses containing subscriber information.

This project does not provide, host, store, or sell television content or stream sources. It is not affiliated with any operator, equipment vendor, broadcaster, EPG provider, or logo provider. Users are responsible for the third-party sources they configure and for how they use this software.

## License

Code and documentation are licensed under the [Apache License 2.0](LICENSE). See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and [SECURITY.md](SECURITY.md) for third-party notices and security guidance.
