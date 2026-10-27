<?php
// PHP functions passed to eval / evalWithTimeout / evalFile / callFunction.
// exit() and fatal errors end the request, so they are tested separately by
// test_php_functions_exit.php (make test-php-functions).

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

class Greeter {
    public function __construct(private string $greeting) {}
    public function greet(string $name): string { return "{$this->greeting}, $name"; }
    public static function shout(string $s): string { return strtoupper($s) . "!"; }
}

class Doubler {
    public function __invoke(int $x): int { return $x * 2; }
}

class Tracked {
    public static int $destroyed = 0;
    public function __destruct() { self::$destroyed++; }
}

$vm = new Scriptling();

echo "=== Callables ===\n";

$customers = [42 => ['name' => 'Acme Pty Ltd', 'balance' => 1250.5]];
$php = [
    'get_customer' => fn(int $id) => $customers[$id] ?? null,
    'upper'        => 'strtoupper',
    'greet'        => [new Greeter('Hello'), 'greet'],
    'shout'        => 'Greeter::shout',
    'shout2'       => [Greeter::class, 'shout'],
    'double'       => new Doubler(),
    'first_class'  => strrev(...),
];

assert_eq($vm->eval('get_customer(42)["name"]', $php), "Acme Pty Ltd", "closure returning an array");
assert_eq($vm->eval('upper("abc")', $php), "ABC", "function name string");
assert_eq($vm->eval('greet("Bob")', $php), "Hello, Bob", "[object, method]");
assert_eq($vm->eval('shout("hi")', $php), "HI!", "'Class::method' string");
assert_eq($vm->eval('shout2("hi")', $php), "HI!", "[class, static method]");
assert_eq($vm->eval('double(21)', $php), "42", "invokable object");
assert_eq($vm->eval('first_class("abc")', $php), "cba", "first-class callable");
assert_eq($vm->eval('get_customer(7) is None', $php), "True", "null return is None");
assert_false($vm->hasError(), "no error after successful calls");

$vm->eval(<<<'PY'
c = get_customer(42)
result = f"{upper(c['name'])} owes {c['balance'] * 1.1:.2f}"
PY, $php);
assert_eq($vm->getVar("result"), "ACME PTY LTD owes 1375.55", "multi-statement script");

echo "=== Arguments ===\n";

$seen = null;
$capture = ['capture' => function (...$args) use (&$seen) { $seen = $args; return count($args); }];

$vm->eval('capture(1, 2.5, True, False, None, "s")', $capture);
assert_eq($seen, [1, 2.5, true, false, null, "s"], "scalars and None");

$vm->eval('capture([1, 2, 3], (4, 5))', $capture);
assert_eq($seen, [[1, 2, 3], [4, 5]], "list and tuple become lists");

$vm->eval('capture({"b": 1, "a": 2, "c": {"x": [1, {"y": None}]}})', $capture);
assert_eq($seen, [["b" => 1, "a" => 2, "c" => ["x" => [1, ["y" => null]]]]], "dict keeps order, nested values");
assert_eq(array_keys($seen[0]), ["b", "a", "c"], "dict key order");

$vm->eval('capture(b"bytes")', $capture);
assert_eq($seen, ["bytes"], "bytes become a string");

$vm->eval('capture({1: "one"})', $capture);
assert_eq($seen, [[1 => "one"]], "int dict key");
assert_true(is_int(array_key_first($seen[0])), "numeric dict key becomes an int key");
$vm->eval('capture({"7": "seven", "x": 1})', $capture);
assert_eq($seen[0][7] ?? null, "seven", "numeric string key is found as an int key");

$vm->eval('capture()', $capture);
assert_eq($seen, [], "no arguments");

$big = $vm->eval('capture(list(range(10000)))', $capture);
assert_eq(count($seen[0]), 10000, "large list");
assert_eq($seen[0][9999], 9999, "large list content");

$r = $vm->eval('capture(lambda x: x)', $capture);
assert_eq($r, "", "function argument refused");
assert_contains($vm->getLastError(), "cannot pass", "function argument error message");

$vm->eval(<<<'PY'
try:
    capture(lambda x: x)
    result = "no error"
except TypeError as e:
    result = "TypeError"
PY, $capture);
assert_eq($vm->getVar("result"), "TypeError", "unconvertible argument is a TypeError");

