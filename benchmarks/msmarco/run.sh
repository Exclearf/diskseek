#!/usr/bin/env bash

set -euo pipefail

if (( $# != 3 )); then
	printf 'usage: %s DATA TOOLS OUTPUT\n' "$0" >&2
	printf 'DATA must contain collection.tsv, queries.dev.small.tsv, and qrels.dev.small.tsv.\n' >&2
	printf 'TOOLS must contain ms_marco_eval.py and an executable trec_eval.\n' >&2
	exit 2
fi

repository=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
data=$1
tools=$2
output=$3

collection=$data/collection.tsv
queries=$data/queries.dev.small.tsv
qrels=$data/qrels.dev.small.tsv
msmarco_eval=$tools/ms_marco_eval.py
trec_eval=$tools/trec_eval
codec=vbyte
flush_target=$((64 << 20))
merge_fan_in=16
merge_workers=1
result_depth=1000

if [[ ! -f $msmarco_eval || ! -x $trec_eval ]]; then
	printf 'TOOLS must contain ms_marco_eval.py and an executable trec_eval.\n' >&2
	exit 1
fi

collection_count=$(wc -l < "$collection")
query_count=$(wc -l < "$queries")
qrels_count=$(wc -l < "$qrels")

mkdir "$output"
mkdir "$output/tmp"
output=$(cd -- "$output" && pwd)
binary=$output/diskseek
index=$output/index-vbyte
run=$output/run.diskseek.tsv

(
	cd -- "$repository"
	go build -o "$binary" ./cmd/diskseek
)

"$binary" index \
	--codec "$codec" \
	--flush-target "$flush_target" \
	--merge-fan-in "$merge_fan_in" \
	--merge-workers "$merge_workers" \
	--temp-dir "$output/tmp" \
	"$collection" \
	"$index"
"$binary" verify "$index"
"$binary" query --batch --limit "$result_depth" "$index" "$queries" > "$run"

cut -f1-3 "$run" > "$output/run.msmarco.tsv"
awk -F '\t' 'BEGIN { OFS = "\t" } { print $1, "Q0", $2, $3, -$3, "diskseek" }' \
	"$run" > "$output/run.trec"

python3 "$msmarco_eval" "$qrels" "$output/run.msmarco.tsv" > "$output/msmarco-eval.txt"
{
	"$trec_eval" -c -M 10 -m recip_rank "$qrels" "$output/run.trec"
	"$trec_eval" -c -m recall.100,1000 "$qrels" "$output/run.trec"
} > "$output/trec-eval.txt"

{
	printf 'timestamp_utc\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	printf 'revision\t%s\n' "$(git -C "$repository" rev-parse HEAD)"
	printf 'go\t%s\n' "$(go version)"
	printf 'python\t%s\n' "$(python3 --version)"
	printf 'system\t%s\n' "$(uname -srm)"
	printf 'collection_rows\t%d\n' "$collection_count"
	printf 'query_rows\t%d\n' "$query_count"
	printf 'qrels_rows\t%d\n' "$qrels_count"
	printf 'codec\t%s\n' "$codec"
	printf 'flush_target\t%d\n' "$flush_target"
	printf 'merge_fan_in\t%d\n' "$merge_fan_in"
	printf 'merge_workers\t%d\n' "$merge_workers"
	printf 'result_depth\t%d\n' "$result_depth"
	printf '\nworktree\n'
	git -C "$repository" status --short
	printf '\nsha256\n'
	sha256sum \
		"$collection" \
		"$queries" \
		"$qrels" \
		"$msmarco_eval" \
		"$trec_eval" \
		"$repository/benchmarks/msmarco/run.sh" \
		"$binary" \
		"$index"/* \
		"$run" \
		"$output/run.msmarco.tsv" \
		"$output/run.trec"
} > "$output/manifest.txt"
