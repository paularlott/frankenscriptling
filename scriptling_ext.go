package frankenscriptling

// #include <Zend/zend_types.h>
import "C"
import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unsafe"

	"github.com/dunglas/frankenphp"
	scriptling "github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/build"
	"github.com/paularlott/scriptling/extlibs/agent"
	extai "github.com/paularlott/scriptling/extlibs/ai"
	"github.com/paularlott/scriptling/extlibs/ai/memory"
	extmcp "github.com/paularlott/scriptling/extlibs/mcp"
	scriptlingresolve "github.com/paularlott/scriptling/extlibs/net/resolve"
	"github.com/paularlott/scriptling/extlibs/similarity"
	"github.com/paularlott/scriptling/libloader"
	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/plugin"
	"github.com/paularlott/scriptling/stdlib"

	extlibs "github.com/paularlott/scriptling/extlibs"
)

// export_php:class Scriptling
type ScriptlingVM struct {
	vm          *scriptling.Scriptling
	policy      *LibraryPolicy
	err         string
	autoloadDir string
}

func (s *ScriptlingVM) ensureVM() bool {
	if s.vm == nil {
		// Resolve the policy before creating the VM so a malformed policy
		// fails every call, not just the first.
		policy, err := resolveGlobalPolicy()
		if err != nil {
			s.setErr(fmt.Sprintf("scriptling security policy: %s", err))
			return false
		}
		s.policy = policy
		s.vm = scriptling.New(
			scriptling.WithMemoryLimit(policy.MemoryLimit),
			scriptling.WithMaxValueBytes(policy.MaxValueBytes),
		)
		registerLibraries(s.vm, policy)

		if s.autoloadDir != "" {
			loader := libloader.NewFilesystem(s.autoloadDir)
			s.vm.SetLibraryLoader(loader)
		}
	}
	return true
}

