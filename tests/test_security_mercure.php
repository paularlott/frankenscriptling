<?php

// scriptling.mercure is network-gated. Run via `make test-security-mercure`,
// once per mode:
//   enabled    enabled, with a policy allowing loopback, against the image's
//              own Mercure hub (tests/fixtures/mercure.Caddyfile)
//   denied     enabled, but the network policy denies loopback
//   disabled   a network policy, but scriptling.mercure not enabled
//   nonetwork  enabled, but no network policy

$mode = $argv[1] ?? 'enabled';

const HUB = 'http://127.0.0.1:8090/.well-known/mercure';
const KEY = 'frankenscriptling-mercure-test-key-0123456789';
$hub = HUB;
$key = KEY;

$pass = 0;
$fail = 0;
$errors = [];

function assert_true($test, $label) {
    global $pass, $fail, $errors;
    if ($test === true) { $pass++; } else { $fail++; $errors[] = "FAIL $label: got " . var_export($test, true); }
}

function finish() {
    global $pass, $fail, $errors;
    echo "\n" . str_repeat("=", 50) . "\n";
    echo "Results: $pass passed, $fail failed\n";
    if ($fail > 0) {
        foreach ($errors as $e) { echo "  $e\n"; }
        if (is_readable('/tmp/hub.log')) { echo "\nhub log:\n" . file_get_contents('/tmp/hub.log'); }
        exit(1);
    }
    echo "All tests passed!\n";
    exit(0);
}

// open_subscriptions counts the hub's active subscriptions to $topic, through
// its subscriptions API, with a token from a VM of its own (the VM under test
// gets reset); -1 when the API does not answer.
$last_api_answer = '';
function open_subscriptions($topic) {
    global $last_api_answer;
    static $token = null;
    if ($token === null) {
        $admin = new Scriptling();
        $admin->eval('import scriptling.mercure as mercure
token = mercure.create_token("' . KEY . '", "' . HUB . '", subscribe=["*"])');
        $token = $admin->getVarAsString('token');
    }
    $ctx = stream_context_create(['http' => [
        'header' => "Authorization: Bearer $token",
        'ignore_errors' => true,
        'timeout' => 5,
    ]]);
    $body = @file_get_contents(HUB . '/subscriptions', false, $ctx);
    $last_api_answer = ($http_response_header[0] ?? 'no answer') . ' ' . substr((string)$body, 0, 300);
    $doc = json_decode((string)$body, true);
    if (!is_array($doc) || !isset($doc['subscriptions'])) { return -1; }
    $n = 0;
    foreach ($doc['subscriptions'] as $s) {
        // Mercure 1.0 names the topic in the subscription's ID:
        // /.well-known/mercure/subscriptions/<match type>/<topic>/<subscriber>
        if (str_contains($s['id'] ?? '', '/' . rawurlencode($topic) . '/') && ($s['active'] ?? true)) { $n++; }
    }
    return $n;
}

function wait_for($fn, $seconds) {
    $deadline = microtime(true) + $seconds;
    do {
        if ($fn()) { return true; }
        usleep(100000);
    } while (microtime(true) < $deadline);
    return false;
}

echo "=== Security Policy: scriptling.mercure ($mode) ===\n";

$vm = new Scriptling();

if ($mode === 'disabled' || $mode === 'nonetwork') {
    $vm->eval("import scriptling.mercure as mercure");
    assert_true($vm->hasError(), "mercure: not importable ($mode)");
    finish();
}

$vm->eval("import scriptling.mercure as mercure");
assert_true(!$vm->hasError(), "mercure: library imports when enabled with a network policy");

if ($mode === 'denied') {
    $vm->clearError();
    $vm->eval('mercure.Client("' . HUB . '", key="' . KEY . '").publish("https://example.com/x", "hi")');
    assert_true($vm->hasError(), "mercure: a loopback hub is refused by the network policy");
    assert_true(!str_contains($vm->getLastError(), "connection refused"), "mercure: refused by the policy, not by the network (" . $vm->getLastError() . ")");
    finish();
}

// enabled: the hub must be up.
$up = wait_for(fn() => ($c = @fsockopen('127.0.0.1', 8090, $e, $s, 0.2)) && fclose($c), 15);
assert_true($up, "mercure: the hub started");
if (!$up) { finish(); }

// Publish and receive through the built-in hub.
$vm->clearError();
$vm->eval(<<<PY
hub = mercure.Client("{$hub}", key="{$key}")
sub = hub.subscribe("https://example.com/books/1")
hub.publish("https://example.com/books/1", {"title": "Dune"}, type="book")
event = sub.receive(timeout=10)
title = event.json()["title"] if event else None
etype = event.type if event else None
PY);
assert_true(!$vm->hasError(), "mercure: publish and subscribe run (" . $vm->getLastError() . ")");
assert_true($vm->getVarAsString("title") === "Dune", "mercure: the subscriber receives the update");
assert_true($vm->getVarAsString("etype") === "book", "mercure: the event keeps its type");

// A CA bundle must lie within the allowed paths, and none are allowed here.
$vm->clearError();
$vm->eval('mercure.Client("https://127.0.0.1:8443/.well-known/mercure", key="' . KEY . '", verify="/etc/ssl/certs/ca-certificates.crt").publish("https://example.com/x", "hi")');
assert_true(str_contains($vm->getLastError(), "outside allowed directories"), "mercure: verify= path outside the allowed paths is refused (" . $vm->getLastError() . ")");

// reset() closes what the script left open, and the VM keeps working.
$vm->clearError();
$vm->eval(<<<PY
left = mercure.Client("{$hub}", key="{$key}")
left_sub = left.subscribe("https://example.com/left-open")
PY);
assert_true(!$vm->hasError(), "mercure: a subscription is opened and left open (" . $vm->getLastError() . ")");
assert_true(wait_for(fn() => open_subscriptions("https://example.com/left-open") === 1, 5), "mercure: the hub sees the open subscription ($last_api_answer)");
$vm->reset();
assert_true(wait_for(fn() => open_subscriptions("https://example.com/left-open") === 0, 10), "mercure: reset() closes the subscription the script left open ($last_api_answer)");

$vm->eval("import scriptling.mercure as mercure");
$vm->eval(<<<PY
again = mercure.Client("{$hub}", key="{$key}")
again_sub = again.subscribe("https://example.com/after-reset")
again.publish("https://example.com/after-reset", "still here")
got = again_sub.receive(timeout=10)
got = got.data if got else None
again.close()
PY);
assert_true(!$vm->hasError(), "mercure: clients work after reset() (" . $vm->getLastError() . ")");
assert_true($vm->getVarAsString("got") === "still here", "mercure: a client made after reset() receives");

finish();
