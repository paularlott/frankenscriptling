// A minimal admin-supplied test plugin, built via `make build-test-plugin`
// as a fixture for tests/test_security_plugins.php. Uses scriptling's own
// plugin.Server SDK directly — the same one scriptling's own Go-plugin docs
// and tests use — rather than a hand-rolled stand-in for the JSON-RPC-over-
// stdio protocol.
package main

import (
	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/plugin"
)

func main() {
	greet := object.NewFunctionBuilder()
	greet.Function(func(name string) string { return "hello, " + name })

	server := plugin.NewServer("frankenscriptling-test-plugin", "1.0.0", "admin-supplied test plugin").
		RegisterFunc("greet", greet)

	if err := server.Run(); err != nil {
		panic(err)
	}
}
