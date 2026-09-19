ARG FRANKENPHP_VERSION=1.12.3
ARG PHP_VERSION=8.5.6
ARG GO_MODULE=example.com/frankenscriptling
ARG SCRIPTLING_VERSION=v0.8.1

FROM dunglas/frankenphp:${FRANKENPHP_VERSION}-builder-php${PHP_VERSION} AS builder

ARG FRANKENPHP_VERSION=1.12.3
ARG PHP_VERSION=8.5.6
ARG GO_MODULE=example.com/frankenscriptling
ARG SCRIPTLING_VERSION=v0.8.1

COPY --from=caddy:builder /usr/bin/xcaddy /usr/bin/xcaddy

WORKDIR /app

COPY scriptling_ext.go security.go ./

ENV GOTOOLCHAIN=auto

RUN go mod init ${GO_MODULE} \
    && go get github.com/dunglas/frankenphp@v${FRANKENPHP_VERSION} \
    && go get github.com/paularlott/scriptling@${SCRIPTLING_VERSION} \
    && go mod tidy

ADD https://www.php.net/distributions/php-${PHP_VERSION}.tar.gz /tmp/php.tar.gz
RUN tar xzf /tmp/php.tar.gz -C /tmp

RUN GEN_STUB_SCRIPT=/tmp/php-${PHP_VERSION}/build/gen_stub.php \
    frankenphp extension-init scriptling_ext.go

# FrankenPHP v1.12.2 extension generator bug: when a class method returns int/float/bool,
# the generated C code incorrectly casts ALL parameters based on the return type instead of
# the parameter type. For example, evalInt(string $code): int generates (long)code instead
# of passing code as zend_string*. This sed fixes the affected methods.
RUN sed -i \
    -e 's/getVarInt_wrapper(intern->go_handle, (long)name)/getVarInt_wrapper(intern->go_handle, name)/g' \
    -e 's/getVarFloat_wrapper(intern->go_handle, (double)name)/getVarFloat_wrapper(intern->go_handle, name)/g' \
    -e 's/getVarBool_wrapper(intern->go_handle, (int)name)/getVarBool_wrapper(intern->go_handle, name)/g' \
    -e 's/hasVar_wrapper(intern->go_handle, (int)name)/hasVar_wrapper(intern->go_handle, name)/g' \
    scriptling_ext.c

RUN CGO_ENABLED=1 \
    XCADDY_GO_BUILD_FLAGS="-ldflags '-w -s' -tags nobadger,nomysql,nopgx" \
    CGO_CFLAGS="-D_GNU_SOURCE $(php-config --includes)" \
    CGO_LDFLAGS="$(php-config --ldflags) $(php-config --libs)" \
    xcaddy build \
    --output /usr/local/bin/frankenphp \
    --with github.com/dunglas/frankenphp/caddy@v${FRANKENPHP_VERSION} \
    --with github.com/dunglas/mercure/caddy@latest \
    --with github.com/dunglas/vulcain/caddy@latest \
    --with github.com/dunglas/caddy-cbrotli@latest \
    --with github.com/caddy-dns/cloudflare@latest \
    --with github.com/caddyserver/transform-encoder@latest \
    --with ${GO_MODULE}=/app

# Test-only: real, already-tested scriptling plugin binaries, built as
# fixtures for the security-policy plugin tests (make build-test-plugin).
# Three binaries: the original stdio fixture (directory-scanned by
# SCRIPTLING_PLUGIN_DIR in most tests), a second, distinctly-named stdio
# fixture (loaded via an explicit SCRIPTLING_PLUGIN path, to prove the two
# env vars combine), and an HTTP-mode fixture (a real http(s) JSON-RPC
# endpoint, for SCRIPTLING_PLUGIN=http(s)://... and for a script-initiated
# load() the network policy actually allows). The HTTP fixture is exported
# to its own directory, never the one SCRIPTLING_PLUGIN_DIR scans — a
# directory scan spawns every executable it finds as a stdio JSON-RPC peer,
# and this one is an HTTP listener, not a stdio peer.
# Sits between builder and runtime deliberately — the LAST stage in this
# file is what a target-less build (`docker build .` / bake with no
# `target` field) produces, and that must always be `runtime` below, never
# this test-only one.
FROM builder AS test-plugin
ARG SCRIPTLING_VERSION=v0.8.1
COPY tests/fixtures/plugin-src/ /test-plugin-src/
RUN cd /test-plugin-src \
    && go mod init frankenscriptling-test-plugin \
    && go get github.com/paularlott/scriptling@${SCRIPTLING_VERSION} \
    && go mod tidy \
    && mkdir -p /out/plugins /out/plugins-explicit /out/plugins-http \
    && CGO_ENABLED=0 go build -o /out/plugins/test-plugin . \
    && CGO_ENABLED=0 go build -o /out/plugins-explicit/second-plugin ./second-plugin \
    && CGO_ENABLED=0 go build -o /out/plugins-http/http-plugin ./http-plugin

FROM scratch AS test-plugin-export
COPY --from=test-plugin /out/plugins/test-plugin /plugins/test-plugin
COPY --from=test-plugin /out/plugins-explicit/second-plugin /plugins-explicit/second-plugin
COPY --from=test-plugin /out/plugins-http/http-plugin /plugins-http/http-plugin

FROM dunglas/frankenphp:${FRANKENPHP_VERSION}-php${PHP_VERSION} AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends procps vim \
    && rm -rf /var/lib/apt/lists/*

COPY Caddyfile /etc/frankenphp/Caddyfile

COPY --from=builder /usr/local/bin/frankenphp /usr/local/bin/frankenphp
