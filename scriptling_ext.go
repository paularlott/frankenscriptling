package frankenscriptling

// #include <Zend/zend_types.h>
import "C"
import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unsafe"

	scriptling "github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/build"
	extai "github.com/paularlott/scriptling/extlibs/ai"
	"github.com/paularlott/scriptling/extlibs/ai/memory"
	"github.com/paularlott/scriptling/extlibs/agent"
	extmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/extlibs/similarity"
	"github.com/paularlott/scriptling/libloader"
	"github.com/paularlott/scriptling/stdlib"
	"github.com/dunglas/frankenphp"

	extlibs "github.com/paularlott/scriptling/extlibs"
)

//export_php:class Scriptling
type ScriptlingVM struct {
	vm          *scriptling.Scriptling
	err         string
	autoloadDir string
}

func (s *ScriptlingVM) ensureVM() bool {
	if s.vm == nil {
		s.vm = scriptling.New()

		stdlib.RegisterAll(s.vm)
		extlibs.RegisterTOMLLibrary(s.vm)
		extlibs.RegisterYAMLLibrary(s.vm)
		extai.Register(s.vm)
		extmcp.Register(s.vm)
		extmcp.RegisterToon(s.vm)
		similarity.Register(s.vm)
		memory.Register(s.vm)
		extlibs.RegisterTemplateHTMLLibrary(s.vm)
		extlibs.RegisterTemplateTextLibrary(s.vm)
		agent.Register(s.vm)
		agent.RegisterInteract(s.vm)

		if s.autoloadDir != "" {
			loader := libloader.NewFilesystem(s.autoloadDir)
			s.vm.SetLibraryLoader(loader)
		}
	}
	return true
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

//export_php:method Scriptling::eval(string $code): string
func (s *ScriptlingVM) Eval(code *C.zend_string) unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	result, err := s.vm.Eval(frankenphp.GoString(unsafe.Pointer(code)))
	s.setError(err)
	if err != nil {
		return frankenphp.PHPString("", false)
	}
	return frankenphp.PHPString(result.Inspect(), false)
}

//export_php:method Scriptling::evalWithTimeout(int $timeoutMs, string $code): string
func (s *ScriptlingVM) EvalWithTimeout(timeoutMs int64, code *C.zend_string) unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	result, err := s.vm.EvalWithTimeout(
		time.Duration(timeoutMs)*time.Millisecond,
		frankenphp.GoString(unsafe.Pointer(code)),
	)
	s.setError(err)
	if err != nil {
		return frankenphp.PHPString("", false)
	}
	return frankenphp.PHPString(result.Inspect(), false)
}

//export_php:method Scriptling::evalFile(string $path): string
func (s *ScriptlingVM) EvalFile(path *C.zend_string) unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	result, err := s.vm.EvalFile(frankenphp.GoString(unsafe.Pointer(path)))
	s.setError(err)
	if err != nil {
		return frankenphp.PHPString("", false)
	}
	return frankenphp.PHPString(result.Inspect(), false)
}

// --- Set Variables ---

//export_php:method Scriptling::setVar(string $name, string $value): void
func (s *ScriptlingVM) SetVar(name *C.zend_string, value *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(
		frankenphp.GoString(unsafe.Pointer(name)),
		frankenphp.GoString(unsafe.Pointer(value)),
	))
}

//export_php:method Scriptling::setVarInt(string $name, int $value): void
func (s *ScriptlingVM) SetVarInt(name *C.zend_string, value int64) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(frankenphp.GoString(unsafe.Pointer(name)), value))
}

//export_php:method Scriptling::setVarFloat(string $name, float $value): void
func (s *ScriptlingVM) SetVarFloat(name *C.zend_string, value float64) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(frankenphp.GoString(unsafe.Pointer(name)), value))
}

//export_php:method Scriptling::setVarBool(string $name, bool $value): void
func (s *ScriptlingVM) SetVarBool(name *C.zend_string, value bool) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(frankenphp.GoString(unsafe.Pointer(name)), value))
}

//export_php:method Scriptling::setVarNull(string $name): void
func (s *ScriptlingVM) SetVarNull(name *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.SetVar(frankenphp.GoString(unsafe.Pointer(name)), nil))
}

//export_php:method Scriptling::setVarJSON(string $name, string $json): void
func (s *ScriptlingVM) SetVarJSON(name *C.zend_string, jsonStr *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	goName := frankenphp.GoString(unsafe.Pointer(name))
	goJSON := frankenphp.GoString(unsafe.Pointer(jsonStr))
	var val interface{}
	if err := json.Unmarshal([]byte(goJSON), &val); err != nil {
		s.setErr(fmt.Sprintf("invalid JSON: %s", err))
		return
	}
	s.setError(s.vm.SetVar(goName, val))
}

//export_php:method Scriptling::setVarList(string $name, string $json): void
func (s *ScriptlingVM) SetVarList(name *C.zend_string, jsonStr *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	goName := frankenphp.GoString(unsafe.Pointer(name))
	goJSON := frankenphp.GoString(unsafe.Pointer(jsonStr))
	var val []interface{}
	if err := json.Unmarshal([]byte(goJSON), &val); err != nil {
		s.setErr(fmt.Sprintf("invalid JSON array: %s", err))
		return
	}
	s.setError(s.vm.SetVar(goName, val))
}

