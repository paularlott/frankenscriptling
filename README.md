# FrankenScriptling

A Docker image combining [FrankenPHP](https://frankenphp.dev/) with the [Scriptling](https://github.com/paularlott/scriptling) scripting language, exposed as a PHP class.

Pre-built images are available at [hub.docker.com/r/paularlott/frankenscriptling](https://hub.docker.com/r/paularlott/frankenscriptling):

```bash
docker pull paularlott/frankenscriptling:latest          # newest release, newest PHP line
docker pull paularlott/frankenscriptling:php8.4          # newest release for PHP 8.4
docker pull paularlott/frankenscriptling:0.33.0-php8.5   # a fixed release, pin this in production
```

See [Image tags](#image-tags) for what each tag means and which ones move.

## Quick Start

```bash
# Build and push all PHP versions in parallel (docker buildx bake)
make

# Build a specific PHP version
make frankenscriptling-8.5.12

# Build and push a test build from an uncommitted checkout (0.0.0-dev.<sha> tags only)
make dev-push

# Print the resolved bake configuration without building
make print

# Run tests against the default PHP version
make test
```

## Usage

```php
<?php
$vm = new Scriptling();

// Evaluate scriptling code
$result = $vm->eval("2 + 3");          // "5"
$result = $vm->eval("[1, 2, 3]");      // "[1, 2, 3]"
$result = $vm->eval("{'name': 'Alice', 'age': 30}");  // "{'name': Alice, 'age': 30}"

// With timeout (milliseconds)
$result = $vm->evalWithTimeout(5000, "some_long_computation()");

// From a file
$result = $vm->evalFile("/app/scripts/myscript.py");
```

## Variables

```php
// Set variables
$vm->setVar("name", "Alice");
$vm->setVarInt("count", 42);
$vm->setVarFloat("pi", 3.14159);
$vm->setVarBool("active", true);
$vm->setVarNull("empty");
$vm->setVarJSON("data", '{"x": 1, "y": [2, 3]}');
$vm->setVarList("items", '[1, 2, 3]');
$vm->setVarDict("config", '{"host": "localhost", "port": 8080}');

// Get variables
$name   = $vm->getVar("name");           // "Alice" (string representation)
$name   = $vm->getVarAsString("name");   // "Alice" (as string)
$count  = $vm->getVarInt("count");       // 42
$pi     = $vm->getVarFloat("pi");        // 3.14159
$active = $vm->getVarBool("active");     // true
$data   = $vm->getVarJSON("data");       // '{"x":1,"y":[2,3]}'

// Check and manage
$exists = $vm->hasVar("name");           // true
$vars   = $vm->listVars();               // JSON array of variable names
$vm->unsetVar("name");
$vm->reset();                            // Clear all user variables
```

Variables are available in scriptling code:

```php
$vm->setVarInt("x", 10);
$vm->setVarInt("y", 20);
echo $vm->eval("x + y");  // "30"
```

## Libraries

### Script Libraries

Register libraries written in the scriptling language:

```php
$vm->registerScriptLibrary("mymath", '
def add(a, b):
    return a + b

def multiply(a, b):
    return a * b
');
$vm->import("mymath");

echo $vm->eval("mymath.add(10, 20)");        // "30"
echo $vm->eval("mymath.multiply(5, 6)");      // "30"
```

### Import Multiple Libraries

```php
$vm->registerScriptLibrary("lib1", '...');
$vm->registerScriptLibrary("lib2", '...');
$vm->importMultiple('["lib1", "lib2"]');
```

### Register Functions

```php
$vm->registerScriptFunc("square", "lambda x: x * x");
echo $vm->eval("square(7)");  // "49"
```

## Function Calling

Call registered or defined functions directly:

```php
$vm->registerScriptFunc("add", "lambda a, b: a + b");

// Arguments passed as JSON array
$result = $vm->callFunction("add", "[10, 20]");  // "30"

// No arguments
$vm->registerScriptFunc("hello", 'lambda: "Hello!"');
$result = $vm->callFunction("hello", "");         // "Hello!"
```

## Autoload Path

Load libraries from the filesystem automatically. **The autoload path must be set before the first `eval()`/`import()`/`callFunction()` call**, because the VM is lazily initialized on first use:

```php
$vm = new Scriptling();
$vm->setAutoloadPath("/app/libs");  // MUST be set before any eval/import
$vm->import("mylib");               // loads /app/libs/mylib.py
```

The filesystem loader follows Python conventions:

- `import foo` → looks for `/app/libs/foo.py` or `/app/libs/foo/__init__.py`
- `import foo.bar` → looks for `/app/libs/foo/bar.py` or `/app/libs/foo.bar.py`

You can add multiple search paths:

```php
$vm->addAutoloadPath("/app/custom-libs");
$vm->addAutoloadPath("/app/shared-libs");
```

Directories are searched in order — first match wins.

**Important:** Call `setAutoloadPath` or `addAutoloadPath` immediately after `new Scriptling()`, before any `eval()`, `import()`, or other VM calls. The VM is created lazily on first use, so the autoload path must be configured first:

```php
// Correct
$vm = new Scriptling();
$vm->setAutoloadPath("/app/libs");
$vm->import("mylib");  // works

// Wrong
$vm = new Scriptling();
$vm->eval("1 + 1");           // VM created here, no loader set
$vm->setAutoloadPath("/app/libs"); // too late
```

## Output Capture

```php
$vm->enableOutputCapture();
$vm->eval('print("Hello from scriptling!")');
$output = $vm->getOutput();  // "Hello from scriptling!"
```

## Resource Limits

The operator sets per-VM resource limits with `SCRIPTLING_MEMORY_LIMIT` and
`SCRIPTLING_MAX_VALUE_BYTES` (bytes, unset or `0` = no limit), or with a
`[limits]` table in `SCRIPTLING_POLICY_FILE` — see
[docs/security-policy.md](docs/security-policy.md#resource-limits). PHP can
read them, and lower them for one VM, but never raise or remove them:

```php
$vm->setMemoryLimit(16 * 1024 * 1024);   // estimated memory the VM may hold
$vm->setMaxValueBytes(1024 * 1024);      // largest single string/bytes/container
$vm->eval('x = "a" * 2000000');          // fails: MemoryError
$usage = json_decode($vm->getResourceUsage(), true);  // memory_bytes, peak_memory_bytes, ...
```

A VM that goes over its memory limit has the running call cancelled with an
error containing `memory limit exceeded`; the VM stays usable afterwards.

## Error Handling

```php
$result = $vm->eval("1/0");
if ($vm->hasError()) {
    echo "Error: " . $vm->getLastError();  // "division by zero (line 1)"
}
$vm->clearError();
```

## API Reference

### Evaluation

| Method            | Parameters                     | Return   | Description              |
| ----------------- | ------------------------------ | -------- | ------------------------ |
| `eval`            | `string $code`                 | `string` | Evaluate scriptling code |
| `evalWithTimeout` | `int $timeoutMs, string $code` | `string` | Evaluate with timeout    |
| `evalFile`        | `string $path`                 | `string` | Evaluate a .sling file   |

### Variables

| Method           | Parameters                    | Return   | Description                  |
| ---------------- | ----------------------------- | -------- | ---------------------------- |
| `setVar`         | `string $name, string $value` | `void`   | Set string variable          |
| `setVarInt`      | `string $name, int $value`    | `void`   | Set integer variable         |
| `setVarFloat`    | `string $name, float $value`  | `void`   | Set float variable           |
| `setVarBool`     | `string $name, bool $value`   | `void`   | Set boolean variable         |
| `setVarNull`     | `string $name`                | `void`   | Set null variable            |
| `setVarJSON`     | `string $name, string $json`  | `void`   | Set from JSON value          |
| `setVarList`     | `string $name, string $json`  | `void`   | Set list from JSON array     |
| `setVarDict`     | `string $name, string $json`  | `void`   | Set dict from JSON object    |
| `getVar`         | `string $name`                | `string` | Get string representation    |
| `getVarAsString` | `string $name`                | `string` | Get as string                |
| `getVarInt`      | `string $name`                | `int`    | Get as integer               |
| `getVarFloat`    | `string $name`                | `float`  | Get as float                 |
| `getVarBool`     | `string $name`                | `bool`   | Get as boolean               |
| `getVarJSON`     | `string $name`                | `string` | Get as JSON string           |
| `hasVar`         | `string $name`                | `bool`   | Check if variable exists     |
| `listVars`       |                               | `string` | JSON array of variable names |
| `unsetVar`       | `string $name`                | `void`   | Delete a variable            |
| `reset`          |                               | `void`   | Clear all user variables     |

### Libraries

| Method                  | Parameters                     | Return | Description                            |
| ----------------------- | ------------------------------ | ------ | -------------------------------------- |
| `registerScriptLibrary` | `string $name, string $source` | `void` | Register a script library              |
| `registerScriptFunc`    | `string $name, string $script` | `void` | Register a script function             |
| `import`                | `string $name`                 | `void` | Import a library                       |
| `importMultiple`        | `string $names`                | `void` | Import multiple libraries (JSON array) |

### Functions

| Method         | Parameters                       | Return   | Description                    |
| -------------- | -------------------------------- | -------- | ------------------------------ |
| `callFunction` | `string $name, string $argsJSON` | `string` | Call a function with JSON args |

### Autoload Path

| Method            | Parameters     | Return   | Description                                     |
| ----------------- | -------------- | -------- | ----------------------------------------------- |
| `setAutoloadPath` | `string $path` | `void`   | Set directory for filesystem library loading    |
| `addAutoloadPath` | `string $path` | `void`   | Add additional directory to the autoload search |
| `getAutoloadPath` |                | `string` | Get current autoload path(s)                    |

### Output

| Method                | Parameters | Return   | Description            |
| --------------------- | ---------- | -------- | ---------------------- |
| `enableOutputCapture` |            | `void`   | Capture print() output |
| `getOutput`           |            | `string` | Get captured output    |

### Resource Limits

| Method             | Parameters   | Return   | Description                                                                  |
| ------------------ | ------------ | -------- | ---------------------------------------------------------------------------- |
| `setMemoryLimit`   | `int $bytes` | `void`   | Lower this VM's memory limit (cannot exceed the operator's limit)            |
| `getMemoryLimit`   |              | `int`    | Memory limit in force, `0` = none                                            |
| `setMaxValueBytes` | `int $bytes` | `void`   | Lower this VM's single-value limit (cannot exceed the operator's limit)      |
| `getMaxValueBytes` |              | `int`    | Value limit in force, `0` = none                                             |
| `getResourceUsage` |              | `string` | JSON: `memory_bytes`, `peak_memory_bytes`, `allocated_bytes`, `memory_limit_cancellations` |

### Error Handling

| Method         | Parameters | Return   | Description                     |
| -------------- | ---------- | -------- | ------------------------------- |
| `hasError`     |            | `bool`   | Check if last operation errored |
| `getLastError` |            | `string` | Get last error message          |
| `clearError`   |            | `void`   | Clear error state               |

### Info

| Method                 | Parameters | Return   | Description                |
| ---------------------- | ---------- | -------- | -------------------------- |
| `getScriptlingVersion` |            | `string` | Scriptling library version                                              |
| `getImageVersion`      |            | `string` | Version of this image (its tag; `0.0.0-dev` outside a release build)    |
| `getBuildInfo`         |            | `string` | JSON: `image`, `revision`, `scriptling`, `frankenphp`, `php` versions    |

## Architecture

```
PHP Request
    |
    v
FrankenPHP (PHP 8.5 + Caddy)
    |
    v
Scriptling PHP Class (FrankenPHP extension)
    |
    v
Scriptling Go VM (github.com/paularlott/scriptling)
```

Each `new Scriptling()` creates an isolated VM with its own environment. The VM is lazily initialized on first use, and all standard and extension libraries are registered automatically.

## Build Configuration

Builds go through the Makefile, which is the one place that sets versions and
exports them to `docker-bake.hcl`; a bare `docker buildx bake` fails on
purpose. Override any variable via the environment, a local `.env` file
(gitignored), or the command line.

| Variable             | Default         | Description                                                  |
| -------------------- | --------------- | ------------------------------------------------------------ |
| `TAG_BASE`           | `paularlott`    | Registry/namespace for image tags                            |
| `CACHE_TAG_BASE`     | `$(TAG_BASE)`   | Registry for the build cache                                 |
| `FRANKENPHP_VERSION` | `1.13.2`        | FrankenPHP version                                           |
| `SCRIPTLING_VERSION` | `v0.33.0`       | Scriptling version                                           |
| `PHP_VERSIONS`       | `8.4.27 8.5.12` | Space-separated PHP versions (built in parallel)             |
| `VERSION`            | from git        | Image version: the tag on HEAD (`v0.33.0` becomes `0.33.0`)  |

`make version` prints what a build would use. `make all` and
`make frankenscriptling-<php>` build and push a release and refuse to run
unless HEAD is a clean checkout at a version tag; `make dev` builds the
default PHP version for this machine as a local image with a `0.0.0-dev.<sha>`
version and no push.

### Image tags

The image has its own version, independent of the scriptling version inside
it: a FrankenPHP security rebuild or an extension fix is a new image version
with the same scriptling. What went into a build is recorded on the image as
labels (`org.opencontainers.image.version`, `org.opencontainers.image.revision`,
`dev.scriptling.scriptling-version`, `dev.scriptling.frankenphp-version`,
`dev.scriptling.php-version`, shown by `docker inspect`) and reported from
inside by `Scriptling::getBuildInfo()`.

| Tag                  | Moves? | Meaning                                                   |
| -------------------- | ------ | --------------------------------------------------------- |
| `0.33.0-php8.5.12`   | no     | release 0.33.0 on exactly PHP 8.5.12; built once           |
| `0.33.0-php8.5`      | no     | the same build, named by PHP line                          |
| `php8.5`, `php8.4`   | yes    | the newest release for that PHP line                       |
| `latest`             | yes    | the newest release on the newest PHP line                  |

Builder images are tagged the way FrankenPHP tags its own
(`dunglas/frankenphp:1.13.2-builder-php8.4`):

| Tag                        | Moves? | Meaning                                              |
| -------------------------- | ------ | ---------------------------------------------------- |
| `0.33.0-builder-php8.5.12` | no     | the builder for `0.33.0-php8.5.12`                   |
| `0.33.0-builder-php8.5`    | no     | the same build, named by PHP line                    |
| `builder-php8.5`           | yes    | the builder for the newest release on that PHP line  |
| `builder`                  | yes    | the builder for `latest`                             |

A builder is the build stage with libphp, xcaddy and the prepared extension
source. Use one to add your own Caddy modules without copying this build:

```dockerfile
FROM paularlott/frankenscriptling:0.33.0-builder-php8.5 AS builder
RUN build-frankenscriptling --with github.com/acme/caddy-module@v1.2.3 \
    && setcap cap_net_bind_service=+ep /usr/local/bin/frankenphp

FROM paularlott/frankenscriptling:0.33.0-php8.5
COPY --from=builder /usr/local/bin/frankenphp /usr/local/bin/frankenphp
```

The new binary has no capabilities of its own, so `setcap` it in the builder
stage: `COPY` keeps the capability (letting a non-root user bind ports 80/443),
whereas a `setcap` after the `COPY` stores the binary a second time in a new
layer.

`build-frankenscriptling` uses the same Caddy modules and build tags as the
published binary (Mercure, Vulcain, cbrotli, Cloudflare DNS,
transform-encoder) and appends its arguments to `xcaddy build`. Pair a builder
with the runtime tag of the same release so libphp matches.

Pin a fixed tag (or a digest, `paularlott/frankenscriptling@sha256:…`) in
production; use the floating tags to track releases. Versions bump by patch
for rebuilds and extension fixes, by minor for a scriptling upgrade, and by
major if the PHP API or the security-policy semantics change; `CHANGELOG.md`
maps each image version to the scriptling and FrankenPHP versions in it. The
numbering starts at 0.33.0, matching the scriptling release it ships;
earlier `<scriptling>-php<php>` tags were retired when the scheme changed.

To release: tag the commit (`git tag v0.33.0`), then `make all`.

## Image and PHP extensions

The runtime image is the official FrankenPHP image rebuilt on a bare
`debian:trixie-slim`: the same PHP (`/usr/local`, plus the PHP source in
`/usr/src` for building extensions), the Scriptling-enabled `frankenphp`, and
only the Debian libraries they link against. It leaves out the compiler
toolchain the official image keeps (`$PHPIZE_DEPS`, ~350MB) and that image's
own FrankenPHP binary, so it is less than half the size (about 470MB against
1GB). Everything else matches: environment (`PHP_INI_DIR`, `PHPIZE_DEPS`, …),
entrypoint, `/app`, ports, health check.

Two differences from the official image:

- `frankenphp` already has `cap_net_bind_service`, and `/config` and `/data`
  belong to `www-data`, so `USER www-data` is all it takes to run without root
  (FrankenPHP's rootless recipe works too, but its `setcap` copies the binary
  into a new layer).
- There is no compiler in the image. `install-php-extensions` and
  `docker-php-ext-configure` / `-install` / `-enable` install the toolchain
  themselves when they need it and remove it afterwards, keeping only the
  libraries the new extensions link against — the official Alpine images do
  the same. A toolchain you installed yourself is left alone.

So the usual ways of adding extensions work unchanged:

```dockerfile
FROM paularlott/frankenscriptling:php8.5

# FrankenPHP docs: core and PECL extensions alike
RUN install-php-extensions pdo_mysql gd intl zip redis xdebug

# Official PHP image docs: core extensions
RUN apt-get update && apt-get install -y libfreetype-dev libjpeg62-turbo-dev libpng-dev \
    && docker-php-ext-configure gd --with-freetype --with-jpeg \
    && docker-php-ext-install -j$(nproc) gd
```

Keep `docker-php-ext-configure` and `docker-php-ext-install` in the same `RUN`:
in separate ones the toolchain stays in the configure layer.

A bare `pecl install` (or `phpize` on your own sources) has no such hook and
fails with `Cannot find autoconf`, as on the official Alpine images. Use
`install-php-extensions` for PECL extensions, or install the toolchain in the
same `RUN` and remove it yourself:

```dockerfile
RUN apt-get update && apt-get install -y $PHPIZE_DEPS \
    && pecl install redis && docker-php-ext-enable redis \
    && apt-get purge -y --auto-remove $PHPIZE_DEPS && rm -rf /var/lib/apt/lists/*
```

## Security Policy

Scripts only get filesystem or network access for libraries a vhost
explicitly enables — everything fs/net-capable is closed by default. Set
`SCRIPTLING_ENABLED_LIBRARIES`, `SCRIPTLING_ALLOWED_PATHS`, and/or
`SCRIPTLING_NETWORK_POLICY_FILE` (globally or per Caddy site block via the
`env` directive) to open specific libraries up. See
[docs/security-policy.md](docs/security-policy.md) for the full reference,
including which libraries are gated, excluded, and the network policy file
format.

`scriptling.plugin` is separate again: `SCRIPTLING_PLUGIN`/`SCRIPTLING_PLUGIN_DIR`
let the operator pre-load specific plugin binaries, and scripts can only use
what got pre-loaded this way — `load()`/`unload()` always fail unless
`SCRIPTLING_PLUGIN_HTTP_ENABLED` opts into new HTTP(S)-only plugin loading
under the same network policy. See
[Plugins](docs/security-policy.md#plugins-scriptlingplugin) for details.

## Built-in Libraries

All Scriptling VM instances come pre-loaded with the following libraries. Use `import <name>` to access them.

### Standard Libraries

| Library     | Import Name    | Description                                                        |
| ----------- | -------------- | ------------------------------------------------------------------ |
| JSON        | `json`         | JSON encoding/decoding (`json.dumps`, `json.loads`)                |
| Regex       | `re`           | Regular expressions (`re.match`, `re.findall`, `re.sub`)           |
| Time        | `time`         | Time functions (`time.time`, `time.sleep`, `time.strftime`)        |
| Datetime    | `datetime`     | Date/time objects (`datetime.now`, `datetime.timedelta`)           |
| Math        | `math`         | Math functions (`math.sqrt`, `math.pi`, `math.sin`)                |
| Base64      | `base64`       | Base64 encoding (`base64.b64encode`, `base64.b64decode`)           |
| Hashlib     | `hashlib`      | Hashing (`hashlib.sha256`, `hashlib.md5`)                          |
| Random      | `random`       | Random numbers (`random.randint`, `random.choice`)                 |
| URL Lib     | `urllib`       | URL utilities                                                      |
| URL Parse   | `urllib.parse` | URL parsing (`urllib.parse.quote`, `urllib.parse.urlencode`)       |
| String      | `string`       | String constants (`string.ascii_letters`, `string.digits`)         |
| UUID        | `uuid`         | UUID generation (`uuid.uuid4`)                                     |
| HTML        | `html`         | HTML utilities (`html.escape`, `html.unescape`)                    |
| Statistics  | `statistics`   | Statistical functions (`statistics.mean`, `statistics.stdev`)      |
| Functools   | `functools`    | Higher-order functions (`functools.partial`, `functools.reduce`)   |
| Textwrap    | `textwrap`     | Text wrapping and indentation                                      |
| Platform    | `platform`     | Platform information                                               |
| Itertools   | `itertools`    | Iterator functions (`itertools.count`, `itertools.chain`)          |
| Collections | `collections`  | Data structures (`collections.Counter`, `collections.defaultdict`) |
| IO          | `io`           | I/O streams                                                        |
| Contextlib  | `contextlib`   | Context manager utilities                                          |
| Difflib     | `difflib`      | Diffing sequences (`difflib.unified_diff`)                         |

### Extension Libraries

| Library           | Import Name                    | Description                                                  |
| ----------------- | ------------------------------ | ------------------------------------------------------------ |
| TOML              | `toml`                         | TOML parsing/generation (`toml.loads`, `toml.dumps`)         |
| YAML              | `yaml`                         | YAML parsing/generation (`yaml.safe_load`, `yaml.safe_dump`) |
| HTML Parser       | `html.parser`                  | HTML tag parsing                                             |
| Logging           | `logging`                      | Structured logging                                           |
| Sys               | `sys`                          | Interpreter/platform info (`sys.argv`, `sys.platform`)       |
| Secrets           | `secrets`                      | Cryptographically strong random values                       |
| Shlex             | `shlex`                        | Shell-like string splitting/quoting                          |
| CSV               | `scriptling.csv`               | CSV parsing/generation                                       |
| XML               | `scriptling.xml`               | XML marshal/unmarshal                                        |
| Markdown          | `scriptling.markdown`          | Markdown to HTML rendering                                    |
| AI                | `scriptling.ai`                | AI/LLM provider integration (network-gated, see [Security Policy](#security-policy)) |
| AI Agent          | `scriptling.ai.agent`          | AI agent framework with tool calling                         |
| AI Agent Interact | `scriptling.ai.agent.interact` | Interactive agent sessions                                   |
| AI Memory         | `scriptling.ai.memory`         | Persistent memory for AI agents                              |
| MCP               | `scriptling.mcp`               | Model Context Protocol tool interaction (network-gated)      |
| TOON              | `scriptling.toon`              | TOON (Token-Oriented Object Notation) encoding               |
| Similarity        | `scriptling.similarity`        | Fuzzy matching and MinHash similarity search                 |
| HTML Templates    | `scriptling.template.html`     | HTML template rendering                                      |
| Text Templates    | `scriptling.template.text`     | Text template rendering                                      |

Filesystem- and network-capable libraries (`pathlib`, `os`, `fs`, `glob`,
`shutil`, `tempfile`, `tarfile`, `zipfile`, `scriptling.find`,
`scriptling.grep`, `scriptling.sed`, `requests`,
`scriptling.net.websocket`, `scriptling.net.resolve`, `subprocess`) are
**not** in this table — they're closed by default and must be enabled per the
[Security Policy](#security-policy) section above. `scriptling.plugin` is
gated the same way but by its own env vars
(`SCRIPTLING_PLUGIN`/`SCRIPTLING_PLUGIN_DIR`/`SCRIPTLING_PLUGIN_HTTP_ENABLED`),
also documented there.

### Example

```php
$vm = new Scriptling();

// JSON
echo $vm->eval('import json; json.dumps({"key": "value"})');

// Math
echo $vm->eval('import math; math.sqrt(144)');  // "12"

// TOML
echo $vm->eval('import toml; toml.loads("[db]\nhost = \\"localhost\\"")["db"]["host"]');  // "localhost"

// YAML
echo $vm->eval('import yaml; yaml.safe_load("name: test\nage: 42")["name"]');  // "test"

// Regex
echo $vm->eval('import re; re.findall(r"\\d+", "abc123def456")');  // "[123, 456]"
```

## Extensions Included

- **Scriptling** - Full Scriptling VM integration class

The binary also carries FrankenPHP's standard Caddy modules, including the
[Mercure hub](https://frankenphp.dev/docs/mercure/) (`mercure` directive and
`mercure_publish()`), built with the same `deprecated_topic`/`deprecated_claim`
tags as the official image.

## Notes

- The Scriptling language uses Python-like syntax (e.g., `True`/`False`, `def`, `lambda`, `import`)
- The `.` operator in scriptling is attribute access (like Python), not string concatenation
- Complex data (lists, dicts) is passed via JSON strings
- Each `Scriptling` instance is independent - no shared state between PHP objects
- FrankenPHP is built from a patched copy so the `Scriptling` class survives config reloads (admin API `/load`, `--watch`); stock FrankenPHP only registers Go extensions on the first PHP startup ([php/frankenphp#2572](https://github.com/php/frankenphp/issues/2572)). Drop the patch in the `Dockerfile` once that is fixed upstream.
