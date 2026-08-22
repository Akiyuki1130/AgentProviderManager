# Security Policy

## Supported versions

Security fixes target the latest published release. Please include the exact version and commit when reporting an issue.

## Reporting a vulnerability

Do not open a public issue for an undisclosed vulnerability. Contact the repository owner through the private security reporting mechanism configured on GitHub, or request a private contact channel from the owner.

Please include:

- affected version and Windows version;
- reproduction steps using placeholder credentials only;
- impact and any relevant logs with secrets removed;
- a suggested mitigation if available.

Never send real API keys, `.dsh` credentials, configuration files, backups, or DPAPI-protected keychain files in a report.

## Security boundaries

The application is a local Windows desktop tool. It sends API keys only when the user explicitly requests model discovery or saves a provider to the selected local configuration. The configured URL is restricted to HTTP/HTTPS and host validation rejects localhost, loopback, private, link-local, unspecified, and reserved addresses. HTTP can be explicitly enabled by the user and is not confidential.

The application does not provide a network proxy, sandbox, malware protection, or a guarantee that a third-party provider is trustworthy. Protect configuration files, backups, the DeepSeek credentials file, session storage, and the system clipboard.

Optional updates contact only the fixed HTTPS GitHub API/release hosts for this repository. Release metadata accepts stable versions only; the Windows amd64 executable is size-limited and SHA-256 checked before staging. Installation waits for the current process to exit and requires an explicit restart confirmation. The updater does not accept arbitrary URLs or shell commands from the frontend.
