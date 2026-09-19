package frankenscriptling

// Resolves which scriptling libraries are registered and under what
// filesystem/network policy. The policy comes only from sources an operator
// controls (process environment, files on disk) — never from PHP. PHP script
// code has no way to call into this file; ensureVM() just reads the result.
//
// Default posture is closed: with nothing configured, no filesystem- or
// network-capable library is registered at all.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	scriptlingresolve "github.com/paularlott/scriptling/extlibs/net/resolve"
	"github.com/paularlott/scriptling/extlibs/netsecurity"
	"github.com/paularlott/scriptling/extlibs/secretprovider"
	"github.com/paularlott/scriptling/plugin"
)

const (
	envAllowedPaths      = "SCRIPTLING_ALLOWED_PATHS"
	envEnabledLibraries  = "SCRIPTLING_ENABLED_LIBRARIES"
	envNetworkPolicyFile = "SCRIPTLING_NETWORK_POLICY_FILE"
	envPolicyFile        = "SCRIPTLING_POLICY_FILE"

	// scriptling.secret: either point at one or more TOML files (each with
	// a [[providers]] array in secretprovider.Config's own schema, for
	// multiple aliased providers), or configure a single provider directly
	// from env vars — the latter keeps a credential like a Vault token out
	// of any file on disk, which is the point of "envs" for this one.
	envSecretProvidersFile   = "SCRIPTLING_SECRET_PROVIDERS_FILE"
	envSecretProvider        = "SCRIPTLING_SECRET_PROVIDER"
	envSecretAlias           = "SCRIPTLING_SECRET_ALIAS"
	envSecretAddress         = "SCRIPTLING_SECRET_ADDRESS"
	envSecretToken           = "SCRIPTLING_SECRET_TOKEN"
	envSecretAppRoleID       = "SCRIPTLING_SECRET_APP_ROLE_ID"
	envSecretAppRoleSecret   = "SCRIPTLING_SECRET_APP_ROLE_SECRET"
	envSecretNamespace       = "SCRIPTLING_SECRET_NAMESPACE"
	envSecretKVVersion       = "SCRIPTLING_SECRET_KV_VERSION"
	envSecretCacheTTL        = "SCRIPTLING_SECRET_CACHE_TTL"
	envSecretDefaultField    = "SCRIPTLING_SECRET_DEFAULT_FIELD"
	envSecretInsecureSkipTLS = "SCRIPTLING_SECRET_INSECURE_SKIP_TLS"

	// scriptling.plugin: admin-supplied plugin binaries only, never a
	// script-driven load(). SCRIPTLING_PLUGIN (specific executable paths or
	// http(s) URLs, comma-separated — same env var name and Path-is-either
	// convention as the scriptling CLI's own --plugin flag) and
	// SCRIPTLING_PLUGIN_DIR (directory scan, matches --plugin-dir) both
	// pre-load plugins the admin trusts, on our own unrestricted Go-side
	// manager — never reachable from a script. Scripts get
	// list/describe/call_function against whatever got pre-loaded this way,
	// but load()/unload() always fail (plugin.TransportNone).
	// SCRIPTLING_PLUGIN_HTTP_ENABLED (off by default) additionally lets
	// scripts load *new* HTTP(S) plugins on top of whatever was pre-loaded,
	// but only through the same network policy that governs requests/
	// scriptling.ai/scriptling.mcp — never re-enabling stdio/exec loading
	// from scripts. Unlike the CLI's --plugin, there's no env-var
	// equivalent of --plugin-arg/--plugin-env/--plugin-header/
	// --plugin-insecure: entries here are plain paths/URLs with no per-entry
	// customization.
	envPlugin            = "SCRIPTLING_PLUGIN"
	envPluginDir         = "SCRIPTLING_PLUGIN_DIR"
	envPluginHTTPEnabled = "SCRIPTLING_PLUGIN_HTTP_ENABLED"
)

// LibraryPolicy is the resolved policy for one VM: which libraries may be
// registered, and the fs/net restrictions those libraries get.
type LibraryPolicy struct {
	Enabled      map[string]bool
	AllowedPaths []string                 // always non-nil; empty means deny-all
	Network      *netsecurity.Config      // nil means "don't register net-capable libraries"
	Secrets      *secretprovider.Registry // nil means "don't register scriptling.secret"
	PluginScope  *plugin.Manager          // nil means "don't register scriptling.plugin"
}

