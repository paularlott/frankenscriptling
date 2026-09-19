<?php

// Smoke checks for the security-policy code paths not covered by
// test_security_open.php: explicit deny-all ("-"), disable-only libraries,
// and multi-file network policy merging. Run via `make test-security-edge`.

$pass = 0;
$fail = 0;
$errors = [];

function assert_true($test, $label) {
    global $pass, $fail, $errors;
    if ($test === true) { $pass++; } else { $fail++; $errors[] = "FAIL $label: got " . var_export($test, true); }
}

$vm = new Scriptling();

echo "=== Security Policy: edge cases ===\n";

// SCRIPTLING_ALLOWED_PATHS="-" and SCRIPTLING_ENABLED_LIBRARIES=os,subprocess,requests,
// SCRIPTLING_NETWORK_POLICY_FILE=open-network-policy.toml,second-network-policy.toml
$vm->eval("import os.path");
assert_true(!$vm->hasError(), "edge: os.path imports (enabled) even with deny-all paths");

$vm->clearError();
$vm->eval('os.path.exists("/app/tests")');
assert_true($vm->hasError(), 'edge: explicit "-" allowed_paths still denies everything');

$vm->clearError();
$vm->eval("import subprocess");
assert_true(!$vm->hasError(), "edge: subprocess imports when explicitly enabled");

$vm->clearError();
$vm->eval("import requests");
assert_true(!$vm->hasError(), "edge: requests imports with merged multi-file network policy");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
