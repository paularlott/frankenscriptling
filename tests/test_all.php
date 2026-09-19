<?php

$pass = 0;
$fail = 0;
$errors = [];

function assert_eq($test, $expected, $label) {
    global $pass, $fail, $errors;
    if ($expected === $test) {
        $pass++;
    } else {
        $fail++;
        $errors[] = "FAIL $label: expected " . var_export($expected, true) . ", got " . var_export($test, true);
    }
}

function assert_true($test, $label) {
    global $pass, $fail, $errors;
    if ($test === true) {
        $pass++;
    } else {
        $fail++;
        $errors[] = "FAIL $label: expected true, got " . var_export($test, true);
    }
}

function assert_false($test, $label) {
    global $pass, $fail, $errors;
    if ($test === false) {
        $pass++;
    } else {
        $fail++;
        $errors[] = "FAIL $label: expected false, got " . var_export($test, true);
    }
}

function assert_contains($haystack, $needle, $label) {
    global $pass, $fail, $errors;
    if (str_contains($haystack, $needle)) {
        $pass++;
    } else {
        $fail++;
        $errors[] = "FAIL $label: expected '$needle' in '$haystack'";
    }
}

function assert_not_contains($haystack, $needle, $label) {
    global $pass, $fail, $errors;
    if (!str_contains($haystack, $needle)) {
        $pass++;
    } else {
        $fail++;
        $errors[] = "FAIL $label: did not expect '$needle' in '$haystack'";
    }
}

$vm = new Scriptling();

echo "=== Evaluation ===\n";

assert_eq($vm->eval("2 + 3"), "5", "eval: basic addition");
assert_eq($vm->eval("10 * 4"), "40", "eval: multiplication");
assert_eq($vm->eval("\"hello\""), "hello", "eval: string literal");
assert_eq($vm->eval("True"), "True", "eval: boolean True");
assert_eq($vm->eval("False"), "False", "eval: boolean False");
assert_eq($vm->eval("None"), "None", "eval: None");
assert_eq($vm->eval("[1, 2, 3]"), "[1, 2, 3]", "eval: list");
assert_eq($vm->eval("1 + 2 * 3"), "7", "eval: operator precedence");
assert_eq($vm->eval("(1 + 2) * 3"), "9", "eval: parentheses");

assert_eq($vm->evalWithTimeout(5000, "2 + 3"), "5", "evalWithTimeout: basic");
assert_eq($vm->evalWithTimeout(1000, "100 * 100"), "10000", "evalWithTimeout: larger computation");

$tmpfile = tempnam(sys_get_temp_dir(), 'sling_');
file_put_contents($tmpfile, "42 * 2");
assert_eq($vm->evalFile($tmpfile), "84", "evalFile: basic file");
unlink($tmpfile);

echo "=== Set Variables ===\n";

$vm->setVar("strvar", "hello");
assert_eq($vm->eval("strvar"), "hello", "setVar: string");

$vm->setVarInt("intvar", 42);
assert_eq($vm->eval("intvar"), "42", "setVarInt: integer");

$vm->setVarFloat("floatvar", 3.14);
assert_contains($vm->eval("floatvar"), "3.14", "setVarFloat: float");

$vm->setVarBool("boolvar", true);
assert_eq($vm->eval("boolvar"), "True", "setVarBool: true");

$vm->setVarBool("boolfalse", false);
assert_eq($vm->eval("boolfalse"), "False", "setVarBool: false");

$vm->setVarNull("nullvar");
assert_eq($vm->eval("nullvar"), "None", "setVarNull");

$vm->setVarJSON("jsonvar", '{"x": 1, "y": [2, 3]}');
$jsonResult = $vm->getVarJSON("jsonvar");
assert_contains($jsonResult, '"x"', "setVarJSON: object");

$vm->setVarList("listvar", '[10, 20, 30]');
$listResult = $vm->eval("listvar");
assert_contains($listResult, "10", "setVarList: basic");

$vm->setVarDict("dictvar", '{"name": "test", "value": 99}');
$dictResult = $vm->eval("dictvar");
assert_contains($dictResult, "test", "setVarDict: basic");

echo "=== Get Variables ===\n";

$vm->setVar("getstr", "world");
assert_eq($vm->getVar("getstr"), "world", "getVar: string");
assert_eq($vm->getVarAsString("getstr"), "world", "getVarAsString: string");

