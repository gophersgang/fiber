// ⚡️ Fiber is an Express inspired web framework written in Go with ☕️
// 🤖 Github Repository: https://github.com/gofiber/fiber
// 📌 API Documentation: https://docs.gofiber.io

package fiber

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	fasthttp "github.com/valyala/fasthttp"
)

// Version of current package
const Version = "1.10.0"

// Map is a shortcut for map[string]interface{}, usefull for JSON returns
type Map map[string]interface{}

// App denotes the Fiber application.
type App struct {
	// Layer stack
	stack [][]*Layer
	// Fasthttp server
	server *fasthttp.Server
	mutex  sync.Mutex
	// App settings
	Settings *Settings
}

// Settings holds is a struct holding the server settings
type Settings struct {
	// This will spawn multiple Go processes listening on the same port
	Prefork bool // default: false
	// Enable strict routing. When enabled, the router treats "/foo" and "/foo/" as different.
	StrictRouting bool // default: false
	// Enable case sensitivity. When enabled, "/Foo" and "/foo" are different routes.
	CaseSensitive bool // default: false
	// Enables the "Server: value" HTTP header.
	ServerHeader string // default: ""
	// Enables handler values to be immutable even if you return from handler
	Immutable bool // default: false
	// Enable or disable ETag header generation, since both weak and strong etags are generated
	// using the same hashing method (CRC-32). Weak ETags are the default when enabled.
	// Optional. Default value false
	ETag bool
	// Max body size that the server accepts
	BodyLimit int // default: 4 * 1024 * 1024
	// Maximum number of concurrent connections.
	Concurrency int // default: 256 * 1024
	// Disable keep-alive connections, the server will close incoming connections after sending the first response to client
	DisableKeepalive bool // default: false
	// When set to true causes the default date header to be excluded from the response.
	DisableDefaultDate bool // default: false
	// When set to true, causes the default Content-Type header to be excluded from the Response.
	DisableDefaultContentType bool // default: false
	DisableHeaderNormalizing  bool // default: false
	// When set to true, it will not print out the fiber ASCII and "listening" on message
	DisableStartupMessage bool
	// Folder containing template files
	TemplateFolder string // default: ""
	// Template engine: html, amber, handlebars , mustache or pug
	TemplateEngine func(raw string, bind interface{}) (string, error) // default: nil
	// Extension for the template files
	TemplateExtension string // default: ""
	// The amount of time allowed to read the full request including body.
	ReadTimeout time.Duration // default: unlimited
	// The maximum duration before timing out writes of the response.
	WriteTimeout time.Duration // default: unlimited
	// The maximum amount of time to wait for the next request when keep-alive is enabled.
	IdleTimeout time.Duration // default: unlimited
}

// Group struct
type Group struct {
	prefix string
	app    *App
}

// Static struct
type Static struct {
	// Transparently compresses responses if set to true
	// This works differently than the github.com/gofiber/compression middleware
	// The server tries minimizing CPU usage by caching compressed files.
	// It adds ".fiber.gz" suffix to the original file name.
	// Optional. Default value false
	Compress bool
	// Enables byte range requests if set to true.
	// Optional. Default value false
	ByteRange bool
	// Enable directory browsing.
	// Optional. Default value false.
	Browse bool
	// Index file for serving a directory.
	// Optional. Default value "index.html".
	Index string
}

// New creates a new Fiber named instance.
// You can pass optional settings when creating a new instance.
func New(settings ...*Settings) *App {
	schemaDecoderForm.SetAliasTag("form")
	schemaDecoderForm.IgnoreUnknownKeys(true)
	schemaDecoderQuery.SetAliasTag("query")
	schemaDecoderQuery.IgnoreUnknownKeys(true)
	// Create app
	app := new(App)
	// Create route stack
	app.stack = make([][]*Layer, len(methodINT))
	// Create settings
	app.Settings = new(Settings)
	// Set default settings
	app.Settings.Prefork = getArgument("-prefork")
	app.Settings.BodyLimit = 4 * 1024 * 1024
	// If settings exist, set defaults
	if len(settings) > 0 {
		app.Settings = settings[0] // Set custom settings
		if !app.Settings.Prefork { // Default to -prefork flag if false
			app.Settings.Prefork = getArgument("-prefork")
		}
		if app.Settings.BodyLimit <= 0 { // Default MaxRequestBodySize
			app.Settings.BodyLimit = 4 * 1024 * 1024
		}
		if app.Settings.Concurrency <= 0 {
			app.Settings.Concurrency = 256 * 1024
		}
		if app.Settings.Immutable { // Replace unsafe conversion funcs
			getString = getStringImmutable
			getBytes = getBytesImmutable
		}
	}
	// Create server
	app.init()
	// Return application
	return app
}

