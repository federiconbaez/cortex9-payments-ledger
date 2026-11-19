.PHONY: test race sim fuzz bench tlc vet

test:
	go test ./...

race:
	go test -race -count=1 ./...

sim:
	go test -race -count=1 -timeout 3m ./internal/sim

fuzz:
	go test -fuzz=FuzzDecodeOpen -fuzztime=10s ./internal/adapters/httpapi
	go test -fuzz=FuzzDecodeTransfer -fuzztime=10s ./internal/adapters/httpapi
	go test -fuzz=FuzzComputeHash -fuzztime=10s ./internal/domain

bench:
	go test -bench=. -benchmem ./internal/domain

tlc:
	./scripts/tla-check.sh

vet:
	go vet ./...
	gofmt -l .
