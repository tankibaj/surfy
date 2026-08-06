# Serve Them Agent Guide

## Architecture and invariants

- The primary release target is native `darwin/arm64` for Apple Silicon (M1 and newer); keep the core portable for later Linux support.
- `cmd/serve-them` is the only binary. The installed copy at `/Library/PrivilegedHelperTools/serve-them-edge` runs only the `edge` subcommand as a root LaunchDaemon.
- The edge binds `127.0.0.1:80` and `127.0.0.1:443`. Never broaden these listeners.
- The CA and host keys are root-only under `/Library/Application Support/serve-them/certificates`. Application processes must never receive or read the CA key.
- Unprivileged supervisors request certificates over the mode-`0600` Unix socket in the user state directory. Validate all names again in the edge process.
- Application processes run in separate process groups as the current user and bind dynamic loopback ports.
- The registry contains paths, ports, PIDs, and status only. Never persist resolved environment values.
- Registry mutations use `state.Update`; do not bypass its lock and atomic rename.

## Verification

```sh
make check
```

Use `SERVE_THEM_STATE_DIR`, `SERVE_THEM_CERT_DIR`, `SERVE_THEM_HTTP_ADDR`, and `SERVE_THEM_HTTPS_ADDR` to isolate tests. Never exercise the system trust store or LaunchDaemon from automated tests.