// Use registers a middleware route.
// Middleware matches requests beginning with the provided prefix.
// Providing a prefix is optional, it defaults to "/".
//
// - app.Use(handler)
// - app.Use("/api", handler)
// - app.Use("/api", handler, handler)
func (app *App) Use(args ...interface{}) *App {
	var prefix string
	var handlers []func(*Ctx)

	for i := 0; i < len(args); i++ {
		switch arg := args[i].(type) {
		case string:
			prefix = arg
		case func(*Ctx):
			handlers = append(handlers, arg)
		default:
			log.Fatalf("Use: Invalid func(c *fiber.Ctx) handler %v", reflect.TypeOf(arg))
		}
	}
	return app.register("USE", prefix, handlers...)
}

// All ...
func (app *App) All(path string, handlers ...func(*Ctx)) *App {
	for m := range methodINT {
		app.register(m, path, handlers...)
	}
	return app
}

// Get ...
func (app *App) Get(path string, handlers ...func(*Ctx)) *App {
	return app.register(MethodGet, path, handlers...)
}

// Head ...
func (app *App) Head(path string, handlers ...func(*Ctx)) *App {
	return app.register(MethodHead, path, handlers...)
}

// Post ...
func (app *App) Post(path string, handlers ...func(*Ctx)) *App {
	return app.register(MethodPost, path, handlers...)
}

// Put ...
func (app *App) Put(path string, handlers ...func(*Ctx)) *App {
	return app.register(MethodPut, path, handlers...)
}

// Delete ...
func (app *App) Delete(path string, handlers ...func(*Ctx)) *App {
	return app.register(MethodDelete, path, handlers...)
}

// Connect ...
func (app *App) Connect(path string, handlers ...func(*Ctx)) *App {
	return app.register(MethodConnect, path, handlers...)
}

// Options ...
func (app *App) Options(path string, handlers ...func(*Ctx)) *App {
	return app.register(MethodOptions, path, handlers...)
}

// Trace ...
func (app *App) Trace(path string, handlers ...func(*Ctx)) *App {
	return app.register(MethodTrace, path, handlers...)
}

// Patch ...
func (app *App) Patch(path string, handlers ...func(*Ctx)) *App {
	return app.register(MethodPatch, path, handlers...)
}

// Add ...
func (app *App) Add(method, path string, handlers ...func(*Ctx)) *App {
	method = toUpper(method)
	if methodINT[method] == 0 && method != MethodGet {
		log.Fatalf("Add: Invalid HTTP method %s", method)
	}
	return app.register(method, path, handlers...)
}

// Group is used for Routes with common prefix to define a new sub-router with optional middleware.
func (app *App) Group(prefix string, handlers ...func(*Ctx)) *Group {
	if len(handlers) > 0 {
		app.register("USE", prefix, handlers...)
	}
	return &Group{
		prefix: prefix,
		app:    app,
	}
}

// Static registers a new route with path prefix to serve static files from the provided root directory.
func (app *App) Static(prefix, root string, config ...Static) *App {
	app.registerStatic(prefix, root, config...)
	return app
}

// Group is used for Routes with common prefix to define a new sub-router with optional middleware.
func (grp *Group) Group(prefix string, handlers ...func(*Ctx)) *Group {
	prefix = getGroupPath(grp.prefix, prefix)
	if len(handlers) > 0 {
		grp.app.register("USE", prefix, handlers...)
	}
	return &Group{
		prefix: prefix,
		app:    grp.app,
	}
}

// Static : https://fiber.wiki/application#static
func (grp *Group) Static(prefix, root string, config ...Static) *Group {
	prefix = getGroupPath(grp.prefix, prefix)
	grp.app.registerStatic(prefix, root, config...)
	return grp
}

