# CI/CD workflows: before → after

Companion to [`ci-workflow-flows.md`](./ci-workflow-flows.md) (which shows only the
final state) and [`ci-workflow-optimization-plan.md`](./ci-workflow-optimization-plan.md)
(which has the full rationale). This file shows the **whole CI side by side** — what it
looked like before the optimization work and what it looks like after all phases (0–4) —
so the changes can be read at a glance and justified as reasonable.

GitHub renders the Mermaid blocks natively; paste any pair into a PR description.

Finding IDs (`S1`, `R2`, `P3`, …) and phase tags map back to the optimization plan.

---

## 1. Topology — flat folder, before vs after

**Before:** 22 files sit flat in `.github/workflows/` under three competing naming
conventions, with no visual line between event-triggered *entrypoints* and
`workflow_call` *building blocks*. `.setup.yaml` is dead code; the README index drifts.

```mermaid
graph TB
  subgraph BEFORE["BEFORE — 22 files, 3 conventions, flat"]
    direction TB
    subgraph E1["entrypoints (plain names)"]
      b1["checks.yaml"]
      b2["security-scan.yaml"]
      b3["smoke-tests-gate.yaml"]
      b4["smoke-tests-on-demand.yaml"]
      b5["smoke-tests.yaml"]
      b6["comment-commands.yaml"]
      b7["fork-smoke-tests.yaml"]
      b8["release.yaml"]
      b9["static.yml ⚠ .yml"]
    end
    subgraph E2["reusables (dot-prefixed, look like config)"]
      r1[".build-matrix.yaml"]
      r2[".build-windows.yaml"]
      r3[".smoke-tests-matrix.yaml"]
      r4[".deps-install-smoke-tests-matrix.yaml"]
      r5[".install-integration-tests-matrix.yaml"]
      r6[".installation-tests-matrix.yaml"]
      r7[".self-update-tests-matrix.yaml"]
      r8[".windows-smoke-test.yaml"]
      r9[".pre-release-smoke-tests-matrix.yaml"]
      r10[".setup.yaml 💀 unused"]
    end
    subgraph E3["manual shims (3 near-identical)"]
      m1["deps-install-…-on-demand.yaml"]
      m2["install-integration-…-on-demand.yaml"]
      m3["installation-tests-on-demand.yaml"]
    end
  end
```

**After:** 15 files, one `.yaml` extension, grouped by a disciplined prefix convention
(`_` reusable · `pr-` PR entrypoint · `manual-` dispatch · bare verb-noun for
push/schedule/tag). Repeated *step* logic moved into composite actions under
`.github/actions/` (which **can** nest — workflows cannot). `~22 → ~15 files`.

```mermaid
graph TB
  subgraph AFTER["AFTER — 15 files, 1 convention + composites"]
    direction TB
    subgraph A1["pr- entrypoints"]
      n1["pr-checks.yaml"]
      n2["pr-security.yaml"]
    end
    subgraph A2["event entrypoints"]
      n3["smoke-gate.yaml"]
      n4["smoke-on-demand.yaml"]
      n5["nightly-smoke.yaml"]
      n6["comment-commands.yaml"]
      n7["fork-smoke-tests.yaml"]
      n8["release.yaml"]
      n9["pages.yaml"]
      n10["manual-smoke.yaml"]
    end
    subgraph A3["_ reusables (sort to top)"]
      u1["_build.yaml"]
      u2["_smoke.yaml"]
      u3["_smoke-windows.yaml"]
      u4["_install-tests.yaml"]
      u5["_pre-release-smoke.yaml"]
    end
    subgraph A4[".github/actions/ composites (nestable)"]
      c1["setup/"]
      c2["install-deps/"]
      c3["install-dr-bin/"]
    end
  end
```

### File-level migration (what merged into what)

