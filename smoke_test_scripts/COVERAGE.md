# Smoke Test Coverage

This document tracks what is covered by smoke tests across all platforms. Use it to identify coverage gaps and maintain consistency.

## Test Scripts

| Script | Platform | Trigger |
|--------|----------|---------|
| `run_smoke_test.sh` | Unix (Linux / macOS) | `task smoke-test` |
| `windows/run_smoke_test.ps1` | Windows | `task smoke-test-windows` |
| `run_plugin_update_smoke_test.sh` | Unix | `task smoke-test` |
| `run_self_update_smoke_test.sh` | Unix (macOS for brew tests) | manual / CI |

---

## Coverage Matrix

Legend: ✅ Covered · ⚠️ Partial · ❌ Not covered · ⏭️ Intentionally skipped

| Test Area | Unix | Windows | Notes |
|-----------|:----:|:-------:|-------|
| **Installation** | | | |
| `dr` binary accessible in PATH | ✅ | ✅ | |
| `datarobot` alias available | ✅ | ❌ | Windows uses `dr` only |
| **Help & Basics** | | | |
| `dr help` output content | ✅ | ✅ | Unix checks specific header copy |
| `dr help run` | ❌ | ✅ | |
| `datarobot help` (alias) | ✅ | ❌ | |
| **Versioning** | | | |
| `dr self version` | ✅ | ✅ | |
| `dr self version --format=json` has `version` key | ✅ | ❌ | |
| **Shell Detection** | | | |
| Detects bash / zsh / fish | ✅ | ❌ | Via `dr --debug self version` |
| Detects PowerShell | ❌ | ✅ | Via `dr --debug self version` |
| Detects cmd.exe | ❌ | ⚠️ | Standalone `.bat` test only |
| **Completion** | | | |
| `dr self completion bash` generates file | ✅ | ❌ | Checks `__start_dr()` function |
| `dr self completion powershell` generates file | ❌ | ✅ | Checks `Register-ArgumentCompleter` |
| Completion install / uninstall (interactive) | ✅ | ❌ | Uses `expect` |
| **Task / Run** | | | |
| `dr run` outside template shows informative message | ✅ | ✅ | |
| **Auth** | | | |
| `dr auth setURL` (interactive) | ✅ | ⚠️ | Windows sets config directly |
| Config file updated with correct endpoint | ✅ | ✅ | |
| `dr auth login` (interactive) | ✅ | ⚠️ | Windows falls back to `dr auth check` |
| **Templates** | | | |
| `dr templates setup` (interactive clone) | ✅ | ⏭️ | Skipped on Windows (no `expect`) |
| Cloned directory exists after setup | ✅ | ⏭️ | |
| `SESSION_SECRET_KEY` auto-generated in `.env` | ✅ | ⏭️ | |
| **Dotenv** | | | |
| `dr dotenv setup` inside template directory | ✅ | ⏭️ | |
| `DATAROBOT_ENDPOINT` preserved in `.env` | ✅ | ⏭️ | |
| **Plugin Auto-Update** (`run_plugin_update_smoke_test.sh`) | | | |
| Install plugin at pinned version | ✅ | ❌ | |
| Auto-update prompt appears on plugin run | ✅ | ❌ | |
| Decline update → version unchanged | ✅ | ❌ | |
| Cooldown state file written after prompt | ✅ | ❌ | |
| Accept update → version changes | ✅ | ❌ | |
| **Self Update** (`run_self_update_smoke_test.sh`) | | | |
| curl install latest → `dr self update` is no-op | ✅ | ❌ | |
| curl install old version → `dr self update` upgrades | ✅ | ❌ | |
| brew install → `dr self update` uses brew path | ✅ (macOS) | ❌ | Skipped on Linux |
| Template min-version satisfied → update is no-op | ✅ | ❌ | Stretch test |
| Template min-version satisfied → `dr self update -f` upgrades | ✅ | ❌ | Stretch test |

---

## Known Gaps

### Windows
- No interactive testing (no `expect` equivalent): `dr auth setURL`, `dr auth login`, `dr templates setup`, `dr dotenv setup`, and all plugin-update flows are untested or manually simulated.
- `datarobot` alias is not verified.
- Plugin auto-update and self-update flows are not covered.
- Shell detection only covers PowerShell (not cmd.exe in the main suite; covered by a standalone `.bat` script).

### Unix
- `dr help run` is not checked (only `dr help`).
- PowerShell completion is not generated or verified.

---

## Adding a New Test

1. Implement the test case in the relevant script(s).
2. Update the matrix above — add a new row if it's a new area, or change ❌ to ✅ / ⚠️ as appropriate.
3. If the test covers a new platform, add a column and fill in all existing rows.
