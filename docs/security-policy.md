# Security policy: filesystem and network restrictions

FrankenScriptling registers a curated subset of [Scriptling](https://github.com/paularlott/scriptling)'s
libraries. Libraries that only do in-process computation (JSON, regex, math,
templating, etc.) are always available. Libraries that can touch the
filesystem or the network are **closed by default** — they aren't registered
at all until an operator's policy explicitly turns them on. Nothing PHP script
code does can change this policy; it's resolved once, from configuration only
an operator controls (process environment variables and/or files on disk).

## Library categories

| Category | Meaning | Libraries |
| --- | --- | --- |
| Always on | No fs/net capability, no restriction needed | stdlib (`json`, `re`, `time`, `datetime`, `math`, `base64`, `hashlib`, `hmac`, `random`, `urllib`, `urllib.parse`, `string`, `uuid`, `html`, `statistics`, `functools`, `textwrap`, `platform`, `itertools`, `collections`, `contextlib`, `difflib`, `io`, `msgpack`), plus `toml`, `yaml`, `html.parser`, `logging`, `sys`, `secrets`, `shlex`, `scriptling.csv`, `scriptling.xml`, `scriptling.markdown`, `scriptling.similarity`, `scriptling.template.html`, `scriptling.template.text`, `scriptling.ai.agent`, `scriptling.ai.agent.interact`, `scriptling.ai.memory` |
| Filesystem-gated | Registered only when enabled *and* `allowed_paths` grants access | `pathlib`, `os`/`os.path`, `fs`, `glob`, `shutil`, `tempfile`, `tarfile`, `zipfile`, `scriptling.find`, `scriptling.grep`, `scriptling.sed` |
| Network-gated | Registered only when enabled *and* a network policy is configured | `requests`, `scriptling.net.websocket`, `scriptling.net.resolve`, `scriptling.ai`, `scriptling.mcp` |
| Disable-only | On/off, no restriction possible (upstream doesn't support it) | `subprocess` |
| Secret-provider-gated | Registered only when enabled *and* at least one secret provider is configured | `scriptling.secret` |
| Plugin-gated | Registered only when enabled *and* a plugin directory and/or HTTP-loading is configured; scripts can never `load`/`unload` a plugin themselves, only use whichever ones the operator pre-loaded (see [Plugins](#plugins-scriptlingplugin) below) | `scriptling.plugin` |
| Not shipped | Excluded from this image entirely — no config can turn them on | `scriptling.wait_for`, `scriptling.messaging.*`, `scriptling.container` (Docker/Podman/Apple socket access), `scriptling.valkey`, `scriptling.badgerdb`, `scriptling.sql`, `scriptling.sqlite`, `scriptling.net.gossip`, `scriptling.net.multicast`, `scriptling.net.unicast`, `scriptling.provision.file`, `scriptling.provision.fetch`, `scriptling.runtime.*`, `scriptling.nomad` |

Notes:
- `scriptling.ai.agent`, `scriptling.ai.agent.interact`, and `scriptling.ai.memory`
  don't construct their own HTTP clients — they call through `scriptling.ai`'s
  guarded client, so `scriptling.ai`'s network policy governs them too even
  though they're always registered.
- `scriptling.mcp` also registers its `RegisterToolHelpers`/`RegisterToon`
  companions when enabled.
- The excluded libraries are either a materially bigger blast radius than
  fs/net access (container runtime sockets) or need infrastructure this
  image doesn't wire up (the compiled-in storage-backend plugins —
  `valkey`/`badgerdb`/`sql`/`sqlite` — need a plugin `Manager` populated with
  those specific packages, which is a separate task from `scriptling.plugin`
  itself). They may be revisited as their own task.

## Configuring the policy

Three environment variables, each optional:

| Variable | Meaning |
| --- | --- |
| `SCRIPTLING_ENABLED_LIBRARIES` | Comma-separated library names (from the tables above) to register from the filesystem-gated, network-gated, and disable-only categories. Unset = none of them. |
| `SCRIPTLING_ALLOWED_PATHS` | Colon-separated list of absolute directories filesystem-gated libraries may access. Unset or `-` = deny all (still closed even if the library is "enabled"). |
| `SCRIPTLING_NETWORK_POLICY_FILE` | One or more paths (comma-separated) to a TOML network policy file, in the schema below. Unset = network-gated libraries stay unregistered regardless of `SCRIPTLING_ENABLED_LIBRARIES`. Multiple files are merged in order — later files' non-empty fields win. |

Alternatively, `SCRIPTLING_POLICY_FILE` points at one or more TOML files that
combine all three concerns (useful for a single file per vhost). Its schema:

```toml
[libraries]
enabled = ["pathlib", "os", "requests"]

[filesystem]
allowed_paths = ["/app/data"]

[network]
# same keys as the standalone network policy file, see below
allow_hosts = ["api.example.com"]
```

When both `SCRIPTLING_POLICY_FILE` and the plain env vars are set, the env
vars act as the base and the combined file's sections override them where
the file sets something.

### Network policy file schema

Identical to [Scriptling's own `netsecurity` policy format](https://github.com/paularlott/scriptling/blob/main/extlibs/netsecurity/netsecurity.go):

```toml
https_only = true
allow_ip_literals = false
allow_loopback = false
allow_private_ips = false
allow_hosts = ["api.example.com", ".internal.corp"]
deny_hosts = []
allow_cidrs = []
deny_cidrs = []
dns_servers = ["1.1.1.1", "8.8.8.8:53"]
client_timeout = "30s"
```

Every key is optional. Loopback, link-local (including cloud metadata
endpoints), private, unspecified, and multicast addresses are denied unless
explicitly allowed — and IP-literal URLs are rejected outright unless
`allow_ip_literals` is set or the literal falls inside `allow_cidrs`. There is
**no `allow_all` escape hatch** in this file format, deliberately — Scriptling
itself won't let a policy file fully disable its checks. A vhost that
genuinely needs broad access should grant it explicitly via `allow_hosts` /
`allow_cidrs`, not a single bypass flag.

### Secret provider (`scriptling.secret`)

`scriptling.secret` resolves secrets through a provider (currently Vault or
1Password, both upstream in Scriptling) rather than exposing raw filesystem
or network access. It's configured entirely from environment variables so
credentials like a Vault token never sit in a policy file on disk:

| Variable | Meaning |
| --- | --- |
| `SCRIPTLING_SECRET_PROVIDER` | `vault` or `onepassword`. Unset = no env-configured provider. |
| `SCRIPTLING_SECRET_ALIAS` | Alias scripts pass to `secret.get(alias, path, field)`. Defaults to the provider's own ID. |
| `SCRIPTLING_SECRET_ADDRESS` | Provider address (Vault: its URL). |
| `SCRIPTLING_SECRET_TOKEN` | Vault token, or the credential the provider needs. |
| `SCRIPTLING_SECRET_APP_ROLE_ID` / `SCRIPTLING_SECRET_APP_ROLE_SECRET` | Vault AppRole auth, as an alternative to a token. |
| `SCRIPTLING_SECRET_NAMESPACE` | Vault namespace (Enterprise). |
| `SCRIPTLING_SECRET_KV_VERSION` | Vault KV engine version (`1` or `2`). |
| `SCRIPTLING_SECRET_CACHE_TTL` | How long a resolved value is cached, e.g. `"5m"`. Defaults to 5 minutes. |
| `SCRIPTLING_SECRET_DEFAULT_FIELD` | Field name used when a script omits one. |
| `SCRIPTLING_SECRET_INSECURE_SKIP_TLS` | `"true"` to skip TLS verification (testing only). |

For more than one provider/alias, point `SCRIPTLING_SECRET_PROVIDERS_FILE` at
one or more TOML files (comma-separated, concatenated) with a `[[providers]]`
array, one table per provider — same fields as above, snake_case, e.g.:

```toml
[[providers]]
provider = "vault"
alias = "prod"
address = "https://vault.internal:8200"
token = "s.xxxxxx"

[[providers]]
provider = "onepassword"
alias = "team"
token = "ops_xxx"
```

The two mechanisms combine: a `SCRIPTLING_SECRET_PROVIDERS_FILE` for shared,
non-sensitive settings plus `SCRIPTLING_SECRET_PROVIDER`/`_TOKEN` for the
credential is a reasonable split. Like the other categories, `scriptling.secret`
still needs `scriptling.secret` in `SCRIPTLING_ENABLED_LIBRARIES` — configuring
a provider alone doesn't register it.

### Plugins (`scriptling.plugin`)

`scriptling.plugin` gives scripts access to plugin processes the operator
pre-loaded — `list`, `describe`, `call_function`, `batch_call`, and
`call_method` — but **scripts can never `load()` or `unload()` a plugin
themselves**, regardless of configuration. Upstream, `load()` takes a
script-supplied path or URL and executes/fetches it directly with no
`allowed_paths` or network-policy check at all — registering the plugin
control library without this restriction would hand scripts unrestricted
subprocess execution and unrestricted network fetch combined. This image
always registers it against a [`plugin.TransportNone`](https://github.com/paularlott/scriptling/blob/main/plugin/client.go)
scope (or, only when HTTP-loading is explicitly enabled below, a
network-policy-gated `TransportHTTP` scope) — never an unrestricted one.

| Variable | Meaning |
| --- | --- |
| `SCRIPTLING_PLUGIN` | One or more specific plugin executable paths or `http(s)://` URLs (comma-separated) the operator trusts — same env var name and either-a-path-or-a-URL convention as the `scriptling` CLI's own `--plugin` flag. Loaded first, at startup. A bad entry here fails the whole policy closed. |
| `SCRIPTLING_PLUGIN_DIR` | One or more directories (comma-separated) of executable plugin binaries the operator trusts. Scanned once at startup, after `SCRIPTLING_PLUGIN`. Upstream, a bad directory or a plugin that fails to start is only a warning — directory discovery is meant to tolerate a stray non-plugin file — so we promote any such warning to a hard failure ourselves here, closing the whole policy the same way a malformed network-policy file does, rather than silently registering fewer plugins than intended. |
| `SCRIPTLING_PLUGIN_HTTP_ENABLED` | `"true"`, unset otherwise. Off by default. When true **and** a network policy is configured, scripts may `load()` *new* HTTP(S) plugins — but only through that same network policy (the exact `allow_hosts`/`deny_hosts`/loopback/private-IP rules that govern `requests`). Stdio/executable loading is never re-enabled by this flag, under any configuration, and it has no effect on `SCRIPTLING_PLUGIN`/`SCRIPTLING_PLUGIN_DIR` themselves — those are always admin-controlled, never script-controlled. |

Unlike the CLI's `--plugin`, there's no env-var equivalent of `--plugin-arg`/
`--plugin-env`/`--plugin-header`/`--plugin-insecure` — `SCRIPTLING_PLUGIN`
entries are plain paths/URLs with no per-entry customization.

`SCRIPTLING_PLUGIN` and `SCRIPTLING_PLUGIN_DIR` combine freely — load a
directory of executables *and* one specific trusted URL. Whatever gets
preloaded this way stays fully usable via `scriptling.plugin` regardless of
`SCRIPTLING_PLUGIN_HTTP_ENABLED`: that flag only governs whether *scripts*
can additionally load something new, never what the operator already chose.

Like every other category, `scriptling.plugin` still needs to be in
`SCRIPTLING_ENABLED_LIBRARIES` — setting `SCRIPTLING_PLUGIN`/`_DIR` alone
doesn't register it, matching every other "enabled and configured" library.

```bash
SCRIPTLING_ENABLED_LIBRARIES=scriptling.plugin
SCRIPTLING_PLUGIN=https://billing.internal/rpc
SCRIPTLING_PLUGIN_DIR=/etc/frankenscriptling/plugins
```

```python
import scriptling.plugin as plugin

plugin.list()                                  # shows what the operator pre-loaded, from both sources above
plugin.call_function("widgets", "build", ["chair"])
plugin.load("evil", "/bin/sh")                 # error: plugin loading is disabled in this scope
```

## Global default vs. per-vhost override

These env vars can be set globally (the whole FrankenPHP process — e.g. in
the Dockerfile or the Caddyfile's global options) or per site block, via
FrankenPHP's own [`env` directive](https://frankenphp.dev/docs/config/)
(`php_ini` is *not* per-vhost in FrankenPHP today, which is why this uses
`env` instead):

```caddyfile
{
    # process-wide default: locked down
}

api.example.com {
    php_server {
        root /app/public
        env SCRIPTLING_ENABLED_LIBRARIES pathlib,os,requests
        env SCRIPTLING_ALLOWED_PATHS /app/data
        env SCRIPTLING_NETWORK_POLICY_FILE /etc/scriptling/api-network-policy.toml
    }
}

internal.example.com {
    php_server {
        root /app/public
        # inherits the process-wide default: no fs/net libraries at all
    }
}
```

> Per-vhost override via `env` requires the extension to read that
> per-request value directly (never through a PHP-callable method — PHP
> application code must never be able to loosen its own policy). At the time
> of writing this is still being verified against FrankenPHP's extension API;
> until confirmed, treat the env vars as **global-only** and check this file's
> git history / the project's issue tracker for the current status.

## Examples

Enable `pathlib`/`os` restricted to one directory, network access restricted
to one API host:

```bash
SCRIPTLING_ENABLED_LIBRARIES=pathlib,os,requests
SCRIPTLING_ALLOWED_PATHS=/app/data
```

`/etc/scriptling/api-network-policy.toml`:

```toml
https_only = true
allow_hosts = ["api.example.com"]
```

A script can now do:

```python
import pathlib
import requests

data = pathlib.Path("/app/data/input.json").read_text()
resp = requests.get("https://api.example.com/status")
```

...but `pathlib.Path("/etc/passwd").read_text()` and
`requests.get("https://not-api.example.com")` both fail with a policy error,
and `import subprocess` fails because it was never enabled.