$vm->eval(<<<'PY'
l = [1]
l.append(l)
try:
    capture(l)
    result = "no error"
except TypeError as e:
    result = str(e)
PY, $capture);
assert_contains($vm->getVar("result"), "reference to itself", "cyclic argument refused");

echo "=== Named arguments ===\n";

$named = ['add' => fn(int $a, int $b = 10, int $c = 100) => $a + $b + $c];
assert_eq($vm->eval('add(1)', $named), "111", "defaults");
assert_eq($vm->eval('add(1, c=5)', $named), "16", "keyword maps to named argument");
assert_eq($vm->eval('add(a=1, b=2, c=3)', $named), "6", "all named");
$vm->eval(<<<'PY'
try:
    add(1, nope=2)
    result = "no error"
except Exception as e:
    result = str(e)
PY, $named);
assert_contains($vm->getVar("result"), "nope", "unknown named argument raises");

echo "=== Return values ===\n";

$ret = [
    'r_int'   => fn() => 7,
    'r_float' => fn() => 1.5,
    'r_true'  => fn() => true,
    'r_str'   => fn() => "x",
    'r_list'  => fn() => [1, "two", 3.0],
    'assoc'  => fn() => ["z" => 1, "a" => ["n" => [1, 2]]],
    'sparse' => fn() => [5 => "five", 1 => "one"],
    'obj'    => fn() => new stdClass(),
    'nested_obj' => fn() => ["a" => new stdClass()],
    'void'   => function (): void {},
];
assert_eq($vm->eval('r_int() + 1', $ret), "8", "int return");
assert_eq($vm->eval('r_float() * 2', $ret), "3.0", "float return");
assert_eq($vm->eval('r_true() == True', $ret), "True", "bool return");
assert_eq($vm->eval('r_str() * 3', $ret), "xxx", "string return");
assert_eq($vm->eval('r_list()', $ret), "[1, 'two', 3.0]", "list return");
assert_eq($vm->eval('list(assoc().keys())', $ret), "['z', 'a']", "assoc return keeps key order");
assert_eq($vm->eval('assoc()["a"]["n"][1]', $ret), "2", "nested assoc return");
assert_eq($vm->eval('sparse()["5"]', $ret), "five", "non-list int keys become a dict");
assert_eq($vm->eval('void() is None', $ret), "True", "void returns None");
assert_eq($vm->eval('hashlist()', ['hashlist' => fn() => array_filter([1, 2, 3], fn($x) => true)]), "[1, 2, 3]", "unpacked 0..n-1 array is a list");
assert_eq($vm->eval('gap()', ['gap' => fn() => array_filter([1, 2, 3], fn($x) => $x != 2)]), "{'0': 1, '2': 3}", "array with a gap is a dict");

$vm->eval(<<<'PY'
try:
    obj()
    result = "no error"
except TypeError as e:
    result = str(e)
PY, $ret);
assert_contains($vm->getVar("result"), "obj() returned", "object return is a TypeError");
$vm->eval(<<<'PY'
try:
    nested_obj()
    result = "no error"
except TypeError as e:
    result = "TypeError"
PY, $ret);
assert_eq($vm->getVar("result"), "TypeError", "nested object return is a TypeError");

$roundtrip = ['echo' => fn($v) => $v];
$vm->eval(<<<'PY'
data = {"name": "x", "tags": ["a", "b"], "n": 3, "f": 0.25, "ok": True, "none": None}
result = echo(data) == data
PY, $roundtrip);
assert_true($vm->getVarBool("result"), "dict round trip");

echo "=== Exceptions ===\n";

$ex = [
    'boom'    => fn() => throw new RuntimeException('nope'),
    'typed'   => fn(int $x) => $x,
    'value'   => fn() => throw new ValueError('bad value'),
    'div'     => fn() => intdiv(1, 0),
    'oor'     => fn() => throw new OutOfRangeException('too far'),
    'custom'  => fn() => throw new class('mine') extends Exception {},
];

$vm->eval(<<<'PY'
try:
    boom()
except Exception as e:
    result = str(e)
PY, $ex);
assert_eq($vm->getVar("result"), "nope", "PHP exception caught by except Exception");
assert_false($vm->hasError(), "caught exception leaves no error");

$vm->eval(<<<'PY'
try:
    boom()
except RuntimeError as e:
    result = "RuntimeError"
PY, $ex);
assert_eq($vm->getVar("result"), "RuntimeError", "RuntimeException is a RuntimeError");