// registerLibraries registers the curated set of scriptling libraries this
// extension ships. Pure-compute libraries are always on; filesystem- and
// network-capable libraries are registered only when policy explicitly
// enables them (closed by default — see security.go and
// docs/security-policy.md). policy is resolved solely from operator-controlled
// sources (env/files); this function never receives input from PHP.
func registerLibraries(vm *scriptling.Scriptling, policy *LibraryPolicy) {
	reg := func(name string, fn func()) {
		if policy.isEnabled(name) {
			fn()
		}
	}

	// Always-on: pure computation, no filesystem or network access.
	stdlib.RegisterAll(vm)
	extlibs.RegisterTOMLLibrary(vm)
	extlibs.RegisterYAMLLibrary(vm)
	extlibs.RegisterHTMLParserLibrary(vm)
	extlibs.RegisterLoggingLibraryDefault(vm)
	extlibs.RegisterSysLibrary(vm, nil, nil)
	extlibs.RegisterSecretsLibrary(vm)
	extlibs.RegisterShlexLibrary(vm)
	extlibs.RegisterCsvLibrary(vm)
	extlibs.RegisterXmlLibrary(vm)
	extlibs.RegisterMarkdownLibrary(vm)
	extlibs.RegisterTemplateHTMLLibrary(vm)
	extlibs.RegisterTemplateTextLibrary(vm)
	similarity.Register(vm)
	memory.Register(vm)
	agent.Register(vm)
	agent.RegisterInteract(vm)

	// Filesystem-gated: closed unless policy.AllowedPaths is non-empty AND
	// the library is individually enabled.
	reg(extlibs.PathlibLibraryName, func() { extlibs.RegisterPathlibLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.OSLibraryName, func() { extlibs.RegisterOSLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.FSLibraryName, func() { extlibs.RegisterFSLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.GlobLibraryName, func() { extlibs.RegisterGlobLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.ShutilLibraryName, func() { extlibs.RegisterShutilLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.TempfileLibraryName, func() { extlibs.RegisterTempfileLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.TarfileLibraryName, func() { extlibs.RegisterTarfileLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.ZipfileLibraryName, func() { extlibs.RegisterZipfileLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.FindLibraryName, func() { extlibs.RegisterFindLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.GrepLibraryName, func() { extlibs.RegisterGrepLibrary(vm, policy.AllowedPaths) })
	reg(extlibs.SedLibraryName, func() { extlibs.RegisterSedLibrary(vm, policy.AllowedPaths) })

	// Disable-only: no upstream restriction knob, so it's all-or-nothing and
	// off unless explicitly enabled.
	reg(extlibs.SubprocessLibraryName, func() { extlibs.RegisterSubprocessLibrary(vm) })

	// Secret-provider-gated: closed unless a provider is configured (see
	// SCRIPTLING_SECRET_* / SCRIPTLING_SECRET_PROVIDERS_FILE in security.go)
	// AND the library is individually enabled.
	if policy.Secrets != nil {
		reg(extlibs.SecretLibraryName, func() { extlibs.RegisterSecretLibrary(vm, policy.Secrets) })
	}

	// Plugin-gated: closed unless SCRIPTLING_PLUGIN_DIR and/or
	// SCRIPTLING_PLUGIN_HTTP_ENABLED configured something (see security.go)
	// AND the library is individually enabled. policy.PluginScope is always
	// either a plugin.TransportNone scope (load/unload always fail; admin's
	// pre-loaded plugins stay fully usable) or, only when HTTP-loading is
	// explicitly enabled with a network policy present, a TransportHTTP
	// scope routed through that same policy's guarded transport.
	if policy.PluginScope != nil {
		reg(plugin.ControlLibraryName, func() {
			plugin.RegisterLibraries(vm, policy.PluginScope, plugin.PolicyFromSecurity(policy.Network, policy.AllowedPaths))
		})
	}

	// Network-gated: closed unless policy.Network is non-nil AND the library
	// is individually enabled.
	if policy.Network != nil {
		reg(extlibs.RequestsLibraryName, func() { extlibs.RegisterRequestsLibrary(vm, policy.Network) })
		reg(extlibs.WebSocketLibraryName, func() { extlibs.RegisterWebSocketLibrary(vm, policy.Network) })
		reg(extlibs.ResolveLibraryName, func() { scriptlingresolve.Register(vm, netResolverFor(policy.Network)) })
		reg(extlibs.AILibraryName, func() { extai.Register(vm, policy.Network) })
		reg(extlibs.MCPLibraryName, func() {
			// A stdio MCP server is a subprocess the script names and starts,
			// so it is allowed only when the subprocess library itself is
			// enabled; otherwise only HTTP(S) MCP servers, under the network
			// policy, can be reached.
			extmcp.RegisterConfigured(vm, policy.Network, policy.isEnabled(extlibs.SubprocessLibraryName))
			extmcp.RegisterToon(vm)
			extmcp.RegisterToolHelpers(vm)
		})
	}
}

func (s *ScriptlingVM) setError(err error) {
	if err != nil {
		s.err = err.Error()
	} else {
		s.err = ""
	}
}

func (s *ScriptlingVM) setErr(msg string) {
	s.err = msg
}

func (s *ScriptlingVM) clearErr() {
	s.err = ""
}

// --- Evaluation ---

// The eval methods and callFunction take an optional $functions array of
// name => PHP callable, bound as script functions for that call only (see
// php_functions.go).

// export_php:method Scriptling::eval(string $code, array $functions = []): string
func (s *ScriptlingVM) Eval(code *C.zend_string, functions *C.zend_array) unsafe.Pointer {
	return s.run(functions, func() (object.Object, error) {
		return s.vm.Eval(frankenphp.GoString(unsafe.Pointer(code)))
	})
}

// export_php:method Scriptling::evalWithTimeout(int $timeoutMs, string $code, array $functions = []): string
func (s *ScriptlingVM) EvalWithTimeout(timeoutMs int64, code *C.zend_string, functions *C.zend_array) unsafe.Pointer {
	return s.run(functions, func() (object.Object, error) {
		return s.vm.EvalWithTimeout(
			time.Duration(timeoutMs)*time.Millisecond,
			frankenphp.GoString(unsafe.Pointer(code)),
		)
	})
}

// export_php:method Scriptling::evalFile(string $path, array $functions = []): string
func (s *ScriptlingVM) EvalFile(path *C.zend_string, functions *C.zend_array) unsafe.Pointer {
	return s.run(functions, func() (object.Object, error) {
		return s.vm.EvalFile(frankenphp.GoString(unsafe.Pointer(path)))
	})
}

// run binds functions, runs fn and returns its result as a PHP string ("" on
// error, with the error kept for getLastError()).
func (s *ScriptlingVM) run(functions *C.zend_array, fn func() (object.Object, error)) unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	bound, err := s.bindPHPFunctions(functions)
	if err != nil {
		s.setErr(err.Error())
		return frankenphp.PHPString("", false)
	}
	result, err := fn()
	s.setError(err)
	stopped := bound != nil && bound.stopped
	bound.unbind()
	if err != nil || stopped {
		return frankenphp.PHPString("", false)
	}
	return frankenphp.PHPString(result.Inspect(), false)
}

// decodeJSON is json.Unmarshal with numbers kept as json.Number, so whole
// numbers reach scripts as ints and the rest as floats, as Python's
// json.loads does; plain Unmarshal makes every number a float64.
func decodeJSON(data string, v any) error {
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	// Like Unmarshal, reject anything after the value.
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("invalid character after top-level value")
	}
	return nil
}

// --- Set Variables ---

// export_php:method Scriptling::setVar(string $name, string $value): void
func (s *ScriptlingVM) SetVar(name *C.zend_string, value *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(
		frankenphp.GoString(unsafe.Pointer(name)),
		frankenphp.GoString(unsafe.Pointer(value)),
	))
}

// export_php:method Scriptling::setVarInt(string $name, int $value): void
func (s *ScriptlingVM) SetVarInt(name *C.zend_string, value int64) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(frankenphp.GoString(unsafe.Pointer(name)), value))
}

// export_php:method Scriptling::setVarFloat(string $name, float $value): void
func (s *ScriptlingVM) SetVarFloat(name *C.zend_string, value float64) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(frankenphp.GoString(unsafe.Pointer(name)), value))
}

// export_php:method Scriptling::setVarBool(string $name, bool $value): void
func (s *ScriptlingVM) SetVarBool(name *C.zend_string, value bool) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(frankenphp.GoString(unsafe.Pointer(name)), value))
}

