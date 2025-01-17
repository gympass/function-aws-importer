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
	docker build . --tag=acme.com/runtime/func -v $(shell pwd):/fn --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64
	rm package/*.xpkg; crossplane xpkg build -f package --embed-runtime-image=acme.com/runtime/func
	crossplane xpkg push -f package/*.xpkg $(FUNCTION_REGISTRY)/function-aws-importer:dev

prep-code:
	go generate ./...
	go fmt ./...
	go vet ./...
