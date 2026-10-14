// Package good uses rt in every shape the analyzer must accept: it reports
// nothing here.
package good

import (
	"context"
	"encoding/json"
	"regexp"
	"time"

	"github.com/faintghost/effectgo/rt/schema"
	"github.com/faintghost/effectgo/rt/web"
)

func init() {
	schema.RegisterPattern("slug", regexp.MustCompile(`^[a-z-]+$`))
}

// Segment is a sealed string enum with a pure Valid.
//
//effectgo:sealed
type Segment string // want Segment:"sealed enum"

const (
	SegA Segment = "a"
	SegB Segment = "b"
)

// Valid reports whether s is one of the constants.
func (s Segment) Valid() bool { return s == SegA || s == SegB }

// Level is a sealed int enum whose Valid has a pointer receiver: still accepted.
//
//effectgo:sealed
type Level int // want Level:"sealed enum"

const (
	LowLevel Level = iota
	HighLevel
)

// Valid reports whether l is one of the constants.
func (l *Level) Valid() bool { return *l >= LowLevel && *l <= HighLevel }

// Address is a nested struct with its own tags.
type Address struct {
	City string `json:"city" schema:"trim,nonempty,max=80"`
}

// Contact covers the field types and rules that schema supports.
type Contact struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"     schema:"trim,nonempty,max=200,singleline"`
	Email    string            `json:"email"    schema:"email"`
	Segment  Segment           `json:"segment"`
	Level    *Level            `json:"level"`
	Notes    *string           `json:"notes"    schema:"max=2000"`
	Tags     []string          `json:"tags"     schema:"nonempty,max=10"`
	Attrs    map[string]string `json:"attrs"`
	Addr     Address           `json:"addr"`
	Due      time.Time         `json:"due"`
	Raw      json.RawMessage   `json:"raw"`
	Count    uint32            `json:"count"    schema:"min=1,max=99"`
	Ratio    float64           `json:"ratio"    schema:"min=0,max=1.5"`
	Visible  bool              `json:"visible"`
	Extra    any               `json:"extra"`
	Skipped  chan int          `json:"-"`
	hidden   int
	Nickname *string    `json:"nickname" schema:"pattern=slug"`
	Created  *time.Time `json:"created"`
}

// ContactPatch is the input of a PATCH: a path field and a body.
type ContactPatch struct {
	ID   int64   `path:"id"`
	Name *string `json:"name" schema:"trim,max=200"`
}

// ListIn is a GET input with a query field and a slice query field.
type ListIn struct {
	Page  int       `query:"page"  schema:"min=1"`
	Tags  []string  `query:"tag"`
	Since time.Time `query:"since"`
}

// SlugIn reads a wildcard that the pattern declares.
type SlugIn struct {
	Slug string `path:"slug" schema:"pattern=slug"`
}

// DeleteIn is a DELETE input: a path field only, no body.
type DeleteIn struct {
	ID int64 `path:"id"`
}

// Done is an output with no body.
type Done = web.NoContent

type getIn struct {
	ID int64 `path:"id"`
}

func getContact(ctx context.Context, in getIn) (Contact, error) { return Contact{}, nil }

func createContact(ctx context.Context, in Contact) (Contact, error) { return in, nil }

func patchContact(ctx context.Context, in ContactPatch) (Contact, error) { return Contact{}, nil }

func listContacts(ctx context.Context, in ListIn) ([]Contact, error) { return nil, nil }

func noInput(ctx context.Context, in web.NoInput) ([]Contact, error) { return nil, nil }

func slugged(ctx context.Context, in SlugIn) (Done, error) { return Done{}, nil }

func deleteContact(ctx context.Context, in DeleteIn) (Done, error) { return Done{}, nil }

// Routes registers every route of the package.
func Routes(r *web.Router) {
	web.Get(r, "/api/contacts/{id}", getContact)
	web.Post(r, "/api/contacts", createContact)
	web.Patch(r, "/api/contacts/{id}", patchContact)
	web.Get(r, "/api/contacts", listContacts)
	web.Get(r, "/api/list", noInput)
	web.Get(r, "/api/slug/{slug}", slugged)
	web.Delete(r, "/api/contacts/{id}", deleteContact)
	_, _ = schema.Decode[Contact](nil)
	_ = schema.Compile[Contact]()
}
