<?php
// exit() and fatal errors inside a PHP function called from a script end the
// request, as they would in plain PHP. Each mode runs in its own process;
// `make test-php-functions` checks the exit code and output of each.
//
//   exit           exit(3) in a PHP function: process exits 3, nothing after runs
//   exit_in_try    the same inside try/except Exception: the script cannot catch it
//   fatal          memory exhaustion in a PHP function: exits 255, no crash
//   fatal_caught   the same with PHP try/catch around eval: not catchable either
//   fatal_timeout  max_execution_time reached inside a PHP function

ini_set('display_errors', 'stdout');
$mode = $argv[1] ?? '';
register_shutdown_function(function () { echo "SHUTDOWN\n"; });

$vm = new Scriptling();
$functions = [
    'quit' => function () { echo "QUITTING\n"; exit(3); },
    'hog'  => function () { ini_set('memory_limit', '32M'); return strlen(str_repeat('x', 128 * 1024 * 1024)); },
    'spin' => function () { set_time_limit(1); $t = microtime(true); while (true) { if (microtime(true) - $t > 30) { break; } } return 1; },
    'never' => function () { echo "NEVER\n"; return 1; },
];

switch ($mode) {
case 'exit':
    $vm->eval("quit()\nnever()", $functions);
    break;
case 'exit_in_try':
    $vm->eval("try:\n    quit()\nexcept Exception as e:\n    pass\nnever()", $functions);
    break;
case 'fatal':
    $vm->eval("hog()\nnever()", $functions);
    break;
case 'fatal_caught':
    try {
        $vm->eval("try:\n    hog()\nexcept Exception as e:\n    pass\nnever()", $functions);
    } catch (Throwable $t) {
        echo "CAUGHT\n";
    }
    break;
case 'fatal_timeout':
    $vm->eval("spin()\nnever()", $functions);
    break;
default:
    fwrite(STDERR, "unknown mode '$mode'\n");
    exit(2);
}

echo "AFTER\n";
