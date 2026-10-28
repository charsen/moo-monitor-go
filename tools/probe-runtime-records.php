<?php

declare(strict_types=1);

// Fixed fake inputs only. Never invoke record(), persistence, Host or Composer.
if ($argc !== 2) {
    fwrite(STDERR, "Usage: php tools/probe-runtime-records.php <RuntimeErrorRecorder.php>\n");
    exit(2);
}
$file = realpath($argv[1]);
if ($file === false || !str_ends_with($file, '/src/Recorder/RuntimeErrorRecorder.php')) {
    throw new RuntimeException('Expected reviewed RuntimeErrorRecorder source.');
}
spl_autoload_register(static function (string $class): void {
    throw new RuntimeException('Unexpected autoload: ' . $class);
});
$root = dirname($file, 3);
require $root . '/src/Concerns/SafelyLogs.php';
require dirname($file) . '/SensitiveMasker.php';
require dirname($file) . '/BucketedYamlRecorder.php';
require $file;
function base_path(): string { return $GLOBALS['record_fixture_base'] ?? '/__go_moo_record_fixture__'; }
class FixedRecordException extends Exception {
    public function __construct(string $message, int $code = 0, string $file = '/__go_moo_record_fixture__/app/fake.php', int $line = 41, ?Throwable $previous = null) {
        parent::__construct($message, $code, $previous);
        $this->file = $file;
        $this->line = $line;
    }
}
class OtherRecordException extends FixedRecordException {}
class FixedRecorder extends Mooeen\MonitorLaravel\Recorder\RuntimeErrorRecorder {
    public string $clock = '2026-10-29T12:34:56.789+08:00';
    protected function nowIso(): string { return $this->clock; }
}
$keys = ['password', 'token', 'secret', 'authorization'];
$recorder = new FixedRecorder('/__go_moo_record_fixture_never_written__', ['mask_keys' => $keys]);
$call = static fn (string $method, ...$args) => (new ReflectionMethod(Mooeen\MonitorLaravel\Recorder\RuntimeErrorRecorder::class, $method))->invoke($recorder, ...$args);
$messages = [
    'plain' => 'Missing item 123 at attempt 42',
    'sql-before-secret' => "select * from t where token = 'fake-token' and id = 123 password=fake-password",
    'quoted' => "item 'alpha 123' and \"beta 456\"",
    'uuid-hex' => 'id 123e4567-e89b-12d3-a456-426614174000 ptr 0xABC123 hash abcdef0123456789 number 456',
    'byte-cut' => str_repeat('中', 342),
    'tail-a' => str_repeat('z', 1024) . 'A',
    'tail-b' => str_repeat('z', 1024) . 'B',
    'ascii-digit-only' => '数字１２３ 123',
];
$hashCases = [];
foreach ($messages as $name => $message) {
    $e = new FixedRecordException($message);
    $normalized = $call('normalizeMessage', $message);
    $hashCases[] = ['name' => $name, 'class' => get_class($e), 'file' => $e->getFile(), 'line' => $e->getLine(), 'message' => $message, 'normalized_base64' => base64_encode($normalized), 'normalized_bytes' => strlen($normalized), 'hash' => $call('makeHash', $e)];
}
foreach ([['class', new OtherRecordException($messages['plain'])], ['line', new FixedRecordException($messages['plain'], 0, '/__go_moo_record_fixture__/app/fake.php', 42)], ['prefix-collision', new FixedRecordException($messages['plain'], 0, '/__go_moo_record_fixture__suffix/fake.php')], ['outside', new FixedRecordException($messages['plain'], 0, '/outside/fake.php')], ['code-irrelevant', new FixedRecordException($messages['plain'], 999)]] as [$name, $e]) {
    $normalized = $call('normalizeMessage', $e->getMessage());
    $hashCases[] = ['name' => $name, 'class' => get_class($e), 'file' => $e->getFile(), 'line' => $e->getLine(), 'message' => $e->getMessage(), 'normalized_base64' => base64_encode($normalized), 'normalized_bytes' => strlen($normalized), 'hash' => $call('makeHash', $e)];
}
$pathCases = [];
foreach ([['/root/', '/root-other/fake.php'], ['/root/', '/root/app/fake.php'], ['/', '/outside/fake.php']] as [$base, $path]) {
    $GLOBALS['record_fixture_base'] = $base;
    $pathCases[] = ['base_path' => $base, 'file' => $path, 'relative' => $call('relPath', $path)];
}
unset($GLOBALS['record_fixture_base']);
$metaCases = [];
foreach ([
    ['fresh', ' HTTP 5xx! ', ['password' => 'fake', 'note' => 'Bearer fake-token', 'zero' => 0, 'no' => false, 'null' => null, 'empty' => '', 'nested' => ['token' => 'fake']], []],
    ['priority-kept', 'reportable', ['note' => "token='fake'", 'zero' => 0], ['source' => 'queue_failed', 'sources' => ['http_5xx', 'queue_failed'], 'legacy_password' => 'fake', 'old' => 'Bearer fake-old']],
    ['priority-raised', 'schedule_exit', [], ['source' => 'http_5xx', 'sources' => ['reportable']]],
    ['unknown-tie', 'custom-next', [], ['source' => 'custom-old', 'sources' => ['custom-old']]],
    ['zero-source', '0', [], []],
    ['fallback', '---', ['source' => 'fake-injected', 'sources' => 'fake'], []],
    ['truncate-after-mask', str_repeat('x', 70), ['long' => str_repeat('中', 510), 'credential' => 'Bearer ' . str_repeat('a', 600)], []],
] as [$name, $source, $meta, $existing]) {
    $metaCases[] = compact('name', 'source', 'meta', 'existing') + ['output' => $call('runtimeMeta', $source, $meta, $existing)];
}
$previous = null;
for ($i = 4; $i >= 1; --$i) { $previous = new FixedRecordException('previous ' . $i . ' password=fake', $i, '/__go_moo_record_fixture__/app/previous.php', $i, $previous); }
$e = new FixedRecordException("select * from t where token='fake'", 17, '/__go_moo_record_fixture__/app/fake.php', 41, $previous);
$hash = $call('makeHash', $e);
$now = $recorder->clock;
$built = $call('build', $hash, $e, null, $now, 'http_5xx', ['password' => 'fake', 'attempt' => 0]);
$existing = $built;
$existing['status'] = 'resolved';
$existing['resolved_at'] = $now;
$existing['resolved_by'] = 'fixture-actor';
$existing['resolved_note'] = 'fixture note';
$recorder->clock = '2026-10-29T13:00:00.123+08:00';
$sameDay = $call('refresh', $existing, $e, null, $recorder->clock, 'reportable', ['latest' => 'Bearer fake'], false, 3);
$recorder->clock = '2026-10-30T00:00:00.001+08:00';
$nextDay = $call('refresh', $sameDay, $e, null, $recorder->clock, 'queue_failed', [], false, -2);
$hashes = [];
foreach (['SafelyLogs' => $root . '/src/Concerns/SafelyLogs.php', 'SensitiveMasker' => dirname($file) . '/SensitiveMasker.php', 'BucketedYamlRecorder' => dirname($file) . '/BucketedYamlRecorder.php', 'RuntimeErrorRecorder' => $file] as $name => $path) { $hashes[$name] = hash_file('sha256', $path); }
$process = proc_open(['git', '-C', $root, 'rev-parse', 'HEAD'], [0 => ['pipe','r'], 1 => ['pipe','w'], 2 => ['pipe','w']], $pipes);
if (!is_resource($process)) { throw new RuntimeException('Cannot read source commit.'); }
fclose($pipes[0]); $commit = trim(stream_get_contents($pipes[1])); fclose($pipes[1]); $error = stream_get_contents($pipes[2]); fclose($pipes[2]);
if (proc_close($process) !== 0 || !preg_match('/^[a-f0-9]{40}$/', $commit)) { throw new RuntimeException('Cannot read source commit: ' . $error); }
echo json_encode(['evidence' => 'fixed fake inputs; four declarations; guarded autoload; private build/refresh/hash/meta only; nonexistent snippet path only stat; no record/Host/DB/Cloud/persistence', 'source_commit' => $commit, 'source_sha256' => $hashes, 'php_version' => PHP_VERSION, 'keys' => $keys, 'base_path' => base_path(), 'hash_cases' => $hashCases, 'path_cases' => $pathCases, 'meta_cases' => $metaCases, 'build' => $built, 'same_day_refresh' => $sameDay, 'next_day_refresh' => $nextDay], JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR), "\n";
