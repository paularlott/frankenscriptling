# Include optional .env file
-include .env

# =============================================================================
# Build configuration (override via env, .env, or command line)
# =============================================================================

TAG_BASE ?= paularlott
CACHE_TAG_BASE ?= $(TAG_BASE)
FRANKENPHP_VERSION ?= 1.12.7
SCRIPTLING_VERSION ?= v0.23.1
PHP_VERSIONS ?= 8.4.24 8.5.9
PHP_VERSION := $(firstword $(PHP_VERSIONS))

# =============================================================================
# Internal helpers — bake parses list-typed env vars as CSV, so we convert
# Make's space-separated lists before exporting.
# =============================================================================

empty :=
space := $(empty) $(empty)
comma := ,

export TAG_BASE
export CACHE_TAG_BASE
export FRANKENPHP_VERSION
export SCRIPTLING_VERSION
export PHP_VERSIONS := $(subst $(space),$(comma),$(PHP_VERSIONS))

# Optional extra flags passed to bake (e.g. make BAKE_FLAGS=--print)
BAKE_FLAGS ?=

.DEFAULT_GOAL := all

.PHONY: all
## Build all PHP versions (uses docker buildx bake for parallel builds)
all:
	docker buildx bake $(BAKE_FLAGS)

.PHONY: print
## Print the resolved bake configuration without building
print:
	docker buildx bake --print

.PHONY: list
## List available bake targets
list:
	docker buildx bake --list=targets

.PHONY: frankenscriptling-%
## Build a specific PHP version (e.g. make frankenscriptling-8.5.9 or frankenscriptling-8-5-9)
frankenscriptling-%:
	docker buildx bake $(BAKE_FLAGS) frankenscriptling-$(subst .,-,$*)

.PHONY: test
## Run PHP tests in the container (uses the pushed image for the default PHP version)
test:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_all.php

.PHONY: help
## This help screen
help:
	@printf "Available targets:\n\n"
	@awk '/^[a-zA-Z\-_0-9%:\\]+/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = $$1; \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			gsub("\\\\", "", helpCommand); \
			gsub(":+$$", "", helpCommand); \
			printf "  \x1b[32;01m%-20s\x1b[0m %s\n", helpCommand, helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST) | sort -u
	@printf "\nBake targets (invoke as 'make frankenscriptling-<php>'):\n\n"
	@docker buildx bake --list=targets 2>/dev/null | tail -n +2 || true
	@printf "\n"
