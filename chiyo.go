package chiyo

import (
	"net/http"
	"strings"
)

type (
	Route struct {
		method  string
		pattern string
		handler http.HandlerFunc
	}

	Router struct {
		staticRoutes  map[string]http.HandlerFunc
		dynamicRoutes map[string]*node
		middleware    []func(http.HandlerFunc) http.HandlerFunc
		notFound      http.HandlerFunc
	}

	Group struct {
		prefix     string
		middleware []func(http.HandlerFunc) http.HandlerFunc
		router     *Router
	}
)

type node struct {
	children   map[string]*node
	handler    http.HandlerFunc
	isParam    bool
	isWillcard bool
}

func NewRouter() *Router {
	return &Router{
		staticRoutes:  make(map[string]http.HandlerFunc),
		dynamicRoutes: make(map[string]*node),
		middleware:    []func(http.HandlerFunc) http.HandlerFunc{},
		notFound:      http.NotFound,
	}
}

func (r *Router) Group(prefix string) *Group {
	return &Group{
		prefix: strings.Trim(prefix, "/"),
		router: r,
	}
}

func (r *Router) Use(mw func(http.HandlerFunc) http.HandlerFunc) {
	r.middleware = append(r.middleware, mw)
}

func (g *Group) Use(mw func(http.HandlerFunc) http.HandlerFunc) {
	g.middleware = append(g.middleware, mw)
}

func (r *Router) AddRoute(method, path string, handler http.HandlerFunc) {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")

	if strings.Contains(path, ":") || strings.Contains(path, "*") {
		if r.dynamicRoutes[method] == nil {
			r.dynamicRoutes[method] = &node{
				children: make(map[string]*node),
			}
		}

		r.insertDynamicRoute(method, parts, handler)
	} else {
		r.staticRoutes[method+" "+path] = handler
	}
}

func (g *Group) AddRoute(method, path string, handler http.HandlerFunc) {
	fullPath := g.prefix + "/" + strings.Trim(path, "/")
	wrappedHandler := handler

	for i := len(g.middleware); i >= 0; i-- {
		wrappedHandler = g.middleware[i](wrappedHandler)
	}

	g.router.AddRoute(method, fullPath, wrappedHandler)
}

func (r *Router) insertDynamicRoute(method string, parts []string, handler http.HandlerFunc) {
	current := r.dynamicRoutes[method]

	for _, part := range parts {
		var key string
		var isParam, isWillcard bool

		if strings.HasPrefix(part, ":") {
			key = ":param"
			isParam = true
		} else if strings.HasPrefix(part, "*") {
			key = "*"
			isWillcard = true
		} else {
			key = part
		}

		if _, exists := current.children[key]; !exists {
			current.children[key] = &node{
				children:   make(map[string]*node),
				isParam:    isParam,
				isWillcard: isWillcard,
			}
		}

		current = current.children[key]
	}

	current.handler = handler
}

func (r *Router) searchDynamicRoute(root *node, path string) http.HandlerFunc {
	parts := strings.Split(path, "/")
	current := root

	for _, part := range parts {
		if child, exists := current.children[part]; exists {
			current = child
		} else if paramChild, exists := current.children[":param"]; exists {
			current = paramChild
		} else if wildcardChild, exists := current.children["*"]; exists {
			return wildcardChild.handler
		} else {
			return nil
		}
	}

	return current.handler
}

func (r *Router) serveWithMiddleware(handler http.HandlerFunc, w http.ResponseWriter, req *http.Request) {
	if len(r.middleware) == 0 {
		handler(w, req)
		return
	}

	for i := len(r.middleware) - 1; i >= 0; i-- {
		handler = r.middleware[i](handler)
	}

	handler(w, req)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.Trim(req.URL.Path, "/")
	method := req.Method
	fullPath := method + " " + path

	if handler, exists := r.staticRoutes[fullPath]; exists {
		r.serveWithMiddleware(handler, w, req)
		return
	}

	if root, exists := r.dynamicRoutes[method]; exists {
		if handler := r.searchDynamicRoute(root, path); handler != nil {
			r.serveWithMiddleware(handler, w, req)
			return
		}
	}

	r.notFound(w, req)
}
