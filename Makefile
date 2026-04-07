TARGETS := build package release test validate validate-ci version

$(TARGETS):
	./scripts/$@

ci:
	docker buildx build --target artifacts --output=. -f Dockerfile.local .
	./scripts/package

.DEFAULT_GOAL := ci

.PHONY: $(TARGETS) ci
