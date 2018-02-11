SHELL="/bin/bash"

default: build validate test

get-deps:
	go get -u github.com/golang/lint/golint honnef.co/go/tools/cmd/megacheck

build:
	go fmt ./...
	go build ./cmd/dep

licenseok:
	go build ./hack/licenseok

validate: build licenseok
	./hack/lint.bash
	./hack/validate-vendor.bash
	./hack/validate-licence.bash

test:
	go test -i ./...

install:
	go install ./cmd/dep

docusaurus:
	docker run --rm -it -v `pwd`:/dep -p 3000:3000 \
		-w /dep/website node \
		bash -c "npm i --only=dev && npm start"

.PHONY: build validate test install docusaurus
