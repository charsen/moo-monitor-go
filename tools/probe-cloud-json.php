<?php

declare(strict_types=1);

// Native encoding step used by the reviewed Guzzle json option, fixed fake values only.
if ($argc !== 2) { fwrite(STDERR, "Usage: php tools/probe-cloud-json.php <GuzzleClient.php>\n"); exit(2); }
$file = realpath($argv[1]);
if ($file === false || !str_ends_with($file, '/vendor/guzzlehttp/guzzle/src/Client.php')) { throw new RuntimeException('Expected exact reviewed Guzzle Client source.'); }
$cases = [
    ['empty-array', ['type' => 'array', 'values' => []], []],
    ['empty-object', ['type' => 'object', 'entries' => []], (object) []],
    ['ordered-object', ['type' => 'object', 'entries' => [['key' => 'z', 'value' => 1], ['key' => 'a', 'value' => null]]], (object) ['z' => 1, 'a' => null]],
    ['large-int', ['type' => 'int64', 'value' => '9007199254740993'], 9007199254740993],
    ['max-int', ['type' => 'int64', 'value' => (string) PHP_INT_MAX], PHP_INT_MAX],
    ['zero-float', ['type' => 'float', 'value' => 0.0], 0.0],
    ['fraction', ['type' => 'float', 'value' => 1.5], 1.5],
    ['unicode', ['type' => 'string', 'value' => '固定中文😀/path'], '固定中文😀/path'],
    ['null', ['type' => 'null'], null],
    ['previous', ['type' => 'json', 'value' => [['class' => 'FixturePrevious', 'message' => 'fake', 'file' => 'app/fake.php', 'line' => 1]]], [['class' => 'FixturePrevious', 'message' => 'fake', 'file' => 'app/fake.php', 'line' => 1]]],
    ['php-context', ['type' => 'json', 'value' => ['env' => 'testing', 'project' => 'fixture', 'php' => 'fixture-php', 'laravel' => 'fixture-laravel', 'occurred_at' => '2026-10-29T12:00:00.123+08:00']], ['env' => 'testing', 'project' => 'fixture', 'php' => 'fixture-php', 'laravel' => 'fixture-laravel', 'occurred_at' => '2026-10-29T12:00:00.123+08:00']],
    ['go-context', ['type' => 'json', 'value' => ['env' => 'testing', 'project' => 'fixture', 'go' => 'fixture-go', 'goravel' => 'fixture-goravel', 'occurred_at' => '2026-10-29T12:00:00.123+08:00']], ['env' => 'testing', 'project' => 'fixture', 'go' => 'fixture-go', 'goravel' => 'fixture-goravel', 'occurred_at' => '2026-10-29T12:00:00.123+08:00']],
    ['invalid-utf8', ['type' => 'bytes', 'base64' => base64_encode("x\xff\xfe")], "x\xff\xfe"],
    ['cut-trace', ['type' => 'bytes', 'base64' => base64_encode(substr(str_repeat('中', 10), 0, 5))], substr(str_repeat('中', 10), 0, 5)],
    ['nan', ['type' => 'nonfinite', 'value' => 'nan'], NAN],
    ['infinity', ['type' => 'nonfinite', 'value' => 'infinity'], INF],
];
$output = [];
foreach ($cases as [$name, $spec, $value]) {
    $encoded = json_encode($value);
    $output[] = ['name' => $name, 'input_spec' => $spec, 'json' => $encoded === false ? null : $encoded, 'error_code' => json_last_error()];
}
echo json_encode(['evidence' => 'native json_encode fixed fake values; matches reviewed Guzzle json-option call; no require/autoload/Host/HTTP', 'guzzle_source_sha256' => hash_file('sha256', $file), 'php_version' => PHP_VERSION, 'cases' => $output], JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR), "\n";
