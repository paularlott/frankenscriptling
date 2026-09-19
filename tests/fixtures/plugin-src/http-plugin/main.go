// An HTTP-mode admin-supplied test plugin, built via `make build-test-plugin`
// as a fixture for the plugin tests that need a real HTTP(S) JSON-RPC
// endpoint: SCRIPTLING_PLUGIN pointed at an http(s) URL (admin preload, no
// network policy involved), and a script-initiated load() of a new HTTP
// plugin that the network policy actually allows. Same plugin.Server SDK as
// the stdio fixtures, served over HTTP instead of stdio — the same pattern
// as scriptling's own examples/plugins/http-go example.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/plugin"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8199", "HTTP listen address")
	path := flag.String("path", "/json-rpc", "JSON-RPC endpoint path")
	flag.Parse()

	greet := object.NewFunctionBuilder()
	greet.Function(func(name string) string { return "hello-http, " + name })

	server := plugin.NewServer("frankenscriptling-http-plugin", "1.0.0", "HTTP admin-supplied test plugin").
		RegisterFunc("greet", greet)

	mux := http.NewServeMux()
	mux.Handle(*path, server)
	log.Printf("frankenscriptling-http-plugin listening at http://%s%s", *addr, *path)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
