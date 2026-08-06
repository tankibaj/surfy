# Logs Project Selector

**Status:** Done | **Owner:** main-agent | **Updated:** 2026-08-06 23:49

## Goal

Allow `serve-them logs <project-name>` to read logs for a registered project from any directory.
Make combined service logs human-readable and quickly scannable with stable service colors and level indicators.

## Scope

- Resolve an optional project name or project path independently from the optional `web`/`api` service selector.
- Preserve `--follow` and current-directory behavior.
- Return an actionable error for unknown projects.
- Derive stable colors from every service name, respect `NO_COLOR`, and annotate debug/info/warn/error lines.

## Affected Files

- `internal/app/app.go`
- `internal/app/app_test.go`
- `internal/logview/logview.go`
- `internal/logview/logview_test.go`
- `internal/supervisor/supervisor.go`
- `README.md`

## Test Strategy

- [x] Test project-name selection outside the project directory.
- [x] Test combined project/service selection.
- [x] Test unknown-project errors.
- [x] Test service colors, severity indicators, and plain redirected output.
- [x] Run the full Go suite.

## Definition of Done

- [x] `serve-them logs deliverydeck` reads all available logs.
- [x] `serve-them logs deliverydeck api --follow` targets API logs.
- [x] Existing current-directory syntax remains compatible.
- [x] Foreground and retained logs share the same visual language.
- [x] Tests pass (23/23).