func (p *LibraryPolicy) isEnabled(name string) bool {
	return p != nil && p.Enabled[name]
}

var (
	globalPolicy     *LibraryPolicy
	globalPolicyOnce sync.Once
	globalPolicyErr  error
)

// resolveGlobalPolicy loads the process-wide default policy once. A later
// per-vhost override mechanism (tracked separately, not implemented yet —
// see docs/security-policy.md) would layer on top of this rather than
// replace it.
func resolveGlobalPolicy() (*LibraryPolicy, error) {
	globalPolicyOnce.Do(func() {
		globalPolicy, globalPolicyErr = loadPolicy(
			os.Getenv(envAllowedPaths),
			os.Getenv(envEnabledLibraries),
			os.Getenv(envNetworkPolicyFile),
			os.Getenv(envPolicyFile),
		)
		if globalPolicyErr == nil {
			globalPolicy.Secrets, globalPolicyErr = loadSecretRegistryFromEnv()
		}
		if globalPolicyErr == nil {
			globalPolicy.PluginScope, globalPolicyErr = loadPluginScopeFromEnv(globalPolicy.Network)
		}
	})
	return globalPolicy, globalPolicyErr
}

// loadPluginScopeFromEnv builds the scope registerLibraries hands to
// scriptling.plugin, if any plugin config is present at all. The admin's own
// unrestricted parent manager pre-loads whatever SCRIPTLING_PLUGIN_DIR
// points at (never a script-driven call); the returned scope is what scripts
// actually see, and it is always restricted:
//   - SCRIPTLING_PLUGIN_HTTP_ENABLED unset/false (the default): TransportNone
//     — load()/unload() fail unconditionally, pre-loaded plugins remain
//     fully usable.
//   - SCRIPTLING_PLUGIN_HTTP_ENABLED=true and a network policy is configured:
//     TransportHTTP + WithHTTPTransport(guard's own transport) — scripts may
//     load new HTTP(S) plugins, but only within that network policy. Stdio/
//     exec loading from scripts is never re-enabled by this flag.
//   - SCRIPTLING_PLUGIN_HTTP_ENABLED=true with no network policy configured:
//     falls back to TransportNone — "enabled and configured" applies here
//     exactly like every other net-gated feature.
//
// Returns (nil, nil) when none of SCRIPTLING_PLUGIN, SCRIPTLING_PLUGIN_DIR,
// or SCRIPTLING_PLUGIN_HTTP_ENABLED is set, meaning scriptling.plugin is
// never registered regardless of SCRIPTLING_ENABLED_LIBRARIES.
func loadPluginScopeFromEnv(network *netsecurity.Config) (*plugin.Manager, error) {
	plugins := splitComma(os.Getenv(envPlugin))
	dirs := splitComma(os.Getenv(envPluginDir))
	httpEnabled := os.Getenv(envPluginHTTPEnabled) == "true"
	if len(plugins) == 0 && len(dirs) == 0 && !httpEnabled {
		return nil, nil
	}

	parent := plugin.NewManager(nil)

	// Explicit entries first, then the directory scan — same order and the
	// same identity rule (resolved path/URL) as the CLI's --plugin before
	// --plugin-dir: an executable also found via the dir scan is a no-op,
	// the explicit entry wins.
	if len(plugins) > 0 {
		specs := make([]plugin.PluginSpec, len(plugins))
		for i, p := range plugins {
			specs[i] = plugin.PluginSpec{Path: p}
		}
		if err := parent.LoadPlugins(context.Background(), specs); err != nil {
			return nil, fmt.Errorf("%s: %w", envPlugin, err)
		}
	}

	for _, dir := range dirs {
		parent.AddDir(dir)
	}
	if len(dirs) > 0 {
		// Load() itself only warns on a bad directory or a plugin that
		// failed to start (directory discovery is meant to tolerate a
		// stray non-plugin file) — it does not return an error for either.
		// That's the wrong default for us: a typo'd SCRIPTLING_PLUGIN_DIR
		// must not silently register nothing, so promote any warning to a
		// hard failure ourselves, same as every other malformed-config case.
		if err := parent.Load(context.Background()); err != nil {
			return nil, fmt.Errorf("%s: %w", envPluginDir, err)
		}
		if warnings := parent.Warnings(); len(warnings) > 0 {
			return nil, fmt.Errorf("%s: %s", envPluginDir, strings.Join(warnings, "; "))
		}
	}

	if httpEnabled && network != nil {
		guard, err := netsecurity.NewGuard(network)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", envPluginHTTPEnabled, err)
		}
		// HTTPClient's transport, not the bare NewTransport() dialer: it
		// wraps every request in guard.CheckURL (scheme, host allow/deny
		// lists, IP-literal handling) *and* validates every dialed address,
		// the same two-layer enforcement requests/scriptling.ai/scriptling.mcp
		// get. The dial-only transport alone would silently skip the
		// allow_hosts/deny_hosts checks for plugin loading.
		guardedTransport := guard.HTTPClient().Transport
		return parent.NewScope(plugin.WithTransport(plugin.TransportHTTP), plugin.WithHTTPTransport(guardedTransport)), nil
	}
	return parent.NewScope(plugin.WithTransport(plugin.TransportNone)), nil
}

