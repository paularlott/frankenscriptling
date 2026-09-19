// A second, distinctly-named admin-supplied test plugin, built via
// `make build-test-plugin` as a fixture for tests/test_security_plugins_dual.php.
// It exists to prove SCRIPTLING_PLUGIN and SCRIPTLING_PLUGIN_DIR can preload
// two different plugins from two different sources at once — main.go (in
// this same source tree) is the one discovered by a directory scan, this one
// is loaded by an explicit path instead.
package main

import (
	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/plugin"
)

func main() {
	wave := object.NewFunctionBuilder()
	wave.Function(func(name string) string { return "wave, " + name })

	server := plugin.NewServer("frankenscriptling-second-plugin", "1.0.0", "second admin-supplied test plugin").
		RegisterFunc("wave", wave)

	if err := server.Run(); err != nil {
		panic(err)
	}
}
