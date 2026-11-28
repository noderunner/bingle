#!/bin/sh
# Downloads the job lists and test logs of recent runs of the CI workflow, as
# the repository's GitHub App, for ci-report.sh to read.
#
#   scripts/audit/ci-logs.sh OUT [RUNS]
#
# RUNS (default 20) runs are taken of each of push-to-main and pull_request;
# every attempt of a rerun is kept. GHAPP names the helper (default: go run
# ./scripts/ghapp). Needs jq and BINGLE_GH_REMOTE.
set -eu
out=${1:?usage: ci-logs.sh OUT [RUNS]}
runs=${2:-20}
ghapp=${GHAPP:-go run ./scripts/ghapp}
r=${BINGLE_GH_REMOTE:?BINGLE_GH_REMOTE is not set}
r=${r#https://github.com/}
r=${r%.git}
mkdir -p "$out"

api() { $ghapp api GET "$1"; }

: >"$out/runs.tsv"
for q in "branch=main&event=push" "event=pull_request"; do
	api "/repos/$r/actions/workflows/ci.yml/runs?$q&status=completed&per_page=$runs" |
		jq -r '.workflow_runs[] | [.id, .event, .head_branch, .run_attempt, .conclusion, .created_at] | @tsv' >>"$out/runs.tsv"
done

: >"$out/jobs.tsv"
while IFS="$(printf '\t')" read -r id event branch attempts conclusion created; do
	n=1
	while [ "$n" -le "$attempts" ]; do
		api "/repos/$r/actions/runs/$id/attempts/$n/jobs?per_page=50" |
			jq -r --arg run "$id" --arg ev "$event" --arg br "$branch" --arg at "$n" \
				'.jobs[] | [$run, $at, $ev, $br, .id, .name, (.conclusion // "none"),
				 (((.completed_at // .started_at) | fromdateiso8601) - (.started_at | fromdateiso8601))] | @tsv' >>"$out/jobs.tsv"
		n=$((n + 1))
	done
done <"$out/runs.tsv"

# Logs of the test jobs, and of every job that did not succeed.
while IFS="$(printf '\t')" read -r run at ev br job name conclusion secs; do
	f="$out/$run-$at-$job.log"
	[ -s "$f" ] && continue
	case "$name" in
	unit | sims | browser*) ;;
	*) [ "$conclusion" = success ] && continue ;;
	esac
	api "/repos/$r/actions/jobs/$job/logs" >"$f" || rm -f "$f"
done <"$out/jobs.tsv"
