# Security policy

## Supported versions

Security fixes are provided for the latest tagged release and the current
default branch while Wirely API is in pre-1.0 development.

## Reporting a vulnerability

Please do not open a public issue for a suspected vulnerability. Use GitHub's
**Security → Report a vulnerability** flow in this repository so credentials,
reproduction steps, and affected installations remain private.

Include the affected version, deployment model, impact, and the smallest safe
reproduction you can provide. You should receive an acknowledgement within
seven days. A fix and disclosure timeline will be coordinated after validation.

Never include real WhatsApp session databases, API tokens, webhook secrets,
backup archives, or `token.key` in a report.

## Deployment responsibility

Wirely stores sensitive WhatsApp sessions and integration credentials. Restrict
filesystem access to the service user, keep backups private, use unique panel
accounts, and rotate any secret that may have been exposed.