$vm->setVarInt("getint", 99);
assert_eq($vm->getVarInt("getint"), 99, "getVarInt: integer");

$vm->setVarFloat("getfloat", 2.718);
$result = $vm->getVarFloat("getfloat");
assert_true(abs($result - 2.718) < 0.001, "getVarFloat: float");

$vm->setVarBool("getbool_t", true);
assert_true($vm->getVarBool("getbool_t"), "getVarBool: true");

$vm->setVarBool("getbool_f", false);
assert_false($vm->getVarBool("getbool_f"), "getVarBool: false");

$vm->setVarJSON("getjson", '{"a": 1}');
$json = $vm->getVarJSON("getjson");
assert_contains($json, '"a"', "getVarJSON: basic");
assert_contains($json, '1', "getVarJSON: contains value");

echo "=== Variable Management ===\n";

$vm->setVar("exists_test", "yes");
assert_true($vm->hasVar("exists_test"), "hasVar: existing");
assert_false($vm->hasVar("does_not_exist_xyz"), "hasVar: non-existing");

$vars = json_decode($vm->listVars(), true);
assert_true(in_array("exists_test", $vars), "listVars: contains test var");
assert_true(in_array("strvar", $vars), "listVars: contains strvar");

$vm->setVar("to_delete", "temp");
$vm->unsetVar("to_delete");
assert_false($vm->hasVar("to_delete"), "unsetVar: var removed");

$vm->setVar("reset_test", "before");
$vm->reset();
assert_false($vm->hasVar("reset_test"), "reset: var cleared");

echo "=== Libraries ===\n";

