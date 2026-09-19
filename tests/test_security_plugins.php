<?php

// Happy + unhappy path for admin-supplied plugins. Run via
// `make test-security-plugins`, which sets:
//   SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin
//   SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins
// (the fixture binary is built by `make build-test-plugin` first).
//
// The whole point: an admin pre-loading a trusted plugin must NOT also hand
// scripts a way to load a *different* plugin — load()/unload() must fail
// unconditionally, even though list/describe/call_function work fine.

$pass = 0;
$fail = 0;
$errors = [];

function assert_true($test, $label) {
    global $pass, $fail, $errors;
    if ($test === true) { $pass++; } else { $fail++; $errors[] = "FAIL $label: got " . var_export($test, true); }
}

function assert_contains($haystack, $needle, $label) {
    global $pass, $fail, $errors;
    if (str_contains($haystack, $needle)) { $pass++; }
    else { $fail++; $errors[] = "FAIL $label: expected '$needle' in '$haystack'"; }
}

$vm = new Scriptling();

echo "=== Plugins: happy path (admin pre-loaded, scripts may use it) ===\n";

$vm->eval("import scriptling.plugin as plugin");
assert_true(!$vm->hasError(), "plugins: scriptling.plugin imports when enabled and a plugin dir is configured");

$vm->clearError();
$listResult = $vm->eval("plugin.list()");
assert_true(!$vm->hasError(), "plugins: list() works");
assert_contains($listResult, "frankenscriptling-test-plugin", "plugins: list() shows the admin-preloaded plugin");

$vm->clearError();
$describeResult = $vm->eval('plugin.describe("frankenscriptling-test-plugin")');
assert_true(!$vm->hasError(), "plugins: describe() works");
assert_contains($describeResult, "1.0.0", "plugins: describe() reports the plugin's declared version");

$vm->clearError();
$callResult = $vm->eval('plugin.call_function("frankenscriptling-test-plugin", "greet", "Ada")');
assert_true(!$vm->hasError(), "plugins: call_function() works");
assert_contains($callResult, "hello, Ada", "plugins: call_function() returns the plugin's real result");

echo "=== Plugins: unhappy path (load/unload must always fail) ===\n";

// Arbitrary local executable — must not run, and /tmp/frankenscriptling-plugin-pwned must never appear.
@unlink("/tmp/frankenscriptling-plugin-pwned");
$vm->clearError();
$vm->eval('plugin.load("evil", "/bin/sh", args=["-c", "touch /tmp/frankenscriptling-plugin-pwned"])');
assert_true($vm->hasError(), "plugins: load() of an arbitrary executable fails");
assert_contains($vm->getLastError(), "disabled", "plugins: load() failure explicitly says loading is disabled");
assert_true(!file_exists("/tmp/frankenscriptling-plugin-pwned"), "plugins: the arbitrary executable never actually ran");

// Arbitrary outbound fetch, including the classic SSRF target.
$vm->clearError();
$vm->eval('plugin.load("evil", "http://169.254.169.254/")');
assert_true($vm->hasError(), "plugins: load() of an arbitrary HTTP(S) URL fails");

// The admin's own pre-loaded plugin cannot be removed by a script either,
// and stays fully usable afterward.
$vm->clearError();
$vm->eval('plugin.unload("frankenscriptling-test-plugin")');
assert_true($vm->hasError(), "plugins: unload() of the admin-preloaded plugin fails");

$vm->clearError();
$stillWorks = $vm->eval('plugin.call_function("frankenscriptling-test-plugin", "greet", "still-here")');
assert_true(!$vm->hasError(), "plugins: the preloaded plugin is still callable after the failed unload");
assert_contains($stillWorks, "hello, still-here", "plugins: and it still returns real results");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    echo "\nFailures:\n";
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
