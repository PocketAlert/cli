# Pocket Alert CLI

Send push notifications to your phone from the terminal, scripts and CI — and manage your [Pocket Alert](https://pocketalert.app) applications, devices, webhooks and API keys.

```bash
pocketalert send "Deploy finished" "v1.4.2 is live"
pocketalert exec -- make release        # get a push when a long job ends
```

## Install

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/PocketAlert/cli/main/install.sh | sh
```

**Homebrew**

```bash
brew install pocketalert/tap/pocketalert
```

**Go**

```bash
go install github.com/PocketAlert/cli/cmd/pocketalert@latest
```

**Manual download** — always the latest release:

| Platform | Archive |
|---|---|
| macOS Apple Silicon | [pocketalert_darwin_arm64.tar.gz](https://github.com/PocketAlert/cli/releases/latest/download/pocketalert_darwin_arm64.tar.gz) |
| macOS Intel | [pocketalert_darwin_amd64.tar.gz](https://github.com/PocketAlert/cli/releases/latest/download/pocketalert_darwin_amd64.tar.gz) |
| Linux x86-64 | [pocketalert_linux_amd64.tar.gz](https://github.com/PocketAlert/cli/releases/latest/download/pocketalert_linux_amd64.tar.gz) |
| Linux ARM64 | [pocketalert_linux_arm64.tar.gz](https://github.com/PocketAlert/cli/releases/latest/download/pocketalert_linux_arm64.tar.gz) |
| Windows x86-64 | [pocketalert_windows_amd64.zip](https://github.com/PocketAlert/cli/releases/latest/download/pocketalert_windows_amd64.zip) |

Checksums: [checksums.txt](https://github.com/PocketAlert/cli/releases/latest/download/checksums.txt).
On macOS, a binary downloaded with a browser may be blocked by Gatekeeper: `xattr -d com.apple.quarantine pocketalert`.

## Authenticate

Create an API key at [pocketalert.app/account/apikeys](https://pocketalert.app/account/apikeys), then:

```bash
pocketalert auth            # paste the key at the hidden prompt
```

The key is saved to `~/.pocketalert/config.json` (mode 0600). In CI, set `POCKETALERT_API_KEY` instead — it takes precedence over the saved key.

| Variable | Purpose |
|---|---|
| `POCKETALERT_API_KEY` | API key |
| `POCKETALERT_API_URL` | API base URL (default `https://api.pocketalert.app`) |
| `POCKETALERT_CONFIG` | Config file path |

## Send notifications

```bash
pocketalert send "Title" "Message"
pocketalert send -t "Title" -m "Message" -a <app-tid> -d <device-tid>
df -h | pocketalert send "Disk report"              # message from stdin
pocketalert send "Backup" "Done" -q                  # print only the TID
```

**Levels** — `--level silent|low|default|high|critical` (or `-2…2`). `critical` bypasses Do Not Disturb (paid plans; free plans receive `high`).

**Scheduling** (paid plans) — up to 30 days ahead:

```bash
pocketalert send "Standup" "In 10 minutes" --at "2026-10-20 09:50"   # account timezone
pocketalert send "Reminder" "Rotate keys" --in 1d12h
pocketalert scheduled list
pocketalert scheduled cancel <tid>
```

**Action buttons** (paid plans) — up to 3, as `type:label:value`:

```bash
pocketalert send "Deploy failed" "main@4f2c" -l critical \
  --action "view:Open logs:https://ci.example.com/run/1" \
  --action "http:Retry:https://ci.example.com/run/1/retry" \
  --action "copy:Copy SHA:4f2c9e1"
```

`http` sends a POST by default; use `http:Ack:GET https://…` for another method or a full JSON value `http:Ack:{"url":"…","method":"PUT","headers":{…},"body":"…"}`.

**Images** (paid plans) — `--image ./screenshot.png` or `--image https://…/chart.png`.

## Wrap a command

`exec` runs a command, streams its output, then notifies with the exit status, duration and the last lines of output. The exit code is passed through.

```bash
pocketalert exec -- ./backup.sh
pocketalert exec --on failure -l critical -- terraform apply -auto-approve
pocketalert exec -t "Nightly import" --tail 20 -- python import.py
```

Failures default to level `high`.

## Watch a stream

`watch` sends piped lines as notifications. Lines arriving within `--batch` (default 5s) are grouped into one push, so a burst of errors costs one message from your daily quota.

```bash
tail -f app.log | grep --line-buffered ERROR | pocketalert watch -t "app errors"
journalctl -fu nginx | pocketalert watch --batch 1m -l high
```

## Manage resources

```bash
pocketalert messages list [-n 20] [--offset 0] [-d <device>]
pocketalert messages get <tid>
pocketalert messages read <tid…> | --all
pocketalert messages delete <tid…> | --all --yes
pocketalert messages stats
pocketalert usage                                   # today's quota

pocketalert apps list | get | create | update | delete
pocketalert apps create -n "Production" -c "#EF4444" -l high

pocketalert devices list | get | rename <tid> <name> | delete

pocketalert webhooks list | get | create | update | history | delete
pocketalert webhooks create -n Grafana -m "%title%: %message%" -l high
pocketalert webhooks history <tid> --status error

pocketalert forms list | delete
pocketalert apikeys list | create -n "CI" | delete

pocketalert profile
pocketalert config [set|reset] [api_key|base_url]
```

Every command accepts `--json` and prints the raw API response, ready for `jq`:

```bash
pocketalert messages list --json | jq -r '.data[].title'
```

## Shell completion

```bash
pocketalert completion zsh  > "${fpath[1]}/_pocketalert"
pocketalert completion bash > /etc/bash_completion.d/pocketalert
pocketalert completion fish > ~/.config/fish/completions/pocketalert.fish
```

## Development

```bash
make test        # go test -race ./...
make build       # ./pocketalert
make snapshot    # local GoReleaser build into dist/
```

One workflow, `.github/workflows/release.yml`, runs the tests on Linux, macOS and Windows and then builds with GoReleaser:

- **Push to main** — releases `v<VERSION>` when that tag does not exist yet. Bump it first:
  ```bash
  scripts/bump-version.sh minor      # or patch, major, 2.1.0
  git commit -am "chore: release v2.1.0" && git push
  ```
  Pushes that keep the version only run the check.
- **Push a tag** `v2.1.0` — releases that tag; it must match `VERSION`.
- **Pull request or manual run** — tests and builds a snapshot, attached to the run as an artifact, without publishing.

## License

MIT