// export_php:method Scriptling::setVarNull(string $name): void
func (s *ScriptlingVM) SetVarNull(name *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(frankenphp.GoString(unsafe.Pointer(name)), nil))
}

// export_php:method Scriptling::setVarJSON(string $name, string $json): void
func (s *ScriptlingVM) SetVarJSON(name *C.zend_string, jsonStr *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	goName := frankenphp.GoString(unsafe.Pointer(name))
	goJSON := frankenphp.GoString(unsafe.Pointer(jsonStr))
	var val interface{}
	if err := decodeJSON(goJSON, &val); err != nil {
		s.setErr(fmt.Sprintf("invalid JSON: %s", err))
		return
	}
	s.setError(s.vm.SetVar(goName, val))
}

// export_php:method Scriptling::setVarList(string $name, string $json): void
func (s *ScriptlingVM) SetVarList(name *C.zend_string, jsonStr *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	goName := frankenphp.GoString(unsafe.Pointer(name))
	goJSON := frankenphp.GoString(unsafe.Pointer(jsonStr))
	var val []interface{}
	if err := decodeJSON(goJSON, &val); err != nil {
		s.setErr(fmt.Sprintf("invalid JSON array: %s", err))
		return
	}
	s.setError(s.vm.SetVar(goName, val))
}

// export_php:method Scriptling::setVarDict(string $name, string $json): void
func (s *ScriptlingVM) SetVarDict(name *C.zend_string, jsonStr *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	goName := frankenphp.GoString(unsafe.Pointer(name))
	goJSON := frankenphp.GoString(unsafe.Pointer(jsonStr))
	var val map[string]interface{}
	if err := decodeJSON(goJSON, &val); err != nil {
		s.setErr(fmt.Sprintf("invalid JSON object: %s", err))
		return
	}
	s.setError(s.vm.SetVar(goName, val))
}

// --- Get Variables ---

