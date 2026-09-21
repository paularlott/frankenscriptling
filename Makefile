# Include optional .env file
-include .env

# =============================================================================
# Build configuration (override via env, .env, or command line)
# =============================================================================

TAG_BASE ?= paularlott
CACHE_TAG_BASE ?= $(TAG_BASE)
FRANKENPHP_VERSION ?= 1.12.7
SCRIPTLING_VERSION ?= v0.26.0
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

.PHONY: build-test-plugin
## Build the real scriptling plugin binaries used as fixtures by the plugin security tests
build-test-plugin:
	docker buildx build \
		--target test-plugin-export \
		--build-arg FRANKENPHP_VERSION=$(FRANKENPHP_VERSION) \
		--build-arg PHP_VERSION=$(PHP_VERSION) \
		--build-arg SCRIPTLING_VERSION=$(SCRIPTLING_VERSION) \
		--output type=local,dest=tests/fixtures \
		.
	chmod +x tests/fixtures/plugins/test-plugin tests/fixtures/plugins-explicit/second-plugin tests/fixtures/plugins-http/http-plugin

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
		"SCRIPTLING_PLUGIN=/app/tests/fixtures/plugins/does-not-exist" \
		"SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins-does-not-exist" \
	; do \
		echo "--- $$envs ---"; \
		docker run --rm -v $(CURDIR)/tests:/app/tests:ro $$(printf -- '-e %s ' $$envs) \
			$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
			frankenphp php-cli /app/tests/test_security_failclosed.php || exit 1; \
	done

.PHONY: test-security-plugins
## Run admin-supplied plugin tests (build-test-plugin first): happy path (list/describe/call_function) and unhappy path (load/unload always fail)
test-security-plugins:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_plugins.php

.PHONY: test-security-plugins-http
## Run opt-in HTTP plugin-loading tests: still gated by the network policy, stdio/exec still impossible
test-security-plugins-http:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN_HTTP_ENABLED=true \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/permissive-ip-literals-policy.toml,/app/tests/fixtures/deny-hosts-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_plugins_http.php

.PHONY: test-security-plugins-explicit
## Run SCRIPTLING_PLUGIN tests: a single explicit plugin path, as an alternative to SCRIPTLING_PLUGIN_DIR
test-security-plugins-explicit:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN=/app/tests/fixtures/plugins/test-plugin \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
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
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_plugins_combined.php

.PHONY: test-security-plugins-explicit-http
## Run SCRIPTLING_PLUGIN=http(s)://... tests: the admin's own preload can be a real HTTP endpoint, not just a local path
test-security-plugins-explicit-http:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN=http://127.0.0.1:8199/json-rpc \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		sh -c '/app/tests/fixtures/plugins-http/http-plugin -addr 127.0.0.1:8199 -path /json-rpc >/tmp/http-plugin.log 2>&1 & sleep 0.5 && frankenphp php-cli /app/tests/test_security_plugins_explicit_http.php'

.PHONY: test-security-plugins-dual
## Run the SCRIPTLING_PLUGIN + SCRIPTLING_PLUGIN_DIR combined test: two distinct plugins preloaded from two different sources at once
test-security-plugins-dual:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN=/app/tests/fixtures/plugins-explicit/second-plugin \
		-e SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		frankenphp php-cli /app/tests/test_security_plugins_dual.php

.PHONY: test-security-plugins-http-allowed
## Run the positive opt-in-HTTP-loading test: a policy-permitted load() actually succeeds and is callable, not just that a denied one fails
test-security-plugins-http-allowed:
	docker run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		-e SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin \
		-e SCRIPTLING_PLUGIN_HTTP_ENABLED=true \
		-e SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/allow-loopback-network-policy.toml \
		$(TAG_BASE)/frankenscriptling:$(FRANKENPHP_VERSION)-php$(PHP_VERSION) \
		sh -c '/app/tests/fixtures/plugins-http/http-plugin -addr 127.0.0.1:8199 -path /json-rpc >/tmp/http-plugin.log 2>&1 & sleep 0.5 && frankenphp php-cli /app/tests/test_security_plugins_http_allowed.php'

.PHONY: test-all-security
## Run every security-policy test suite
test-all-security: test-security test-security-edge test-security-adversarial test-security-failclosed test-security-plugins test-security-plugins-http test-security-plugins-explicit test-security-plugins-combined test-security-plugins-explicit-http test-security-plugins-dual test-security-plugins-http-allowed

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
