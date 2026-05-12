# FrankenScriptling

A Docker image combining [FrankenPHP](https://frankenphp.dev/) with the [Scriptling](https://github.com/paularlott/scriptling) scripting language, exposed as a PHP class.

## Quick Start

```bash
# Build for local architecture (Apple Containers)
make build-apple

# Build multi-arch (Docker buildx)
make build-docker

# Build and push multi-arch
make build-docker-push
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

| Variable             | Default             | Description        |
| -------------------- | ------------------- | ------------------ |
| `FRANKENPHP_VERSION` | `1.12.2`            | FrankenPHP version |
| `PHP_VERSION`        | `8.5.6`             | PHP version        |
| `GO_VERSION`         | `1.26.3`            | Go version         |
| `SCRIPTLING_VERSION` | `v0.8.0`            | Scriptling version |
| `IMAGE_NAME`         | `frankenscriptling` | Docker image name  |
| `IMAGE_TAG`          | `1.12.2`            | Docker image tag   |

## Built-in Libraries

All Scriptling VM instances come pre-loaded with the following libraries. Use `import <name>` to access them.

### Standard Libraries

| Library | Import Name | Description |
|---|---|---|
| JSON | `json` | JSON encoding/decoding (`json.dumps`, `json.loads`) |
| Regex | `re` | Regular expressions (`re.match`, `re.findall`, `re.sub`) |
| Time | `time` | Time functions (`time.time`, `time.sleep`, `time.strftime`) |
| Datetime | `datetime` | Date/time objects (`datetime.now`, `datetime.timedelta`) |
| Math | `math` | Math functions (`math.sqrt`, `math.pi`, `math.sin`) |
| Base64 | `base64` | Base64 encoding (`base64.b64encode`, `base64.b64decode`) |
| Hashlib | `hashlib` | Hashing (`hashlib.sha256`, `hashlib.md5`) |
| Random | `random` | Random numbers (`random.randint`, `random.choice`) |
| URL Lib | `urllib` | URL utilities |
| URL Parse | `urllib.parse` | URL parsing (`urllib.parse.quote`, `urllib.parse.urlencode`) |
| String | `string` | String constants (`string.ascii_letters`, `string.digits`) |
| UUID | `uuid` | UUID generation (`uuid.uuid4`) |
| HTML | `html` | HTML utilities (`html.escape`, `html.unescape`) |
| Statistics | `statistics` | Statistical functions (`statistics.mean`, `statistics.stdev`) |
| Functools | `functools` | Higher-order functions (`functools.partial`, `functools.reduce`) |
| Textwrap | `textwrap` | Text wrapping and indentation |
| Platform | `platform` | Platform information |
| Itertools | `itertools` | Iterator functions (`itertools.count`, `itertools.chain`) |
| Collections | `collections` | Data structures (`collections.Counter`, `collections.defaultdict`) |
| IO | `io` | I/O streams |
| Contextlib | `contextlib` | Context manager utilities |
| Difflib | `difflib` | Diffing sequences (`difflib.unified_diff`) |

### Extension Libraries

| Library | Import Name | Description |
|---|---|---|
| TOML | `toml` | TOML parsing/generation (`toml.loads`, `toml.dumps`) |
| YAML | `yaml` | YAML parsing/generation (`yaml.safe_load`, `yaml.safe_dump`) |
| AI | `scriptling.ai` | AI/LLM provider integration (OpenAI, etc.) |
| AI Agent | `scriptling.ai.agent` | AI agent framework with tool calling |
| AI Agent Interact | `scriptling.ai.agent.interact` | Interactive agent sessions |
| AI Memory | `scriptling.ai.memory` | Persistent memory for AI agents |
| MCP | `scriptling.mcp` | Model Context Protocol tool interaction |
| TOON | `scriptling.toon` | TOON (Token-Oriented Object Notation) encoding |
| Similarity | `scriptling.similarity` | Fuzzy matching and MinHash similarity search |
| HTML Templates | `scriptling.template.html` | HTML template rendering |
| Text Templates | `scriptling.template.text` | Text template rendering |

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