// export_php:method Scriptling::getVar(string $name): string
func (s *ScriptlingVM) GetVar(name *C.zend_string) unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	val, errObj := s.vm.GetVar(frankenphp.GoString(unsafe.Pointer(name)))
	if errObj != nil {
		s.setErr(errObj.Inspect())
		return frankenphp.PHPString("", false)
	}
	s.clearErr()
	return frankenphp.PHPString(fmt.Sprintf("%v", val), false)
}

// export_php:method Scriptling::getVarAsString(string $name): string
func (s *ScriptlingVM) GetVarAsString(name *C.zend_string) unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	val, errObj := s.vm.GetVarAsString(frankenphp.GoString(unsafe.Pointer(name)))
	if errObj != nil {
		s.setErr(errObj.Inspect())
		return frankenphp.PHPString("", false)
	}
	s.clearErr()
	return frankenphp.PHPString(val, false)
}

// export_php:method Scriptling::getVarInt(string $name): int
func (s *ScriptlingVM) GetVarInt(name *C.zend_string) int64 {
	if !s.ensureVM() {
		return 0
	}
	val, errObj := s.vm.GetVarAsInt(frankenphp.GoString(unsafe.Pointer(name)))
	if errObj != nil {
		s.setErr(errObj.Inspect())
		return 0
	}
	s.clearErr()
	return val
}

// export_php:method Scriptling::getVarFloat(string $name): float
func (s *ScriptlingVM) GetVarFloat(name *C.zend_string) float64 {
	if !s.ensureVM() {
		return 0
	}
	val, errObj := s.vm.GetVarAsFloat(frankenphp.GoString(unsafe.Pointer(name)))
	if errObj != nil {
		s.setErr(errObj.Inspect())
		return 0
	}
	s.clearErr()
	return val
}

// export_php:method Scriptling::getVarBool(string $name): bool
func (s *ScriptlingVM) GetVarBool(name *C.zend_string) bool {
	if !s.ensureVM() {
		return false
	}
	val, errObj := s.vm.GetVarAsBool(frankenphp.GoString(unsafe.Pointer(name)))
	if errObj != nil {
		s.setErr(errObj.Inspect())
		return false
	}
	s.clearErr()
	return val
}

// export_php:method Scriptling::getVarJSON(string $name): string
func (s *ScriptlingVM) GetVarJSON(name *C.zend_string) unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("null", false)
	}
	val, errObj := s.vm.GetVar(frankenphp.GoString(unsafe.Pointer(name)))
	if errObj != nil {
		s.setErr(errObj.Inspect())
		return frankenphp.PHPString("null", false)
	}
	s.clearErr()
	bytes, err := json.Marshal(val)
	if err != nil {
		return frankenphp.PHPString("null", false)
	}
	return frankenphp.PHPString(string(bytes), false)
}

// --- Variable Management ---

// export_php:method Scriptling::unsetVar(string $name): void
func (s *ScriptlingVM) UnsetVar(name *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.vm.UnsetVar(frankenphp.GoString(unsafe.Pointer(name)))
	s.clearErr()
}

// export_php:method Scriptling::listVars(): string
func (s *ScriptlingVM) ListVars() unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("[]", false)
	}
	vars := s.vm.ListVars()
	bytes, err := json.Marshal(vars)
	if err != nil {
		return frankenphp.PHPString("[]", false)
	}
	return frankenphp.PHPString(string(bytes), false)
}

// export_php:method Scriptling::hasVar(string $name): bool
func (s *ScriptlingVM) HasVar(name *C.zend_string) bool {
	if !s.ensureVM() {
		return false
	}
	goName := frankenphp.GoString(unsafe.Pointer(name))
	for _, v := range s.vm.ListVars() {
		if v == goName {
			return true
		}
	}
	return false
}

// --- Environment ---

// export_php:method Scriptling::reset(): void
func (s *ScriptlingVM) Reset() {
	if !s.ensureVM() {
		return
	}
	s.vm.ResetEnv()
	s.clearErr()
}

// --- Libraries ---