// loadSecretRegistryFromEnv builds the secretprovider.Registry for
// scriptling.secret from process env: SCRIPTLING_SECRET_PROVIDERS_FILE for
// one or more TOML files of [[providers]] (secretprovider.Config's own
// schema — no reimplemented parsing), plus an optional single provider
// configured directly from SCRIPTLING_SECRET_* env vars, so a credential
// like a Vault token never has to sit in a file on disk. Both can be used
// together; returns nil (no error) if neither is set, meaning
// scriptling.secret stays unregistered regardless of SCRIPTLING_ENABLED_LIBRARIES.
func loadSecretRegistryFromEnv() (*secretprovider.Registry, error) {
	var configs []secretprovider.Config

	if filesEnv := os.Getenv(envSecretProvidersFile); filesEnv != "" {
		for _, path := range splitComma(filesEnv) {
			var file struct {
				Providers []secretprovider.Config `toml:"providers"`
			}
			if _, err := toml.DecodeFile(path, &file); err != nil {
				return nil, fmt.Errorf("%s: parse %s: %w", envSecretProvidersFile, path, err)
			}
			configs = append(configs, file.Providers...)
		}
	}

	if providerEnv := os.Getenv(envSecretProvider); providerEnv != "" {
		cfg := secretprovider.Config{
			Provider:        providerEnv,
			Alias:           os.Getenv(envSecretAlias),
			Address:         os.Getenv(envSecretAddress),
			Token:           os.Getenv(envSecretToken),
			AppRoleID:       os.Getenv(envSecretAppRoleID),
			AppRoleSecret:   os.Getenv(envSecretAppRoleSecret),
			Namespace:       os.Getenv(envSecretNamespace),
			CacheTTL:        os.Getenv(envSecretCacheTTL),
			DefaultField:    os.Getenv(envSecretDefaultField),
			InsecureSkipTLS: os.Getenv(envSecretInsecureSkipTLS) == "true",
		}
		if kv := os.Getenv(envSecretKVVersion); kv != "" {
			n, err := strconv.Atoi(kv)
			if err != nil {
				return nil, fmt.Errorf("%s: %q is not an integer", envSecretKVVersion, kv)
			}
			cfg.KVVersion = n
		}
		configs = append(configs, cfg)
	}

	if len(configs) == 0 {
		return nil, nil
	}

	registry := secretprovider.NewRegistry()
	if err := secretprovider.RegisterConfigs(registry, configs); err != nil {
		return nil, fmt.Errorf("scriptling.secret: %w", err)
	}
	return registry, nil
}

