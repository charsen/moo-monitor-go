<?php

declare(strict_types=1);

// Pure fixed-fake-input source probe: load only the exact masker class, never Host/autoload.
if ($argc !== 2 && !($argc === 3 && $argv[2] === '--sql')) {
    fwrite(STDERR, "Usage: php tools/probe-sensitive-masker.php <SensitiveMasker.php> [--sql]\n");
    exit(2);
}
$file = realpath($argv[1]);
if ($file === false || !str_ends_with($file, '/src/Recorder/SensitiveMasker.php')) {
    throw new RuntimeException('Expected the reviewed SensitiveMasker source file.');
}
require $file;

$base = ['password', 'pwd', 'token', 'secret', 'api_key', 'authorization'];
$jwt = 'eyJhbGciOiJub25lIn0.eyJzdWIiOiJmaXh0dXJlIn0.Zml4dHVyZQ';
$cases = [
    ['ascii-substring', ['token'], 'shouldMaskKey', 'ACCESS_TOKEN'],
    ['ordinary-key', ['token'], 'shouldMaskKey', 'page'],
    ['empty-keys', [], 'shouldMaskKey', 'token'],
    ['filter-zero-empty', ['', '0'], 'shouldMaskKey', '0'],
    ['ascii-not-unicode-fold', ['Ä'], 'shouldMaskKey', 'ä'],
    ['unicode-exact', ['Ä'], 'shouldMaskKey', 'Ä'],
    ['url-basic', $base, 'maskUrl', 'https://example.test/x?token=fake-a&api_key=fake-b&page=1#frag'],
    ['url-encoded-repeat', $base, 'maskUrl', 'https://example.test/x?%74oken=fake-a&TOKEN=fake-b&page=1'],
    ['url-invalid-percent', $base, 'maskUrl', '/x?token%ZZ=fake-a&pa%ZZge=1'],
    ['url-bare-empty', $base, 'maskUrl', '/x?token&&password=fake&blank=&=ordinary&'],
    ['url-plus-key', ['api key'], 'maskUrl', '/x?api+key=fake&other=ok'],
    ['url-no-query', $base, 'maskUrl', 'https://example.test/token/fake#token=fake'],
    ['url-fragment-gap', $base, 'maskUrl', '/x?page=1#token=fake-fragment'],
    ['url-userinfo-gap', $base, 'maskUrl', 'https://fake-user:fake-password@example.test/x'],
    ['url-empty-query', $base, 'maskUrl', '/x?#frag'],
    ['url-empty-keys', [], 'maskUrl', '/x?token=fake'],
    ['auth-basic', $base, 'maskSecrets', 'upstream Authorization: Basic Zml4dHVyZTpmYWtl'],
    ['auth-bearer', $base, 'maskSecrets', 'Authorization=Bearer fake-token; rest=ok'],
    ['auth-digest', $base, 'maskSecrets', 'Authorization: Digest fake-token,realm=test'],
    ['auth-vertical-space', $base, 'maskSecrets', "Authorization:\vBasic fake-token"],
    ['jwt', [], 'maskSecrets', 'token ' . $jwt],
    ['bearer-jwt-order', [], 'maskSecrets', 'Bearer ' . $jwt],
    ['bearer-empty-keys', [], 'maskSecrets', 'prefix bEaReR fake-token suffix'],
    ['auth-empty-keys', [], 'maskSecrets', 'authorization: Basic fake-token'],
    ['key-value', ['secret'], 'maskSecrets', 'my_secret=fake-value keep=ok'],
    ['key-quoted-colon', ['secret'], 'maskSecrets', 'my_secret: "fake-value", keep=ok'],
    ['key-regex-escaped', ['a+b'], 'maskSecrets', 'a+b=fake-value keep=ok'],
    ['already-masked', $base, 'maskSecrets', 'Authorization: *** password=***'],
    ['ordinary-text', $base, 'maskSecrets', 'ordinary 中文 text stays unchanged'],
    ['free-text-gap', $base, 'maskSecrets', 'unlabelled-fake-sensitive-value'],
    ['free-text-partial-gap', $base, 'maskSecrets', 'password: fake first second'],
];
if ($argc === 3) {
    $cases = [
        ['equals', $base, 'maskSensitiveSql', "select * from t where token = 'fake-token' and id = 7"],
        ['not-equal', $base, 'maskSensitiveSql', "select * from t where password != 'fake'"],
        ['not-equal-angle', $base, 'maskSensitiveSql', 'select * from t where password <> "fake"'],
        ['greater-equal', $base, 'maskSensitiveSql', 'select * from t where token >= 12.5'],
        ['less-equal', $base, 'maskSensitiveSql', 'select * from t where token <= 9'],
        ['greater', $base, 'maskSensitiveSql', 'select * from t where token > 8'],
        ['less', $base, 'maskSensitiveSql', 'select * from t where token < 7'],
        ['like', $base, 'maskSensitiveSql', "select * from t where API_TOKEN LIKE 'fake%'"],
        ['in-list', $base, 'maskSensitiveSql', "select * from t where secret IN ('fake-a', 'fake-b')"],
        ['quoted-qualified', $base, 'maskSensitiveSql', "select * from t where `t`.`api_token` = 'fake' and [password] = 'fake'"],
        ['escaped-single-backslash', $base, 'maskSensitiveSql', "update t set password = 'fake\\'quote\\\\tail', note = 'O\\'Brien'"],
        ['escaped-double', $base, 'maskSensitiveSql', 'update t set password = "fake\\"quote\\\\tail", note = "normal"'],
        ['insert-single', $base, 'maskSensitiveSql', "insert into users (name, password) values ('fake-user','fake-password')"],
        ['insert-multiple', $base, 'maskSensitiveSql', "INSERT INTO users (`api_token`, name, password) VALUES ('fake-a','one','fake-p'), ('fake-b', 'two', 'fake-q')"],
        ['insert-sensitive-last', $base, 'maskSensitiveSql', "insert into users (name, note, secret) values ('fake-user', 'comma, inside', 'fake')"],
        ['insert-bracket-qualified', $base, 'maskSensitiveSql', "insert into t ([profile].[password], name) values ('fake', 'keep')"],
        ['insert-quotes-commas', $base, 'maskSensitiveSql', "insert into t (name, password, note) values ('O\\'Brien, Jr', 'fake', 'ordinary')"],
        ['insert-empty-column', $base, 'maskSensitiveSql', "insert into t () values ('fake')"],
        ['insert-missing-value', $base, 'maskSensitiveSql', "insert into t (name, password) values ('ordinary')"],
        ['empty-keys', [], 'maskSensitiveSql', "select * from t where token = 'fake'"],
        ['no-sensitive-column', $base, 'maskSensitiveSql', "insert into t (name, note) values ('fake-user','ordinary')"],
        ['placeholder', $base, 'maskSensitiveSql', 'select * from t where password = ?'],
        ['negative-gap', $base, 'maskSensitiveSql', 'select * from t where password = -42'],
        ['bare-word-gap', $base, 'maskSensitiveSql', 'select * from t where password = fake'],
        ['null-gap', $base, 'maskSensitiveSql', 'select * from t where password = NULL'],
        ['nested-function-gap', $base, 'maskSensitiveSql', "insert into t (name, password) values ('ordinary', concat('fake','value'))"],
        ['in-nested-function-gap', $base, 'maskSensitiveSql', "select * from t where token IN (concat('fake','value'))"],
        ['no-auto-secrets', $base, 'maskSensitiveSql', "select * from t where note = 'Bearer fake-token'"],
        ['regex-key-escaped', ['a+b'], 'maskSensitiveSql', "select * from t where a+b = 'fake'"],
        ['ordinary-text', $base, 'maskSensitiveSql', 'ordinary 中文 text stays unchanged'],
    ];
}
$output = [];
foreach ($cases as [$name, $keys, $method, $input]) {
    $masker = new Mooeen\MonitorLaravel\Recorder\SensitiveMasker($keys);
    $output[] = ['name' => $name, 'keys' => $keys, 'method' => $method, 'input' => $input, 'output' => $masker->$method($input)];
}
$pipes = [];
$process = proc_open(['git', '-C', dirname($file, 3), 'rev-parse', 'HEAD'], [0 => ['pipe', 'r'], 1 => ['pipe', 'w'], 2 => ['pipe', 'w']], $pipes);
if (!is_resource($process)) {
    throw new RuntimeException('Cannot read source commit.');
}
fclose($pipes[0]);
$commit = trim(stream_get_contents($pipes[1]));
fclose($pipes[1]);
$error = stream_get_contents($pipes[2]);
fclose($pipes[2]);
if (proc_close($process) !== 0 || !preg_match('/^[a-f0-9]{40}$/', $commit)) {
    throw new RuntimeException('Cannot identify source commit: ' . $error);
}
echo json_encode([
    'evidence' => 'fixed fake inputs; exact SensitiveMasker class only; no Composer, Host, env, DB or Cloud',
    'source_commit' => $commit,
    'source_file_sha256' => hash_file('sha256', $file),
    'php_version' => PHP_VERSION,
    'cases' => $output,
], JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR), "\n";