// export_php:method Scriptling::registerScriptLibrary(string $name, string $source): void
func (s *ScriptlingVM) RegisterScriptLibrary(name *C.zend_string, source *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.RegisterScriptLibrary(
		frankenphp.GoString(unsafe.Pointer(name)),
		frankenphp.GoString(unsafe.Pointer(source)),
	))
}

// export_php:method Scriptling::import(string $name): void
func (s *ScriptlingVM) Import(name *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.Import(frankenphp.GoString(unsafe.Pointer(name))))
}

// export_php:method Scriptling::importMultiple(string $names): void
func (s *ScriptlingVM) ImportMultiple(names *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	goNames := frankenphp.GoString(unsafe.Pointer(names))
	var nameList []string
	if err := json.Unmarshal([]byte(goNames), &nameList); err != nil {
		s.setErr(fmt.Sprintf("invalid JSON array: %s", err))
		return
	}
	for _, n := range nameList {
		if err := s.vm.Import(n); err != nil {
			s.setError(err)
			return
		}
	}
	s.clearErr()
}

// export_php:method Scriptling::registerScriptFunc(string $name, string $script): void
func (s *ScriptlingVM) RegisterScriptFunc(name *C.zend_string, script *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.RegisterScriptFunc(
		frankenphp.GoString(unsafe.Pointer(name)),
		frankenphp.GoString(unsafe.Pointer(script)),
	))
}

// --- Function Calling ---

func (s *ScriptlingVM) parseArgsJSON(argsJSON string) ([]interface{}, error) {
	if argsJSON == "" || argsJSON == "[]" {
		return nil, nil
	}
	var args []interface{}
	if err := decodeJSON(argsJSON, &args); err != nil {
		return nil, fmt.Errorf("invalid JSON args: %s", err)
	}
	return args, nil
}

// export_php:method Scriptling::callFunction(string $name, string $argsJSON, array $functions = []): string
func (s *ScriptlingVM) CallFunction(name *C.zend_string, argsJSON *C.zend_string, functions *C.zend_array) unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	args, err := s.parseArgsJSON(frankenphp.GoString(unsafe.Pointer(argsJSON)))
	if err != nil {
		s.setErr(err.Error())
		return frankenphp.PHPString("", false)
	}
	return s.run(functions, func() (object.Object, error) {
		return s.vm.CallFunction(frankenphp.GoString(unsafe.Pointer(name)), args...)
	})
}

// --- Output Capture ---

// export_php:method Scriptling::enableOutputCapture(): void
func (s *ScriptlingVM) EnableOutputCapture() {
	if !s.ensureVM() {
		return
	}
	s.vm.EnableOutputCapture()
	s.clearErr()
}

// export_php:method Scriptling::getOutput(): string
func (s *ScriptlingVM) GetOutput() unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	return frankenphp.PHPString(s.vm.GetOutput(), false)
}

// --- Resource Limits ---

// tightenLimit checks a limit PHP asks for against the operator's: with no
// operator limit anything non-negative goes (0 = no limit); with one, PHP may
// only lower it, never raise or remove it.
func tightenLimit(what string, requested, operator int64) error {
	if requested < 0 {
		return fmt.Errorf("%s must not be negative", what)
	}
	if operator > 0 && (requested == 0 || requested > operator) {
		return fmt.Errorf("%s cannot be raised above the configured limit of %d bytes", what, operator)
	}
	return nil
}

// export_php:method Scriptling::setMemoryLimit(int $bytes): void
func (s *ScriptlingVM) SetMemoryLimit(bytes int64) {
	if !s.ensureVM() {
		return
	}
	if err := tightenLimit("memory limit", bytes, s.policy.MemoryLimit); err != nil {
		s.setError(err)
		return
	}
	s.vm.SetMemoryLimit(bytes)
	s.clearErr()
}

// export_php:method Scriptling::getMemoryLimit(): int
func (s *ScriptlingVM) GetMemoryLimit() int64 {
	if !s.ensureVM() {
		return 0
	}
	return s.vm.MemoryLimit()
}

// export_php:method Scriptling::setMaxValueBytes(int $bytes): void
func (s *ScriptlingVM) SetMaxValueBytes(bytes int64) {
	if !s.ensureVM() {
		return
	}
	if err := tightenLimit("max value bytes", bytes, s.policy.MaxValueBytes); err != nil {
		s.setError(err)
		return
	}
	s.vm.SetMaxValueBytes(bytes)
	s.clearErr()
}

