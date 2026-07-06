OLD ?= output
NEW ?= output
OUTPUT ?= output
RECOVERED ?= recovered-missing-books.json
TARGETS ?= topreads-missing-books-to-double-check.json
PRIORITY ?= P0
LIMIT ?= 0
DELAY ?= 100ms
WORKERS ?= 50

.PHONY: test compare-missing recover-missing merge-outputs crawl-qa update-titles split-output

test:
	go test ./...

compare-missing:
	go run ./cmd/compare-missing -old '$(OLD)' -new '$(NEW)' -out '$(TARGETS)'

recover-missing:
	go run ./cmd/recover-missing -targets '$(TARGETS)' -priority '$(PRIORITY)' -limit $(LIMIT) \
		-workers $(WORKERS) -delay $(DELAY) -out '$(RECOVERED)' -keep-previous-on-fail

merge-outputs:
	go run ./cmd/merge-outputs -new '$(NEW)' -recovered '$(RECOVERED)' -out '$(OUTPUT)'

crawl-qa:
	go run ./cmd/crawl-qa -old '$(OLD)' -new '$(OUTPUT)' -recovered '$(RECOVERED)'

update-titles:
	go run ./cmd/update-titles -output '$(OUTPUT)' -workers $(WORKERS) -delay $(DELAY) -limit $(LIMIT)

split-output:
	go run ./cmd/split-output -in '$(NEW)' -out '$(OUTPUT)'
