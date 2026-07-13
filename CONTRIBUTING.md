# Contributing

Thanks for considering a contribution.

## Development

**Requirements:** Go 1.22+, `tmux` (for integration tests).

```bash
git clone https://github.com/reloadlife/agents.git
cd agents
make test
make test-integration   # needs tmux
make build
```

Local smoke:

```bash
export AGENTSD_TOKEN=dev-token
./bin/agentsd serve --config config.example.toml
# other terminal
./bin/agentsctl --url http://127.0.0.1:8787 --token dev-token status
./bin/agentsctl --token dev-token agents
```

## Code style

- `gofmt` / `go vet` clean
- Prefer small packages under `internal/`
- No secrets in the repo; use env vars and example configs only
- No personal host IPs or production tokens in docs/examples

## License

By contributing, you agree that your contributions are licensed under the
**GNU Affero General Public License v3.0** (see [LICENSE](LICENSE)).

## Pull requests

1. Open a PR against `main`
2. Describe *why* and *how*
3. Include tests when changing path allowlist, auth, or session resolution
4. Keep scope focused (one concern per PR when possible)

## Feature ideas that fit

- Better multi-agent picker UX
- Homebrew packaging
- Optional mTLS / Tailscale identity auth
- Structured metrics / Prometheus
- Session recording (with clear privacy docs)

## Feature ideas that need design first

- Multi-user isolation
- Unrestricted remote shell
- Public internet exposure without a reverse proxy + auth
