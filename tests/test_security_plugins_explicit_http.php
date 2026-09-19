<?php

// Proves SCRIPTLING_PLUGIN can name an http(s) URL, not just a local
// executable path, for the admin's own unrestricted preload — this is the
// host's own trusted config, resolved before any script runs, so no
// network policy is involved at all (unlike a script-initiated load(),
// which is always policy-gated when it's even allowed). Run via
// `make test-security-plugins-explicit-http`, which starts a real HTTP
// scriptling plugin server on loopback first, then sets:
//   SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin
//   SCRIPTLING_PLUGIN=http://127.0.0.1:8199/json-rpc

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

echo "=== Plugins: SCRIPTLING_PLUGIN as an http(s) URL (admin preload) ===\n";

$vm->eval("import scriptling.plugin as plugin");
assert_true(!$vm->hasError(), "plugins-explicit-http: scriptling.plugin imports with an HTTP SCRIPTLING_PLUGIN");

$vm->clearError();
$listResult = $vm->eval("plugin.list()");
assert_true(!$vm->hasError(), "plugins-explicit-http: list() works");
assert_contains($listResult, "frankenscriptling-http-plugin", "plugins-explicit-http: list() shows the HTTP-preloaded plugin");

$vm->clearError();
$callResult = $vm->eval('plugin.call_function("frankenscriptling-http-plugin", "greet", "Ada")');
assert_true(!$vm->hasError(), "plugins-explicit-http: call_function() works over HTTP");
assert_contains($callResult, "hello-http, Ada", "plugins-explicit-http: and returns the real result");

// Still admin-only: scripts still can't load or unload anything, even
// though the preloaded plugin itself arrived over HTTP rather than stdio.
$vm->clearError();
$vm->eval('plugin.load("evil", "/bin/sh")');
assert_true($vm->hasError(), "plugins-explicit-http: load() still fails even with an HTTP plugin explicitly preloaded");

$vm->clearError();
$vm->eval('plugin.unload("frankenscriptling-http-plugin")');
assert_true($vm->hasError(), "plugins-explicit-http: unload() of the HTTP-preloaded plugin still fails");

$vm->clearError();
$stillWorks = $vm->eval('plugin.call_function("frankenscriptling-http-plugin", "greet", "still-here")');
assert_true(!$vm->hasError(), "plugins-explicit-http: the preloaded plugin is still callable after the failed unload");
assert_contains($stillWorks, "hello-http, still-here", "plugins-explicit-http: and still returns real results");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    echo "\nFailures:\n";
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