// export_php:method Scriptling::getMaxValueBytes(): int
func (s *ScriptlingVM) GetMaxValueBytes() int64 {
	if !s.ensureVM() {
		return 0
	}
	return s.vm.MaxValueBytes()
}

// export_php:method Scriptling::getResourceUsage(): string
func (s *ScriptlingVM) GetResourceUsage() unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("{}", false)
	}
	u := s.vm.ResourceUsage()
	bytes, err := json.Marshal(map[string]any{
		"memory_bytes":               u.MemoryBytes,
		"peak_memory_bytes":          u.PeakMemoryBytes,
		"allocated_bytes":            u.AllocatedBytes,
		"memory_limit_cancellations": u.MemoryLimitCancellations,
	})
	if err != nil {
		return frankenphp.PHPString("{}", false)
	}
	return frankenphp.PHPString(string(bytes), false)
}

// --- Autoload Path ---

// export_php:method Scriptling::setAutoloadPath(string $path): void
func (s *ScriptlingVM) SetAutoloadPath(path *C.zend_string) {
	goPath := frankenphp.GoString(unsafe.Pointer(path))
	s.autoloadDir = goPath
	if s.vm != nil {
		loader := libloader.NewFilesystem(goPath)
		s.vm.SetLibraryLoader(loader)
	}
	s.clearErr()
}

// export_php:method Scriptling::addAutoloadPath(string $path): void
func (s *ScriptlingVM) AddAutoloadPath(path *C.zend_string) {
	goPath := frankenphp.GoString(unsafe.Pointer(path))
	if s.vm == nil {
		if s.autoloadDir == "" {
			s.autoloadDir = goPath
		} else {
			s.autoloadDir = s.autoloadDir + ":" + goPath
		}
		return
	}

	existing := s.vm.GetLibraryLoader()
	var chain *libloader.Chain
	if existing != nil {
		if c, ok := existing.(*libloader.Chain); ok {
			chain = c
		} else {
			chain = libloader.NewChain(existing)
		}
	} else {
		chain = libloader.NewChain()
	}
	chain.Add(libloader.NewFilesystem(goPath))
	s.vm.SetLibraryLoader(chain)
	s.clearErr()
}

// export_php:method Scriptling::getAutoloadPath(): string
func (s *ScriptlingVM) GetAutoloadPath() unsafe.Pointer {
	return frankenphp.PHPString(s.autoloadDir, false)
}

// --- Error Handling ---

// export_php:method Scriptling::getLastError(): string
func (s *ScriptlingVM) GetLastError() unsafe.Pointer {
	return frankenphp.PHPString(s.err, false)
}

// export_php:method Scriptling::hasError(): bool
func (s *ScriptlingVM) HasError() bool {
	return s.err != ""
}

// export_php:method Scriptling::clearError(): void
func (s *ScriptlingVM) ClearError() {
	s.err = ""
}

// --- Version ---

// export_php:method Scriptling::getScriptlingVersion(): string
func (s *ScriptlingVM) GetScriptlingVersion() unsafe.Pointer {
	return frankenphp.PHPString(build.Version, false)
}

// export_php:method Scriptling::getImageVersion(): string
func (s *ScriptlingVM) GetImageVersion() unsafe.Pointer {
	return frankenphp.PHPString(imageVersion, false)
}

// GetBuildInfo reports what this image was built from, as JSON: the image
// version and git revision, and the scriptling, FrankenPHP and PHP versions.
// The same values are on the image as OCI labels for docker inspect.
//
// export_php:method Scriptling::getBuildInfo(): string
func (s *ScriptlingVM) GetBuildInfo() unsafe.Pointer {
	info, _ := json.Marshal(map[string]string{
		"image":      imageVersion,
		"revision":   imageRevision,
		"scriptling": build.Version,
		"frankenphp": frankenPHPVersion,
		"php":        phpBuildVersion,
	})
	return frankenphp.PHPString(string(info), false)
}

func init() {
	_ = strings.Builder{}
}
