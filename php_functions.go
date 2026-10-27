package frankenscriptling

// PHP functions for scripts: eval(), evalWithTimeout(), evalFile() and
// callFunction() take an optional array of name => PHP callable. Each is bound
// as a Scriptling builtin for that one call only, then removed, so the VM
// never holds a PHP value between calls (no lifetime or worker-mode leak).
//
// A PHP callable can only run on the PHP thread that passed it, while that
// call is in progress: the builtins check both, so a script that stores one
// (f = upper) or calls it from a background task gets an error, not a crash.

/*
#include <pthread.h>
#include <stdlib.h>
#include <php.h>
#include <Zend/zend_exceptions.h>

// Outcome of fs_call.
#define FS_CALL_OK        0
#define FS_CALL_EXCEPTION 1 // a PHP exception: *msg and *cls set, exception cleared
#define FS_CALL_EXIT      2 // exit()/die(): the unwind exception is left pending
#define FS_CALL_FAILED    3 // the callable could not be called
#define FS_CALL_FATAL     4 // a fatal error (bailout): the request must end

// fs_funcs_copy copies the entries of ht into emalloc'd arrays: the keys
// (NULL for an integer key) and the values, each ZVAL_COPY'd so they stay
// valid whatever a callback does to the caller's array.
static uint32_t fs_funcs_copy(zend_array *ht, zend_string ***keys, zval **vals) {
	uint32_t n = zend_hash_num_elements(ht), i = 0;
	zend_string *k;
	zval *v;

	*keys = NULL;
	*vals = NULL;
	if (n == 0) {
		return 0;
	}
	*keys = safe_emalloc(n, sizeof(zend_string *), 0);
	*vals = safe_emalloc(n, sizeof(zval), 0);
	ZEND_HASH_FOREACH_STR_KEY_VAL(ht, k, v) {
		ZVAL_DEREF(v);
		(*keys)[i] = k ? zend_string_copy(k) : NULL;
		ZVAL_COPY(&(*vals)[i], v);
		i++;
	} ZEND_HASH_FOREACH_END();
	return i;
}

static void fs_funcs_free(zend_string **keys, zval *vals, uint32_t n) {
	for (uint32_t i = 0; i < n; i++) {
		if (keys[i]) {
			zend_string_release(keys[i]);
		}
		zval_ptr_dtor(&vals[i]);
	}
	if (keys) {
		efree(keys);
	}
	if (vals) {
		efree(vals);
	}
}

static zend_string *fs_key_at(zend_string **keys, uint32_t i) { return keys[i]; }
static zval *fs_val_at(zval *vals, uint32_t i) { return &vals[i]; }
static int fs_is_callable(zval *v) { return zend_is_callable(v, 0, NULL) ? 1 : 0; }
static int fs_same_thread(pthread_t t) { return pthread_equal(t, pthread_self()) ? 1 : 0; }

// Building PHP values from Go. Arrays use zend_symtable_*, which turns
// numeric string keys into integer keys as PHP itself does ("1" => 1).
static zval *fs_zvals_new(uint32_t n) {
	zval *z = safe_emalloc(n, sizeof(zval), 0);
	for (uint32_t i = 0; i < n; i++) {
		ZVAL_NULL(&z[i]);
	}
	return z;
}
static zval *fs_zval_at(zval *z, uint32_t i) { return &z[i]; }
static void fs_zvals_free(zval *z, uint32_t n) {
	for (uint32_t i = 0; i < n; i++) {
		zval_ptr_dtor(&z[i]);
	}
	efree(z);
}
static void fs_set_null(zval *z) { ZVAL_NULL(z); }
static void fs_set_bool(zval *z, int b) { ZVAL_BOOL(z, b); }
static void fs_set_long(zval *z, zend_long v) { ZVAL_LONG(z, v); }
static void fs_set_double(zval *z, double v) { ZVAL_DOUBLE(z, v); }
static void fs_set_string(zval *z, const char *s, size_t len) {
	if (len == 0) {
		ZVAL_EMPTY_STRING(z);
	} else {
		ZVAL_STRINGL(z, s, len);
	}
}
static HashTable *fs_set_array(zval *z, uint32_t size) {
	array_init_size(z, size);
	return Z_ARRVAL_P(z);
}
// The insert functions take over v's value.
static void fs_ht_append(HashTable *ht, zval *v) { zend_hash_next_index_insert(ht, v); }
static void fs_ht_set(HashTable *ht, const char *k, size_t klen, zval *v) { zend_symtable_str_update(ht, k, klen, v); }
static HashTable *fs_named_new(uint32_t n) { return zend_new_array(n); }
static void fs_named_set(HashTable *ht, const char *k, size_t klen, zval *v) { zend_hash_str_update(ht, k, klen, v); }
static void fs_ht_release(HashTable *ht) { zend_array_release(ht); }

// Reading PHP values into Go.
static int fs_type(zval *z) {
	ZVAL_DEREF(z);
	return Z_TYPE_P(z);
}
static zend_long fs_long(zval *z) { ZVAL_DEREF(z); return Z_LVAL_P(z); }
static double fs_double(zval *z) { ZVAL_DEREF(z); return Z_DVAL_P(z); }
static const char *fs_str(zval *z, size_t *len) {
	ZVAL_DEREF(z);
	*len = Z_STRLEN_P(z);
	return Z_STRVAL_P(z);
}
static HashTable *fs_arr(zval *z) { ZVAL_DEREF(z); return Z_ARRVAL_P(z); }
static uint32_t fs_arr_count(HashTable *ht) { return zend_hash_num_elements(ht); }
static int fs_arr_is_list(HashTable *ht) { return zend_array_is_list(ht) ? 1 : 0; }
// fs_arr_entry fetches the entry at *pos and advances it: returns 0 at the
// end. A string key comes back in key/klen, an integer key in idx (key NULL).
static int fs_arr_entry(HashTable *ht, HashPosition *pos, zval **val,
		const char **key, size_t *klen, zend_ulong *idx) {
	zend_string *k = NULL;
	zval *v = zend_hash_get_current_data_ex(ht, pos);
	if (!v) {
		return 0;
	}
	zend_hash_get_current_key_ex(ht, &k, idx, pos);
	*key = k ? ZSTR_VAL(k) : NULL;
	*klen = k ? ZSTR_LEN(k) : 0;
	*val = v;
	zend_hash_move_forward_ex(ht, pos);
	return 1;
}
static void fs_arr_start(HashTable *ht, HashPosition *pos) { zend_hash_internal_pointer_reset_ex(ht, pos); }

// fs_call calls cb with argc positional params and the named_params table
// (either may be empty/NULL; the caller owns and frees both). A fatal error
// inside the callback longjmps to the engine's bailout point, which would
// cross the Go frames between here and the PHP method; zend_try stops it here.
static int fs_call(zval *cb, zval *ret, uint32_t argc, zval *params,
		HashTable *named_params, zend_string **msg, zend_string **cls) {
	zend_fcall_info fci;
	zend_fcall_info_cache fcc;
	zend_execute_data *saved_ex = EG(current_execute_data);
	volatile int rc = FS_CALL_OK;

	ZVAL_UNDEF(ret);
	*msg = NULL;
	*cls = NULL;

	if (zend_fcall_info_init(cb, 0, &fci, &fcc, NULL, NULL) != SUCCESS) {
		return FS_CALL_FAILED;
	}
	fci.retval = ret;
	fci.params = params;
	fci.param_count = argc;
	fci.named_params = named_params;

	zend_try {
		if (zend_call_function(&fci, &fcc) != SUCCESS) {
			rc = FS_CALL_FAILED;
		}
	} zend_catch {
		rc = FS_CALL_FATAL;
	} zend_end_try();

	if (rc == FS_CALL_FATAL) {
		EG(current_execute_data) = saved_ex;
		ZVAL_UNDEF(ret);
		return rc;
	}

	if (EG(exception)) {
		zval_ptr_dtor(ret);
		ZVAL_UNDEF(ret);
		if (zend_is_unwind_exit(EG(exception)) || zend_is_graceful_exit(EG(exception))) {
			return FS_CALL_EXIT;
		}
		zend_object *ex = EG(exception);
		zval rv;
		zval *m = zend_read_property_ex(zend_get_exception_base(ex), ex,
			ZSTR_KNOWN(ZEND_STR_MESSAGE), 1, &rv);
		*msg = zval_get_string(m);
		*cls = zend_string_copy(ex->ce->name);
		zend_clear_exception();
		return FS_CALL_EXCEPTION;
	}
	return rc;
}

static void fs_zval_dtor(zval *v) { zval_ptr_dtor(v); }
static void fs_string_release(zend_string *s) { zend_string_release(s); }

// fs_end_request ends the request after a fatal error in a callback, as PHP
// would have: the error is already reported, and exit's unwind exception
// unwinds the PHP stack without running more code (try/catch cannot stop it).
static void fs_end_request(void) {
	if (!EG(exception)) {
		EG(exit_status) = 255;
		zend_throw_unwind_exit();
	}
}
*/
import "C"
import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"unsafe"

	"github.com/dunglas/frankenphp"
	scriptlingerrors "github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// phpValueMaxDepth bounds how deeply nested a value converted between PHP and
