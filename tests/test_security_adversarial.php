<?php

// Adversarial checks for the security policy — things a hostile or careless
// script would try. Run via `make test-security-adversarial`, which sets:
//   SCRIPTLING_ALLOWED_PATHS=/app/tests
//   SCRIPTLING_ENABLED_LIBRARIES=<every gated library, including every
//     excluded one — the point is proving the excluded ones stay absent
//     even when a policy tries to turn them on>
//   SCRIPTLING_NETWORK_POLICY_FILE=permissive-ip-literals-policy.toml,deny-hosts-network-policy.toml

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

echo "=== Adversarial: filesystem path traversal ===\n";

$vm->eval("import os.path");
assert_true(!$vm->hasError(), "adversarial: os.path imports (enabled + allowed_paths set)");

// Legitimate path inside the allow-list.
$vm->clearError();
$result = $vm->eval('os.path.exists("/app/tests/test_security_adversarial.php")');
assert_true($result === "True" && !$vm->hasError(), "adversarial: allowed_paths permits a real path inside it");

// ".." collapses to a path outside the allow-list — must still be denied,
// not silently resolved and allowed.
$vm->clearError();
$vm->eval('os.path.exists("/app/tests/../etc/passwd")');
assert_true($vm->hasError(), "adversarial: '..' traversal out of the allow-list is denied");

// Symlink inside the allow-list pointing at a real file outside it — the
// classic bypass fssecurity's symlink evaluation exists to stop.
$vm->clearError();
$vm->eval('import pathlib');
$vm->eval('pathlib.Path("/app/tests/fixtures/escape_passwd").read_text()');
assert_true($vm->hasError(), "adversarial: symlink inside allow-list escaping to /etc/passwd is denied");

echo "=== Adversarial: SSRF-style network targets ===\n";

$vm->clearError();
$vm->eval("import requests");
assert_true(!$vm->hasError(), "adversarial: requests imports (enabled + network policy set)");

// permissive-ip-literals-policy.toml sets allow_ip_literals=true. Loopback,
// link-local (cloud metadata), and private ranges must still be blocked —
// there's no single flag that opens all of them at once.
$vm->clearError();
$vm->eval('requests.get("http://127.0.0.1:9/")');
assert_true($vm->hasError(), "adversarial: loopback still denied despite allow_ip_literals=true");

$vm->clearError();
$vm->eval('requests.get("http://169.254.169.254/latest/meta-data/")');
assert_true($vm->hasError(), "adversarial: cloud metadata / link-local address still denied (no toggle exists for it)");

$vm->clearError();
$vm->eval('requests.get("http://192.168.1.1/")');
assert_true($vm->hasError(), "adversarial: private address still denied despite allow_ip_literals=true");

// deny-hosts-network-policy.toml lists "evil.example" in both allow_hosts
// and deny_hosts — deny must win.
$vm->clearError();
$vm->eval('requests.get("https://evil.example/")');
assert_true($vm->hasError(), "adversarial: deny_hosts wins over allow_hosts for the same host");
assert_contains($vm->getLastError(), "denied", "adversarial: deny_hosts error is explicit about the denial");

// A host that's simply not on the allow list.
$vm->clearError();
$vm->eval('requests.get("https://not-example.com/")');
assert_true($vm->hasError(), "adversarial: host not in allow_hosts is denied");

echo "=== Adversarial: disable-only library actually works when enabled ===\n";

$vm->clearError();
$vm->eval("import subprocess");
assert_true(!$vm->hasError(), "adversarial: subprocess imports when explicitly enabled");

$vm->clearError();
$out = $vm->eval('subprocess.run(["echo", "adversarial-ok"], capture_output=True).stdout');
assert_contains($out, "adversarial-ok", "adversarial: enabled subprocess actually executes, not just imports");

echo "=== Adversarial: excluded libraries never register, even if you try to enable them ===\n";

$excluded = [
    "scriptling.wait_for",
    "scriptling.container",
    "scriptling.valkey",
    "scriptling.badgerdb",
    "scriptling.sql",
    "scriptling.sqlite",
    "scriptling.net.gossip",
    "scriptling.net.multicast",
    "scriptling.net.unicast",
    "scriptling.provision.file",
    "scriptling.provision.fetch",
    "scriptling.runtime.sandbox",
    "scriptling.runtime.plugin",
    "scriptling.nomad",
    "scriptling.messaging.console",
];
foreach ($excluded as $lib) {
    $vm->clearError();
    $vm->eval("import $lib");
    assert_true($vm->hasError(), "adversarial: $lib stays unregistered even though the policy tried to enable it");
}

echo "=== Adversarial: scriptling.secret without a configured provider ===\n";

// SCRIPTLING_ENABLED_LIBRARIES includes scriptling.secret in this run, but
// no SCRIPTLING_SECRET_PROVIDER(S) env var is set — must stay unregistered.
$vm->clearError();
$vm->eval("import scriptling.secret");
assert_true($vm->hasError(), "adversarial: scriptling.secret stays unregistered without a configured provider");

echo "=== Adversarial: scriptling.plugin without SCRIPTLING_PLUGIN_DIR/HTTP_ENABLED ===\n";

// SCRIPTLING_ENABLED_LIBRARIES includes scriptling.plugin in this run too,
// but neither SCRIPTLING_PLUGIN_DIR nor SCRIPTLING_PLUGIN_HTTP_ENABLED is
// set — unlike the permanently-excluded libraries above, scriptling.plugin
// is a real, supported library (see test_security_plugins.php for its full
// enabled+configured behaviour); it just still needs "configured", not only
// "enabled", exactly like scriptling.secret and the network-gated libraries.
$vm->clearError();
$vm->eval("import scriptling.plugin");
assert_true($vm->hasError(), "adversarial: scriptling.plugin stays unregistered without a plugin dir or HTTP-loading enabled");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    echo "\nFailures:\n";
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
