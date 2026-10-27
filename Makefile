# Include optional .env file
-include .env

# =============================================================================
# Build configuration (override via env, .env, or command line)
# =============================================================================

TAG_BASE ?= paularlott
CACHE_TAG_BASE ?= $(TAG_BASE)
FRANKENPHP_VERSION ?= 1.13.4
# Go toolchain for the FrankenPHP/Scriptling build (GOTOOLCHAIN=go<version>:
# used exactly, never switched by a dependency)
GO_VERSION ?= 1.27.3
SCRIPTLING_VERSION ?= v0.33.0
PHP_VERSIONS ?= 8.4.27 8.5.12
PHP_VERSION := $(firstword $(PHP_VERSIONS))
# The newest PHP line gets the floating `latest` tag.
PHP_LATEST := $(lastword $(shell printf '%s\n' $(PHP_VERSIONS) | sort -V))

# =============================================================================
# Image version: the git tag on HEAD (v0.33.0 -> 0.33.0). An untagged or
# dirty checkout gets a dev version that can never collide with a release,
# and `all` / `frankenscriptling-%` refuse to build and push it (see
# check-release). Images are tagged <version>-php<php>, <version>-php<x.y>,
# php<x.y> and, for the newest PHP line, latest; see README "Image tags".
# =============================================================================

GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null)
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GIT_DIRTY := $(shell git status --porcelain 2>/dev/null | grep -q . && echo -dirty)
VERSION ?= $(if $(GIT_TAG),$(patsubst v%,%,$(GIT_TAG)),0.0.0-dev.$(GIT_SHA))
REVISION := $(GIT_SHA)$(GIT_DIRTY)

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
export GO_VERSION
export SCRIPTLING_VERSION
export PHP_LATEST
export VERSION
export REVISION
export PHP_VERSIONS := $(subst $(space),$(comma),$(PHP_VERSIONS))

# Optional extra flags passed to bake (e.g. make BAKE_FLAGS=--print)
BAKE_FLAGS ?=

.DEFAULT_GOAL := all

.PHONY: check-release
## Refuse to build a release image from an untagged or dirty checkout
check-release:
	@if [ -z "$(GIT_TAG)" ] || [ -n "$(GIT_DIRTY)" ]; then \
		echo "error: release builds need a clean checkout at a version tag (git tag v0.33.0); HEAD is $(REVISION), version would be $(VERSION)" >&2; \
		exit 1; \
	fi

.PHONY: all
## Build and push every PHP version of release $(VERSION) (docker buildx bake, in parallel)
all: check-release
	docker buildx bake $(BAKE_FLAGS)

.PHONY: dev
## Build the default PHP version for this machine only, as a local image, no push, no release tag needed
dev:
	docker buildx bake $(BAKE_FLAGS) \
		--set '*.platform=$(shell docker version -f '{{.Server.Os}}/{{.Server.Arch}}')' \
		--set '*.output=type=docker' \
		frankenscriptling-$(subst .,-,$(PHP_VERSION)) \
		frankenscriptling-builder-$(subst .,-,$(PHP_VERSION))

.PHONY: dev-push
## Build and push every PHP version as 0.0.0-dev.<sha>[-dirty] for testing: no release tag needed, floating tags untouched
dev-push: export VERSION := 0.0.0-dev.$(REVISION)
dev-push:
	docker buildx bake $(BAKE_FLAGS)

.PHONY: version
## Print the image version, revision and the dependency versions this build would use
version:
	@echo "version:            $(VERSION)"
	@echo "revision:           $(REVISION)"
	@echo "scriptling:         $(SCRIPTLING_VERSION)"
	@echo "frankenphp:         $(FRANKENPHP_VERSION)"
	@echo "go:                 $(GO_VERSION)"
	@echo "php:                $(PHP_VERSIONS) (latest: $(PHP_LATEST))"

.PHONY: print
## Print the resolved bake configuration without building
print:
	docker buildx bake --print

.PHONY: list
## List available bake targets
list:
	docker buildx bake --list=targets

.PHONY: frankenscriptling-%
## Build and push a specific PHP version of the release and its builder (e.g. make frankenscriptling-8.5.12 or frankenscriptling-8-5-12)
frankenscriptling-%: check-release
	docker buildx bake $(BAKE_FLAGS) frankenscriptling-$(subst .,-,$*) frankenscriptling-builder-$(subst .,-,$*)

