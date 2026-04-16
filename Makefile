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
		echo "Using Docker buildx (parallel linux/amd64 + linux/arm64)..."; \
		docker buildx build --platform linux/amd64 --output=type=docker,dest=package/runtime-amd64.tar . & \
		_amd64_pid=$$!; \
		docker buildx build --platform linux/arm64 --output=type=docker,dest=package/runtime-arm64.tar . & \
		_arm64_pid=$$!; \
		_ec=0; wait $$_amd64_pid || _ec=1; wait $$_arm64_pid || _ec=1; \
		exit $$_ec; \
	else \
		echo "Using Podman (parallel linux/amd64 + linux/arm64)..."; \
		podman build --platform linux/amd64 --tag acme.com/runtime/func:amd64 . & \
		_amd64_pid=$$!; \
		podman build --platform linux/arm64 --tag acme.com/runtime/func:arm64 . & \
		_arm64_pid=$$!; \
		_ec=0; wait $$_amd64_pid || _ec=1; wait $$_arm64_pid || _ec=1; \
		if [ $$_ec -ne 0 ]; then exit $$_ec; fi; \
		podman save acme.com/runtime/func:amd64 -o package/runtime-amd64.tar & \
		_save_amd64_pid=$$!; \
		podman save acme.com/runtime/func:arm64 -o package/runtime-arm64.tar & \
		_save_arm64_pid=$$!; \
		wait $$_save_amd64_pid || _ec=1; wait $$_save_arm64_pid || _ec=1; \
		exit $$_ec; \
	fi
	rm -f package/*.xpkg
	crossplane xpkg build -f package --embed-runtime-image-tarball=package/runtime-amd64.tar -o package/function-amd64.xpkg
	crossplane xpkg build -f package --embed-runtime-image-tarball=package/runtime-arm64.tar -o package/function-arm64.xpkg
	crossplane xpkg --verbose push -f package/function-amd64.xpkg,package/function-arm64.xpkg $(FUNCTION_REGISTRY)/function-aws-importer:dev

prep-code:
	go generate ./...
	go fmt ./...
	go vet ./...