// Scriptling may be.
const phpValueMaxDepth = 512

var phpFunctionName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// phpFunctions is the set of PHP callables bound for one call.
type phpFunctions struct {
	vm     *ScriptlingVM
	keys   **C.zend_string
	vals   *C.zval
	n      C.uint32_t
	thread C.pthread_t

	active bool
	// stopped is set once a callback calls exit() or hits a fatal error: the
	// script is stopped and no further PHP calls are made.
	stopped bool
	fatal   bool
	reason  string

	bound []boundName
}

type boundName struct {
	name    string
	builtin *object.Builtin
	prev    object.Object
	hadPrev bool
}

// bindPHPFunctions validates functions and binds each as a builtin. A nil
// array (the parameter left out) binds nothing. On error nothing is bound.
func (s *ScriptlingVM) bindPHPFunctions(functions *C.zend_array) (*phpFunctions, error) {
	if functions == nil {
		return nil, nil
	}
	f := &phpFunctions{vm: s, thread: C.pthread_self()}
	f.n = C.fs_funcs_copy(functions, &f.keys, &f.vals)
	if f.n == 0 {
		f.free()
		return nil, nil
	}

	type entry struct {
		name string
		cb   *C.zval
	}
	entries := make([]entry, 0, int(f.n))
	for i := C.uint32_t(0); i < f.n; i++ {
		key := C.fs_key_at(f.keys, i)
		if key == nil {
			f.free()
			return nil, fmt.Errorf("functions: keys must be function names, got an integer key")
		}
		name := frankenphp.GoString(unsafe.Pointer(key))
		if !phpFunctionName.MatchString(name) {
			f.free()
			return nil, fmt.Errorf("functions: %q is not a valid function name", name)
		}
		cb := C.fs_val_at(f.vals, i)
		if C.fs_is_callable(cb) == 0 {
			f.free()
			return nil, fmt.Errorf("functions: %q is not callable", name)
		}
		entries = append(entries, entry{name, cb})
	}

	f.active = true
	for _, e := range entries {
		b := &object.Builtin{
			Fn:       f.builtin(e.name, e.cb),
			HelpText: fmt.Sprintf("%s(...) - PHP function", e.name),
		}
		prev, err := s.vm.GetVarAsObject(e.name)
		f.bound = append(f.bound, boundName{name: e.name, builtin: b, prev: prev, hadPrev: err == nil})
		s.vm.SetObjectVar(e.name, b)
	}
	return f, nil
}

