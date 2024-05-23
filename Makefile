test:
	go generate ./...
	go test ./...

run:
	go generate ./...
	go run . --insecure --debug

render:
	crossplane beta render example/xr.yaml example/composition.yaml example/functions.yaml -r

build-and-push-dev:
ifndef FUNCTION_REGISTRY
	$(error FUNCTION_REGISTRY env var is undefined)
endif
	go generate ./...
	docker build . --tag=runtime
	rm package/*.xpkg; crossplane xpkg build -f package --embed-runtime-image=runtime
	crossplane xpkg push -f package/*.xpkg $(FUNCTION_REGISTRY)/function-aws-importer:dev

