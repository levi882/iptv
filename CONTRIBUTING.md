# Contributing

Contributions are welcome under the Apache License 2.0.

Before submitting a change:

1. Use only synthetic credentials and provider responses in tests.
2. Never attach `provider.creds.env`, API tokens, Home Assistant webhook URLs, packet
   captures, router backups, or unredacted runtime logs.
3. Run `go test ./...`, `go vet ./...`, the LuCI checks in `.github/workflows/ci.yml`,
   and `sh tools/test-openwrt-shell.sh` where those runtimes are available.
4. Keep provider-specific behavior configurable and document its scope.

By intentionally submitting a contribution, you agree that it is licensed
under Apache License 2.0 as described in the repository `LICENSE` file.
