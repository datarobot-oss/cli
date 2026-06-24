# CI/CD workflow flows (post Phase 4)

Reference diagrams for the restructured `.github/workflows/`. GitHub renders the
Mermaid blocks below natively — paste any of them into a PR description or issue.

Legend: solid arrows are job `needs:` dependencies; dotted arrows are optional /
user-triggered paths; `→ _x.yaml` means the job calls that reusable workflow.

---

## 0. Entrypoints → reusable building blocks

Which entrypoint calls which `_`-prefixed reusable.

```mermaid
graph LR
  subgraph Entrypoints
    PRC["pr-checks.yaml"]
    NS["nightly-smoke.yaml"]
    SOD["smoke-on-demand.yaml"]
    FST["fork-smoke-tests.yaml"]
    MS["manual-smoke.yaml"]
    REL["release.yaml"]
  end
  subgraph Reusables
    B["_build.yaml"]
    SM["_smoke.yaml"]
    SW["_smoke-windows.yaml"]
    IT["_install-tests.yaml"]
    PRS["_pre-release-smoke.yaml"]
  end
  PRC --> SM
  NS --> B
  NS --> SM
  NS --> SW
  NS --> IT
  SOD --> B
  SOD --> SM
  SOD --> SW
  FST --> B
  FST --> SM
  FST --> SW
  MS --> SM
  MS --> IT
  REL --> IT
  REL --> PRS
```

---

## 1. Forked PR

A fork PR cannot use repo secrets, so the `Smoke Tests` required check is left
pending until a maintainer explicitly approves (runs) or skips it.

```mermaid
flowchart TD
  A["Contributor opens PR from a fork"] --> PRC["pr-checks.yaml + pr-security.yaml run<br/>(no secrets needed)"]
  A --> G["smoke-gate.yaml (pull_request)"]
  G -->|fork detected| GP["Does NOT set 'Smoke Tests' status"]
  GP --> GATE{{"Required check 'Smoke Tests'<br/>stays pending → merge blocked"}}

  M["Maintainer reviews the diff"] --> C{"Comment on PR"}
  C -->|"/approve-smoke-tests"| CC["comment-commands.yaml"]
  C -->|"/skip-smoke-tests"| SK["comment-commands.yaml<br/>sets 'Smoke Tests' = success"]
  CC --> PERM["check author has write/maintain/admin"]
  PERM --> DISP["workflow_dispatch → fork-smoke-tests.yaml"]

  subgraph FST["fork-smoke-tests.yaml"]
    P1["check-permissions"] --> P2["resolve-pr (SHA, is_fork)"]
    P2 --> SEC["security-scan: Trivy CRITICAL/HIGH (exit 1)"]
    SEC --> BW["build-windows → _build.yaml"]
    SEC --> NST["notify-start (PR comment)"]
    SEC --> LST["linux-smoke-tests → _smoke.yaml<br/>(install-deps, install-bin, DR_API_TOKEN)"]
    BW --> WST["windows-smoke-tests → _smoke-windows.yaml"]
    LST --> NR["notify-results"]
    WST --> NR
    NST --> NR
    NR --> STAT["set 'Smoke Tests' commit status<br/>success / failure"]
  end

  DISP --> P1
  STAT --> GATE
  SK --> GATE
```

---

## 2. Regular PR (same-repo / maintainer)

`pr-checks` and `pr-security` run automatically; the gate auto-passes for
non-fork PRs. Full smoke tests are opt-in via a label or slash-command.

