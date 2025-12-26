# Releasing (backend service)

## Updating the changelog in PRs

- Add an entry to `CHANGELOG.md` under **`## [Unreleased]`**.
- Put it under the most appropriate section:
  - `### Added`, `### Changed`, `### Deprecated`, `### Removed`, `### Fixed`, `### Security`
- Focus on service-impacting changes: runtime behavior, deploy/ops, migrations, configuration, performance, and compatibility notes.
- If externally-visible API/behavior changes are needed, they must be specified in the **spec repo first**.

## Spec pinning policy (`spec.lock`)

This service repo must pin the spec version it implements in `spec.lock` (a spec git tag like `v1.2.3`).

- Update `spec.lock` in the same PR that updates generated code and/or behavioral implementation tied to a spec change.
- Each service release should include a changelog line: `- Implements spec \`vX.Y.Z\`` (the release script will ensure this).

## Cutting a service release

1. Ensure `spec.lock` is updated to the spec tag implemented (for example: `v1.2.3`).
2. Ensure `CHANGELOG.md` has entries under `## [Unreleased]`.
3. Cut the release section (moves Unreleased entries into a dated version section and ensures it includes the pinned spec version):

```bash
make changelog-release VERSION=x.y.z
```

4. Commit the changelog update (and `spec.lock` if it changed):

```bash
git add CHANGELOG.md spec.lock
git commit -m "chore(release): vX.Y.Z"
```

5. Create and push the git tag (this repo uses plain `vX.Y.Z` tags unless a different convention is introduced later):

```bash
git tag vX.Y.Z
git push --tags
```

## SemVer (very short)

- **MAJOR**: breaking changes
- **MINOR**: backwards-compatible features
- **PATCH**: backwards-compatible fixes


