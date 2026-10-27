<?php
// Shared by the PHP-functions server tests: one request's worth of calls,
// with per-request data in the closures so a mix-up between threads shows.

function php_functions_request(Scriptling $vm): array {
    $id = (int) ($_GET['id'] ?? 0);
    $record = ['id' => $id, 'items' => range(1, 50), 'name' => "req$id"];
    $vm->setVar('suffix', "-$id");
    $out = $vm->eval(<<<'PY'
r = get()
try:
    boom()
    caught = "no"
except RuntimeError as e:
    caught = str(e)
f"{r['id']}:{total(r['items'])}:{upper(r['name'])}{suffix}:{caught}"
PY, [
        'get'   => fn() => $record,
        'total' => fn(array $xs) => array_sum($xs),
        'upper' => 'strtoupper',
        'boom'  => fn() => throw new RuntimeException("boom$id"),
    ]);
    return ['id' => $id, 'out' => $out, 'error' => $vm->hasError() ? $vm->getLastError() : '', 'mem' => memory_get_usage()];
}