```mermaid
flowchart TD
  PR["PR opened / updated → main"] --> GATE["smoke-gate.yaml"]
  GATE -->|non-fork| OK["auto-set 'Smoke Tests' = success"]

  subgraph PRC["pr-checks.yaml"]
    DC["detect-changes (paths-filter)"] --> LINT["lint"]
    DC --> TEST["test"]
    DC --> GEN["generate"]
    DC --> COPY["copyright"]
    DC --> BUILD["build (cross-compile, GoReleaser)"]
    DC --> LABEL["auto-label / dependabot reminder"]
    DC -->|deps changed| DEP["deps-install-smoke → _smoke.yaml"]
    DC -->|install.sh changed| II["install-integration → _smoke.yaml"]
    BUILD -->|completion changed| COMP["completion-tests"]
  end

  subgraph PRS["pr-security.yaml"]
    TRIV["trivy-scan (gating)"]
    DEPR["dependency-review (PR only, gating)"]
    GOV["govulncheck (non-blocking)"]
    AN["analyze · CodeQL Go (non-blocking)"]
  end

  PR --> PRC
  PR --> PRS

  PR -. "label: run-smoke-tests / go" .-> SOD["smoke-on-demand.yaml"]
  PR -. "/trigger-smoke-test" .-> CC["comment-commands.yaml"]
  CC -. "adds run-smoke-tests label" .-> SOD
```

---

## 3. Smoke tests — on-demand (labelled, non-fork)

```mermaid
flowchart TD
  L["PR labelled 'run-smoke-tests' or 'go'"] --> CF["check-fork"]
  CF -->|fork| REJ["comment: forks must use maintainer dispatch;<br/>remove label"]
  CF -->|non-fork| NST["notify-start"]
  CF -->|non-fork| BW["build-windows → _build.yaml"]
  NST --> LST["linux-smoke-tests → _smoke.yaml (DR_API_TOKEN)"]
  BW --> WST["windows-smoke-tests → _smoke-windows.yaml (DR_API_TOKEN)"]
  LST --> NR["notify-results · remove 'run-smoke-tests' label"]
  WST --> NR
```

## 3b. Smoke tests — nightly / scheduled

Trigger: push to `main`, schedule (`0 */4 * * 1-5`), or manual dispatch.

```mermaid
flowchart TD
  T["push main / schedule / dispatch"] --> BW["build-windows → _build.yaml"]
  T --> SMK["smoke-test → _smoke.yaml (DR_API_TOKEN)"]
  T --> IT["installs-smoke-test → _install-tests.yaml"]
  T --> SU["self-update-smoke-test → _smoke.yaml (setup off, debug-brew)"]
  BW --> WST["windows-smoke-test → _smoke-windows.yaml (DR_API_TOKEN)"]
  BW --> NF
  SMK --> NF
  WST --> NF
  IT --> NF
  SU --> NF["notify-failure → Slack (on failure only)"]
```

## 3c. Smoke tests — manual dispatch (suite picker)

```mermaid
flowchart TD
  D["workflow_dispatch (suite, version inputs)"] --> S{"suite"}
  S -->|"deps / all"| DEP["deps → _smoke.yaml<br/>(build + run_deps_install_smoke_test.sh)"]
  S -->|"install-integration / all"| II["install-integration → _smoke.yaml<br/>(build + run_install_integration_test.sh)"]
  S -->|"installation / all"| INST["installation → _install-tests.yaml (version)"]
```

---

## 4. Release process

Trigger: push tag matching `v*.*.*` / `v*.*.*.*`.

```mermaid
flowchart TD
  TAG["push tag v*.*.*"] --> REL["release<br/>GoReleaser · Homebrew tap · GitHub release · Slack"]
  REL --> VI["verify-installation → _install-tests.yaml<br/>(install from cli.datarobot.com on all OS)"]
  VI --> PRS["pre-release-smoke-test → _pre-release-smoke.yaml<br/>(heavier E2E, DR_API_TOKEN)"]
  PRS --> PROMO["promote-release<br/>(promote pre-release → stable if applicable)"]
```

---

## 5. Pages publication (Deploy static content to Pages)

Trigger: push to `main` or manual dispatch. Single `deploy` job in the
`github-pages` environment.

```mermaid
flowchart TD
  T["push main / workflow_dispatch"] --> DEP
  subgraph DEP["pages.yaml · deploy (env: github-pages)"]
    CO["checkout"] --> UV["install uv + Python"]
    UV --> MK["mkdocs build (docs/)"]
    MK --> CP["configure-pages"]
    CP --> UP["upload-pages-artifact (docs/)"]
    UP --> DP["deploy-pages"]
  end
```