// unbind removes the builtins, restoring whatever each name held before
// (unless the script has since rebound it), releases the callables and, after
// a fatal error in a callback, ends the request. Safe on nil.
func (f *phpFunctions) unbind() {
	if f == nil {
		return
	}
	f.active = false
	for i := len(f.bound) - 1; i >= 0; i-- {
		b := f.bound[i]
		cur, err := f.vm.vm.GetVarAsObject(b.name)
		if err != nil || cur != object.Object(b.builtin) {
			continue
		}
		if b.hadPrev {
			f.vm.vm.SetObjectVar(b.name, b.prev)
		} else {
			f.vm.vm.UnsetVar(b.name)
		}
	}
	f.free()
	if f.fatal {
		f.vm.setErr(f.reason)
		C.fs_end_request()
	} else if f.stopped {
		f.vm.setErr(f.reason)
	}
}

func (f *phpFunctions) free() {
	if f.n > 0 || f.keys != nil || f.vals != nil {
		C.fs_funcs_free(f.keys, f.vals, f.n)
	}
	f.keys, f.vals, f.n = nil, nil, 0
}

func (f *phpFunctions) builtin(name string, cb *C.zval) func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
	return func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if f.stopped {
			return scriptlingerrors.NewCancelledError()
		}
		if !f.active {
			return &object.Error{
				Message:       fmt.Sprintf("PHP function %s() can only be called during the call it was passed to", name),
				ExceptionType: object.ExceptionTypeRuntimeError,
			}
		}
		if C.fs_same_thread(f.thread) == 0 {
			return &object.Error{
				Message:       fmt.Sprintf("PHP function %s() can only be called from the script's own thread", name),
				ExceptionType: object.ExceptionTypeRuntimeError,
			}
		}

		var params *C.zval
		if len(args) > 0 {
			params = C.fs_zvals_new(C.uint32_t(len(args)))
			defer C.fs_zvals_free(params, C.uint32_t(len(args)))
		}
		for i, a := range args {
			if err := toZval(a, C.fs_zval_at(params, C.uint32_t(i)), map[object.Object]struct{}{}, 0); err != nil {
				return typeError("%s() argument %d: %s", name, i+1, err)
			}
		}
		var named *C.HashTable
		if kwargs.Len() > 0 {
			named = C.fs_named_new(C.uint32_t(kwargs.Len()))
			defer C.fs_ht_release(named)
			for _, k := range kwargs.Keys() {
				var v C.zval
				if err := toZval(kwargs.Get(k), &v, map[object.Object]struct{}{}, 0); err != nil {
					C.fs_zval_dtor(&v)
					return typeError("%s() argument %s: %s", name, k, err)
				}
				C.fs_named_set(named, cString(k), C.size_t(len(k)), &v)
			}
		}

		var ret C.zval
		var msg, cls *C.zend_string
		rc := C.fs_call(cb, &ret, C.uint32_t(len(args)), params, named, &msg, &cls)

		switch rc {
		case C.FS_CALL_OK:
			result, err := phpToScriptling(&ret, 0)
			C.fs_zval_dtor(&ret)
			if err != nil {
				return typeError("%s() returned %s", name, err)
			}
			return result
		case C.FS_CALL_EXCEPTION:
			message := frankenphp.GoString(unsafe.Pointer(msg))
			class := frankenphp.GoString(unsafe.Pointer(cls))
			C.fs_string_release(msg)
			C.fs_string_release(cls)
			return &object.Error{Message: message, ExceptionType: exceptionTypeFor(class)}
		case C.FS_CALL_EXIT:
			f.stopped = true
			f.reason = fmt.Sprintf("PHP function %s() called exit()", name)
			return scriptlingerrors.NewCancelledError()
		case C.FS_CALL_FATAL:
			f.stopped = true
			f.fatal = true
			f.reason = fmt.Sprintf("PHP function %s() failed with a fatal error", name)
			return scriptlingerrors.NewCancelledError()
		default:
			return &object.Error{
				Message:       fmt.Sprintf("PHP function %s() could not be called", name),
				ExceptionType: object.ExceptionTypeRuntimeError,
			}
		}
	}
}