.PHONY: build-test-plugin
## Build the real scriptling plugin binaries used as fixtures by the plugin security tests
build-test-plugin:
	docker buildx build \
		--target test-plugin-export \
		--build-arg FRANKENPHP_VERSION=$(FRANKENPHP_VERSION) \
		--build-arg GO_VERSION=$(GO_VERSION) \
		--build-arg PHP_VERSION=$(PHP_VERSION) \
		--build-arg SCRIPTLING_VERSION=$(SCRIPTLING_VERSION) \
		--build-arg IMAGE_VERSION=$(VERSION) \
		--build-arg IMAGE_REVISION=$(REVISION) \
		--output type=local,dest=tests/fixtures \
		.
	chmod +x tests/fixtures/plugins/test-plugin tests/fixtures/plugins-explicit/second-plugin tests/fixtures/plugins-http/http-plugin

.PHONY: test
## Run PHP tests in the container (uses the pushed image for the default PHP version)
test:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_all.php

.PHONY: test-php-functions
## Run the PHP-functions tests: calls, conversions, exceptions, scope, leaks, then exit() and fatal errors (each must end the process cleanly)
test-php-functions:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_php_functions.php
	@for spec in \
		"exit:3:QUITTING:NEVER AFTER" \
		"exit_in_try:3:QUITTING:NEVER AFTER" \
		"fatal:255:Allowed memory size:NEVER AFTER CAUGHT" \
		"fatal_caught:255:Allowed memory size:NEVER AFTER CAUGHT" \
		"fatal_timeout:255:Maximum execution time:NEVER AFTER" \
	; do \
		mode=$${spec%%:*}; rest=$${spec#*:}; want=$${rest%%:*}; rest=$${rest#*:}; expect=$${rest%%:*}; refuse=$${rest#*:}; \
		out=$$(docker run --rm -v $(CURDIR)/tests:/app/tests:ro \
			$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
			frankenphp php-cli /app/tests/test_php_functions_exit.php $$mode 2>&1); code=$$?; \
		ok=1; [ "$$code" = "$$want" ] || ok=0; \
		echo "$$out" | grep -q "$$expect" || ok=0; \
		echo "$$out" | grep -q "SHUTDOWN" || ok=0; \
		for word in $$refuse; do echo "$$out" | grep -qx "$$word" && ok=0; done; \
		if [ $$ok = 1 ]; then echo "PASS $$mode (exit $$code)"; \
		else echo "FAIL $$mode: exit $$code (want $$want), output:"; echo "$$out" | sed 's/^/    /'; exit 1; fi; \
	done

.PHONY: test-php-functions-server
## Run the PHP-functions server tests: thousands of concurrent requests in classic and worker mode, each must get its own data back and worker memory must not grow
test-php-functions-server:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		sh -c 'frankenphp php-server --listen 127.0.0.1:8080 --root /app/tests/web >/tmp/classic.log 2>&1 & \
			frankenphp php-server --listen 127.0.0.1:8081 --root /app/tests/web --worker /app/tests/web/php_functions_worker.php >/tmp/worker.log 2>&1 & \
			sleep 2; rc=0; \
			frankenphp php-cli /app/tests/test_php_functions_server.php http://127.0.0.1:8080/php_functions.php 3000 48 0 || rc=1; \
			frankenphp php-cli /app/tests/test_php_functions_server.php http://127.0.0.1:8081/php_functions_worker.php 5000 48 1 || rc=1; \
			if grep -qi "panic\|SIGSEGV\|SIGABRT\|heap corrupted" /tmp/classic.log /tmp/worker.log; then echo "FAIL server log:"; tail -30 /tmp/classic.log /tmp/worker.log; rc=1; fi; \
			exit $$rc'

.PHONY: test-security
## Run security-policy tests with fs/net libraries enabled and restricted
test-security:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ALLOWED_PATHS=/app/tests \
		-e SCRIPTLING_ENABLED_LIBRARIES=os,pathlib,requests \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/open-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_open.php

.PHONY: test-security-edge
## Run security-policy edge case tests (deny-all sentinel, disable-only libs, multi-file policy merge)
test-security-edge:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ALLOWED_PATHS=- \
		-e SCRIPTLING_ENABLED_LIBRARIES=os,subprocess,requests \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/open-network-policy.toml,/app/tests/fixtures/second-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
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
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_adversarial.php

.PHONY: test-security-failclosed
## Verify a malformed security policy locks out the whole VM instead of falling back open
test-security-failclosed:
	@for envs in \
		"SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/malformed-network-policy.toml" \
		"SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/malformed-syntax.toml" \
		"SCRIPTLING_POLICY_FILE=/app/tests/fixtures/malformed-syntax.toml" \
		"SCRIPTLING_SECRET_PROVIDER=vault" \
		"SCRIPTLING_PLUGIN=/app/tests/fixtures/plugins/does-not-exist" \
		"SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins-does-not-exist" \
		"SCRIPTLING_MEMORY_LIMIT=64MB" \
		"SCRIPTLING_MAX_VALUE_BYTES=-1" \
	; do \
		echo "--- $$envs ---"; \
		docker run --rm -v $(CURDIR)/tests:/app/tests:ro $$(printf -- '-e %s ' $$envs) \
			$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
			frankenphp php-cli /app/tests/test_security_failclosed.php || exit 1; \
	done

.PHONY: test-limits
## Run resource-limit tests with limits set by env vars, then by a policy file
test-limits:
	@for envs in \
		"SCRIPTLING_MEMORY_LIMIT=67108864 SCRIPTLING_MAX_VALUE_BYTES=1048576" \
		"SCRIPTLING_POLICY_FILE=/app/tests/fixtures/limits-policy.toml" \
	; do \
		echo "--- $$envs ---"; \
		docker run --rm -v $(CURDIR)/tests:/app/tests:ro $$(printf -- '-e %s ' $$envs) \
			$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
			frankenphp php-cli /app/tests/test_limits.php || exit 1; \
	done

.PHONY: test-security-plugins
## Run admin-supplied plugin tests (build-test-plugin first): happy path (list/describe/call_function) and unhappy path (load/unload always fail)
test-security-plugins:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_plugins.php

.PHONY: test-security-plugins-http
## Run opt-in HTTP plugin-loading tests: still gated by the network policy, stdio/exec still impossible
test-security-plugins-http:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN_HTTP_ENABLED=true \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/permissive-ip-literals-policy.toml,/app/tests/fixtures/deny-hosts-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_plugins_http.php

.PHONY: test-security-plugins-explicit
## Run SCRIPTLING_PLUGIN tests: a single explicit plugin path, as an alternative to SCRIPTLING_PLUGIN_DIR
test-security-plugins-explicit:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN=/app/tests/fixtures/plugins/test-plugin \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_plugins_explicit.php

.PHONY: test-security-plugins-combined
## Run the combined scenario: SCRIPTLING_PLUGIN_DIR preload + SCRIPTLING_PLUGIN_HTTP_ENABLED at once
test-security-plugins-combined:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins \
		-e SCRIPTLING_PLUGIN_HTTP_ENABLED=true \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/permissive-ip-literals-policy.toml,/app/tests/fixtures/deny-hosts-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_plugins_combined.php

.PHONY: test-security-plugins-explicit-http
## Run SCRIPTLING_PLUGIN=http(s)://... tests: the admin's own preload can be a real HTTP endpoint, not just a local path
test-security-plugins-explicit-http:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN=http://127.0.0.1:8199/json-rpc \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		sh -c '/app/tests/fixtures/plugins-http/http-plugin -addr 127.0.0.1:8199 -path /json-rpc >/tmp/http-plugin.log 2>&1 & sleep 0.5 && frankenphp php-cli /app/tests/test_security_plugins_explicit_http.php'

.PHONY: test-security-plugins-dual
## Run the SCRIPTLING_PLUGIN + SCRIPTLING_PLUGIN_DIR combined test: two distinct plugins preloaded from two different sources at once
test-security-plugins-dual:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN=/app/tests/fixtures/plugins-explicit/second-plugin \
		-e SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_plugins_dual.php

.PHONY: test-security-plugins-http-allowed
## Run the positive opt-in-HTTP-loading test: a policy-permitted load() actually succeeds and is callable, not just that a denied one fails
test-security-plugins-http-allowed:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN_HTTP_ENABLED=true \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/allow-loopback-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		sh -c '/app/tests/fixtures/plugins-http/http-plugin -addr 127.0.0.1:8199 -path /json-rpc >/tmp/http-plugin.log 2>&1 & sleep 0.5 && frankenphp php-cli /app/tests/test_security_plugins_http_allowed.php'

.PHONY: test-security-mcp
## Run the scriptling.mcp test: stdio servers refused unless subprocess is enabled
test-security-mcp:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.mcp \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/open-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_mcp.php

.PHONY: test-all-security
## Run every security-policy test suite
test-all-security: test-limits test-security test-security-edge test-security-mcp test-security-adversarial test-security-failclosed test-security-plugins test-security-plugins-http test-security-plugins-explicit test-security-plugins-combined test-security-plugins-explicit-http test-security-plugins-dual test-security-plugins-http-allowed

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