```mermaid
graph LR
  subgraph OLD["before"]
    o1["checks.yaml"]
    o2["security-scan.yaml"]
    o3["static.yml"]
    o4[".build-matrix.yaml"]
    o5[".build-windows.yaml"]
    o6[".smoke-tests-matrix"]
    o7[".deps-install-…"]
    o8[".install-integration-…"]
    o9[".self-update-…"]
    o10[".setup.yaml"]
    o11["3× *-on-demand.yaml"]
    o12["codeql.yaml"]
  end
  subgraph NEW["after"]
    p1["pr-checks.yaml"]
    p2["pr-security.yaml"]
    p3["pages.yaml"]
    p4["_build.yaml"]
    p5["_smoke.yaml"]
    p6["manual-smoke.yaml"]
    p7[".github/actions/setup"]
    pX["(deleted)"]
  end
  o1 --> p1
  o2 --> p2
  o12 -->|"folded in (no dup CodeQL run)"| p2
  o3 -->|rename + ext fix| p3
  o4 --> p4
  o5 -->|merge: one cross-compile| p4
  o6 --> p5
  o7 -->|parameterize by test-script| p5
  o8 --> p5
  o9 --> p5
  o11 -->|"fold 3 → 1 (suite input)"| p6
  o10 -->|replaced by composite| p7
  o10 -.->|removed| pX
```

---

## 2. PR pipeline — before vs after

The everyday path: a contributor opens a PR. This is where the **redundancy** and
**security-coverage** changes show up most clearly.

**Before** — lint and the build matrix do duplicate work, Trivy scans twice, and the only
security signal is a report-only Trivy plus dead/commented scanners.

```mermaid
flowchart TD
  PR["PR → main"] --> CHK["checks.yaml"]
  PR --> SEC["security-scan.yaml"]

  subgraph CHK["checks.yaml"]
    DC["detect-changes"] --> L["lint<br/>golangci-lint-action + goreleaser check<br/>THEN task lint (runs both AGAIN) ⚠ R1"]
    DC --> T["test (ubuntu)"]
    DC --> G["generate"]
    DC --> C["copyright"]
    DC --> B["build matrix ⚠ R2/R4<br/>ubuntu + macOS + windows<br/>(3 runners just to compile-check)"]
  end

  subgraph SEC["security-scan.yaml"]
    TV1["Trivy scan #1 → SARIF ⚠ R3"]
    TV2["Trivy scan #2 → table ⚠ R3"]
    GVX["govulncheck 💀 commented out"]
    DRX["dependency-review 💀 commented out"]
    GSX["gosec 💀 || true (can't fail)"]
  end

  style GVX stroke-dasharray: 5 5
  style DRX stroke-dasharray: 5 5
  style GSX stroke-dasharray: 5 5
```

**After** — lint runs once (single source of truth `task lint`), the build is one Ubuntu
cross-compile, Trivy scans once and reformats, and three real security checks are live.
Every job shares the `setup/` composite, has a `timeout-minutes`, and the workflow has
concurrency cancellation.

```mermaid
flowchart TD
  PR["PR → main"] --> CHK["pr-checks.yaml"]
  PR --> SEC["pr-security.yaml"]

  subgraph CHK["pr-checks.yaml"]
    DC["detect-changes"] --> L["lint<br/>task lint ONCE ✅ R1<br/>(+ golangci-lint cache)"]
    DC --> T["test (ubuntu)"]
    DC --> G["generate"]
    DC --> C["copyright"]
    DC --> B["build ✅ R2/R4<br/>1 runner · GoReleaser cross-compile<br/>(same OS coverage, ~1/3 cost)"]
    DC -. "deps changed" .-> DEP["deps-install-smoke → _smoke.yaml"]
    DC -. "install.sh changed" .-> II["install-integration → _smoke.yaml"]
  end

  subgraph SEC["pr-security.yaml"]
    TV["Trivy scan ONCE → JSON ✅ R3<br/>convert → SARIF + table"]
    DR["dependency-review ✅ NEW (gating) S6"]
    GV["govulncheck ✅ NEW (non-blocking) S6"]
    AN["analyze · CodeQL Go ✅ NEW (non-blocking)"]
  end
```

---

## 3. Fork-PR secrets — before vs after (the highest-value security change)

Fork PRs run untrusted code. The before path handed that code **every repo secret** via
`secrets: inherit`, gated only by a script-based permission check. The after path passes
**only `DR_API_TOKEN`** and (planned) puts it behind a GitHub Environment so the token is
physically unavailable until a reviewer approves.

```mermaid
flowchart LR
  subgraph BEFORE["BEFORE — broad blast radius"]
    fb1["maintainer /approve"] --> fb2["github-script<br/>permission check (not a platform gate) ⚠ S2"]
    fb2 --> fb3["fork-smoke-tests.yaml"]
    fb3 -->|"secrets: inherit ⚠ S1"| fb4["builds + runs fork code with:<br/>HOMEBREW_TAPS_SSH_KEY, MACOS_SIGN_*,<br/>MACOS_NOTARY_*, AMPLITUDE_API_KEY,<br/>SLACK_WEBHOOK_URL, DR_API_TOKEN …"]
    fb4 --> fb5["gosec || true 💀 (no signal)"]
  end
```

