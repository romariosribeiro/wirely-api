# Changelog

All notable changes to Wirely API are documented here. The project follows
[Semantic Versioning](https://semver.org/) and keeps unreleased work at the top.

## [Unreleased]

## [0.17.6] - 2026-10-08

- Update whatsmeow from `9399289b022b` to `c386243a72ba` with WhatsApp protocol definitions v1049558099.
- Include upstream error events when fetching app-state synchronization data fails.

## [0.17.5] - 2026-10-06

- Fix saving SMTP and Telegram integrations in the panel by excluding read-only credential flags from update requests.
- Preserve saved credentials when password and token fields are left blank.

## [0.17.4] - 2026-10-06

- Update whatsmeow from `8b41cfe6d9c4` to `9399289b022b` with updated WhatsApp protocols, split socket frame fixes, and improved retry receipt handling.
- Adapt blocklist queries and updates to the new upstream API, preserving phone numbers for LID entries and excluding inactive contacts.

## [0.17.3] - 2026-10-01

- Update whatsmeow from `2e338d0ee73d` to `8b41cfe6d9c4` with the latest WhatsApp protocol definitions.
- Include upstream handshake validation, rich-response parsing fixes, prekey signaling, and WASA root-secret persistence for message sending.

## [0.17.2] - 2026-10-01

- Show whether each connected dashboard instance is currently using its configured proxy or the direct VPS route.
- Include `connectionRoute` and proxy fallback state in the administrative instance list without additional panel requests.

## [0.17.1] - 2026-10-01

- Make management copy buttons use a synchronous clipboard event fallback on plain HTTP.
- Only show copy success after the browser actually dispatches and accepts the clipboard event.

## [0.17.0] - 2026-10-01

- Add per-instance automatic proxy recovery with a configurable check interval of at least 60 seconds.
- Allow a finite recovery attempt limit or unlimited attempts with zero.
- Verify real connectivity through HTTP, HTTPS, or SOCKS5 before moving a live instance back from the VPS fallback route.
- Add responsive recovery controls to the bilingual management panel and document them in OpenAPI.

## [0.16.2] - 2026-10-01

- Create restore staging and rollback directories inside the systemd-approved data area.
- Prevent a prepared restore from trapping the service in a restart loop under `ProtectHome=read-only`.
- Prevent the alert integrations screen from crashing before its first saved configuration.

## [0.16.1] - 2026-10-01

- Exclude received-media caches and staged update binaries from safety backups, keeping update backups small and fast.
- Remove interrupted backup staging directories safely at startup and honor cancellation while copying large files.
- Let the update modal survive the expected connection interruption, poll the health endpoint, and reload after the target version starts.
- Flush the accepted update response and allow administrative middleware to finish before systemd activation.

## [0.16.0] - 2026-10-01

- Add encrypted Telegram and SMTP alert integrations to the management panel.
- Allow administrators to select monitored instances independently for each channel and send delivery tests.
- Notify on disconnected, error, logout, and subsequent recovery states without producing false recovery alerts during startup.
- Keep bot tokens and SMTP passwords encrypted with the installation key and out of API responses and logs.
- Fix proxy-guide copy buttons on plain HTTP by providing a clipboard fallback and inline confirmation.

## [0.15.3] - 2026-09-22

- Reconcile the persisted instance status with the live WhatsApp socket before
  returning the dashboard instance list.
- Stop background reconnects for deleted devices and recreate the WhatsApp
  session on the next connect without incorrectly bypassing the configured proxy.
- Stop reconnect attempts cleanly for canceled and already-connected clients.

## [0.15.2] - 2026-09-20

- Recreate the WhatsApp client after external logout instead of reusing a deleted
  device, allowing a new QR code without deleting the Wirely instance.
- Preserve the instance token, configuration and chat history during re-pairing.
- Serialize session recreation and ignore late state updates from retired clients.
- Keep deleted-device errors separate from proxy failures.
- Cover deleted devices, delayed cleanup, concurrent reconnects and QR generation
  with regression tests.

## [0.15.0] - 2026-09-20

- Automatically retry through the VPS direct connection when an instance proxy
  connection attempt fails, preserving the saved proxy configuration.
- Expose the selected connection route and proxy fallback in the status API and
  instance management panel.
- Track keepalive failures and replaced sessions, and log connection transitions
  without exposing proxy credentials.
- Preserve manual disconnect behavior and cancel background reconnects when
  sessions are closed or deleted.
- Add regression tests for proxy failures, direct retries, canceled connections,
  saved configuration, and live socket status.

## [0.14.2] - 2026-09-18

- Documented required and optional body, path, query, and multipart fields
  throughout the panel using the OpenAPI contract as the source of truth.
- Added complete advanced examples to the existing text, media, location, and
  contact endpoints instead of presenting them as replacement routes.
- Fixed the Advanced API copy button fallback inside the documentation dialog.
- Declared the required group JID path parameter on every advanced group route.

## [0.14.1] - 2026-09-18

- Made every Advanced API documentation endpoint selectable, with a description,
  instance-aware cURL example, and copy action.

## [0.14.0] - 2026-09-18

- Authenticated SSE stream with filtering, heartbeat, and reconnect hint.
- Dependency-free PHP 8.1 SDK plus Postman and Bruno collections.
- Consolidated PT/EN documentation and OpenAPI coverage.

## [0.13.0] - 2026-09-18

- Text, image, and video Status/Stories publishing.
- Advanced group descriptions, photos, permissions, join approval, request moderation, and leaving.

## [0.12.0] - 2026-09-18

- Replies, mentions, forwarding, link previews, and view-once image/video.
- Disappearing-message timers, presence APIs/events, and experimental live location.

## [0.11.0] - 2026-09-18

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

[Unreleased]: https://github.com/romariosribeiro/wirely-api/compare/v0.17.6...HEAD
[0.17.6]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.17.6
[0.17.5]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.17.5
[0.17.4]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.17.4
[0.17.3]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.17.3
[0.17.2]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.17.2
[0.17.1]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.17.1
[0.17.0]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.17.0
[0.16.2]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.16.2
[0.16.1]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.16.1
[0.16.0]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.16.0
[0.15.3]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.15.3
[0.15.2]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.15.2
[0.15.1]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.15.1
[0.15.0]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.15.0
[0.14.2]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.14.2
[0.14.1]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.14.1
[0.14.0]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.14.0
[0.13.0]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.13.0
[0.12.0]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.12.0
[0.11.0]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.11.0
[0.9.0]: https://github.com/romariosribeiro/wirely-api/releases/tag/v0.9.0
