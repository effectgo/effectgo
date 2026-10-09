// Package gin is a fake of the gin framework, with the signatures the adapter
// reads. The real rules are in stubs/gin.stubs of the repository.
package gin

type H map[string]any

type HandlerFunc func(*Context)

type Context struct{}

func (c *Context) JSON(code int, obj any)                   {}
func (c *Context) AbortWithStatusJSON(code int, obj any)    {}
func (c *Context) String(code int, format string, v ...any) {}
func (c *Context) AbortWithStatus(code int)                 {}
func (c *Context) Status(code int)                          {}
func (c *Context) Query(key string) string                  { return "" }

type RouterGroup struct{}

func (g *RouterGroup) GET(path string, h ...HandlerFunc) *RouterGroup    { return g }
func (g *RouterGroup) POST(path string, h ...HandlerFunc) *RouterGroup   { return g }
func (g *RouterGroup) PUT(path string, h ...HandlerFunc) *RouterGroup    { return g }
func (g *RouterGroup) PATCH(path string, h ...HandlerFunc) *RouterGroup  { return g }
func (g *RouterGroup) DELETE(path string, h ...HandlerFunc) *RouterGroup { return g }
func (g *RouterGroup) Group(path string, h ...HandlerFunc) *RouterGroup  { return g }

type Engine struct {
	RouterGroup
}

func Default() *Engine { return &Engine{} }
