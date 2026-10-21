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

Delete a chat on the connected WhatsApp account with
`$wirely->deleteChat('5511999999999')`. Pass `true` as the second argument to
request media deletion on WhatsApp. This does not revoke messages for other
participants, leave groups, or erase Wirely's local history/cache.

`request()` remains public so every documented endpoint can be used immediately,
even before a convenience method is added.
