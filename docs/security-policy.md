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
| Not shipped | Excluded from this image entirely — no config can turn them on | `scriptling.wait_for`, `scriptling.messaging.*`, `scriptling.container` (Docker/Podman/Apple socket access), `scriptling.plugin`, `scriptling.valkey`, `scriptling.badgerdb`, `scriptling.sql`, `scriptling.sqlite`, `scriptling.net.gossip`, `scriptling.net.multicast`, `scriptling.net.unicast`, `scriptling.provision.file`, `scriptling.provision.fetch`, `scriptling.runtime.*`, `scriptling.nomad` |

Notes:
- `scriptling.ai.agent`, `scriptling.ai.agent.interact`, and `scriptling.ai.memory`
  don't construct their own HTTP clients — they call through `scriptling.ai`'s
  guarded client, so `scriptling.ai`'s network policy governs them too even
  though they're always registered.
- `scriptling.mcp` also registers its `RegisterToolHelpers`/`RegisterToon`
  companions when enabled.
- The excluded libraries are either a materially bigger blast radius than
  fs/net access (container runtime sockets, arbitrary plugin processes) or
  need infrastructure this image doesn't wire up (a plugin `Manager` with
  compiled-in binaries for the storage backends). They may be revisited as
  their own task.

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