$vm->eval(<<<'PY'
try:
    typed("abc")
except TypeError as e:
    result = "TypeError"
PY, $ex);
assert_eq($vm->getVar("result"), "TypeError", "PHP TypeError is a TypeError");

$vm->eval(<<<'PY'
try:
    typed()
except TypeError as e:
    result = "TypeError"
PY, $ex);
assert_eq($vm->getVar("result"), "TypeError", "ArgumentCountError is a TypeError");

$vm->eval(<<<'PY'
try:
    value()
except ValueError as e:
    result = str(e)
PY, $ex);
assert_eq($vm->getVar("result"), "bad value", "ValueError is a ValueError");

$vm->eval(<<<'PY'
try:
    div()
except ZeroDivisionError as e:
    result = "ZeroDivisionError"
PY, $ex);
assert_eq($vm->getVar("result"), "ZeroDivisionError", "DivisionByZeroError is a ZeroDivisionError");

$vm->eval(<<<'PY'
try:
    oor()
except IndexError as e:
    result = "IndexError"
PY, $ex);
assert_eq($vm->getVar("result"), "IndexError", "OutOfRangeException is an IndexError");

$vm->eval(<<<'PY'
try:
    custom()
except Exception as e:
    result = str(e)
PY, $ex);
assert_eq($vm->getVar("result"), "mine", "custom exception class");

$threw = false;
try {
    $r = $vm->eval('boom()', $ex);
} catch (Throwable $t) {
    $threw = true;
}
assert_false($threw, "uncaught PHP exception does not escape eval");
assert_eq($r, "", "uncaught PHP exception: eval returns empty");
assert_true($vm->hasError(), "uncaught PHP exception: hasError");
assert_contains($vm->getLastError(), "nope", "uncaught PHP exception: message in getLastError");

$vm->eval(<<<'PY'
try:
    boom()
except Exception as e:
    pass
result = upper("still running")
PY, $ex + $php);
assert_eq($vm->getVar("result"), "STILL RUNNING", "script continues after a caught PHP exception");

echo "=== Scope ===\n";

$vm->reset();
$vm->eval('x = 1');
$r = $vm->eval('upper("x")');
assert_eq($r, "", "function not defined without the array");
assert_contains($vm->getLastError(), "upper", "NameError names the function");

$vm->eval('upper("x")', $php);
assert_false($vm->hasError(), "defined when passed");
$vm->eval('upper("x")');
assert_true($vm->hasError(), "gone again after the call");

$vm->eval('upper("x")', []);
assert_true($vm->hasError(), "empty array binds nothing");

$vm->setVar("upper", "a string");
assert_eq($vm->eval('upper("abc")', $php), "ABC", "shadows an existing variable during the call");
assert_eq($vm->getVar("upper"), "a string", "existing variable restored after the call");
$vm->unsetVar("upper");

$vm->eval('len("abc")', ['len' => fn($s) => 99]);
assert_eq($vm->eval('len("abc")', ['len' => fn($s) => 99]), "99", "can shadow a builtin during the call");
assert_eq($vm->eval('len("abc")'), "3", "builtin back after the call");

$vm->eval('saved = upper', $php);
$vm->eval(<<<'PY'
try:
    saved("x")
    result = "called"
except RuntimeError as e:
    result = str(e)
PY);
assert_contains($vm->getVar("result"), "only be called during the call", "stored reference refused after the call");

$vm->eval('upper = "rebound"', $php);
assert_eq($vm->getVar("upper"), "rebound", "name rebound by the script is kept");
$vm->unsetVar("upper");

echo "=== Validation ===\n";

$ran = false;
$side = ['side' => function () use (&$ran) { $ran = true; return 1; }];

$r = $vm->eval('side()', $side + ['bad' => 'no_such_function_xyz']);
assert_eq($r, "", "non-callable: returns empty");
assert_contains($vm->getLastError(), "\"bad\" is not callable", "non-callable: error names it");
assert_false($ran, "non-callable: script not run");

$r = $vm->eval('side()', $side + [5 => 'strtoupper']);
assert_contains($vm->getLastError(), "integer key", "integer key refused");
assert_false($ran, "integer key: script not run");