$vm->registerScriptLibrary("testlib", '
VALUE = 42

def add(a, b):
    return a + b

def multiply(a, b):
    return a * b
');
$vm->import("testlib");
assert_eq($vm->eval("testlib.VALUE"), "42", "registerScriptLibrary + import: constant");
assert_eq($vm->eval("testlib.add(3, 4)"), "7", "registerScriptLibrary + import: function");

$vm2 = new Scriptling();
$vm2->registerScriptLibrary("lib1", 'X = 1');
$vm2->registerScriptLibrary("lib2", 'Y = 2');
$vm2->importMultiple('["lib1", "lib2"]');
assert_eq($vm2->eval("lib1.X + lib2.Y"), "3", "importMultiple: two libs");

$vm->registerScriptFunc("sq", "lambda x: x * x");
assert_eq($vm->eval("sq(7)"), "49", "registerScriptFunc: lambda");

echo "=== Function Calling ===\n";

$vm->registerScriptFunc("adder", "lambda a, b: a + b");
assert_eq($vm->callFunction("adder", "[10, 20]"), "30", "callFunction: two args");

$vm->registerScriptFunc("greeter", 'lambda: "hello"');
assert_eq($vm->callFunction("greeter", ""), "hello", "callFunction: no args");
assert_eq($vm->callFunction("greeter", "[]"), "hello", "callFunction: empty array");

$vm->registerScriptFunc("identity", "lambda x: x");
assert_eq($vm->callFunction("identity", "[42]"), "42", "callFunction: single arg");

echo "=== Output Capture ===\n";

$vm->enableOutputCapture();
$vm->eval('print("captured output")');
assert_contains($vm->getOutput(), "captured output", "outputCapture: basic");

echo "=== Error Handling ===\n";

$vm->eval("1/0");
assert_true($vm->hasError(), "hasError: division by zero");
assert_contains($vm->getLastError(), "division by zero", "getLastError: message");

$vm->clearError();
assert_false($vm->hasError(), "clearError: cleared");
assert_eq($vm->getLastError(), "", "clearError: empty message");

$vm->eval("undefined_var_xyz + 1");
assert_true($vm->hasError(), "hasError: undefined var");

echo "=== Autoload Path ===\n";

$vm3 = new Scriptling();
$autodir = sys_get_temp_dir() . "/sling_autoload_test";
@mkdir($autodir, 0777, true);
file_put_contents("$autodir/autolib.py", "AUTO_VAL = 123\ndef auto_add(a, b):\n    return a + b");

$vm3->setAutoloadPath($autodir);
assert_eq($vm3->getAutoloadPath(), $autodir, "setAutoloadPath: path stored");

$vm3->import("autolib");
assert_eq($vm3->eval("autolib.AUTO_VAL"), "123", "autoload: import from filesystem");
assert_eq($vm3->eval("autolib.auto_add(5, 10)"), "15", "autoload: function from filesystem");

$autodir2 = sys_get_temp_dir() . "/sling_autoload_test2";
@mkdir($autodir2, 0777, true);
file_put_contents("$autodir2/lib2.py", "SECOND = 99");
$vm3->addAutoloadPath($autodir2);
$vm3->import("lib2");
assert_eq($vm3->eval("lib2.SECOND"), "99", "addAutoloadPath: second path");

// Cleanup
@unlink("$autodir/autolib.py");
@rmdir($autodir);
@unlink("$autodir2/lib2.py");
@rmdir($autodir2);

echo "=== Built-in Libraries ===\n";

assert_eq($vm->eval("import json; json.dumps({\"k\": 1})"), "{\"k\":1}", "stdlib: json.dumps");
assert_contains($vm->eval("import json; json.loads('{\"a\":2}')[\"a\"]"), "2", "stdlib: json.loads");
assert_eq($vm->eval("import math; math.sqrt(144)"), "12", "stdlib: math.sqrt");
assert_contains($vm->eval("import math; str(math.pi)"), "3.14", "stdlib: math.pi");
assert_contains($vm->eval("import re; str(re.findall(r\"\\d+\", \"a1b2c3\"))"), "1", "stdlib: re.findall");
assert_contains($vm->eval("import base64; base64.b64encode(\"test\")"), "dGVzdA==", "stdlib: base64");
$vm->eval("import hashlib");
assert_contains($vm->eval("hashlib.sha256(\"hello\").hexdigest()"), "2cf24dba5fb0a30e26e83b2ac5b9e29e", "stdlib: hashlib");
assert_contains($vm->eval("import string; string.ascii_uppercase"), "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "stdlib: string");
assert_eq($vm->eval("import html; html.escape(\"<b>\")"), "&lt;b&gt;", "stdlib: html.escape");
assert_contains($vm->eval("import uuid; str(uuid.uuid4())"), "-", "stdlib: uuid format");
$vm->eval("import itertools");
assert_contains($vm->eval("list(itertools.chain([1,2],[3,4]))"), "1", "stdlib: itertools");

assert_contains($vm->eval("import toml; toml.loads(\"[db]\\nhost = \\\"localhost\\\"\")[\"db\"][\"host\"]"), "localhost", "extlib: toml");
assert_contains($vm->eval("import yaml; yaml.safe_load(\"key: val\")[\"key\"]"), "val", "extlib: yaml");

echo "=== Version ===\n";

$version = $vm->getScriptlingVersion();
assert_true(strlen($version) > 0, "getScriptlingVersion: non-empty");
assert_true(preg_match('/^\d+\.\d+/', $version) === 1, "getScriptlingVersion: semver format");

echo "=== Security Policy: closed by default ===\n";

// No SCRIPTLING_* env vars are set for this run, so every fs- and
// net-capable library must be absent — importing one is an error, not a
// silently-open sandbox.
$vmSec = new Scriptling();
$vmSec->eval("import pathlib");
assert_true($vmSec->hasError(), "security: pathlib not registered by default");
assert_contains($vmSec->getLastError(), "pathlib", "security: pathlib error mentions library name");

$vmSec->clearError();
$vmSec->eval("import requests");
assert_true($vmSec->hasError(), "security: requests not registered by default");

$vmSec->clearError();
$vmSec->eval("import subprocess");
assert_true($vmSec->hasError(), "security: subprocess not registered by default");

echo "=== VM Isolation ===\n";

$iso1 = new Scriptling();
$iso2 = new Scriptling();
$iso1->setVar("shared", "vm1");
$iso2->setVar("shared", "vm2");
assert_eq($iso1->getVar("shared"), "vm1", "isolation: vm1 independent");
assert_eq($iso2->getVar("shared"), "vm2", "isolation: vm2 independent");

echo "\n" . str_repeat("=", 50) . "\n";
echo "Results: $pass passed, $fail failed\n";

if ($fail > 0) {
    echo "\nFailures:\n";
    foreach ($errors as $e) {
        echo "  $e\n";
    }
    exit(1);
}

echo "All tests passed!\n";
exit(0);
