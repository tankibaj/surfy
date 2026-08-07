# Surfy Rebrand

**Status:** Complete | **Owner:** main-agent | **Updated:** 2026-08-07

## Goal

Apply a clean-break Surfy identity across the product, source tree, macOS services, packaging, releases, documentation, and tests.

## Breaking Surface

- The executable and command directory are named `surfy`.
- Projects use `surfy.yaml`.
- Runtime overrides use the `SURFY_` environment-variable prefix.
- State, logs, certificates, privileged helpers, and launchd labels use Surfy paths and identifiers.
- The Go module, Homebrew formula, archives, CI artifacts, and release metadata use `github.com/tankibaj/surfy` and `surfy` names.
- Version 0.2.0 does not read or migrate pre-0.2 manifests, state, services, certificates, or environment variables.

## Release Cutover

1. Have users remove any pre-0.2 installation with its original CLI so the previous edge service releases ports 80 and 443.
2. Rename the GitHub repository only after explicit approval.
3. Merge the prepared local changes after the repository rename so module and documentation URLs resolve.
4. Tag `v0.2.0`; the release workflow publishes the Surfy archive and replaces the formula's temporary `sha256 :no_check` marker.
5. Verify a clean Homebrew install, `surfy install`, `surfy doctor`, and one renamed project manifest on Apple Silicon.

## Verification

- [x] Full Go test suite passes uncached with the race detector.
- [x] `go vet ./...` passes.
- [x] Native and cross-compiled Apple Silicon builds succeed.
- [x] CLI version, help, and `surfy init` smoke tests pass.
- [x] The release archive contains only the `surfy` executable and reports version 0.2.0.
- [x] Ruby formula syntax and GitHub Actions YAML parse successfully.
- [x] Tracked source and documentation contain no stale pre-rebrand identifiers.
- [ ] GitHub repository rename completed after explicit approval.
- [ ] The v0.2.0 release and live Homebrew installation are verified.
