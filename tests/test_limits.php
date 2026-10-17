<?php

// Resource limits set by the operator (SCRIPTLING_MEMORY_LIMIT /
// SCRIPTLING_MAX_VALUE_BYTES, or [limits] in SCRIPTLING_POLICY_FILE). Run via
// `make test-limits`, which runs this file once per way of configuring them,
// always with a 64 MiB memory limit and a 1 MiB value limit.

$pass = 0;
$fail = 0;
$errors = [];

function assert_eq($test, $expected, $label) {
    global $pass, $fail, $errors;
    if ($expected === $test) { $pass++; } else { $fail++; $errors[] = "FAIL $label: expected " . var_export($expected, true) . ", got " . var_export($test, true); }
}

function assert_true($test, $label) {
    global $pass, $fail, $errors;
    if ($test === true) { $pass++; } else { $fail++; $errors[] = "FAIL $label: got " . var_export($test, true); }
}

function assert_false($test, $label) {
    global $pass, $fail, $errors;
    if ($test === false) { $pass++; } else { $fail++; $errors[] = "FAIL $label: got " . var_export($test, true); }
}

function assert_contains($haystack, $needle, $label) {
    global $pass, $fail, $errors;
    if (str_contains($haystack, $needle)) { $pass++; } else { $fail++; $errors[] = "FAIL $label: expected '$needle' in '$haystack'"; }
}

const MEM = 67108864;
const VAL = 1048576;

echo "=== Operator limits ===\n";

$vm = new Scriptling();
assert_eq($vm->getMemoryLimit(), MEM, "limits: memory limit from operator config");
assert_eq($vm->getMaxValueBytes(), VAL, "limits: value limit from operator config");

$vm->eval('x = "a" * 2000000');
assert_true($vm->hasError(), "limits: value over operator limit fails");
assert_contains($vm->getLastError(), "1048576 byte limit", "limits: value error names the limit");

$vm->clearError();
$vm->eval('y = "a" * 1000');
assert_false($vm->hasError(), "limits: value under operator limit works");

echo "=== PHP cannot loosen ===\n";

$vm->setMemoryLimit(0);
assert_true($vm->hasError(), "limits: memory limit cannot be removed");
$vm->setMemoryLimit(MEM * 2);
assert_true($vm->hasError(), "limits: memory limit cannot be raised");
$vm->setMemoryLimit(-1);
assert_true($vm->hasError(), "limits: negative memory limit rejected");
assert_eq($vm->getMemoryLimit(), MEM, "limits: memory limit unchanged after refused changes");

$vm->setMaxValueBytes(0);
assert_true($vm->hasError(), "limits: value limit cannot be removed");
$vm->setMaxValueBytes(VAL * 2);
assert_true($vm->hasError(), "limits: value limit cannot be raised");
assert_eq($vm->getMaxValueBytes(), VAL, "limits: value limit unchanged after refused changes");

echo "=== PHP can tighten ===\n";

$vm->setMaxValueBytes(1000);
assert_false($vm->hasError(), "limits: value limit can be lowered");
assert_eq($vm->getMaxValueBytes(), 1000, "limits: lowered value limit in force");
$vm->eval('z = "a" * 2000');
assert_true($vm->hasError(), "limits: lowered value limit enforced");

$other = new Scriptling();
assert_eq($other->getMaxValueBytes(), VAL, "limits: tightening one VM leaves others alone");

$mem = new Scriptling();
$mem->setMemoryLimit(4 * 1024 * 1024);
assert_false($mem->hasError(), "limits: memory limit can be lowered");
$mem->eval('big = [str(i) * 10 for i in range(500000)]');
assert_true($mem->hasError(), "limits: lowered memory limit enforced");
assert_contains($mem->getLastError(), "memory limit exceeded", "limits: memory error names the limit");

$usage = json_decode($mem->getResourceUsage(), true);
assert_true(is_array($usage), "limits: resource usage is JSON");
// Not >= 1: scriptling may refuse the list's growth up front (an error, not
// a cancelled run), which doesn't count as a cancellation.
assert_true(array_key_exists("memory_limit_cancellations", $usage), "limits: cancellations reported");
assert_true($usage["peak_memory_bytes"] > 0, "limits: peak memory recorded");

// After a collection that didn't help, scriptling refuses calls for 100ms
// without collecting again (so a script can't force GCs in a loop); past
// that, the next call collects the abandoned partial list and runs.
usleep(200000);
$mem->clearError();
$mem->eval('ok = 1 + 1');
assert_false($mem->hasError(), "limits: VM usable after a memory-limit cancellation");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";
if ($fail > 0) {
    foreach ($errors as $e) { echo "  $e\n"; }
    exit(1);
}
echo "All tests passed!\n";
exit(0);