func typeError(format string, args ...any) object.Object {
	return &object.Error{Message: fmt.Sprintf(format, args...), ExceptionType: object.ExceptionTypeTypeError}
}

// exceptionTypeFor maps a PHP exception class to the Scriptling exception
// type `except` matches; anything else is a RuntimeError (all are caught by
// `except Exception`).
func exceptionTypeFor(class string) string {
	switch class {
	case "TypeError", "ArgumentCountError":
		return object.ExceptionTypeTypeError
	case "ValueError", "JsonException", "InvalidArgumentException", "DomainException", "RangeException":
		return object.ExceptionTypeValueError
	case "DivisionByZeroError":
		return object.ExceptionTypeZeroDivisionError
	case "OutOfRangeException", "OutOfBoundsException":
		return object.ExceptionTypeIndexError
	}
	return object.ExceptionTypeRuntimeError
}

// toZval writes a script value into dst as a PHP value: None, bools, ints,
// floats and strings as themselves, bytes as a string, lists and tuples as
// PHP lists, dicts as arrays in key order. dst must be a null or undefined
// zval; on error it may hold a partly built array the caller destroys.
func toZval(obj object.Object, dst *C.zval, path map[object.Object]struct{}, depth int) error {
	if depth > phpValueMaxDepth {
		return fmt.Errorf("value nested more than %d levels", phpValueMaxDepth)
	}
	switch o := obj.(type) {
	case nil, *object.Null:
		C.fs_set_null(dst)
	case *object.Boolean:
		b := C.int(0)
		if o.BoolValue() {
			b = 1
		}
		C.fs_set_bool(dst, b)
	case *object.Integer:
		C.fs_set_long(dst, C.zend_long(o.IntValue()))
	case *object.Float:
		C.fs_set_double(dst, C.double(o.FloatValue()))
	case *object.String:
		v := o.StringValue()
		C.fs_set_string(dst, cString(v), C.size_t(len(v)))
	case *object.Bytes:
		v := o.BytesValue()
		var p *C.char
		if len(v) > 0 {
			p = (*C.char)(unsafe.Pointer(&v[0]))
		}
		C.fs_set_string(dst, p, C.size_t(len(v)))
	case *object.List:
		return listToZval(o, o.Elements, dst, path, depth)
	case *object.Tuple:
		return listToZval(o, o.Elements, dst, path, depth)
	case *object.Dict:
		if _, cyclic := path[o]; cyclic {
			return fmt.Errorf("value contains a reference to itself")
		}
		path[o] = struct{}{}
		defer delete(path, o)
		pairs := o.OrderedPairs()
		ht := C.fs_set_array(dst, C.uint32_t(len(pairs)))
		for _, p := range pairs {
			var v C.zval
			if err := toZval(p.Value, &v, path, depth+1); err != nil {
				C.fs_zval_dtor(&v)
				return err
			}
			k := p.StringKey()
			C.fs_ht_set(ht, cString(k), C.size_t(len(k)), &v)
		}
	default:
		return fmt.Errorf("cannot pass a %s to PHP", obj.Type())
	}
	return nil
}

