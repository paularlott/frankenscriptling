<?php

// Verifies that a malformed security policy fails the whole VM closed,
// rather than silently falling back to "no restrictions" for the part that
// didn't parse. Run via `make test-security-failclosed`, which exercises
// this file once per malformed config (bad CIDR, invalid TOML syntax,
// incomplete secret-provider config).

$pass = 0;
$fail = 0;
$errors = [];

function assert_true($test, $label) {
    global $pass, $fail, $errors;
    if ($test === true) { $pass++; } else { $fail++; $errors[] = "FAIL $label: got " . var_export($test, true); }
}

echo "=== Fail-closed on malformed policy ===\n";

$vm = new Scriptling();

// Even a trivial, library-free eval must fail — the whole VM is locked out,
// not just the network/secret piece that failed to parse.
$vm->eval("1 + 1");
assert_true($vm->hasError(), "failclosed: trivial eval fails when policy config is malformed");
assert_true(strlen($vm->getLastError()) > 0, "failclosed: error message is non-empty");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
