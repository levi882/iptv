# Security Policy

## Supported versions

Security fixes are applied to the latest release and the default branch. Users
should upgrade before reporting an issue that is already fixed there.

## Reporting a vulnerability

Do not open a public issue containing subscriber credentials, API tokens,
Home Assistant webhook IDs, raw packet captures, provider responses, or router
configuration backups.

Report security issues privately through the repository's GitHub Security
Advisory form:

https://github.com/levi882/iptv/security/advisories/new

Include the affected version, impact, reproduction steps, and a redacted log.
Use synthetic values wherever possible.

If a secret was exposed, do not wait for a code fix before rotating it:

- regenerate the router API token;
- replace the Home Assistant webhook ID;
- delete captured traffic and configuration backups from shared locations;
- recapture IPTV credentials, or contact the provider if they cannot be
  invalidated locally.

## Deployment expectations

The backend should remain bound to loopback unless the operator intentionally
configures another protected interface. The nginx compatibility route should
allow only the exact Home Assistant address, not an entire LAN, when a static
address is available.
