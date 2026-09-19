# FrankenScriptling

A Docker image combining [FrankenPHP](https://frankenphp.dev/) with the [Scriptling](https://github.com/paularlott/scriptling) scripting language, exposed as a PHP class.

Pre-built images are available at [hub.docker.com/r/paularlott/frankenscriptling](https://hub.docker.com/r/paularlott/frankenscriptling).

## Quick Start

```bash
# Build and push all PHP versions in parallel (docker buildx bake)
make

# Build a specific PHP version
make frankenscriptling-8.5.9

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

### Error Handling

| Method         | Parameters | Return   | Description                     |
| -------------- | ---------- | -------- | ------------------------------- |
| `hasError`     |            | `bool`   | Check if last operation errored |
| `getLastError` |            | `string` | Get last error message          |
| `clearError`   |            | `void`   | Clear error state               |

### Info

| Method                 | Parameters | Return   | Description                |
| ---------------------- | ---------- | -------- | -------------------------- |
| `getScriptlingVersion` |            | `string` | Scriptling library version |

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

Builds are driven by `docker-bake.hcl` via the Makefile. Override any variable via the environment, a local `.env` file (gitignored), or the command line.

| Variable             | Default        | Description                                        |
| -------------------- | -------------- | -------------------------------------------------- |
| `TAG_BASE`           | `paularlott`   | Registry/namespace for image tags                  |
| `CACHE_TAG_BASE`     | `$(TAG_BASE)`  | Registry for the build cache                       |
| `FRANKENPHP_VERSION` | `1.12.7`       | FrankenPHP version                                 |
| `SCRIPTLING_VERSION` | `v0.25.3`      | Scriptling version                                 |
| `PHP_VERSIONS`       | `8.4.24 8.5.9` | Space-separated PHP versions (built in parallel)   |

Images are tagged `<scriptling>-php<php>` and `<scriptling>-php<major.minor>` (e.g. `0.25.3-php8.5.9` and `0.25.3-php8.5`).

## Security Policy

Scripts only get filesystem or network access for libraries a vhost
explicitly enables — everything fs/net-capable is closed by default. Set
`SCRIPTLING_ENABLED_LIBRARIES`, `SCRIPTLING_ALLOWED_PATHS`, and/or
`SCRIPTLING_NETWORK_POLICY_FILE` (globally or per Caddy site block via the
`env` directive) to open specific libraries up. See
[docs/security-policy.md](docs/security-policy.md) for the full reference,
including which libraries are gated, excluded, and the network policy file
format.

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
[Security Policy](#security-policy) section above.

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

## Notes

- The Scriptling language uses Python-like syntax (e.g., `True`/`False`, `def`, `lambda`, `import`)
- The `.` operator in scriptling is attribute access (like Python), not string concatenation
- Complex data (lists, dicts) is passed via JSON strings
- Each `Scriptling` instance is independent - no shared state between PHP objects
