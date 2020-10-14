TEST_PACKAGES?=$$(go list ./... |grep -Ev 'node_modules|vendor')
GO_FILES?=$$(find . -name '*.go' |grep -Ev 'node_modules|vendor')
PKG_NAME=github.com/labd/terraform-provider-serverless
PROVIDER_VERSION_PLACEHOLDER=version.ProviderVersion
LATEST_COMMIT=$(shell git describe --tags --always)

default: build

# build:
# 	go install -ldflags="-X $(PKG_NAME)/$(PROVIDER_VERSION_PLACEHOLDER)=$(LATEST_COMMIT)"

build:
	go build -o terraform-provider-serverless
	cp terraform-provider-serverless ~/.terraform.d/plugins/registry.terraform.io/labd/serverless/0.2.0/darwin_amd64/.

testacc:
	TF_ACC=1 go test $(TEST_PACKAGES) -v $(TESTARGS) -timeout 120m -ldflags="-X=$(PKG_NAME)/$(PROVIDER_VERSION_PLACEHOLDER)=acc"

fmt:
	gofmt -w $(GO_FILES)

lint:
	golangci-lint run .

.PHONY: build testacc fmt lint

