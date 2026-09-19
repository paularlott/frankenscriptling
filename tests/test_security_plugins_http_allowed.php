<?php

// Proves the *positive* case for opt-in HTTP plugin loading: when the
// network policy actually allows the target, a script's load() succeeds
// and the newly-loaded plugin is genuinely callable over HTTP — not just
// that a denied target fails, which test_security_plugins_http.php already
// covers. Run via `make test-security-plugins-http-allowed`, which starts a
// real HTTP scriptling plugin server on loopback first, then sets:
//   SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin
//   SCRIPTLING_PLUGIN_HTTP_ENABLED=true
//   SCRIPTLING_NETWORK_POLICY_FILE=/app/tests/fixtures/allow-loopback-network-policy.toml

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

echo "=== Plugins: a permitted HTTP load actually succeeds ===\n";

$vm->eval("import scriptling.plugin as plugin");
assert_true(!$vm->hasError(), "plugins-http-allowed: scriptling.plugin imports");

// scriptling=True forces the handshake at load time, same reasoning as
// test_security_plugins_http.php: with the default scriptling=False, load()
// only constructs the client and never dials, which would make this
// assertion pass for the wrong reason.
$vm->clearError();
$name = $vm->eval('plugin.load("remote", "http://127.0.0.1:8199/json-rpc", scriptling=True)');
assert_true(!$vm->hasError(), "plugins-http-allowed: load() succeeds when the policy allows the target");
assert_contains($name, "plugin.remote", "plugins-http-allowed: load() returns the normalised name");

$vm->clearError();
$callResult = $vm->eval('plugin.call_function("remote", "greet", "Ada")');
assert_true(!$vm->hasError(), "plugins-http-allowed: the newly-loaded HTTP plugin is genuinely callable");
assert_contains($callResult, "hello-http, Ada", "plugins-http-allowed: and returns the real result");

// Stdio/exec loading still isn't re-enabled — HTTP loading being allowed
// doesn't relax that.
@unlink("/tmp/frankenscriptling-plugin-http-allowed-pwned");
$vm->clearError();
$vm->eval('plugin.load("evil", "/bin/sh", args=["-c", "touch /tmp/frankenscriptling-plugin-http-allowed-pwned"])');
assert_true($vm->hasError(), "plugins-http-allowed: stdio/exec loading still fails");
assert_true(!file_exists("/tmp/frankenscriptling-plugin-http-allowed-pwned"), "plugins-http-allowed: the arbitrary executable never ran");

// unload() is only blocked for the admin's own preloaded plugins (proven by
// test_security_plugins_combined.php) — a plugin the script itself loaded
// through an allowed transport is the script's own to unload, and doing so
// makes it genuinely gone (not still reachable through some fallback path).
$vm->clearError();
$vm->eval('plugin.unload("remote")');
assert_true(!$vm->hasError(), "plugins-http-allowed: unload() succeeds for a plugin the script itself loaded");

$vm->clearError();
$vm->eval('plugin.call_function("remote", "greet", "gone")');
assert_true($vm->hasError(), "plugins-http-allowed: the unloaded plugin is genuinely gone, not still callable");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    echo "\nFailures:\n";
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
