# Scenario → Ticket Index

Maps each acceptance scenario to its related JIRA ticket, for easier reference
during the bug bash. This is a temporary aid and can be removed once the bash
ends; the canonical per-file tag is the `# Ticket:` header at the top of each
scenario script (grep with `rg '^# Ticket:' smoke_test_scripts/workload`).

| Scenario | File | Ticket | What it guards |
| --- | --- | --- | --- |
| A — whoami round trip | `scenario_a_roundtrip.sh` | RAPTOR-19533 | `up` validates at load time; bind renders a clean manifest |
| B — account sweep | `scenario_b_sweep.sh` | RAPTOR-19533 | binding works against every workload on the account |
| C — re-bind tuned | `scenario_c_rebind.sh` | RAPTOR-19533 | re-bind preserves live tuning; FileExists guard; delete-rebind restore |
| D — built-workload rebuild | `scenario_d_built.sh` | RAPTOR-19533 | no ErrImagePull after re-bind; platform rebuilds from imageBuildConfig |
| Artifact lifecycle | `scenario_artifact.sh` | none — basic acceptance | `dr artifact` create/get/list/code sync/versions/del CLI-side state |

## Adding a scenario for a new ticket

1. Create `scenario_<name>.sh` with `# Ticket: RAPTOR-XXXXX` as the first
   comment after the shebang.
2. Register it in `run_workload_smoke_test.sh`'s `scenario_script` map.
3. Add a row here.
