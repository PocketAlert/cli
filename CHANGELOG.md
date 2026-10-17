# Changelog

## 2.0.0

First release from the standalone repository, built and published by GoReleaser.

### Added
- `send`: `--level`, `--at` / `--in` scheduling (days supported: `1d12h`), up to 3 `--action` buttons, `--image` (file or URL), positional `title message`, message from stdin, `-q` to print only the TID.
- `exec -- <command>`: run a command and notify with exit status, duration and output tail; the exit code is passed through.
- `watch`: turn piped lines (e.g. `tail -f | grep`) into batched notifications.
- `scheduled list|cancel`, `usage`, `messages read|stats`, `messages delete --all`, multiple TIDs per delete.
- `apps update`, `devices rename`, `webhooks update|history`, `forms list|delete`.
- Default levels and action templates for applications and webhooks.
- `--json` on every command for scripting.
- `POCKETALERT_API_KEY`, `POCKETALERT_API_URL`, `POCKETALERT_CONFIG` environment variables.
- `auth` without an argument reads the key from a hidden prompt or stdin; `auth` verifies the key before saving it.
- `version` with commit and build date; shell completion.

### Fixed
- `webhooks create` ignored `--application` / `--device` (sent outdated field names).
- Webhook URLs pointed to the API host instead of `https://p4a.me/wh/…`.
- `config` crashed on API keys shorter than 12 characters.
- Errors were printed to stdout; they now go to stderr.
- Titles were truncated mid-character for non-Latin text.
- Daily limit (429) errors now show usage, reset time and the upgrade link.

### Changed
- Release archives are named `pocketalert_<os>_<arch>.tar.gz` (`.zip` on Windows) and include checksums.
- `messages list` uses the paginated API (`--limit`, `--offset`).
