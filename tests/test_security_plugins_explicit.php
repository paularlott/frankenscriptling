<?php

// Proves SCRIPTLING_PLUGIN (a specific plugin executable path, matching the
// scriptling CLI's own --plugin) works as an alternative to
// SCRIPTLING_PLUGIN_DIR's directory scan. Run via
// `make test-security-plugins-explicit`, which sets:
//   SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin
//   SCRIPTLING_PLUGIN=/app/tests/fixtures/plugins/test-plugin

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

echo "=== Plugins: SCRIPTLING_PLUGIN (explicit path, not a directory) ===\n";

$vm->eval("import scriptling.plugin as plugin");
assert_true(!$vm->hasError(), "plugins-explicit: scriptling.plugin imports with SCRIPTLING_PLUGIN alone");

$vm->clearError();
$listResult = $vm->eval("plugin.list()");
assert_contains($listResult, "frankenscriptling-test-plugin", "plugins-explicit: list() shows the explicitly-named plugin");

$vm->clearError();
$callResult = $vm->eval('plugin.call_function("frankenscriptling-test-plugin", "greet", "explicit")');
assert_true(!$vm->hasError(), "plugins-explicit: call_function() works");
assert_contains($callResult, "hello, explicit", "plugins-explicit: call_function() returns the real result");

$vm->clearError();
$vm->eval('plugin.load("evil", "/bin/sh")');
assert_true($vm->hasError(), "plugins-explicit: load() still fails even with a plugin explicitly preloaded");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
