// Package plain uses gorilla/schema style tags (schema:"name,required") on types
// that never reach rt. None of them is a finding: the analyzer reads schema tags
// only on the types of rt routes and decodes.
package plain

// Form is decoded by another library, which reads the schema tag key.
type Form struct {
	Name   string   `schema:"name"`
	Age    int      `schema:"age,required"`
	Skip   string   `schema:"-"`
	Tags   []string `schema:"tags,omitempty"`
	Bogus  string   `schema:"bogus,required"`
	Nested Inner    `schema:"nested"`
}

// Inner is a nested gorilla form.
type Inner struct {
	Note *string `schema:"note,default=x"`
}

// Use reads a Form in the way a gorilla-style decoder would, without rt.
func Use(f Form) string {
	return f.Name + f.Nested.Note0()
}

// Note0 returns the note, or empty.
func (i Inner) Note0() string {
	if i.Note == nil {
		return ""
	}
	return *i.Note
}
