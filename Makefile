default: lint

lint:
	golangci-lint run ./...

publish:
	bash ci/build.sh
