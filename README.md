# DiskSeek

DiskSeek is a disk-backed search engine written in Go.

It is still at an early stage.

The goal is to implement the core indexing and search code:

- immutable indexes stored on disk
- a binary format and custom postings compression
- BM25 scoring
- exhaustive search and WAND
- benchmarks comparing the different query paths

## Development

Requires Go 1.27 or newer.

```sh
go build -o bin/diskseek ./cmd/diskseek
go test ./...
go vet ./...
```

Run the current CLI:

```sh
go run ./cmd/diskseek help
go run ./cmd/diskseek version
```

## License

DiskSeek is licensed under the [Apache License 2.0](LICENSE).
