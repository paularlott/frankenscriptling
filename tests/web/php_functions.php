<?php
// Classic mode: a new VM per request.
require __DIR__ . '/php_functions_lib.php';
header('Content-Type: application/json');
echo json_encode(php_functions_request(new Scriptling()));
