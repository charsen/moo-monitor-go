<?php

declare(strict_types=1);

// Pure fixed fake receipt/version probe. Never send(), sync(), Host, HTTP or storage.
if ($argc !== 2) {
    fwrite(STDERR, "Usage: php tools/probe-cloud-receipts.php <CloudClient.php>\n");
    exit(2);
}
$file = realpath($argv[1]);
if ($file === false || !str_ends_with($file, '/src/Cloud/CloudClient.php')) {
    throw new RuntimeException('Expected exact reviewed CloudClient source.');
}
$root = dirname($file, 3);
spl_autoload_register(static function (string $class): void { throw new RuntimeException('Unexpected autoload in receipt probe.'); });
require $root . '/src/Concerns/SafelyLogs.php';
require $file;
require dirname($file) . '/CloudSync.php';
$client = new Mooeen\MonitorLaravel\Cloud\CloudClient(['base_url' => 'https://fixture.invalid', 'token' => 'fixed-fake-token', 'timeout' => 1]);
$sync = new Mooeen\MonitorLaravel\Cloud\CloudSync('/__go_moo_receipt_never_written__/cursor.json');
$parse = new ReflectionMethod($client, 'parseItemResults');
$records = [['hash' => 'aaaaaaaaaaaa'], ['hash' => 'bbbbbbbbbbbb'], ['hash' => 'cccccccccccc']];
$valid = [
    ['index' => 0, 'hash' => 'aaaaaaaaaaaa', 'status' => 'filtered', 'retryable' => false, 'reason' => 'ingest_filter'],
    ['index' => 1, 'hash' => 'bbbbbbbbbbbb', 'status' => 'saved', 'retryable' => false, 'reason' => null],
    ['index' => 2, 'hash' => 'cccccccccccc', 'status' => 'skipped', 'retryable' => true, 'reason' => 'upsert_failed'],
];
$cases = [['partial', $valid, 1, 1, 1], ['reordered', [$valid[2], $valid[0], $valid[1]], 1, 1, 1], ['object-values', ['third' => $valid[2], 'first' => $valid[0], 'second' => $valid[1]], 1, 1, 1], ['null', null, 1, 1, 1], ['count-mismatch', $valid, 2, 0, 1], ['missing-item', array_slice($valid, 0, 2), 1, 1, 1]];
foreach ([
    ['duplicate-index', 'index', 0, 1], ['index-string', 'index', '1', 1], ['negative-index', 'index', -1, 1], ['wrong-hash', 'hash', 'dddddddddddd', 1], ['unknown-status', 'status', 'unknown', 1], ['retryable-string', 'retryable', 'false', 1], ['saved-retryable', 'retryable', true, 1], ['skipped-reason-empty', 'reason', '', 2], ['skipped-reason-null', 'reason', null, 2], ['reason-number', 'reason', 42, 2],
] as [$name, $key, $value, $index]) { $bad = $valid; $bad[$index][$key] = $value; $cases[] = [$name, $bad, 1, 1, 1]; }
$bad = $valid; unset($bad[1]['reason']); $cases[] = ['missing-reason', $bad, 1, 1, 1];
$permanent = $valid; $permanent[2]['retryable'] = false; $permanent[2]['reason'] = 'invalid_record'; $cases[] = ['permanent', $permanent, 1, 1, 1];
$out = [];
foreach ($cases as [$name, $raw, $saved, $filtered, $skipped]) {
    $out[] = compact('name', 'raw', 'saved', 'filtered', 'skipped') + ['records' => $records, 'output' => $parse->invoke($client, $raw, $records, $saved, $filtered, $skipped)];
}
$emptyCases = [];
foreach (['[]', '{}', 'null'] as $wire) { $raw = json_decode($wire, true, 512, JSON_THROW_ON_ERROR); $emptyCases[] = ['wire_results_json' => $wire, 'output' => $parse->invoke($client, $raw, [], 0, 0, 0)]; }
$versions = [];
foreach ([
    ['same', '2026-10-29T12:00:00.123+08:00', '2026-10-29T12:00:00.123+08:00'],
    ['newer', '2026-10-29T12:00:00.124+08:00', '2026-10-29T12:00:00.123+08:00'],
    ['same-zone-converted', '2026-10-29T04:00:00.123+00:00', '2026-10-29T12:00:00.123+08:00'],
    ['microsecond', '2026-10-29T12:00:00.123456+08:00', '2026-10-29T12:00:00.123455+08:00'],
    ['invalid-same', 'not-a-time', 'not-a-time'],
] as [$name, $record, $ack]) { $versions[] = compact('name', 'record', 'ack') + ['acknowledged' => (new ReflectionMethod($sync, 'isAcknowledged'))->invoke($sync, $record, $ack)]; }
$timestampCases = [];
foreach ([['meta' => ['updated_at' => 'fixture-update'], 'last_seen' => 'fixture-last'], ['last_seen' => 'fixture-last'], ['meta' => ['updated_at' => ''], 'last_seen' => 'fixture-last']] as $input) { $timestampCases[] = ['input' => $input, 'output' => (new ReflectionMethod($sync, 'recordTimestamp'))->invoke($sync, $input)]; }
$process = proc_open(['git', '-C', $root, 'rev-parse', 'HEAD'], [0 => ['pipe','r'], 1 => ['pipe','w'], 2 => ['pipe','w']], $pipes);
if (!is_resource($process)) { throw new RuntimeException('Cannot read source commit.'); }
fclose($pipes[0]); $commit = trim(stream_get_contents($pipes[1])); fclose($pipes[1]); fclose($pipes[2]);
if (proc_close($process) !== 0 || !preg_match('/^[a-f0-9]{40}$/D', $commit)) { throw new RuntimeException('Cannot read source commit.'); }
echo json_encode(['evidence' => 'fixed fake receipts/versions; three source declarations; constructor fake cfg/cursor; Reflection only; no send/sync/HTTP/Host/storage', 'source_commit' => $commit, 'source_sha256' => hash_file('sha256', $file), 'sync_sha256' => hash_file('sha256', dirname($file) . '/CloudSync.php'), 'php_version' => PHP_VERSION, 'cases' => $out, 'empty_cases' => $emptyCases, 'versions' => $versions, 'timestamps' => $timestampCases], JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR), "\n";
