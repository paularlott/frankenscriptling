# Include optional .env file
-include .env

# =============================================================================
# Build configuration (override via env, .env, or command line)
# =============================================================================

TAG_BASE ?= paularlott
CACHE_TAG_BASE ?= $(TAG_BASE)
FRANKENPHP_VERSION ?= 1.12.7
SCRIPTLING_VERSION ?= v0.25.2
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

.PHONY: test-security
## Run security-policy tests with fs/net libraries enabled and restricted
test-security:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ALLOWED_PATHS=/app/tests \
		-e SCRIPTLING_ENABLED_LIBRARIES=os,pathlib,requests \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/open-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_open.php

.PHONY: test-security-edge
## Run security-policy edge case tests (deny-all sentinel, disable-only libs, multi-file policy merge)
test-security-edge:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ALLOWED_PATHS=- \
		-e SCRIPTLING_ENABLED_LIBRARIES=os,subprocess,requests \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/open-network-policy.toml,/app/tests/fixtures/second-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_edge.php

.PHONY: test-security-adversarial
## Run adversarial security tests: path traversal, symlink escape, SSRF-style
## targets, deny_hosts precedence, and every excluded library trying to enable itself
test-security-adversarial:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ALLOWED_PATHS=/app/tests \
		-e SCRIPTLING_ENABLED_LIBRARIES=os,pathlib,requests,subprocess,scriptling.secret,scriptling.wait_for,scriptling.container,scriptling.plugin,scriptling.valkey,scriptling.badgerdb,scriptling.sql,scriptling.sqlite,scriptling.net.gossip,scriptling.net.multicast,scriptling.net.unicast,scriptling.provision.file,scriptling.provision.fetch,scriptling.runtime.sandbox,scriptling.runtime.plugin,scriptling.nomad,scriptling.messaging.console \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/permissive-ip-literals-policy.toml,/app/tests/fixtures/deny-hosts-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_adversarial.php

.PHONY: test-security-failclosed
## Verify a malformed security policy locks out the whole VM instead of falling back open
test-security-failclosed:
	@for envs in \
		"SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/malformed-network-policy.toml" \
		"SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/malformed-syntax.toml" \
		"SCRIPTLING_POLICY_FILE=/app/tests/fixtures/malformed-syntax.toml" \
		"SCRIPTLING_SECRET_PROVIDER=vault" \
	; do \
		echo "--- $$envs ---"; \
		docker run --rm -v $(CURDIR)/tests:/app/tests:ro $$(printf -- '-e %s ' $$envs) \
			$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
			frankenphp php-cli /app/tests/test_security_failclosed.php || exit 1; \
	done

.PHONY: test-all-security
## Run every security-policy test suite
test-all-security: test-security test-security-edge test-security-adversarial test-security-failclosed

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
