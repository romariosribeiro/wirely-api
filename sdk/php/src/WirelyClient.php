<?php

declare(strict_types=1);

namespace Wirely;

use RuntimeException;

final class WirelyClient
{
    public function __construct(
        private readonly string $baseUrl,
        private readonly string $token,
        private readonly int $timeout = 30,
    ) {}

    /** @param array<string, mixed> $options */
    public function sendText(string $recipient, string $message, array $options = []): array
    {
        return $this->request('POST', '/api/send/text', compact('recipient', 'message') + $options);
    }

    /** @param array<string, scalar|array|null> $fields */
    public function sendMedia(string $recipient, string $type, string $file, array $fields = []): array
    {
        if (!is_file($file)) throw new RuntimeException("Media file not found: {$file}");
        $body = ['recipient' => $recipient, 'type' => $type, 'file' => new \CURLFile($file)] + $fields;
        return $this->request('POST', '/api/send/media', $body, true);
    }

    /** @param array<string, mixed> $payload */
    public function sendLocation(array $payload): array { return $this->request('POST', '/api/send/location', $payload); }
    /** @param array<string, mixed> $payload */
    public function sendLiveLocation(array $payload): array { return $this->request('POST', '/api/send/location/live', $payload); }
    /** @param array<string, mixed> $payload */
    public function forward(array $payload): array { return $this->request('POST', '/api/messages/forward', $payload); }
    public function setDisappearing(string $chat, string $duration): array { return $this->request('PUT', '/api/chats/disappearing', compact('chat', 'duration')); }
    public function setPresence(string $presence): array { return $this->request('POST', '/api/presence', compact('presence')); }
    public function subscribePresence(string $phone): array { return $this->request('POST', '/api/presence/subscribe', compact('phone')); }
    public function getPresence(string $phone): array { return $this->request('GET', '/api/presence/' . rawurlencode($phone)); }
    /** @param array<string, mixed> $payload */
    public function sendStatusText(array $payload): array { return $this->request('POST', '/api/status/text', $payload); }
    /** @param array<string, mixed>|null $body */
    public function request(string $method, string $path, ?array $body = null, bool $multipart = false): array
    {
        $handle = curl_init(rtrim($this->baseUrl, '/') . '/' . ltrim($path, '/'));
        $headers = ['Accept: application/json', 'Authorization: Bearer ' . $this->token];
        if ($body !== null && !$multipart) { $headers[] = 'Content-Type: application/json'; $body = json_encode($body, JSON_THROW_ON_ERROR); }
        curl_setopt_array($handle, [CURLOPT_CUSTOMREQUEST => strtoupper($method), CURLOPT_HTTPHEADER => $headers,
            CURLOPT_POSTFIELDS => $body, CURLOPT_RETURNTRANSFER => true, CURLOPT_TIMEOUT => $this->timeout]);
        $raw = curl_exec($handle);
        if ($raw === false) { $message = curl_error($handle); curl_close($handle); throw new RuntimeException($message); }
        $status = curl_getinfo($handle, CURLINFO_RESPONSE_CODE); curl_close($handle);
        $data = $raw === '' ? [] : json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        if ($status < 200 || $status >= 300) throw new RuntimeException((string)($data['error'] ?? "Wirely returned HTTP {$status}"), $status);
        return $data;
    }
}
