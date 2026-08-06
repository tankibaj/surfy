# FastAPI Package Detection

**Status:** Done | **Owner:** main-agent | **Updated:** 2026-08-07 00:00

## Goal

Detect conventional FastAPI package layouts such as `app/main.py` and use the project-local virtual environment automatically.

## Scope

- Discover Python entry points recursively at a bounded depth while excluding virtual environments, tests, migrations, and caches.
- Convert safe Python file paths into import modules such as `app.main:app`.
- Prefer `.venv/bin/uvicorn`, then a virtual-environment Python module invocation, then system Python.
- Enable Uvicorn reload for the local development runtime.

## Affected Files

- `internal/detect/detect.go`
- `internal/detect/detect_test.go`
- `README.md`

## Test Strategy

- Add the failing package-layout/`.venv` unit test before implementation.
- Cover ambiguity and excluded-directory behavior.
- Run the complete suite separately after focused tests pass.

## Definition of Done

- [x] `app/main.py` is detected as `app.main:app`.
- [x] A local `.venv` is preferred automatically.
- [x] Unsafe or ambiguous module paths are rejected.
- [x] Full test suite passes (19/19).