func loadPolicy(allowedPathsEnv, enabledLibsEnv, networkPolicyFileEnv, combinedPolicyFileEnv string) (*LibraryPolicy, error) {
	policy := &LibraryPolicy{
		Enabled:      enabledSet(splitComma(enabledLibsEnv)),
		AllowedPaths: parseAllowedPaths(allowedPathsEnv),
	}

	var netCfg *netsecurity.Config
	if networkPolicyFileEnv != "" {
		cfg, err := loadNetworkPolicyFiles(splitComma(networkPolicyFileEnv))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", envNetworkPolicyFile, err)
		}
		netCfg = cfg
	}

	if combinedPolicyFileEnv != "" {
		for _, path := range splitComma(combinedPolicyFileEnv) {
			var file combinedPolicyFile
			meta, err := toml.DecodeFile(path, &file)
			if err != nil {
				return nil, fmt.Errorf("%s: parse %s: %w", envPolicyFile, path, err)
			}
			if len(file.Libraries.Enabled) > 0 {
				policy.Enabled = enabledSet(file.Libraries.Enabled)
			}
			if len(file.Filesystem.AllowedPaths) > 0 {
				policy.AllowedPaths = file.Filesystem.AllowedPaths
			}
			// A [network] table with every field at its zero value (e.g.
			// just `https_only = false`, or present purely to opt into the
			// safe defaults with no customization) must still count as
			// "configured" — check whether the key was written, not
			// whether any field ended up non-zero.
			if meta.IsDefined("network") {
				cfg, err := file.Network.toConfig()
				if err != nil {
					return nil, fmt.Errorf("%s: %s: [network]: %w", envPolicyFile, path, err)
				}
				netCfg = mergeNetworkConfig(netCfg, cfg)
			}
		}
	}

	policy.Network = netCfg
	return policy, nil
}

func enabledSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

func splitComma(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseAllowedPaths turns SCRIPTLING_ALLOWED_PATHS into the []string the
// fssecurity-backed registrars expect. Unset or "-" is explicit deny-all
// (the closed-by-default posture); otherwise it's a colon-separated list of
// absolute directories, same convention as $PATH.
func parseAllowedPaths(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return []string{}
	}
	parts := strings.Split(raw, string(os.PathListSeparator))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// combinedPolicyFile is the schema for SCRIPTLING_POLICY_FILE: one file
// covering libraries + filesystem + network together, for operators who'd
// rather manage one file per vhost than several env vars.
type combinedPolicyFile struct {
	Libraries struct {
		Enabled []string `toml:"enabled"`
	} `toml:"libraries"`
	Filesystem struct {
		AllowedPaths []string `toml:"allowed_paths"`
	} `toml:"filesystem"`
	Network networkPolicyFile `toml:"network"`
}

// networkPolicyFile mirrors extlibs/netsecurity's own (unexported) TOML
// schema deliberately, so a [network] table here and a standalone
// netsecurity policy file (loaded via netsecurity.LoadConfig, see
// loadNetworkPolicyFiles below) use identical keys. Keep in sync with
// extlibs/netsecurity.policyFile if that schema ever changes upstream.
type networkPolicyFile struct {
	HTTPSOnly       bool     `toml:"https_only"`
	AllowIPLiterals bool     `toml:"allow_ip_literals"`
	AllowLoopback   bool     `toml:"allow_loopback"`
	AllowPrivateIPs bool     `toml:"allow_private_ips"`
	AllowHosts      []string `toml:"allow_hosts"`
	DenyHosts       []string `toml:"deny_hosts"`
	AllowCIDRs      []string `toml:"allow_cidrs"`
	DenyCIDRs       []string `toml:"deny_cidrs"`
	DNSServers      []string `toml:"dns_servers"`
	ClientTimeout   string   `toml:"client_timeout"`
}

func (f networkPolicyFile) toConfig() (*netsecurity.Config, error) {
	cfg := &netsecurity.Config{
		RequireHTTPS:    f.HTTPSOnly,
		AllowIPLiterals: f.AllowIPLiterals,
		AllowLoopback:   f.AllowLoopback,
		AllowPrivateIPs: f.AllowPrivateIPs,
		AllowHosts:      f.AllowHosts,
		DenyHosts:       f.DenyHosts,
		AllowedCIDRs:    f.AllowCIDRs,
		DeniedCIDRs:     f.DenyCIDRs,
		DNSServers:      f.DNSServers,
	}
	if f.ClientTimeout != "" {
		d, err := time.ParseDuration(f.ClientTimeout)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("client_timeout %q must be a positive duration like \"30s\"", f.ClientTimeout)
		}
		cfg.ClientTimeout = d
	}
	if _, err := netsecurity.NewGuard(cfg); err != nil {
		return nil, fmt.Errorf("invalid network policy: %w", err)
	}
	return cfg, nil
}