// Use registers a middleware route.
// Middleware matches requests beginning with the provided prefix.
// Providing a prefix is optional, it defaults to "/"
func (grp *Group) Use(args ...interface{}) *Group {
	var path = ""
	var handlers []func(*Ctx)
	for i := 0; i < len(args); i++ {
		switch arg := args[i].(type) {
		case string:
			path = arg
		case func(*Ctx):
			handlers = append(handlers, arg)
		default:
			//log.Fatalf("Invalid Use() arguments, must be (prefix, handler) or (handler)")
		}
	}
	grp.app.register("USE", getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Add : https://fiber.wiki/application#http-methods
func (grp *Group) Add(method, path string, handlers ...func(*Ctx)) *Group {
	method = toUpper(method)
	if methodINT[method] == 0 && method != MethodGet {
		log.Fatalf("Add: Invalid HTTP method %s", method)
	}
	grp.app.register(method, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Connect : https://fiber.wiki/application#http-methods
func (grp *Group) Connect(path string, handlers ...func(*Ctx)) *Group {
	grp.app.register(MethodConnect, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Put : https://fiber.wiki/application#http-methods
func (grp *Group) Put(path string, handlers ...func(*Ctx)) *Group {
	grp.app.register(MethodPut, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Post : https://fiber.wiki/application#http-methods
func (grp *Group) Post(path string, handlers ...func(*Ctx)) *Group {
	grp.app.register(MethodPost, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Delete : https://fiber.wiki/application#http-methods
func (grp *Group) Delete(path string, handlers ...func(*Ctx)) *Group {
	grp.app.register(MethodDelete, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Head : https://fiber.wiki/application#http-methods
func (grp *Group) Head(path string, handlers ...func(*Ctx)) *Group {
	grp.app.register(MethodHead, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Patch : https://fiber.wiki/application#http-methods
func (grp *Group) Patch(path string, handlers ...func(*Ctx)) *Group {
	grp.app.register(MethodPatch, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Options : https://fiber.wiki/application#http-methods
func (grp *Group) Options(path string, handlers ...func(*Ctx)) *Group {
	grp.app.register(MethodOptions, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Trace : https://fiber.wiki/application#http-methods
func (grp *Group) Trace(path string, handlers ...func(*Ctx)) *Group {
	grp.app.register(MethodTrace, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// Get : https://fiber.wiki/application#http-methods
func (grp *Group) Get(path string, handlers ...func(*Ctx)) *Group {
	grp.app.register(MethodGet, getGroupPath(grp.prefix, path), handlers...)
	return grp
}

// All matches all HTTP methods and complete paths
func (grp *Group) All(path string, handlers ...func(*Ctx)) *Group {
	for m := range methodINT {
		grp.app.register(m, getGroupPath(grp.prefix, path), handlers...)
	}
	return grp
}

// Serve can be used to pass a custom listener
// This method does not support the Prefork feature
// Preforkin is not available using app.Serve(ln net.Listener)
// You can pass an optional *tls.Config to enable TLS.
func (app *App) Serve(ln net.Listener, tlsconfig ...*tls.Config) error {
	// Update fiber server settings
	app.init()
	// TLS config
	if len(tlsconfig) > 0 {
		ln = tls.NewListener(ln, tlsconfig[0])
	}
	// Print listening message
	if !app.Settings.DisableStartupMessage {
		fmt.Printf("        _______ __\n  ____ / ____(_) /_  ___  _____\n_____ / /_  / / __ \\/ _ \\/ ___/\n  __ / __/ / / /_/ /  __/ /\n    /_/   /_/_.___/\\___/_/ v%s\n", Version)
		fmt.Printf("Started listening on %s\n", ln.Addr().String())
	}
	return app.server.Serve(ln)
}

// Listen serves HTTP requests from the given addr or port.
// You can pass an optional *tls.Config to enable TLS.
func (app *App) Listen(address interface{}, tlsconfig ...*tls.Config) error {
	addr, ok := address.(string)
	if !ok {
		port, ok := address.(int)
		if !ok {
			return fmt.Errorf("Listen: Host must be an INT port or STRING address")
		}
		addr = strconv.Itoa(port)
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	// Update fiber server settings
	app.init()
	// Setup listener
	var ln net.Listener
	var err error
	// Prefork enabled, not available on windows
	if app.Settings.Prefork && runtime.NumCPU() > 1 && runtime.GOOS != "windows" {
		if ln, err = app.prefork(addr); err != nil {
			return err
		}
	} else {
		if ln, err = net.Listen("tcp4", addr); err != nil {
			return err
		}
	}
	// TLS config
	if len(tlsconfig) > 0 {
		ln = tls.NewListener(ln, tlsconfig[0])
	}
	// Print listening message
	if !app.Settings.DisableStartupMessage && !getArgument("-child") {
		fmt.Printf("        _______ __\n  ____ / ____(_) /_  ___  _____\n_____ / /_  / / __ \\/ _ \\/ ___/\n  __ / __/ / / /_/ /  __/ /\n    /_/   /_/_.___/\\___/_/ v%s\n", Version)
		fmt.Printf("Started listening on %s\n", ln.Addr().String())
	}
	return app.server.Serve(ln)
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
// Shutdown works by first closing all open listeners and then waiting indefinitely for all connections to return to idle and then shut down.
//
// When Shutdown is called, Serve, ListenAndServe, and ListenAndServeTLS immediately return nil.
// Make sure the program doesn't exit and waits instead for Shutdown to return.
//
// Shutdown does not close keepalive connections so its recommended to set ReadTimeout to something else than 0.
func (app *App) Shutdown() error {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	if app.server == nil {
		return fmt.Errorf("Server is not running")
	}
	return app.server.Shutdown()
}

// Test is used for internal debugging by passing a *http.Request
// Timeout is optional and defaults to 1s, -1 will disable it completely.
func (app *App) Test(request *http.Request, msTimeout ...int) (*http.Response, error) {
	timeout := 1000 // 1 second default
	if len(msTimeout) > 0 {
		timeout = msTimeout[0]
	}
	// Add Content-Length if not provided with body
	if request.Body != http.NoBody && request.Header.Get("Content-Length") == "" {
		request.Header.Add("Content-Length", strconv.FormatInt(request.ContentLength, 10))
	}
	// Dump raw http request
	dump, err := httputil.DumpRequest(request, true)
	if err != nil {
		return nil, err
	}
	// Update server settings
	app.init()
	// Create test connection
	conn := new(testConn)
	// Write raw http request
	if _, err = conn.r.Write(dump); err != nil {
		return nil, err
	}
	// Serve conn to server
	channel := make(chan error)
	go func() {
		channel <- app.server.ServeConn(conn)
	}()
	// Wait for callback
	if timeout >= 0 {
		// With timeout
		select {
		case err = <-channel:
		case <-time.After(time.Duration(timeout) * time.Millisecond):
			return nil, fmt.Errorf("Timeout error %vms", timeout)
		}
	} else {
		// Without timeout
		err = <-channel
	}
	// Check for errors
	if err != nil {
		return nil, err
	}
	// Read response
	buffer := bufio.NewReader(&conn.w)
	// Convert raw http response to *http.Response
	resp, err := http.ReadResponse(buffer, request)
	if err != nil {
		return nil, err
	}
	// Return *http.Response
	return resp, nil
}

// Sharding: https://www.nginx.com/blog/socket-sharding-nginx-release-1-9-1/
func (app *App) prefork(address string) (ln net.Listener, err error) {
	// Master proc
	if !getArgument("-child") {
		addr, err := net.ResolveTCPAddr("tcp", address)
		if err != nil {
			return ln, err
		}
		tcplistener, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return ln, err
		}
		fl, err := tcplistener.File()
		if err != nil {
			return ln, err
		}
		files := []*os.File{fl}
		childs := make([]*exec.Cmd, runtime.NumCPU()/2)
		// #nosec G204
		for i := range childs {
			childs[i] = exec.Command(os.Args[0], append(os.Args[1:], "-prefork", "-child")...)
			childs[i].Stdout = os.Stdout
			childs[i].Stderr = os.Stderr
			childs[i].ExtraFiles = files
			if err := childs[i].Start(); err != nil {
				return ln, err
			}
		}

		for k := range childs {
			if err := childs[k].Wait(); err != nil {
				return ln, err
			}
		}
		os.Exit(0)
	} else {
		// 1 core per child
		runtime.GOMAXPROCS(1)
		ln, err = net.FileListener(os.NewFile(3, ""))
	}
	return ln, err
}

type disableLogger struct{}

func (dl *disableLogger) Printf(format string, args ...interface{}) {
	// fmt.Println(fmt.Sprintf(format, args...))
}

func (app *App) init() {
	app.mutex.Lock()
	if app.server == nil {
		app.server = &fasthttp.Server{
			Logger:       &disableLogger{},
			LogAllErrors: false,
			ErrorHandler: func(fctx *fasthttp.RequestCtx, err error) {
				if _, ok := err.(*fasthttp.ErrSmallBuffer); ok {
					fctx.Response.SetStatusCode(StatusRequestHeaderFieldsTooLarge)
					fctx.Response.SetBodyString("Request Header Fields Too Large")
				} else if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
					fctx.Response.SetStatusCode(StatusRequestTimeout)
					fctx.Response.SetBodyString("Request Timeout")
				} else if len(err.Error()) == 33 && err.Error() == "body size exceeds the given limit" {
					fctx.Response.SetStatusCode(StatusRequestEntityTooLarge)
					fctx.Response.SetBodyString("Request Entity Too Large")
				} else {
					fctx.Response.SetStatusCode(StatusBadRequest)
					fctx.Response.SetBodyString("Bad Request")
				}
			},
		}
	}
	if app.server.Handler == nil {
		app.server.Handler = app.handler
	}
	app.server.Name = app.Settings.ServerHeader
	app.server.Concurrency = app.Settings.Concurrency
	app.server.NoDefaultDate = app.Settings.DisableDefaultDate
	app.server.NoDefaultContentType = app.Settings.DisableDefaultContentType
	app.server.DisableHeaderNamesNormalizing = app.Settings.DisableHeaderNormalizing
	app.server.DisableKeepalive = app.Settings.DisableKeepalive
	app.server.MaxRequestBodySize = app.Settings.BodyLimit
	app.server.NoDefaultServerHeader = app.Settings.ServerHeader == ""
	app.server.ReadTimeout = app.Settings.ReadTimeout
	app.server.WriteTimeout = app.Settings.WriteTimeout
	app.server.IdleTimeout = app.Settings.IdleTimeout
	app.mutex.Unlock()
}
