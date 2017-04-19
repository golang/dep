HELL="/bin/bash"

GOFILES_NOVENDOR = $(shell go list ./... | grep -v /vendor/)

default:
	go build ./cmd/dep

validate:
	go fmt $(GOFILES_NOVENDOR)
	go vet $(GOFILES_NOVENDOR)

test: validate
	go test -race $(GOFILES_NOVENDOR)

.PHONY: default validate test
