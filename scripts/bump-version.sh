#!/bin/sh
# Sets the release version. Pushing the change to main publishes v<version>.
#
#   scripts/bump-version.sh 2.1.0
#   scripts/bump-version.sh patch|minor|major
set -eu

cd "$(dirname "$0")/.."
current="$(tr -d '[:space:]' < VERSION)"
IFS=. read -r major minor patch <<EOV
$current
EOV

case "${1:-}" in
	patch) next="$major.$minor.$((patch + 1))" ;;
	minor) next="$major.$((minor + 1)).0" ;;
	major) next="$((major + 1)).0.0" ;;
	[0-9]*.[0-9]*.[0-9]*) next="$1" ;;
	*) echo "usage: $0 <x.y.z|patch|minor|major>  (current: $current)" >&2; exit 1 ;;
esac

if git rev-parse -q --verify "refs/tags/v$next" >/dev/null; then
	echo "v$next is already tagged" >&2
	exit 1
fi

printf '%s\n' "$next" > VERSION
echo "VERSION: $current -> $next"
echo "Next: add a '## $next' section to CHANGELOG.md, then"
echo "  git commit -am \"chore: release v$next\" && git push"
