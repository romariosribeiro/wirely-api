# Contributing to Wirely API

Thank you for helping improve Wirely. Before opening a large pull request,
describe the proposed API and compatibility impact in an issue or discussion.

## Development workflow

1. Install Go 1.26+, Node.js 22+, and npm.
2. Run `make web-install` once.
3. Create a focused branch and keep unrelated changes out of the commit.
4. Run `make test` before opening the pull request.
5. Update OpenAPI, tests, README, and `CHANGELOG.md` when the public contract changes.

Public instance endpoints must authenticate through the Bearer token and infer
the instance from it. Administrative endpoints must enforce a server-side role.
Do not add legacy aliases for renamed endpoints without an explicit deprecation
plan. New media kinds belong on the unified `/api/send/media` endpoint when the
payload model permits it. Each instance has one webhook configuration.

Never commit local databases, WhatsApp sessions, generated credentials,
`token.key`, `.env` files, or backup archives.
