# Scenario → Ticket Index

Maps each acceptance scenario to its related JIRA ticket, for easier reference
during the bug bash. This is a temporary aid and can be removed once the bash
ends; the canonical per-file tag is the `# Ticket:` header at the top of each
scenario script (grep with `rg '^# Ticket:' smoke_test_scripts/workload`).

| Scenario | File | Ticket | What it guards |
| --- | --- | --- | --- |
| A — whoami round trip | `RAPTOR-19533-A-roundtrip.sh` | RAPTOR-19533 | `up` validates at load time; bind renders a clean manifest |
| B — account sweep | `RAPTOR-19533-B-sweep.sh` | RAPTOR-19533 | binding works against every workload on the account |
| C — re-bind tuned | `RAPTOR-19533-C-rebind.sh` | RAPTOR-19533 | re-bind preserves live tuning; FileExists guard; delete-rebind restore |
| D — built-workload rebuild | `RAPTOR-19533-D-built.sh` | RAPTOR-19533 | no ErrImagePull after re-bind; platform rebuilds from imageBuildConfig |
| E — recovery from a dead workload | `RAPTOR-19729-E-recovery.sh` | RAPTOR-19729 | every refusal names a command, not a file; `--recreate` recovers without discarding state |
| Artifact lifecycle | `artifact-lifecycle.sh` | none — basic acceptance | `dr artifact` create/get/list/code sync/versions/del CLI-side state |

## Adding a scenario for a new ticket

1. Create `<TICKET>-<LETTER>-<name>.sh` (e.g. `RAPTOR-19533-A-roundtrip.sh`)
   with `# Ticket: RAPTOR-XXXXX` as the first comment after the shebang.
   A scenario with no ticket drops the prefix (e.g. `artifact-lifecycle.sh`).
2. Register it in `run_workload_smoke_test.sh`'s `scenario_script` map.
3. Add a row here.
