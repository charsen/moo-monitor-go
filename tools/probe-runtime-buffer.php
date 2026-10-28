<?php

declare(strict_types=1);

// Controlled real source I/O: only a fresh private owned temp. Never Host or external cache.
if ($argc !== 4) {
    fwrite(STDERR, "Usage: php tools/probe-runtime-buffer.php <RuntimeErrorRecorder.php> <symfony/yaml-dir> <fresh-private-temp>\n");
    exit(2);
}
$source = realpath($argv[1]);
$yaml = realpath($argv[2]);
$temp = realpath($argv[3]);
if ($source === false || !str_ends_with($source, '/src/Recorder/RuntimeErrorRecorder.php') || $yaml === false || !str_ends_with($yaml, '/vendor/symfony/yaml') || $temp === false || is_link($argv[3]) || !is_dir($temp) || (fileperms($temp) & 0077) !== 0 || !str_starts_with(basename($temp), 'go-moo-php-buffer-') || array_diff(scandir($temp), ['.', '..']) !== []) {
    throw new RuntimeException('Expected exact source/Yaml and fresh private temp root.');
}
$sourceRoot = dirname($source, 3);
foreach ([$sourceRoot, dirname($yaml, 3)] as $protected) {
    if (str_starts_with($temp . '/', $protected . '/') || str_starts_with($protected . '/', $temp . '/')) {
        throw new RuntimeException('Temp/source overlap refused.');
    }
}
if (!extension_loaded('ctype') || !extension_loaded('mbstring')) {
    throw new RuntimeException('Reviewed native ctype/mbstring required.');
}
$ownership = bin2hex(random_bytes(16));
if (file_put_contents($temp . '/.owner', $ownership, LOCK_EX) !== strlen($ownership)) {
    throw new RuntimeException('Cannot establish temp ownership.');
}
chmod($temp . '/.owner', 0600);
try {
$loadedYaml = [];
spl_autoload_register(static function (string $class) use ($yaml, &$loadedYaml): void {
    $prefix = 'Symfony\\Component\\Yaml\\';
    if (!str_starts_with($class, $prefix) || !preg_match('/^[A-Za-z_][A-Za-z0-9_]*(?:\\\\[A-Za-z_][A-Za-z0-9_]*)*$/D', substr($class, strlen($prefix)))) {
        throw new RuntimeException('Unexpected autoload in pure buffer probe.');
    }
    $path = realpath($yaml . '/' . str_replace('\\', '/', substr($class, strlen($prefix))) . '.php');
    if ($path === false || !str_starts_with($path, $yaml . '/') || !is_file($path)) {
        throw new RuntimeException('Unreviewed YAML class path.');
    }
    $loadedYaml[substr($path, strlen($yaml) + 1)] = hash_file('sha256', $path);
    require $path;
});
require dirname($yaml) . '/deprecation-contracts/function.php';
require $sourceRoot . '/src/Concerns/SafelyLogs.php';
require $sourceRoot . '/src/StorageScope.php';
require dirname($source) . '/SensitiveMasker.php';
require dirname($source) . '/BucketedYamlRecorder.php';
require $source;

function base_path(): string { return '/__go_moo_buffer_fake_source__'; }
final class BufferFakeCache {
    private array $values = [];
    public function add(string $key, $value, int $ttl): bool { if (array_key_exists($key, $this->values)) { return false; } $this->values[$key] = $value; return true; }
    public function increment(string $key): int { return ++$this->values[$key]; }
    public function get(string $key, $default = null) { return $this->values[$key] ?? $default; }
    public function forget(string $key): void { unset($this->values[$key]); }
    public function remember(string $key, int $ttl, callable $load) { return $this->values[$key] ??= $load(); }
}
function cache(): BufferFakeCache { static $cache; return $cache ??= new BufferFakeCache; }
final class BufferFakeException extends Exception {
    public function __construct(string $message = 'fixture failure 123', int $line = 17) { parent::__construct($message); $this->file = '/__go_moo_buffer_fake_source__/app/fake.php'; $this->line = $line; }
}
final class BufferSourceProbe extends Mooeen\MonitorLaravel\Recorder\RuntimeErrorRecorder {
    public string $clock = '2026-10-29T12:00:00.123+08:00';
    protected function nowIso(): string { return $this->clock; }
    public function persistFake(Throwable $e): ?string {
        $call = fn (string $method, ...$args) => (new ReflectionMethod(Mooeen\MonitorLaravel\Recorder\RuntimeErrorRecorder::class, $method))->invoke($this, ...$args);
        $hash = $call('makeHash', $e);
        return $this->persistAggregated($hash, fn ($now) => $call('build', $hash, $e, null, $now), fn ($existing, $now, $overflow) => $call('refresh', $existing, $e, null, $now, 'reportable', [], false, $overflow), static function (): void { throw new RuntimeException('Source write failed.'); });
    }
}
$base = $temp . '/runtimes';
$steps = [];
    $recorder = new BufferSourceProbe($base, ['mask_keys' => ['password', 'token'], 'daily_cap' => 2, 'max_open' => 1]);
    $e = new BufferFakeException;
    $capture = static function (string $name, string $hash) use (&$steps, $recorder, $base): void {
        $paths = [];
        foreach (['open', 'resolved', 'deleted'] as $bucket) { if (is_file($base . '/' . $bucket . '/' . $hash . '.yaml')) { $paths[] = $bucket; } }
        $steps[] = ['name' => $name, 'hash' => $hash, 'buckets' => $paths, 'record' => $recorder->get($hash), 'count_all' => $recorder->count(), 'count_open' => $recorder->count('open'), 'rows' => $recorder->list('all')];
    };
    $hash = $recorder->persistFake($e);
    if ($hash === null) { throw new RuntimeException('Expected source new record.'); }
    $capture('new', $hash);
    $firstYaml = file_get_contents($base . '/open/' . $hash . '.yaml');
    for ($i = 2; $i <= 5; ++$i) { $recorder->persistFake($e); }
    $frozen = file_get_contents($base . '/open/' . $hash . '.yaml');
    $capture('five-same-day-cap-two', $hash);
    $recorder->clock = '2026-10-30T00:00:00.001+08:00';
    $recorder->persistFake($e);
    $capture('next-day-backfill', $hash);
    $capacityResult = $recorder->persistFake(new BufferFakeException('other identity', 99));
    foreach (['resolved', 'deleted'] as $bucket) {
        $record = $recorder->get($hash);
        $record['status'] = $bucket;
        $record['resolved_at'] = $recorder->clock;
        $record['resolved_by'] = 'fixture-actor';
        $record['resolved_note'] = 'fixture note';
        file_put_contents($base . '/' . $bucket . '/' . $hash . '.yaml', Symfony\Component\Yaml\Yaml::dump($record, 8, 2));
        unlink($base . '/open/' . $hash . '.yaml');
        $recorder->persistFake($e);
        $capture($bucket . '-recurs', $hash);
    }
    $installed = json_decode(file_get_contents(dirname($yaml, 2) . '/composer/installed.json'), true, 512, JSON_THROW_ON_ERROR);
    $version = null;
    foreach ($installed['packages'] ?? $installed as $item) { if ($item['name'] === 'symfony/yaml') { $version = $item['version']; } }
    $process = proc_open(['git', '-C', $sourceRoot, 'rev-parse', 'HEAD'], [0 => ['pipe','r'], 1 => ['pipe','w'], 2 => ['pipe','w']], $pipes);
    if (!is_resource($process)) { throw new RuntimeException('Cannot read source commit.'); }
    fclose($pipes[0]); $commit = trim(stream_get_contents($pipes[1])); fclose($pipes[1]); fclose($pipes[2]);
    if (proc_close($process) !== 0 || !preg_match('/^[a-f0-9]{40}$/D', $commit)) { throw new RuntimeException('Cannot read source commit.'); }
    ksort($loadedYaml);
    $result = ['evidence' => 'fixed fake source buffer in exclusive owned temp; five source declarations; restricted YAML PSR4; in-memory FakeCache; no Host/record/bootstrap/DB/Cloud', 'source_commit' => $commit, 'source_sha256' => hash_file('sha256', $source), 'bucketed_sha256' => hash_file('sha256', dirname($source) . '/BucketedYamlRecorder.php'), 'php_version' => PHP_VERSION, 'yaml_version' => $version, 'yaml_classes_sha256' => $loadedYaml, 'capacity_result' => $capacityResult, 'new_yaml' => $firstYaml, 'frozen_yaml' => $frozen, 'steps' => $steps];
} finally {
    if (!is_file($temp . '/.owner') || file_get_contents($temp . '/.owner') !== $ownership) { throw new RuntimeException('Ownership lost; temp retained.'); }
    // Validate the entire cleanup set first. Unknown entries/symlinks keep all evidence in place.
    $files = []; $dirs = [];
    foreach (new RecursiveIteratorIterator(new RecursiveDirectoryIterator($temp, FilesystemIterator::SKIP_DOTS), RecursiveIteratorIterator::SELF_FIRST) as $entry) {
        $relative = substr($entry->getPathname(), strlen($temp) + 1);
        if ($entry->isLink()) { throw new RuntimeException('Unexpected cleanup symlink; temp retained.'); }
        if ($entry->isDir()) {
            if (!preg_match('~^runtimes(?:/(?:open|resolved|deleted))?$~D', $relative)) { throw new RuntimeException('Unknown cleanup directory; temp retained.'); }
            $dirs[] = $entry->getPathname();
        } else {
            if ($relative !== '.owner' && $relative !== 'runtimes/.gitignore' && !preg_match('~^runtimes/(?:open|resolved|deleted)/[a-f0-9]{12}\.yaml(?:\.tmp[a-f0-9]{8})?$~D', $relative)) { throw new RuntimeException('Unknown cleanup file; temp retained.'); }
            if ($relative !== '.owner') { $files[] = $entry->getPathname(); }
        }
    }
    foreach ($files as $path) { if (!unlink($path)) { throw new RuntimeException('Temp cleanup failed.'); } }
    usort($dirs, static fn ($a, $b) => strlen($b) <=> strlen($a));
    foreach ($dirs as $path) { if (!rmdir($path)) { throw new RuntimeException('Temp cleanup failed.'); } }
    if (!unlink($temp . '/.owner')) { throw new RuntimeException('Temp ownership cleanup failed: ' . $temp); }
    if (!rmdir($temp)) { throw new RuntimeException('Temp root cleanup failed: ' . $temp); }
}
echo json_encode($result, JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR), "\n";
