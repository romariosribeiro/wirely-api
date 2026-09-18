# Wirely PHP SDK

Requires PHP 8.1 and cURL. During development, add this folder as a Composer
path repository or copy it into a standalone package.

```php
use Wirely\WirelyClient;

$wirely = new WirelyClient('http://localhost:8080', 'wly_SEU_TOKEN');
$sent = $wirely->sendText('5511999999999', 'Olá pelo SDK', [
    'options' => ['presence' => 'composing', 'delay' => 1200],
    'mentions' => ['5511888888888'],
]);
```

`request()` remains public so every documented endpoint can be used immediately,
even before a convenience method is added.