// loadNetworkPolicyFiles loads one or more netsecurity policy files (each in
// netsecurity's own flat TOML schema, via netsecurity.LoadConfig — no
// reimplemented parsing) and merges them in order: later files' non-empty
// list fields and any set duration override earlier ones; boolean flags are
// fully replaced by whichever file set them last.
func loadNetworkPolicyFiles(paths []string) (*netsecurity.Config, error) {
	var merged *netsecurity.Config
	for _, path := range paths {
		cfg, err := netsecurity.LoadConfig(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		merged = mergeNetworkConfig(merged, cfg)
	}
	return merged, nil
}

func mergeNetworkConfig(base, override *netsecurity.Config) *netsecurity.Config {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	merged := *base
	merged.RequireHTTPS = override.RequireHTTPS
	merged.AllowIPLiterals = override.AllowIPLiterals
	merged.AllowLoopback = override.AllowLoopback
	merged.AllowPrivateIPs = override.AllowPrivateIPs
	if len(override.AllowHosts) > 0 {
		merged.AllowHosts = override.AllowHosts
	}
	if len(override.DenyHosts) > 0 {
		merged.DenyHosts = override.DenyHosts
	}
	if len(override.AllowedCIDRs) > 0 {
		merged.AllowedCIDRs = override.AllowedCIDRs
	}
	if len(override.DeniedCIDRs) > 0 {
		merged.DeniedCIDRs = override.DeniedCIDRs
	}
	if len(override.DNSServers) > 0 {
		merged.DNSServers = override.DNSServers
	}
	if override.ClientTimeout > 0 {
		merged.ClientTimeout = override.ClientTimeout
	}
	return &merged
}

// stdlibResolver adapts a *net.Resolver (the network policy's own resolver,
// so scriptling.net.resolve answers through the same DNS servers and never
// bypasses the policy's resolve-validate-dial checks) to the interface
// scriptling.net.resolve expects.
type stdlibResolver struct {
	timeout  time.Duration
	resolver *net.Resolver
}

func (r stdlibResolver) LookupIP(host string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	return r.resolver.LookupHost(ctx, host)
}

func (r stdlibResolver) LookupSRV(service string) ([]*net.TCPAddr, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, srvs, err := r.resolver.LookupSRV(ctx, "", "", service)
	if err != nil {
		return nil, err
	}
	var addrs []*net.TCPAddr
	for _, srv := range srvs {
		target := strings.TrimSuffix(srv.Target, ".")
		ips, err := r.resolver.LookupHost(ctx, target)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if parsed := net.ParseIP(ip); parsed != nil {
				addrs = append(addrs, &net.TCPAddr{IP: parsed, Port: int(srv.Port)})
			}
		}
	}
	if len(addrs) == 0 {
		return nil, errors.New("no addresses found")
	}
	return addrs, nil
}

// ResolveSRVHttp resolves a "srv+host" URI to an "https://host:port" one via
// SRV lookup, or passes non-SRV URIs through unchanged (defaulting to
// https://) — required by scriptling.net.resolve's Resolver interface.
func (r stdlibResolver) ResolveSRVHttp(uri string) string {
	if !strings.HasPrefix(uri, "srv+") && !strings.HasPrefix(uri, "SRV+") {
		if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
			return "https://" + uri
		}
		return uri
	}

	u, err := url.Parse(uri[4:])
	if err != nil {
		return uri[4:]
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_, srvs, err := r.resolver.LookupSRV(ctx, "", "", u.Host)
	if err != nil || len(srvs) == 0 {
		return uri[4:]
	}

	port := int(srvs[0].Port)
	if port <= 0 {
		return uri[4:]
	}

	u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
	return u.String()
}

// netResolverFor builds the scriptling.net.resolve backend for a given
// network policy. Only called when the caller has already decided the
// library is enabled and cfg is non-nil.
func netResolverFor(cfg *netsecurity.Config) scriptlingresolve.Resolver {
	guard, err := netsecurity.NewGuard(cfg)
	if err != nil || guard == nil {
		return stdlibResolver{timeout: 2 * time.Second, resolver: net.DefaultResolver}
	}
	return stdlibResolver{timeout: 2 * time.Second, resolver: guard.Resolver()}
}
