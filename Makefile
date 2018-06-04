SHELL="/bin/bash"
PLATFORM=$(shell go env GOOS)
ARCH=$(shell go env GOARCH)
GOPATH=$(shell go env GOPATH)
GOBIN=$(GOPATH)/bin

default: build validate test

get-deps:
	go get -u golang.org/x/lint/golint honnef.co/go/tools/cmd/megacheck

build:
	go fmt ./...
	DEP_BUILD_PLATFORMS=$(PLATFORM) DEP_BUILD_ARCHS=$(ARCH) ./hack/build-all.bash
	cp ./release/dep-$(PLATFORM)-$(ARCH) dep

licenseok:
	go build ./hack/licenseok

validate: build licenseok
	./hack/lint.bash
	./hack/validate-vendor.bash
	./hack/validate-licence.bash

test:
	./hack/test.bash

install: build
	cp ./dep $(GOBIN)

docusaurus:
	docker run --rm -it -v `pwd`:/dep -p 3000:3000 \
		-w /dep/website node \
		bash -c "npm i --only=dev && npm start"

.PHONY: build validate test install docusaurus
