# q43-renovate: Renovate-Update-Inventar v0.4.27

## PLAN

1. Read task t-20261017-release-0-4-27-updates.md (Claim q43)
2. basics-check.py turn-start (from AGENTS.md)
3. `gh pr list --repo mcpdev80/baseharbor --json number,title,headCommit,mergeable,mergeStateStatus,labels,reviewDecision,state` for Renovate PRs
4. `gh pr list --repo mcpdev80/baseharbor-demo --json ...` for Renovate PRs
5. `gh pr list --repo mcpdev80/baseharbor-console --json ...` for Renovate PRs
6. `gh pr list --repo mcpdev80/baseharbor-node-connector --json ...` for Renovate PRs
7. Classify each PR by risk: deps-major / deps-minor / deps-patch / lock-only / docker-base / actions / toolchain
8. For each repo derive a recommended integration order
9. Report evidence only - no merges/closes
10. END Q43-RENOVATE-DONE

## Status

[ wip:q43 ]

## Evidence

### Step 0: basics-check.py turn-start