//export_php:method Scriptling::setVarDict(string $name, string $json): void
func (s *ScriptlingVM) SetVarDict(name *C.zend_string, jsonStr *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	goName := frankenphp.GoString(unsafe.Pointer(name))
	goJSON := frankenphp.GoString(unsafe.Pointer(jsonStr))
	var val map[string]interface{}
	if err := json.Unmarshal([]byte(goJSON), &val); err != nil {
		s.setErr(fmt.Sprintf("invalid JSON object: %s", err))
		return
	}
	s.setError(s.vm.SetVar(goName, val))
}

// --- Get Variables ---

//export_php:method Scriptling::getVar(string $name): string
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

//export_php:method Scriptling::getVarAsString(string $name): string
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

//export_php:method Scriptling::getVarInt(string $name): int
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

//export_php:method Scriptling::getVarFloat(string $name): float
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

//export_php:method Scriptling::getVarBool(string $name): bool
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

//export_php:method Scriptling::getVarJSON(string $name): string
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

//export_php:method Scriptling::unsetVar(string $name): void
func (s *ScriptlingVM) UnsetVar(name *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.vm.UnsetVar(frankenphp.GoString(unsafe.Pointer(name)))
	s.clearErr()
}

//export_php:method Scriptling::listVars(): string
func (s *ScriptlingVM) ListVars() unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("[]", false)
	}
	vars := s.vm.ListVars()
	bytes, err := json.Marshal(vars)
	if err != nil {
		return frankenphp.PHPString("[]", false)
	}
	s.clearErr()
	return frankenphp.PHPString(string(bytes), false)
}

//export_php:method Scriptling::hasVar(string $name): bool
func (s *ScriptlingVM) HasVar(name *C.zend_string) bool {
	if !s.ensureVM() {
		return false
	}
	goName := frankenphp.GoString(unsafe.Pointer(name))
	for _, v := range s.vm.ListVars() {
		if v == goName {
			s.clearErr()
			return true
		}
	}
	s.clearErr()
	return false
}

// --- Environment ---

//export_php:method Scriptling::reset(): void
func (s *ScriptlingVM) Reset() {
	if !s.ensureVM() {
		return
	}
	s.vm.ResetEnv()
	s.clearErr()
}

// --- Libraries ---

//export_php:method Scriptling::registerScriptLibrary(string $name, string $source): void
func (s *ScriptlingVM) RegisterScriptLibrary(name *C.zend_string, source *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.RegisterScriptLibrary(
		frankenphp.GoString(unsafe.Pointer(name)),
		frankenphp.GoString(unsafe.Pointer(source)),
	))
}

//export_php:method Scriptling::import(string $name): void
func (s *ScriptlingVM) Import(name *C.zend_string) {
	if !s.ensureVM() {
		return
	}
	s.setError(s.vm.Import(frankenphp.GoString(unsafe.Pointer(name))))
}

//export_php:method Scriptling::importMultiple(string $names): void
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

//export_php:method Scriptling::registerScriptFunc(string $name, string $script): void
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
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid JSON args: %s", err)
	}
	return args, nil
}

//export_php:method Scriptling::callFunction(string $name, string $argsJSON): string
func (s *ScriptlingVM) CallFunction(name *C.zend_string, argsJSON *C.zend_string) unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	args, err := s.parseArgsJSON(frankenphp.GoString(unsafe.Pointer(argsJSON)))
	if err != nil {
		s.setErr(err.Error())
		return frankenphp.PHPString("", false)
	}
	result, callErr := s.vm.CallFunction(frankenphp.GoString(unsafe.Pointer(name)), args...)
	s.setError(callErr)
	if callErr != nil {
		return frankenphp.PHPString("", false)
	}
	return frankenphp.PHPString(result.Inspect(), false)
}

// --- Output Capture ---

//export_php:method Scriptling::enableOutputCapture(): void
func (s *ScriptlingVM) EnableOutputCapture() {
	if !s.ensureVM() {
		return
	}
	s.vm.EnableOutputCapture()
	s.clearErr()
}

//export_php:method Scriptling::getOutput(): string
func (s *ScriptlingVM) GetOutput() unsafe.Pointer {
	if !s.ensureVM() {
		return frankenphp.PHPString("", false)
	}
	s.clearErr()
	return frankenphp.PHPString(s.vm.GetOutput(), false)
}

// --- Autoload Path ---

//export_php:method Scriptling::setAutoloadPath(string $path): void
func (s *ScriptlingVM) SetAutoloadPath(path *C.zend_string) {
	goPath := frankenphp.GoString(unsafe.Pointer(path))
	s.autoloadDir = goPath
	if s.vm != nil {
		loader := libloader.NewFilesystem(goPath)
		s.vm.SetLibraryLoader(loader)
	}
	s.clearErr()
}

//export_php:method Scriptling::addAutoloadPath(string $path): void
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

//export_php:method Scriptling::getAutoloadPath(): string
func (s *ScriptlingVM) GetAutoloadPath() unsafe.Pointer {
	return frankenphp.PHPString(s.autoloadDir, false)
}

// --- Error Handling ---

//export_php:method Scriptling::getLastError(): string
func (s *ScriptlingVM) GetLastError() unsafe.Pointer {
	return frankenphp.PHPString(s.err, false)
}

//export_php:method Scriptling::hasError(): bool
func (s *ScriptlingVM) HasError() bool {
	return s.err != ""
}

//export_php:method Scriptling::clearError(): void
func (s *ScriptlingVM) ClearError() {
	s.err = ""
}

// --- Version ---

//export_php:method Scriptling::getScriptlingVersion(): string
func (s *ScriptlingVM) GetScriptlingVersion() unsafe.Pointer {
	return frankenphp.PHPString(build.Version, false)
}

func init() {
	_ = strings.Builder{}
}