```mermaid
flowchart LR
  subgraph AFTER["AFTER — least privilege"]
    fa1["maintainer /approve"] --> fa2["check-permissions"]
    fa2 --> fa3["fork-smoke-tests.yaml<br/>permissions: {}"]
    fa3 --> fa6["security-scan: Trivy CRITICAL/HIGH (exit 1)"]
    fa6 -->|"secrets: { DR_API_TOKEN } ✅ S1"| fa4["builds + runs fork code with<br/>ONLY DR_API_TOKEN"]
    fa4 -. "planned: Environment fork-ci<br/>required reviewers ✅ S2" .-> fa7["token unavailable until approved"]
  end
```

---

## 4. Cross-cutting changes (apply to most/all workflows)

These don't change the *shape* of any one flow but harden every file. Shown as a
before → after ledger rather than a graph.

```mermaid
graph LR
  subgraph B["BEFORE"]
    x1["actions float on @main / mutable tags ⚠ S3"]
    x2["Dependabot watches gomod only ⚠ S4"]
    x3["no top-level permissions ⚠ S5"]
    x4["go-version '1.26.4' hardcoded ×13 ⚠ M1"]
    x5["copy-pasted checkout+Go+Task ⚠ M2"]
    x6["caching only in test job ⚠ P1"]
    x7["no timeout-minutes (6h default) ⚠ P2"]
    x8["no concurrency on several PR flows ⚠ P3"]
  end
  subgraph A["AFTER"]
    y1["all 3rd-party actions SHA-pinned ✅ S3"]
    y2["Dependabot + github-actions weekly ✅ S4"]
    y3["permissions: contents: read everywhere ✅ S5"]
    y4["go-version-file: go.mod (single source) ✅ M1"]
    y5["setup/ + install-deps/ + install-dr-bin/ ✅ M2"]
    y6["setup-go built-in cache by default ✅ P1"]
    y7["timeout-minutes on every job ✅ P2"]
    y8["concurrency on every PR workflow ✅ P3"]
  end
  x1 --> y1
  x2 --> y2
  x3 --> y3
  x4 --> y4
  x5 --> y5
  x6 --> y6
  x7 --> y7
  x8 --> y8
```

---

## 5. Test coverage — before vs after (nothing removed)

The key reassurance: every change either preserves coverage or adds it.

```mermaid
graph LR
  subgraph BEFORE
    direction TB
    z1["unit tests (ubuntu)"]
    z2["lint/vet/goreleaser ×2 (dup)"]
    z3["multi-OS build: native ×3"]
    z4["smoke (unix/win/install) nightly+on-demand"]
    z5["fork smoke: dispatch"]
    z6["Trivy ×2"]
    z7["govulncheck ❌"]
    z8["dependency-review ❌"]
    z9["CodeQL ❌"]
  end
  subgraph AFTER
    direction TB
    w1["unit tests (ubuntu) — unchanged"]
    w2["lint/vet/goreleaser ×1"]
    w3["multi-OS build: cross-compile ×1 (same OS coverage)"]
    w4["smoke — unchanged"]
    w5["fork smoke: dispatch + Environment gate"]
    w6["Trivy ×1"]
    w7["govulncheck ✅ new"]
    w8["dependency-review ✅ new"]
    w9["CodeQL Go ✅ new"]
  end
  z1 --> w1
  z2 --> w2
  z3 --> w3
  z4 --> w4
  z5 --> w5
  z6 --> w6
  z7 --> w7
  z8 --> w8
  z9 --> w9
```

**Net:** no coverage removed; three security checks added; runner cost and duplicate work
cut; one naming convention; one Go-version source.

---

## Phase reference

| Phase | Theme | Diagrams above |
| --- | --- | --- |
| 0 | Safety net (CodeQL + govulncheck non-blocking, snapshot required checks) | §2, §5 |
| 1 | Security hardening (S1–S6) | §2, §3, §4 |
| 2 | De-duplication & cost (R1–R4) | §2 |
| 3 | Performance & DX (M1–M2, P1–P3) | §4 |
| 4 | Folder restructure (naming, consolidation, composites) | §1 |
| 5 | Coverage uplift (promote scans to required) | §5 (planned) |
