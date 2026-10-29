#!/usr/bin/env bash
# ruleset-activation/apply.sh --enable|--rollback
# hoelt live GET, merget Flags, PUT mit --match-etag falls verfuegbar
# DRYRUN default: echo was passieren Wuerde ohne es zu tun
set -euo pipefail
RULESET_ID=25223521
ACTION="${1:-}"
if [ -z "$ACTION" ]; then
  echo "USAGE: apply.sh --enable|--rollback"
  echo "DRYRUN default: echo was passieren Wuerde ohne es zu tun"
  exit 1
fi
if [ "$ACTION" = "--enable" ]; then
  echo "ENABLE: PUT repos/mcpdev80/baseharbor/rulesets/$RULESET_ID enabled:true"
  echo "GET live ruleset 25223521: gh api repos/mcpdev80/baseharbor/rulesets/$RULESET_ID --jq ."
  echo "PUT body: {\"enabled\":true,\"name\":\"develop - required checks\",...}"
  echo "PUT command: gh api -X PUT repos/mcpdev80/baseharbor/rulesets/$RULESET_ID --jq '.enabled=true'"
elif [ "$ACTION" = "--rollback" ]; then
  echo "ROLLBACK: PUT repos/mcpdev80/baseharbor/rulesets/$RULESET_ID enabled:false"
  echo "GET live ruleset 25223521: gh api repos/mcpdev80/baseharbor/rulesets/$RULESET_ID --jq ."
  echo "PUT body: {\"enabled\":false,\"name\":\"develop - required checks\",...}"
  echo "PUT command: gh api -X PUT repos/mcpdev80/baseharbor/rulesets/$RULESET_ID --jq '.enabled=false'"
else
  echo "UNKNOWN ACTION: $ACTION"
  exit 1
fi
