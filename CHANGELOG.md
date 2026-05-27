# Changelog

All notable changes to Remora are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `CHANGELOG.md`, `SECURITY.md`, `LICENSE` (MIT).
- Dependabot config covering Go modules, GitHub Actions, and Docker.
- CodeQL workflow (`security-and-quality` query pack) running on push, PR, and a weekly schedule.

### Security
- Pinned every GitHub Action to a full commit SHA (with the major version in a trailing comment) so a moved tag on a third-party action cannot silently land in CI. Dependabot will keep the SHAs current.
- Replaced `aquasecurity/trivy-action@master` with a SHA-pinned version in the publish workflow.

### Changed
- Renamed `build.yaml` → `ci.yml` and `publish.yaml` → `publish.yml` to match the shared standard.

[Unreleased]: https://github.com/yachiko/remora/compare/HEAD...HEAD
