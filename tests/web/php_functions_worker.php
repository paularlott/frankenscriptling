<?php
// Worker mode: one VM per worker thread, reused for every request.
require __DIR__ . '/php_functions_lib.php';
$vm = new Scriptling();
while (frankenphp_handle_request(function () use ($vm) {
    header('Content-Type: application/json');
    echo json_encode(php_functions_request($vm));
})) {
    gc_collect_cycles();
}
