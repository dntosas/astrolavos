#!/usr/bin/env bash
# Dispatch a workflow at a ref and block until it finishes, propagating its
# conclusion as the exit code. Used by release.yml because pushes made with
# GITHUB_TOKEN do not trigger `on: push` workflows, while workflow_dispatch
# through the API does.
#
# usage: dispatch-and-wait.sh <workflow-file> <ref>
# needs: gh authenticated with `actions: write` (GH_TOKEN), jq
set -euo pipefail

workflow=${1:?workflow file, e.g. go-release.yml}
ref=${2:?git ref to run at, e.g. v1.2.0 or main}

# gh run list filters on the run's head branch, which for a tag ref is the
# bare tag name. Remember when we started so we do not pick up an older run.
since=$(date -u -d '-1 minute' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-1M +%Y-%m-%dT%H:%M:%SZ)

echo "dispatching $workflow at $ref"
gh workflow run "$workflow" --ref "$ref"

run_id=""
for _ in $(seq 1 24); do
  run_id=$(gh run list --workflow "$workflow" --event workflow_dispatch --branch "$ref" \
    --created ">=$since" --limit 1 --json databaseId --jq '.[0].databaseId // empty')
  [[ -n "$run_id" ]] && break
  sleep 5
done
if [[ -z "$run_id" ]]; then
  echo "::error::$workflow was dispatched at $ref but no run appeared within 2 minutes"
  exit 1
fi

echo "waiting on ${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-}/actions/runs/$run_id"
# --exit-status makes gh exit non-zero when the run concludes unsuccessfully
gh run watch "$run_id" --exit-status --interval 30
