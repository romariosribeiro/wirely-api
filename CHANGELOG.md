# Changelog

All notable changes to Wirely API are documented here. The project follows
[Semantic Versioning](https://semver.org/) and keeps unreleased work at the top.

## [Unreleased]

### Added

- Portuguese and English panel localization with a persistent PT/EN selector,
  keeping Portuguese as the default language.
- Updated project screenshots for the English dashboard, instance manager, and
  API documentation.
- Complete newsletter API for creation, metadata and invite lookup, listing,
  message history, and subscription.
- Label editing and reversible label associations for chats and messages.
- Community creation and batch linking or unlinking of participant groups.
- Bearer-authenticated message deletion, text editing, read receipts, and
  persisted delivery-status lookup.
- Chat archive/unarchive, mute, pin, and unpin operations backed by WhatsApp
  app-state synchronization.
- Interactive cURL, Laravel, Node.js, and Python documentation for message and
  chat actions.
- Short `POST /api/auth/login` endpoint for creating administrative sessions,
  with cookie-based examples in the API documentation.
- Complete instance lifecycle API for Bearer-authenticated details, connection,
  disconnection, logout, phone pairing, encrypted proxy configuration, QR Code,
  and status.
- Batch `POST /api/contacts/check` validation for up to 100 WhatsApp numbers.
- Persistent, restart-safe webhook delivery queue with event-ID idempotency.
- Bearer-authenticated webhook test, job status, and delivery-history endpoints.
- Five-attempt webhook retry schedule with `Retry-After` support and delivery
  attempt headers.
- Authenticated download of received image, video, audio, document, and sticker
  content as the original binary or Base64 JSON.
- Complete webhook metadata for received media, locations, contacts, and
  reactions, with attachment storage isolated by instance.
- Automatic cleanup of received media after the 30-day history retention period.
- Send presence simulation with nested `options.presence` and `options.delay`.
- Connection-time `subscribe` and `webhookUrl` configuration with an
  Evolution-compatible `eventString` response.
- Webhook events for history synchronization, calls, contacts, labels, and
  newsletters.

### Changed

- Administrative and panel endpoints now use the concise `/api/` prefix across
  the server, frontend, OpenAPI, and documentation.
- Instance cards now show only the creation date, without exposing the internal
  WhatsApp engine name.
- Secondary actions in the instance management modal now have visible button
  surfaces, borders, and hover states instead of looking like plain text.
- The update status card and modal now use green when the system is current and
  red when a newer version is available.
- The proxy manager now includes an expandable Windows SSH tunnel assistant
  that generates personalized PowerShell, VPS test, and SOCKS5 configuration commands.
- The dashboard now performs a fresh update check when it opens, refreshes the
  status automatically every five minutes, and rechecks when the update modal opens.
- The Windows proxy assistant now contains long commands inside its card and
  wraps them cleanly instead of shifting and clipping the management modal.

## [0.9.0] - 2026-09-17

### Added

- Location, contact, poll, and reaction sending.
- Group creation, lookup, participant administration, invite rotation, and join.
- WhatsApp profile, photo, about, and privacy management.
- Persistent login lockout, per-instance API rate limiting, and admin auditing.
- Automatic local backups, panel restore workflow, and pre-restore safety backup.
- Prometheus metrics, operational alerts, and structured JSON logs.
- Complete OpenAPI coverage for public and administrative routes.
- CI, release automation, security policy, contribution guide, and MIT license.

### Changed

- The public media contract uses only `POST /api/send/media`, with the media
  kind selected through `type`.
- Each instance continues to own a single webhook configuration.

[Unreleased]: https://github.com/romariosribeiro/wirely-api/compare/v0.9.0...HEAD
[0.9.0]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.9.0
