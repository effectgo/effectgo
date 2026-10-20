package schema

import "testing"

// ContactEdit is shaped like a novastar contact edit: trimmed and bounded
// strings, an email, an enum, optional fields and a flag.
type ContactEdit struct {
	ID         int64   `json:"id" schema:"min=1"`
	Name       string  `json:"name" schema:"trim,nonempty,max=200,singleline"`
	Email      string  `json:"email" schema:"trim,email,max=254"`
	Segment    Segment `json:"segment"`
	Notes      *string `json:"notes" schema:"max=2000"`
	Phone      *string `json:"phone" schema:"trim,max=32"`
	Suppressed bool    `json:"suppressed"`
	Score      int     `json:"score" schema:"min=0,max=100"`
}

var contactEditBody = []byte(`{"id":42,"name":"  Ada Lovelace  ","email":"ada@example.com","segment":"hot","notes":"met at the conference, follow up in Q4","phone":" +44 20 7946 0000 ","suppressed":false,"score":87}`)

var contactEditInvalid = []byte(`{"id":0,"name":"   ","email":"not an email","segment":"cold","notes":"x","extra":true,"suppressed":"no","score":101}`)

func init() {
	if err := Compile[ContactEdit](); err != nil {
		panic(err)
	}
}

func BenchmarkDecodeContactEdit(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Decode[ContactEdit](contactEditBody); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeContactEditInvalid(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Decode[ContactEdit](contactEditInvalid); err == nil {
			b.Fatal("expected issues")
		}
	}
}