func listToZval(o object.Object, elems []object.Object, dst *C.zval, path map[object.Object]struct{}, depth int) error {
	if _, cyclic := path[o]; cyclic {
		return fmt.Errorf("value contains a reference to itself")
	}
	path[o] = struct{}{}
	defer delete(path, o)
	ht := C.fs_set_array(dst, C.uint32_t(len(elems)))
	for _, e := range elems {
		var v C.zval
		if err := toZval(e, &v, path, depth+1); err != nil {
			C.fs_zval_dtor(&v)
			return err
		}
		C.fs_ht_append(ht, &v)
	}
	return nil
}

// cString points at s's bytes for C to copy (not NUL-terminated: always pass
// the length). nil for "".
func cString(s string) *C.char {
	if s == "" {
		return nil
	}
	return (*C.char)(unsafe.Pointer(unsafe.StringData(s)))
}

// phpToScriptling converts a PHP return value: null, scalars and arrays
// (arrays PHP considers lists become lists, other arrays dicts in PHP's key
// order, integer keys as strings). Objects and resources are refused.
func phpToScriptling(z *C.zval, depth int) (object.Object, error) {
	if depth > phpValueMaxDepth {
		return nil, fmt.Errorf("a value nested more than %d levels", phpValueMaxDepth)
	}
	switch C.fs_type(z) {
	case C.IS_UNDEF, C.IS_NULL:
		return &object.Null{}, nil
	case C.IS_FALSE:
		return object.NewBoolean(false), nil
	case C.IS_TRUE:
		return object.NewBoolean(true), nil
	case C.IS_LONG:
		return object.NewInteger(int64(C.fs_long(z))), nil
	case C.IS_DOUBLE:
		return object.NewFloat(float64(C.fs_double(z))), nil
	case C.IS_STRING:
		var n C.size_t
		p := C.fs_str(z, &n)
		return object.NewString(C.GoStringN(p, C.int(n))), nil
	case C.IS_ARRAY:
		ht := C.fs_arr(z)
		count := int(C.fs_arr_count(ht))
		isList := C.fs_arr_is_list(ht) == 1
		var elems []object.Object
		var d *object.Dict
		if isList {
			elems = make([]object.Object, 0, count)
		} else {
			d = object.NewDictSized(count)
		}
		var pos C.HashPosition
		C.fs_arr_start(ht, &pos)
		for {
			var v *C.zval
			var kp *C.char
			var klen C.size_t
			var idx C.zend_ulong
			if C.fs_arr_entry(ht, &pos, &v, &kp, &klen, &idx) == 0 {
				break
			}
			o, err := phpToScriptling(v, depth+1)
			if err != nil {
				return nil, err
			}
			if isList {
				elems = append(elems, o)
				continue
			}
			k := strconv.FormatUint(uint64(idx), 10)
			if kp != nil {
				k = C.GoStringN(kp, C.int(klen))
			}
			d.Store(object.DictStringKey(k), object.NewString(k), o)
		}
		if isList {
			return &object.List{Elements: elems}, nil
		}
		return d, nil
	}
	return nil, fmt.Errorf("a value Scriptling cannot hold (only null, scalars and arrays can be returned)")
}
