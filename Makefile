OLD ?= output.json
NEW ?= output.json
RECOVERED ?= recovered-missing-books.json
TARGETS ?= topreads-missing-books-to-double-check.json
PRIORITY ?= P0
LIMIT ?= 0
DELAY ?= 3s
WORKERS ?= 1

.PHONY: test compare-missing recover-missing merge-outputs crawl-qa

test:
	go test ./...

compare-missing:
	go run ./cmd/compare-missing -old '$(OLD)' -new '$(NEW)' -out '$(TARGETS)'

recover-missing:
	go run ./cmd/recover-missing -targets '$(TARGETS)' -priority '$(PRIORITY)' -limit $(LIMIT) \
		-workers $(WORKERS) -delay $(DELAY) -out '$(RECOVERED)' -keep-previous-on-fail

merge-outputs:
	go run ./cmd/merge-outputs -new '$(NEW)' -recovered '$(RECOVERED)' -out output.merged.json

crawl-qa:
	go run ./cmd/crawl-qa -old '$(OLD)' -new output.merged.json -recovered '$(RECOVERED)'
