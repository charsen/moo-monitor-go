<?php

declare(strict_types=1);

// Pure fixed fake values only: no record(), handle(), Host, YAML, database, HTTP or file writes.
if ($argc !== 2) {
    fwrite(STDERR, "Usage: php tools/probe-slow-query-records.php <SqlSlowRecorder.php>\n");
    exit(2);
}
$file = realpath($argv[1]);
if ($file === false || !str_ends_with($file, '/src/Recorder/SqlSlowRecorder.php')) {
    throw new RuntimeException('Expected reviewed SqlSlowRecorder source.');
}
$root = dirname($file, 3);
spl_autoload_register(static function (string $class): void {
    throw new RuntimeException('Unexpected autoload: ' . $class);
});
$declarations = [
    'SafelyLogs' => $root . '/src/Concerns/SafelyLogs.php',
    'SensitiveMasker' => $root . '/src/Recorder/SensitiveMasker.php',
    'BucketedYamlRecorder' => $root . '/src/Recorder/BucketedYamlRecorder.php',
    'SqlSlowRecorder' => $file,
    'SqlSlowListener' => $root . '/src/Recorder/SqlSlowListener.php',
];
foreach ($declarations as $path) {
    require $path;
}
function base_path(): string { return '/__go_moo_slow_fixture__'; }
function config($key, $default = null) {
    return match ($key) {
        'app.env' => 'testing',
        'app.name' => 'fixed-slow-fixture',
        default => throw new RuntimeException('Unexpected config access.'),
    };
}
class FixedSlowRecorder extends Mooeen\MonitorLaravel\Recorder\SqlSlowRecorder {
    public string $clock = '2026-10-30T12:34:56.789+08:00';
    protected function nowIso(): string { return $this->clock; }
}
$keys = ['password', 'pwd', 'token', 'secret', 'api_key', 'authorization'];
$recorder = new FixedSlowRecorder('/__go_moo_slow_fixture_never_written__', ['enabled' => true, 'threshold_ms' => 100, 'mask_keys' => $keys]);
$listener = new Mooeen\MonitorLaravel\Recorder\SqlSlowListener($recorder);
$call = static fn (string $method, ...$args) => (new ReflectionMethod(Mooeen\MonitorLaravel\Recorder\SqlSlowRecorder::class, $method))->invoke($recorder, ...$args);
$bindingCall = static fn (string $method, ...$args) => (new ReflectionMethod(Mooeen\MonitorLaravel\Recorder\SqlSlowListener::class, $method))->invoke($listener, ...$args);
$hashCases = [];
foreach ([
    ['plain', 'select * from foods where id = 123', '/__go_moo_slow_fixture__/app/query.php', 11],
    ['numbers', 'select * from foods where id = 456', '/__go_moo_slow_fixture__/app/query.php', 11],
    ['whitespace', " select\t*\nfrom foods\vwhere id = 123 ", '/__go_moo_slow_fixture__/app/query.php', 11],
    ['in-one', 'select * from foods where id in (?)', '/__go_moo_slow_fixture__/app/query.php', 11],
    ['in-many', 'select * from foods where id in ( ? , ? , ? )', '/__go_moo_slow_fixture__/app/query.php', 11],
    ['empty-in', 'select * from foods where id in ()', '/__go_moo_slow_fixture__/app/query.php', 11],
    ['case-preserved', 'SELECT * FROM foods WHERE id = 123', '/__go_moo_slow_fixture__/app/query.php', 11],
    ['quoted-number', "select 'literal 123' as value", '/__go_moo_slow_fixture__/app/query.php', 11],
    ['line', 'select * from foods where id = 123', '/__go_moo_slow_fixture__/app/query.php', 12],
    ['prefix-collision', 'select * from foods where id = 123', '/__go_moo_slow_fixture__suffix/query.php', 11],
    ['outside', 'select * from foods where id = 123', '/outside/query.php', 11],
    ['byte-cut', 'select ' . str_repeat('中', 3000), '/__go_moo_slow_fixture__/app/query.php', 11],
    ['tail-a', str_repeat('x', 8192) . 'A', '/__go_moo_slow_fixture__/app/query.php', 11],
    ['tail-b', str_repeat('x', 8192) . 'B', '/__go_moo_slow_fixture__/app/query.php', 11],
] as [$name, $raw, $fileName, $line]) {
    $normalized = $call('normalizeSql', $raw);
    $hashCases[] = ['name' => $name, 'raw' => $raw, 'file' => $fileName, 'line' => $line, 'normalized_base64' => base64_encode($normalized), 'normalized_bytes' => strlen($normalized), 'hash' => $call('makeHash', $raw, $fileName, $line)];
}
$bindingCases = [];
foreach ([
    ['scalar-values', 'select ?, ?, ?, ?, ?', ['fixed', 42, true, false, null]],
    ['escape-and-percent', "select DATE_FORMAT(created_at,'%Y-%m-%d'), ?", ["fake\\folder'quoted"]],
    ['date', 'select ?', [new DateTimeImmutable('2026-10-30T01:02:03+08:00')]],
    ['float', 'select ?, ?, ?', [1.005, 2.675, 1.0e20]],
    ['missing', 'select ?, ?, ?', [42]],
    ['extra', 'select ?', [42, 43]],
    ['question-in-quote', "select '?' as literal, ? as bound", ['first', 'second']],
    ['no-placeholder', 'select 1', ['ignored']],
] as [$name, $raw, $bindings]) {
    $input = [];
    foreach ($bindings as $value) {
        $input[] = $value instanceof DateTimeInterface ? ['type' => 'date', 'value' => $value->format(DateTimeInterface::ATOM)] : ['type' => get_debug_type($value), 'value' => $value];
    }
    $bindingCases[] = ['name' => $name, 'raw' => $raw, 'bindings' => $input, 'last' => $bindingCall('fillBindings', $raw, $bindings)];
}
$roundCases = [];
foreach ([0.0, 1.0049, 1.005, 1.0051, 1.0049999999999997, 1.0050000000000001, 2.675, 2.6749999999999, 2.67500000000001, 0.005, 1234567890123.455, PHP_FLOAT_MAX] as $value) {
    $roundCases[] = ['input' => $value, 'rounded' => round($value, 2)];
}
$truncateCases = [];
foreach ([['ascii-boundary', str_repeat('a', 4096)], ['ascii-over', str_repeat('a', 4097)], ['unicode-character-over', str_repeat('中', 4097)], ['unicode-byte-gap', str_repeat('中', 2000)], ['unicode-small', '中文SQL']] as [$name, $value]) {
    $output = $call('truncate', $value, 4096);
    $truncateCases[] = ['name' => $name, 'input_base64' => base64_encode($value), 'input_bytes' => strlen($value), 'source_length' => $call('strLen', $value), 'output_base64' => base64_encode($output), 'output_bytes' => strlen($output), 'output_valid_utf8' => preg_match('//u', $output) === 1];
}
$raw = "select * from foods where token = 'fake-raw-secret' and id = ?";
$last = "select * from foods where token = 'fake-last-secret' and id = 42";
$fileName = '/__go_moo_slow_fixture__/app/query.php';
$hash = $call('makeHash', $raw, $fileName, 11);
$built = $call('build', $hash, $raw, $last, 100.125, $fileName, 11, 'fixture-connection', null, $recorder->clock);
$existing = $built;
$existing['status'] = 'resolved';
$existing['resolved_at'] = $recorder->clock;
$existing['resolved_by'] = 'fixture-actor';
$existing['resolved_note'] = 'fixture resolution';
$recorder->clock = '2026-10-30T13:00:00.123+08:00';
$sameDay = $call('refresh', $existing, "select * from foods where token='fake-next-secret' and id=43", 90.0, $fileName, 11, null, null, $recorder->clock, 3);
$recorder->clock = '2026-10-31T00:00:00.001+08:00';
$nextDay = $call('refresh', $sameDay, 'select 1', 200.005, $fileName, 11, 'next-connection', null, $recorder->clock, -2);
$unicode = $call('build', $call('makeHash', 'select 中文', $fileName, 11), 'select 中文', 'select 中文', 100.0, $fileName, 11, null, null, $recorder->clock);
$hashes = [];
foreach ($declarations as $name => $path) {
    $hashes[$name] = hash_file('sha256', $path);
}
$process = proc_open(['git', '-C', $root, 'rev-parse', 'HEAD'], [0 => ['pipe', 'r'], 1 => ['pipe', 'w'], 2 => ['pipe', 'w']], $pipes);
if (!is_resource($process)) {
    throw new RuntimeException('Cannot read source commit.');
}
fclose($pipes[0]);
$commit = trim(stream_get_contents($pipes[1]));
fclose($pipes[1]);
stream_get_contents($pipes[2]);
fclose($pipes[2]);
if (proc_close($process) !== 0 || preg_match('/^[a-f0-9]{40}$/', $commit) !== 1) {
    throw new RuntimeException('Cannot read source commit.');
}
echo json_encode(['evidence' => 'fixed fake values; five declarations; guarded autoload; only private pure components; no record/handle/Host/DB/HTTP/YAML/file writes', 'source_commit' => $commit, 'source_sha256' => $hashes, 'php_version' => PHP_VERSION, 'mb_strlen_available' => function_exists('mb_strlen'), 'precision' => ini_get('precision'), 'keys' => $keys, 'base_path' => base_path(), 'hash_cases' => $hashCases, 'binding_cases' => $bindingCases, 'round_cases' => $roundCases, 'truncate_cases' => $truncateCases, 'build' => $built, 'same_day_refresh' => $sameDay, 'next_day_refresh' => $nextDay, 'unicode_build' => $unicode], JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR), "\n";
