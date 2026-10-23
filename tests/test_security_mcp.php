<?php

// scriptling.mcp is network-gated, but a stdio MCP server is a subprocess
// the script names and starts. It must be refused unless the subprocess
// library is enabled too, and the HTTP transport must keep working. Run via
// `make test-security-mcp` (SCRIPTLING_ENABLED_LIBRARIES=scriptling.mcp with
// a network policy and subprocess NOT enabled).

$pass = 0;
$fail = 0;
$errors = [];

function assert_true($test, $label) {
    global $pass, $fail, $errors;
    if ($test === true) { $pass++; } else { $fail++; $errors[] = "FAIL $label: got " . var_export($test, true); }
}

$vm = new Scriptling();

echo "=== Security Policy: scriptling.mcp stdio ===\n";

$vm->eval("import scriptling.mcp as m");
assert_true(!$vm->hasError(), "mcp: library imports when enabled with a network policy");

$marker = "/tmp/frankenscriptling-mcp-spawned";
@unlink($marker);
$vm->clearError();
$vm->eval("m.Client('/bin/sh', args=['-c', 'touch $marker'])");
assert_true($vm->hasError(), "mcp: stdio server refused when subprocess is not enabled");
assert_true(str_contains($vm->getLastError(), "subprocess"), "mcp: refusal names subprocess");
usleep(200000);
assert_true(!file_exists($marker), "mcp: no process was started");

$vm->clearError();
$vm->eval("import subprocess");
assert_true($vm->hasError(), "mcp: subprocess itself is not enabled in this run");

$vm->clearError();
$vm->eval("m.Client('http://127.0.0.1:9/mcp')");
assert_true(!str_contains($vm->getLastError(), "subprocess"), "mcp: http target is not refused as a subprocess");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
