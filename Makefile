test: prep-code
	go test ./...

run: prep-code
	go run . --insecure --debug

render:
	crossplane render example/xr.yaml example/composition.yaml example/functions.yaml -r

build-and-push-dev: prep-code
ifndef FUNCTION_REGISTRY
	$(error FUNCTION_REGISTRY env var is undefined)
endif
	# acme.com is a hack to make sure crossplane xpkg doens't think this is a dockerhub image
	# preventing: crossplane: error: failed to build package: failed to mutate config for image: Error response from daemon: docker.io/localhost/runtime/func:latest: image not known
	# Detect if we're using Docker or Podman and use appropriate commands
	@if docker version 2>/dev/null | grep -q "Docker Engine"; then \
		echo "Using Docker buildx..."; \
		docker buildx build --platform linux/amd64 --output=type=docker,dest=package/runtime-amd64.tar .; \
		docker buildx build --platform linux/arm64 --output=type=docker,dest=package/runtime-arm64.tar .; \
	else \
		echo "Using Podman..."; \
		podman build --platform linux/amd64 --tag acme.com/runtime/func:amd64 .; \
		podman build --platform linux/arm64 --tag acme.com/runtime/func:arm64 .; \
		podman save acme.com/runtime/func:amd64 -o package/runtime-amd64.tar; \
		podman save acme.com/runtime/func:arm64 -o package/runtime-arm64.tar; \
	fi
	rm -f package/*.xpkg
	crossplane xpkg build -f package --embed-runtime-image-tarball=package/runtime-amd64.tar -o package/function-amd64.xpkg
	crossplane xpkg build -f package --embed-runtime-image-tarball=package/runtime-arm64.tar -o package/function-arm64.xpkg
	crossplane xpkg --verbose push -f package/function-amd64.xpkg,package/function-arm64.xpkg $(FUNCTION_REGISTRY)/function-aws-importer:dev

prep-code:
	go generate ./...
	go fmt ./...
	go vet ./...
