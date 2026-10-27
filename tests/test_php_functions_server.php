<?php
// Hammers a FrankenPHP server running tests/web/php_functions*.php with
// concurrent requests: every response must carry its own request's data, and
// in worker mode memory must not grow. Usage: <url> <requests> <concurrency> <worker 0|1>

[$_, $url, $total, $conc, $worker] = $argv + [null, '', 2000, 32, 0];
$total = (int) $total; $conc = (int) $conc; $worker = (bool) $worker;

$fail = 0; $done = 0; $next = 0; $mems = []; $errors = [];
$mh = curl_multi_init();
$add = function () use (&$next, $total, $url, $mh) {
    if ($next >= $total) return;
    $id = $next++;
    $ch = curl_init("$url?id=$id");
    curl_setopt_array($ch, [CURLOPT_RETURNTRANSFER => true, CURLOPT_TIMEOUT => 30, CURLOPT_PRIVATE => (string) $id]);
    curl_multi_add_handle($mh, $ch);
};
for ($i = 0; $i < $conc; $i++) $add();

do {
    curl_multi_exec($mh, $running);
    curl_multi_select($mh, 1.0);
    while ($info = curl_multi_info_read($mh)) {
        $ch = $info['handle'];
        $id = (int) curl_getinfo($ch, CURLINFO_PRIVATE);
        $code = curl_getinfo($ch, CURLINFO_HTTP_CODE);
        $body = json_decode(curl_multi_getcontent($ch), true);
        $want = "$id:1275:REQ$id-$id:boom$id";
        if ($code !== 200 || !is_array($body) || $body['id'] !== $id || $body['out'] !== $want || $body['error'] !== '') {
            $fail++;
            if (count($errors) < 5) $errors[] = "id $id: HTTP $code " . substr(var_export($body, true), 0, 300);
        } else {
            $mems[] = $body['mem'];
        }
        $done++;
        curl_multi_remove_handle($mh, $ch);
        $add();
    }
} while ($running || $done < $total);

echo ($worker ? "worker" : "classic") . ": $done requests, $fail failed\n";
foreach ($errors as $e) echo "  $e\n";

if ($worker && count($mems) > 400) {
    // Memory per worker thread: compare the highest reading early on with the highest at the end.
    $early = max(array_slice($mems, 100, 200));
    $late = max(array_slice($mems, -200));
    $growth = $late - $early;
    echo "worker memory: early max $early, late max $late, growth $growth\n";
    if ($growth > 1024 * 1024) { echo "FAIL memory grew by $growth bytes\n"; $fail++; }
}
exit($fail > 0 ? 1 : 0);
