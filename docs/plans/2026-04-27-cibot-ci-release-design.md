# Cibot CI/Release Design

Date: 2026-04-27

## Summary

Standardize the `retromusicbox` repo family on one GitHub Actions pattern backed by a single org-wide GitHub App named `cibot`.

Repos in scope:

- `retromusicbox`
- `retromusicbox-telephony`
- `retrocableguide`
- `retrocableguide-teletext`

Each repo keeps its own workflow files. The pattern is shared by convention, not by reusable workflow indirection.

## Goals

- Replace `GITHUB_TOKEN`-driven release automation with a GitHub App identity so release actions can trigger downstream workflows reliably.
- Separate validation, versioning, and publishing into distinct workflows.
- Align all four repos early, before their pipelines drift further apart.
- Support current deliverables now and leave clean extension points for future Docker-enabled repos.
- Keep repo-specific differences small and explicit.

## Non-Goals

- Introducing shared reusable workflows.
- Designing deployment workflows beyond image publication to GHCR and release assets where needed.
- Adding signing, SBOM generation, provenance attestations, or package registries in this phase.
- Requiring Docker publication in repos that do not yet have a `Dockerfile`.

## GitHub App Model

Use one org-wide GitHub App called `cibot`.

Recommended installation scope:

- Install only on the repos that should use it.
- Start with the four repos in scope.

Recommended repository permissions:

- `Contents: Read and write`
- `Pull requests: Read and write`
- `Issues: Read and write`
- `Metadata: Read-only`

Rationale:

- `release-please` needs to create and update release PRs, tags, releases, and changelog commits.
- PR auto-merge automation needs to approve and merge Dependabot PRs.
- A GitHub App token avoids the workflow-trigger suppression that applies to many events created by `GITHUB_TOKEN`.

Per-repo Actions configuration:

- Repository variable: `CIBOT_APP_ID`
- Repository secret: `CIBOT_PRIVATE_KEY`

This keeps workflow code identical across repos while allowing installation and key rotation to be managed centrally.

## Standard Workflow Shape

Every repo should converge on these workflow files:

- `.github/workflows/ci.yml`
- `.github/workflows/release-please.yml`
- `.github/workflows/publish.yml`
- `.github/workflows/dependabot-auto-merge.yml`

### 1. `ci.yml`

Purpose:

- Validate changes on `pull_request`.
- Optionally run on `push` to `main` as a post-merge safety net.

Rules:

- No release creation.
- No publishing.
- No mutation of repository state.

Repo-specific checks stay local to each repo:

- `retromusicbox`: frontend build, `go vet`, `go build`, `go test`
- `retromusicbox-telephony`: Lua lint, XML validation, image smoke build, container health smoke test
- `retrocableguide`: `npm ci`, `npm run build`
- `retrocableguide-teletext`: syntax and import smoke tests

### 2. `release-please.yml`

Purpose:

- Run `release-please` on `push` to `main`
- Run on a daily schedule as a safety net for auto-merged dependency updates
- Optionally allow `workflow_dispatch`

Rules:

- Authenticate as `cibot` using `actions/create-github-app-token`
- Create or update the release PR
- On merge, create the tag and GitHub Release
- Do not build or publish artifacts in this workflow

Outputs are GitHub-native state:

- release PR
- git tag
- GitHub Release

This workflow is the only release-authoring workflow.

### 3. `publish.yml`

Purpose:

- Publish artifacts after a GitHub Release is published
- Support manual rebuilds via `workflow_dispatch`

Triggers:

- `release`
  - `types: [published]`
- `workflow_dispatch`
  - optional `tag_name`

Rules:

- Resolve the release tag from the published release event or manual input
- Check out the tagged commit
- Build and publish repo-specific artifacts
- Append artifact references to release notes where useful

This keeps publish behavior deterministic and tied to immutable release tags.

### 4. `dependabot-auto-merge.yml`

Purpose:

- Approve and enable auto-merge for qualifying Dependabot PRs

Rules:

- Authenticate as `cibot`, not `GITHUB_TOKEN`
- Keep dependency grouping and commit prefixes consistent with `release-please` changelog policy

## Artifact Policy By Repo

### `retromusicbox`

Publish on release:

- Multi-arch GHCR image
- Release binaries for `rmbd` and `rmbctl`
- `SHA256SUMS`

Rationale:

- The server and CLI are explicit deliverables.
- Some users will prefer raw binaries over Docker.

### `retromusicbox-telephony`

Publish on release:

- Multi-arch GHCR image

Rationale:

- The container is the deployment artifact.
- Loose binary assets do not add practical value here.

### `retrocableguide`

Phase 1:

- Add `publish.yml`
- Gate image publication off until a `Dockerfile` exists

Future state:

- Publish GHCR image once a `Dockerfile` lands

### `retrocableguide-teletext`

Phase 1:

- Add `publish.yml`
- Gate image publication off until a `Dockerfile` exists

Future state:

- Publish GHCR image once a `Dockerfile` lands

## Docker Publication Gating

For repos that do not yet have a `Dockerfile`, `publish.yml` should still exist and succeed cleanly.

Recommended behavior:

- Early job checks for `Dockerfile`
- If absent, log that image publication is not enabled for the repo yet
- Exit successfully without attempting Docker login or build steps

Benefits:

- The workflow shape is already aligned across all repos
- Adding a `Dockerfile` later becomes an enablement change, not a workflow redesign

## Release Note Behavior

Recommended release note augmentation:

- Add the GHCR image reference to releases that publish an image
- For `retromusicbox`, add a short asset summary referencing binary downloads and checksums

This should happen in `publish.yml`, after successful publication, so release notes reflect the real published outputs.

## Migration Plan

1. Create and install the `cibot` GitHub App with the agreed permissions.
2. Add `CIBOT_APP_ID` and `CIBOT_PRIVATE_KEY` to each repo.
3. Refactor each repo to the standard workflow shape.
4. Move any build-and-publish logic out of `release-please.yml` into `publish.yml`.
5. Update Dependabot auto-merge to use `cibot`.
6. Add release asset publication for `retromusicbox`.
7. Add gated `publish.yml` skeletons for the two cableguide repos.
8. Validate with manual dispatch before relying on the next production release.

## Risks And Mitigations

### Release duplication or accidental republish

Mitigation:

- Drive publication from the release tag.
- Support explicit manual rebuild input by tag.
- Keep publish logic idempotent where practical.

### App misconfiguration blocks releases

Mitigation:

- Keep the required permissions minimal and documented.
- Validate token creation in each repo with a simple workflow run before cutting releases.

### Drift between repos over time

Mitigation:

- Keep workflow names, trigger shapes, and env variable names identical where possible.
- Treat one repo's change as a family pattern change unless there is a strong repo-specific reason not to.

### Repos without Dockerfiles look "unfinished"

Mitigation:

- Make the gating explicit in workflow logs.
- Document that the workflow is intentionally pre-wired for future Docker support.

## Decision Summary

Approved design decisions:

- One org-wide GitHub App named `cibot`
- Per-repo workflows, no reusable workflow abstraction
- Standard workflow split: `ci.yml`, `release-please.yml`, `publish.yml`
- All four repos aligned immediately
- `retromusicbox` publishes image plus binaries and checksums
- `retromusicbox-telephony` publishes image only
- `retrocableguide` and `retrocableguide-teletext` get gated publish workflows now and enable GHCR once each repo has a `Dockerfile`
