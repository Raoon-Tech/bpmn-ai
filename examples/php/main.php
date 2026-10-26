<?php
// Minimal example using Guzzle. Install: composer require guzzlehttp/guzzle
require __DIR__ . '/vendor/autoload.php';

use GuzzleHttp\Client;

$base = 'http://localhost:8080';
$client = new Client(['base_uri' => $base]);

// 1. CSRF token
$csrfRes = $client->get('/api/v1/csrf-token');
$csrf = json_decode($csrfRes->getBody(), true)['csrf_token'];

// 2. Deploy a process
$xml = file_get_contents(__DIR__ . '/../testdata/simple_sequence.bpmn');
$procRes = $client->post('/api/v1/processes', [
    'headers' => ['X-CSRF-Token' => $csrf, 'Content-Type' => 'application/json'],
    'json' => ['name' => 'PHP demo', 'bpmn_xml' => $xml],
]);
$proc = json_decode($procRes->getBody(), true);
echo "process: " . $proc['id'] . "\n";

// 3. Start a case
$caseRes = $client->post("/api/v1/processes/{$proc['id']}/start", [
    'headers' => ['X-CSRF-Token' => $csrf, 'Content-Type' => 'application/json'],
    'json' => ['variables' => ['approved' => true]],
]);
$case = json_decode($caseRes->getBody(), true);
echo "case: " . $case['id'] . " " . $case['status'] . "\n";
PHP
