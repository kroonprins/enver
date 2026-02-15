.PHONY: build test test-unit test-e2e test-e2e-update clean setup-kind

# Build the binary
build:
	go build -o enver .

# Run all tests
test: test-unit test-e2e

# Run unit tests (excludes e2e)
test-unit:
	go test ./... -v -skip "^TestE2E"

# Run e2e tests (requires Kind cluster named 'kind')
test-e2e:
	@echo "Running E2E tests (requires Kind cluster 'kind-kind')..."
	@cd tests/e2e && E2E_TEST=1 go test -v -timeout 5m

# Run e2e tests and update golden files
test-e2e-update:
	@echo "Running E2E tests and updating golden files..."
	@cd tests/e2e && E2E_TEST=1 go test -v -timeout 5m -update

# Clean build artifacts
clean:
	rm -f enver
	rm -f tests/e2e/enver-test
	rm -rf tests/e2e/testdata/output

# Setup Kind cluster for e2e tests
setup-kind:
	@if ! kind get clusters | grep -q "^kind$$"; then \
		echo "Creating Kind cluster..."; \
		kind create cluster --name kind; \
	else \
		echo "Kind cluster 'kind' already exists"; \
	fi

# Teardown Kind cluster
teardown-kind:
	kind delete cluster --name kind
