# Serve Them

Serve Them is a dependency-free Go CLI for running local frontend and backend applications behind stable, trusted HTTPS `.localhost` URLs. The first release targets Apple Silicon Macs (M1 and newer).

```yaml
# serve.yaml
web: ./frontend
api: ./backend
```

```sh
brew tap tankibaj/server-them https://github.com/tankibaj/server-them
brew install tankibaj/server-them/serve-them
serve-them install # one macOS administrator authorization
```

The formula installs the native Apple Silicon CLI into Homebrew’s managed prefix (`/opt/homebrew/bin` through its stable symlink). For a user-local installation without Homebrew, run `make install-user` and add `~/.local/bin` to `PATH`.

For the optimized Apple Silicon release binary:

```sh
make build-apple-silicon
```

The project is then available at `https://<directory-name>.localhost`; `/` is sent to `web`, and `/api/*` is sent unchanged to `api`. The backend application owns the `/api` route.

## MVP commands

```text
serve-them                         foreground mode
serve-them start [path]            background mode
serve-them stop|restart [path]
serve-them status [path]
serve-them list [--enabled]
serve-them logs [web|api] [--follow]
serve-them open
serve-them enable|disable
serve-them doctor
serve-them certificate status|repair
serve-them install
serve-them uninstall [--remove-ca]
```

`install` is macOS-only in the first release. It creates and trusts a private local CA and installs the narrowly scoped edge service on ports 80 and 443. Application commands always run as the current, unprivileged user. `enable` uses a per-user LaunchAgent and starts the project only after login.

## Detection

- Node.js: package-manager metadata/lockfiles, then `dev` or `start`; Vite receives explicit loopback host and port flags.
- Python: Django, or an unambiguous `FastAPI()`/`Flask()` module.
- Go: `go run .`.
- Anything else: specify `run`; `${PORT}` is expanded at startup.

```yaml
api:
  path: ./backend
  run: ./server --host 127.0.0.1 --port ${PORT}
  env:
    ENV_FILE: ./backend/.env
```

Registry data lives in `~/Library/Application Support/serve-them`, and logs in `~/Library/Logs/serve-them`. The trusted root certificate is installed in the macOS System Keychain; root-only CA and host key material lives in `/Library/Application Support/serve-them/certificates`. Secret environment values are passed directly to child processes and are never written to the registry.

## Architecture

The root-owned edge helper only terminates TLS, issues local certificates through a restricted Unix socket, and proxies registered routes. Per-project supervisors allocate loopback ports and execute application commands under the invoking user. The edge reloads the user-owned registry for each request, so projects can be added and removed without restarting it.

For local non-privileged edge testing, set `SERVE_THEM_HTTP_ADDR=:8080`, `SERVE_THEM_HTTPS_ADDR=:8443`, and `SERVE_THEM_STATE_DIR` before running `serve-them edge`.
