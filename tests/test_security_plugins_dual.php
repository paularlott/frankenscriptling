<?php

// Proves SCRIPTLING_PLUGIN and SCRIPTLING_PLUGIN_DIR combine freely: one
// specific plugin loaded explicitly, a different one discovered by a
// directory scan, both usable from the same script at once. Run via
// `make test-security-plugins-dual`, which sets:
//   SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin
//   SCRIPTLING_PLUGIN=/app/tests/fixtures/plugins-explicit/second-plugin
//   SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins

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

echo "=== Plugins: SCRIPTLING_PLUGIN + SCRIPTLING_PLUGIN_DIR combined ===\n";

$vm->eval("import scriptling.plugin as plugin");
assert_true(!$vm->hasError(), "plugins-dual: scriptling.plugin imports with both sources configured");

$vm->clearError();
$listResult = $vm->eval("plugin.list()");
assert_true(!$vm->hasError(), "plugins-dual: list() works");
assert_contains($listResult, "frankenscriptling-test-plugin", "plugins-dual: the directory-scanned plugin is present");
assert_contains($listResult, "frankenscriptling-second-plugin", "plugins-dual: the explicitly-loaded plugin is present too");

$vm->clearError();
$dirResult = $vm->eval('plugin.call_function("frankenscriptling-test-plugin", "greet", "Ada")');
assert_true(!$vm->hasError(), "plugins-dual: the directory-scanned plugin is callable");
assert_contains($dirResult, "hello, Ada", "plugins-dual: and returns its real result");

$vm->clearError();
$explicitResult = $vm->eval('plugin.call_function("frankenscriptling-second-plugin", "wave", "Bob")');
assert_true(!$vm->hasError(), "plugins-dual: the explicitly-loaded plugin is callable");
assert_contains($explicitResult, "wave, Bob", "plugins-dual: and returns its own, different result");

// Both stay admin-only, regardless of how many sources preloaded them.
@unlink("/tmp/frankenscriptling-plugin-dual-pwned");
$vm->clearError();
$vm->eval('plugin.load("evil", "/bin/sh", args=["-c", "touch /tmp/frankenscriptling-plugin-dual-pwned"])');
assert_true($vm->hasError(), "plugins-dual: load() still fails with two sources preloaded");
assert_true(!file_exists("/tmp/frankenscriptling-plugin-dual-pwned"), "plugins-dual: the arbitrary executable never ran");

$vm->clearError();
$vm->eval('plugin.unload("frankenscriptling-second-plugin")');
assert_true($vm->hasError(), "plugins-dual: unload() of the explicitly-loaded plugin still fails");

$vm->clearError();
$vm->eval('plugin.unload("frankenscriptling-test-plugin")');
assert_true($vm->hasError(), "plugins-dual: unload() of the directory-scanned plugin still fails");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    echo "\nFailures:\n";
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
