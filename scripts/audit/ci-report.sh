#!/bin/sh
# Summarises what ci-logs.sh downloaded: time per job, time per package of
# the unit job, and the tests that failed, with whether the run passed on a
# later attempt.
#
#   scripts/audit/ci-report.sh OUT
set -eu
out=${1:?usage: ci-report.sh OUT}
tab="$(printf '\t')"

echo "Runs: $(wc -l <"$out/runs.tsv"), from $(cut -f6 "$out/runs.tsv" | sort | head -1) to $(cut -f6 "$out/runs.tsv" | sort | tail -1)"
echo
echo "| job | runs | median s | max s |"
echo "|---|---|---|---|"
awk -F"$tab" '$7=="success" {print $6 "\t" $8}' "$out/jobs.tsv" | sed 's/ ([0-9])//' | sort -t"$tab" -k1,1 -k2,2n |
	awk -F"$tab" '{v[$1]=v[$1] " " $2; n[$1]++} END {for (j in v) {split(substr(v[j],2), a, " "); printf "| %s | %d | %d | %d |\n", j, n[j], a[int((n[j]+1)/2)], a[n[j]]}}' | sort

echo
echo "Packages of the unit job, by median seconds (successful runs):"
echo
echo "| package | runs | median s | max s |"
echo "|---|---|---|---|"
for f in "$out"/*.log; do
	grep -E '^[^ ]+ ok  	' "$f" 2>/dev/null | awk -F"$tab" '{sub("github.com/noderunner/bingle/", "", $2); sub("s$", "", $3); print $2 "\t" $3}'
done | grep -v "^tests/app$tab" | sort -t"$tab" -k1,1 -k2,2n |
	awk -F"$tab" '{v[$1]=v[$1] " " $2; n[$1]++} END {for (p in v) {split(substr(v[p],2), a, " "); printf "%s\t%d\t%.1f\t%.1f\n", p, n[p], a[int((n[p]+1)/2)], a[n[p]]}}' |
	sort -t"$tab" -k3,3nr | head -"${TOP:-15}" | awk -F"$tab" '{printf "| %s | %s | %s | %s |\n", $1, $2, $3, $4}'

echo
echo "Tests that failed (run, attempt, event, branch, job, test; the last column says whether the run passed on a later attempt):"
echo
echo "| run | attempt | event | branch | job | test | later attempt |"
echo "|---|---|---|---|---|---|---|"
while IFS="$tab" read -r run at ev br job name conclusion secs; do
	[ "$conclusion" = failure ] || continue
	f="$out/$run-$at-$job.log"
	[ -s "$f" ] || continue
	last=$(awk -F"$tab" -v r="$run" '$1==r {print $4}' "$out/runs.tsv")
	final=$(awk -F"$tab" -v r="$run" '$1==r {print $5}' "$out/runs.tsv")
	later=no
	[ "$at" -lt "$last" ] && later="$final"
	grep -oE -- '--- FAIL: [A-Za-z0-9_/]+' "$f" | sed 's/--- FAIL: //' | sort -u | while read -r t; do
		echo "| $run | $at | $ev | $br | $name | $t | $later |"
	done
done <"$out/jobs.tsv"
