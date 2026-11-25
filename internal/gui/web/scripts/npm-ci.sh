#!/bin/sh
set -eu

web_dir=${1:-internal/gui/web}

if [ "${NPM_CONFIG_CACHE+x}" = x ]; then
	npm_cache=$NPM_CONFIG_CACHE
else
	npm_cache=${TMPDIR:-/tmp}/agentbox-npm-cache
fi

printf '%s\n' "web-install: npm cache: $npm_cache"

cache_is_writable() {
	cache_test=$npm_cache/.agentbox-cache-write-test.$$
	if ( : > "$cache_test" ) 2>/dev/null; then
		rm -f "$cache_test"
		return 0
	fi
	return 1
}

sanitized_diagnostic() {
	sed -n '1p' |
		sed -e 's/[Aa]uthorization:.*/Authorization: [redacted]/g' \
			-e 's/[Tt]oken:.*/token: [redacted]/g' \
			-e 's/[Bb]earer .*/Bearer [redacted]/g'
}

if ! mkdir_output=$(mkdir -p "$npm_cache" 2>&1); then
	printf '%s\n' "web-install: cache is not writable: $npm_cache: $mkdir_output" >&2
	printf '%s\n' "web-install: remediation: select a writable NPM_CONFIG_CACHE" >&2
	exit 1
fi

if ! cache_is_writable; then
	printf '%s\n' "web-install: cache is not writable: $npm_cache" >&2
	printf '%s\n' "web-install: remediation: select a writable NPM_CONFIG_CACHE" >&2
	exit 1
fi

attempt=1
while [ "$attempt" -le 3 ]; do
	if ! cache_is_writable; then
		printf '%s\n' "web-install: cache is not writable: $npm_cache" >&2
		printf '%s\n' "web-install: remediation: select a writable NPM_CONFIG_CACHE" >&2
		exit 1
	fi

	if npm_output=$(NPM_CONFIG_CACHE="$npm_cache" npm --prefix "$web_dir" ci 2>&1); then
		exit 0
	fi

	diagnostic=$(printf '%s\n' "$npm_output" | sanitized_diagnostic)
	printf '%s\n' "web-install: npm ci failed; retry=$attempt/3; cache: $npm_cache" >&2
	printf '%s\n' "web-install: npm diagnostic: $diagnostic" >&2

	if [ "$attempt" -ge 3 ]; then
		printf '%s\n' "web-install: npm ci failed after 3 attempts" >&2
		exit 1
	fi

	attempt=$((attempt + 1))
	sleep 2
done
