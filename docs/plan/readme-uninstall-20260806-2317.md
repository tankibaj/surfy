# README Uninstall Instructions

**Status:** Done | **Owner:** main-agent | **Updated:** 2026-08-06 23:18

## Goal

Document the complete Serve Them and Homebrew removal sequence.

## Scope

- Add an uninstall section to `README.md`.
- Include runtime cleanup, Homebrew package removal, and tap removal.
- Explain how to preserve the trusted local CA.

## Affected Files

- `README.md`

## Test Strategy

- Documentation-only change: no test-first cycle required.
- Verify Markdown formatting and run existing checks before pushing directly to `main`.

## Definition of Done

- [x] Complete removal commands are documented in the correct order.
- [x] CA preservation behavior is explicit.
- [x] Existing checks pass (43/43).
