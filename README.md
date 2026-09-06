# DiskSeek

DiskSeek builds immutable, disk-backed search indexes in Go and ranks results
with BM25 using DAAT or WAND.

## Features

- SPIMI indexing with flushed runs and bounded multipass merging
- versioned binary indexes with CRC32C checksums
- raw and hand-written VByte postings
- concurrent queries and full index verification

## Usage

### Build an index

The corpus must contain one TSV row per document: its ID and text.

```sh
diskseek index corpus.tsv search-index
```

### Search

```sh
diskseek query search-index "computer science"
```

Example output (TSV):

```text
cs-overview	4.21
algorithms	3.77
computing	3.21
```

### Batch search

Pass batch queries in a TSV file or through standard input. Each row must
contain a query ID and query text.

```sh
diskseek query --batch search-index queries.tsv
diskseek query --batch search-index < queries.tsv
```

### Verify an index

```sh
diskseek verify search-index
```

Run `diskseek COMMAND --help` for command options.

## Benchmarks

See the [benchmark results](benchmarks/README.md).

## Development

Requires Go 1.27 or newer.

```sh
go test ./...
go vet ./...
```

## License

DiskSeek is licensed under the [Apache License 2.0](LICENSE).
