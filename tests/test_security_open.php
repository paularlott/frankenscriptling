<?php

// Run only via `make test-security`, which sets:
//   SCRIPTLING_ALLOWED_PATHS=/app/tests
//   SCRIPTLING_ENABLED_LIBRARIES=os,pathlib,requests
// Verifies that enabling a library actually grants access within the
// configured policy, and that the policy's own safe defaults still hold
// (a loopback network request stays blocked even though `requests` is on).

$pass = 0;
$fail = 0;
$errors = [];

function assert_true($test, $label) {
    global $pass, $fail, $errors;
    if ($test === true) {
        $pass++;
    } else {
        $fail++;
        $errors[] = "FAIL $label: expected true, got " . var_export($test, true);
    }
}

function assert_contains($haystack, $needle, $label) {
    global $pass, $fail, $errors;
    if (str_contains($haystack, $needle)) {
        $pass++;
    } else {
        $fail++;
        $errors[] = "FAIL $label: expected '$needle' in '$haystack'";
    }
}

$vm = new Scriptling();

echo "=== Security Policy: enabled + restricted ===\n";

$vm->eval("import os.path");
assert_true(!$vm->hasError(), "security: os.path imports when enabled");

// /app/tests is the configured allowed path — a file inside it is visible.
$result = $vm->eval('os.path.exists("/app/tests/test_security_open.php")');
assert_true($result === "True", "security: allowed_paths permits path inside allow-list");

// Anything outside the allow-list must be denied, not merely absent.
$vm->clearError();
$vm->eval('os.path.exists("/etc/passwd")');
assert_true($vm->hasError(), "security: allowed_paths denies path outside allow-list");

$vm->clearError();
$vm->eval("import requests");
assert_true(!$vm->hasError(), "security: requests imports when enabled");

// requests is enabled, but the network policy's safe defaults still block
// loopback/IP-literal targets unless a policy file explicitly allows them.
$vm->clearError();
$vm->eval('requests.get("http://127.0.0.1:9/")');
assert_true($vm->hasError(), "security: enabling requests doesn't bypass safe network defaults");
assert_contains($vm->getLastError(), "network policy", "security: blocked request reports a network policy error");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";

if ($fail > 0) {
    echo "\nFailures:\n";
    foreach ($errors as $e) {
        echo "  $e\n";
    }
    exit(1);
}

echo "All tests passed!\n";
exit(0);
