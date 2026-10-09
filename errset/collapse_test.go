package errset

import (
	"strings"
	"testing"
)

const testModule = "example.com/app"

func TestCollapseForeignUnexported(t *testing.T) {
	in := []Variant{
		{Key: "example.com/app/domain.ErrNotFound", Name: "domain.ErrNotFound", Kind: KindSentinel},
		{Key: "example.com/app/internal/x.errLocal", Name: "x.errLocal", Kind: KindType},
		{Key: "github.com/jackc/pgx/v5/pgconn.connLockError", Name: "pgconn.connLockError", Kind: KindType, Chain: []string{"Conn.Exec"}, Status: 409},
		{Key: "github.com/jackc/pgx/v5.ErrNoRows", Name: "pgx.ErrNoRows", Kind: KindSentinel},
		{Key: "database/sql.ErrNoRows", Name: "sql.ErrNoRows", Kind: KindSentinel},
		{Key: "iface:github.com/x/y.reader", Name: "some y.reader", Kind: KindIface, Methods: []string{"Read()()"}},
		{Key: "new:github.com/jackc/pgx/v5/internal/pgio.write", Name: "new@pgio.write", Kind: KindFresh, Pkg: "github.com/jackc/pgx/v5/internal/pgio", Ident: "write"},
		{Key: "new:example.com/app.Serve", Name: "new@app.Serve", Kind: KindFresh, Pkg: "example.com/app", Ident: "Serve"},
		// an exported creator in a foreign package: the error is still unmatchable
		{Key: "new:github.com/jackc/pgx/v5/pgconn.Pipeline.Flush", Name: "new@pgconn.Pipeline.Flush", Kind: KindFresh, Pkg: "github.com/jackc/pgx/v5/pgconn", Ident: "Flush"},
		// a prefix of the module path is not the module
		{Key: "example.com/application/x.errY", Name: "x.errY", Kind: KindSentinel},
	}
	out := collapseForeign(in, testModule)
	want := []struct {
		name, orig string
		kind       string
	}{
		{"domain.ErrNotFound", "", KindSentinel},
		{"x.errLocal", "", KindType},
		{"?", "pgconn.connLockError", KindOpaque},
		{"pgx.ErrNoRows", "", KindSentinel},
		{"sql.ErrNoRows", "", KindSentinel},
		{"?", "some y.reader", KindOpaque},
		{"?", "new@pgio.write", KindOpaque},
		{"new@app.Serve", "", KindFresh},
		{"?", "new@pgconn.Pipeline.Flush", KindOpaque},
		{"?", "x.errY", KindOpaque},
	}
	if len(out) != len(want) {
		t.Fatalf("got %d variants, want %d: %+v", len(out), len(want), out)
	}
	for i, w := range want {
		if out[i].Name != w.name || out[i].Orig != w.orig || out[i].Kind != w.kind {
			t.Errorf("variant %d: got name %q orig %q kind %q, want %q %q %q", i, out[i].Name, out[i].Orig, out[i].Kind, w.name, w.orig, w.kind)
		}
	}
	if got := out[2]; got.witnessName() != "? (pgconn.connLockError)" || len(got.Chain) != 1 || got.Chain[0] != "Conn.Exec" {
		t.Errorf("collapsed variant: witness %q chain %v", got.witnessName(), got.Chain)
	}
	if s, ok := ownStatus(out[2]); out[2].Status != 409 || s != 409 || !ok {
		t.Errorf("a collapsed type keeps its status: got %d, ownStatus %d %v", out[2].Status, s, ok)
	}
	if s, ok := ownStatus(Variant{Kind: KindOpaque}); s != 500 || !ok {
		t.Errorf("plain opaque is 500, got %d %v", s, ok)
	}
	if s, _ := ownStatus(Variant{Kind: KindFresh}); s != 500 {
		t.Errorf("fresh is 500, got %d", s)
	}
}

func TestCollapseWithoutModuleIsIdentity(t *testing.T) {
	in := []Variant{{Key: "github.com/jackc/pgx/v5/pgconn.connLockError", Name: "pgconn.connLockError", Kind: KindType}}
	if out := collapseForeign(in, ""); len(out) != 1 || out[0].Name != "pgconn.connLockError" {
		t.Errorf("no module: got %+v", out)
	}
}

func TestVariantNamesShowOpaqueOnce(t *testing.T) {
	vs := collapseForeign([]Variant{
		{Key: "a.Y", Name: "a.Y", Kind: KindType},
		{Key: "github.com/x/z.one", Name: "z.one", Kind: KindType},
		{Key: "github.com/x/z.two", Name: "z.two", Kind: KindType},
	}, testModule)
	if got := variantNames(vs); got != "a.Y | ?" {
		t.Errorf("variantNames = %q, want %q (each name once, ? last)", got, "a.Y | ?")
	}
}

func TestContractNewAndOpaqueDoNotAllowEachOther(t *testing.T) {
	fresh := Variant{Key: "new:p.f", Name: "new@p.f", Kind: KindFresh}
	opaque := opaqueVariant()
	collapsed := Variant{Key: "? x", Name: "?", Kind: KindOpaque, Orig: "x.y"}
	newC := Variant{Key: freshContract, Name: freshContract, Kind: KindFresh}
	opaqueC := opaqueVariant()

	cases := []struct {
		name    string
		v       Variant
		allowed []Variant
		want    bool
	}{
		{"new allows fresh", fresh, []Variant{newC}, true},
		{"new does not allow opaque", opaque, []Variant{newC}, false},
		{"new does not allow collapsed foreign", collapsed, []Variant{newC}, false},
		{"? allows opaque", opaque, []Variant{opaqueC}, true},
		{"? allows collapsed foreign", collapsed, []Variant{opaqueC}, true},
		{"? does not allow fresh", fresh, []Variant{opaqueC}, false},
		{"new and ? together allow both", fresh, []Variant{newC, opaqueC}, true},
		{"no contract allows nothing", fresh, nil, false},
	}
	for _, c := range cases {
		if got := allowedBy(c.v, c.allowed); got != c.want {
			t.Errorf("%s: allowedBy = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFreshNeverCovered(t *testing.T) {
	m := &ErrmapFact{Ifaces: []Iface{{Name: "empty", Methods: nil}}}
	if covered(Variant{Key: "new:p.f", Name: "new@p.f", Kind: KindFresh}, m) {
		t.Error("a fresh variant must not be covered, even by an interface with no methods")
	}
}

// TestChainsStayOnOneLine: a multi-line callee label (a SQL string in a
// dynamic call) is rendered on one line, and a long one is cut.
func TestChainsStayOnOneLine(t *testing.T) {
	got := chainText([]string{"tx.QueryRow(ctx, `\n\tSELECT id\n\tFROM t`).Scan", "Store.get"})
	if got != "tx.QueryRow(ctx, ` SELECT id FROM t`).Scan → Store.get" {
		t.Errorf("chainText = %q", got)
	}
	long := chainText([]string{strings.Repeat("x", 200)})
	if len([]rune(long)) != 80 || !strings.HasSuffix(long, "...") {
		t.Errorf("long element: %d runes, %q", len([]rune(long)), long)
	}
	if chainText(nil) != "(direct)" {
		t.Errorf("empty chain: %q", chainText(nil))
	}
	if strings.Contains(via([]string{"a\nb"}), "\n") {
		t.Errorf("via keeps a newline")
	}
}