$r = $vm->eval('side()', $side + ['1abc' => 'strtoupper']);
assert_contains($vm->getLastError(), "not a valid function name", "invalid name refused");
$r = $vm->eval('side()', $side + ['a.b' => 'strtoupper']);
assert_contains($vm->getLastError(), "not a valid function name", "dotted name refused");
assert_false($ran, "invalid name: script not run");

$vm->eval('side()', $side);
assert_true($ran, "valid set runs");

echo "=== callFunction, registerScriptFunc, evalFile, evalWithTimeout ===\n";

$vm->registerScriptFunc('describe', <<<'PY'
def describe(id):
    c = get_customer(id)
    return "unknown" if c is None else upper(c["name"])
PY);
assert_eq($vm->callFunction('describe', '[42]', $php), "ACME PTY LTD", "callFunction with functions");
assert_eq($vm->callFunction('describe', '[1]', $php), "unknown", "callFunction with functions, None path");
$r = $vm->callFunction('describe', '[42]');
assert_eq($r, "", "callFunction without functions");
assert_true($vm->hasError(), "callFunction without functions: NameError");

assert_eq($vm->evalWithTimeout(1000, 'upper("hello")', $php), "HELLO", "evalWithTimeout with functions");

$calls = 0;
$counter = ['tick' => function () use (&$calls) { $calls++; return $calls; }];
$r = $vm->evalWithTimeout(200, "while True:\n    tick()\n", $counter);
assert_eq($r, "", "timeout stops a loop calling PHP");
assert_contains(strtolower($vm->getLastError()), "timeout", "timeout error");
assert_true($calls > 0, "PHP function was called before the timeout");
$vm->eval(<<<'PY'
try:
    tick()
    result = "called"
except Exception as e:
    result = "gone"
PY);
assert_eq($vm->getVar("result"), "gone", "functions removed after a timeout");

$file = __DIR__ . '/fixtures/php_functions.py';
assert_eq($vm->evalFile($file, $php), "Hello, ACME PTY LTD", "evalFile with functions");
$r = $vm->evalFile($file);
assert_true($vm->hasError(), "evalFile without functions");

echo "=== Re-entrancy ===\n";

$inner = new Scriptling();
$reenter = [
    'nested' => fn(string $s) => $vm->eval("upper(" . json_encode($s) . ")", $php),
    'other_vm' => fn(int $n) => (int) $inner->eval("double($n)", ['double' => fn($x) => $x * 2]),
];
assert_eq($vm->eval('nested("abc")', $reenter), "ABC", "PHP function re-entering the same VM");
assert_eq($vm->eval('other_vm(21)', $reenter), "42", "PHP function using another VM with its own functions");
$vm->eval('nested("x")', $reenter);
$r = $vm->eval('upper("x")');
assert_true($vm->hasError(), "nested binding removed too");
assert_eq($vm->eval('nested("q")', $reenter), "Q", "outer functions still bound after the nested call");

$arr = ['f' => fn() => 1];
$arr['mutate'] = function () use (&$arr) { unset($arr['f']); $arr = null; gc_collect_cycles(); return 1; };
assert_eq($vm->eval('mutate() + f()', $arr), "2", "callback changing the passed array is safe");

echo "=== Output ===\n";

ob_start();
$vm->eval('say("from php")', ['say' => function ($s) { echo $s; }]);
assert_eq(ob_get_clean(), "from php", "echo in a PHP function goes to PHP output");

echo "=== References released ===\n";

Tracked::$destroyed = 0;
$t = new Tracked();
$vm->eval('f()', ['f' => function () use ($t) { return 1; }]);
unset($t);
assert_eq(Tracked::$destroyed, 1, "closure and its captures freed after the call");

for ($i = 0; $i < 200; $i++) {
    $vm->eval('f(1, [1, 2], {"a": 1})', ['f' => fn(...$a) => [$a, str_repeat("x", 100)]]);
}
gc_collect_cycles();
$before = memory_get_usage();
for ($i = 0; $i < 5000; $i++) {
    $obj = new Tracked();
    $vm->eval('f(1, [1, 2], {"a": 1})', ['f' => function (...$a) use ($obj) { return [$a, str_repeat("x", 100)]; }]);
    unset($obj);
}
gc_collect_cycles();
$growth = memory_get_usage() - $before;
assert_true($growth < 256 * 1024, "no PHP memory growth over 5000 calls (grew $growth bytes)");
assert_eq(Tracked::$destroyed, 5001, "every captured object destroyed");

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
