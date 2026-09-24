package compatibility

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"strings"
	"testing"

	old "baseline.example/gjson"
	current "github.com/ad3n/gjson"
)

func equalResult(t testing.TB, got current.Result, want old.Result) {
	t.Helper()

	if int(got.Type) != int(want.Type) || got.Raw != want.Raw || got.Str != want.Str ||
		math.Float64bits(got.Num) != math.Float64bits(want.Num) || got.Index != want.Index ||
		!reflect.DeepEqual(got.Indexes, want.Indexes) {
		t.Fatalf("result differs:\n got: %#v\nwant: %#v", got, want)
	}

	if got.Exists() != want.Exists() || got.Bool() != want.Bool() || got.Int() != want.Int() ||
		got.Uint() != want.Uint() || math.Float64bits(got.Float()) != math.Float64bits(want.Float()) ||
		got.String() != want.String() || got.IsArray() != want.IsArray() ||
		got.IsObject() != want.IsObject() || got.IsBool() != want.IsBool() {
		t.Fatalf("conversion differs: %#v / %#v", got, want)
	}
}

func compare(t testing.TB, doc, path string) {
	t.Helper()
	equalResult(t, current.Get(doc, path), old.Get(doc, path))
	equalResult(t, current.Parse(doc), old.Parse(doc))
	data := []byte(doc)
	got := current.GetBytes(data, path)
	want := old.GetBytes(data, path)
	clear(data)
	equalResult(t, got, want)

	if current.Valid(doc) != old.Valid(doc) {
		t.Fatal("validation differs")
	}
}

func compareCollections(t testing.TB, doc string) {
	t.Helper()

	if !old.Valid(doc) {
		return
	}

	got, want := current.Parse(doc), old.Parse(doc)

	if !reflect.DeepEqual(got.Value(), want.Value()) {
		t.Fatal("Value differs")
	}

	ga, wa := got.Array(), want.Array()

	if len(ga) != len(wa) || (ga == nil) != (wa == nil) {
		t.Fatal("Array differs")
	}

	for i := range ga {
		equalResult(t, ga[i], wa[i])
	}

	gm, wm := got.Map(), want.Map()

	if len(gm) != len(wm) || (gm == nil) != (wm == nil) {
		t.Fatal("Map differs")
	}

	for key, value := range gm {
		expected, ok := wm[key]

		if !ok {
			t.Fatalf("unexpected map key %q", key)
		}

		equalResult(t, value, expected)
	}

	var keys, values []old.Result
	want.ForEach(func(key, value old.Result) bool {
		keys = append(keys, key)
		values = append(values, value)
		return true
	})
	i := 0

	for key, value := range got.All() {
		if i >= len(keys) {
			t.Fatal("extra iteration result")
		}

		equalResult(t, key, keys[i])
		equalResult(t, value, values[i])
		i++
	}

	if i != len(keys) {
		t.Fatal("missing iteration result")
	}
}

func TestCompatibility(t *testing.T) {
	docs := []string{
		``, `[]`, `{}`, `null`, `true`, `37`, `"TrUe"`, `"FALSE"`, `"invalid"`,
		`{"items":[{"id":1},{"id":2},{"skip":0},{"id":null},{"id":"x"}]}`,
		`[{"id":1},{"id":"2"},{"id":2},{"id":null},{"id":3}]`,
		`[{"id":0},{"id":-0},{"id":-1},{"id":NaN},{"id":Inf},{"id":1e999}]`,
		`{"v":"\ud800\u0041","a.b":1,"a":1,"a":2,"nested":{"id":3}}`,
		`{"v":"hello\nworld","items":[{},{}]}`,
		`{"v":"` + strings.Repeat("a", 300) + `\u0041"}`,
		"{\"id\":1}\n{\"id\":2}",
	}
	paths := []string{
		"", "v", "id", "missing", "items", "items.#", "items.#.id", "items.#.id|0",
		"items.#(id>=1)#.id", "items.#(id>=1).id", "items.#.missing", "#.id", "#.missing",
		"#(id>=2)#.id", "#(id>invalid)#.id", "#(id>1e999)#.id", "#(id!=NaN)#.id",
		"#(id=~true)#.id", "#(id=~false)#.id", "#(id=~null)#.id", "#(id=~*)#.id",
		"a\\.b", "a", "a*", "@keys", "@values", "@reverse", "@flatten", "@join", "@dig:id",
		"[v,id,missing]", "{v,id,missing}", "!true", "..#.id", "@pretty", "@ugly", "@valid",
		"@tostr", "@fromstr", "@group", "@this", "v|@tostr", "items|@reverse|0.id",
	}

	for i, doc := range docs {
		compareCollections(t, doc)

		for j, path := range paths {
			t.Run(fmt.Sprintf("%d/%d", i, j), func(t *testing.T) {
				compare(t, doc, path)
			})
		}
	}

	rng := rand.New(rand.NewPCG(1, 2))
	alphabet := []byte("abc012truefalse\\\"[]{}:,.#()|<>!=~ \n\t\x00\xff")

	for range 20000 {
		data := make([]byte, rng.IntN(256))

		for i := range data {
			data[i] = alphabet[rng.IntN(len(alphabet))]
		}

		compare(t, string(data), paths[rng.IntN(len(paths))])
		compare(t, `{"v":"`+string(data)+`"}`, "v")
	}
}

func TestPublicResultLayout(t *testing.T) {
	got, want := reflect.TypeFor[current.Result](), reflect.TypeFor[old.Result]()

	if got.Size() != want.Size() || got.NumField() != want.NumField() {
		t.Fatal("Result layout changed")
	}

	for i := range got.NumField() {
		g, w := got.Field(i), want.Field(i)

		if g.Name != w.Name || g.Offset != w.Offset || g.Type.Kind() != w.Type.Kind() || g.Tag != w.Tag {
			t.Fatalf("Result field %d changed", i)
		}
	}
}

func TestPublicAPI(t *testing.T) {
	signatures := func(path string) map[string]string {
		t.Helper()
		src, err := os.ReadFile(path)

		if err != nil {
			t.Fatal(err)
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)

		if err != nil {
			t.Fatal(err)
		}

		out := make(map[string]string)
		write := func(name string, node any) {
			var b bytes.Buffer

			if err := format.Node(&b, token.NewFileSet(), node); err != nil {
				t.Fatal(err)
			}

			out[name] = strings.ReplaceAll(b.String(), "interface{}", "any")
		}

		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if !decl.Name.IsExported() {
					continue
				}

				decl.Body = nil
				name := decl.Name.Name

				if decl.Recv != nil {
					var receiver bytes.Buffer

					if err := format.Node(&receiver, token.NewFileSet(), decl.Recv.List[0].Type); err != nil {
						t.Fatal(err)
					}

					name = receiver.String() + "." + name
				}

				write(name, decl)
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						if spec.Name.IsExported() {
							write(spec.Name.Name, spec)
						}
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							if name.IsExported() {
								write(name.Name, spec)
							}
						}
					}
				}
			}
		}

		return out
	}

	got, want := signatures("current.txt"), signatures("baseline.txt")

	if !reflect.DeepEqual(got, want) {
		for name, signature := range want {
			if got[name] != signature {
				t.Errorf("API %s changed: got %q, want %q", name, got[name], signature)
			}
		}

		t.Fatalf("public API differs: got %d declarations, want %d", len(got), len(want))
	}

	t.Logf("%d public declarations match", len(got))
}

func FuzzCompatibility(f *testing.F) {
	for _, doc := range []string{
		`{"v":"hello\nworld"}`, `{"v":"\ud800\udfff"}`, `{"v":"\uZZZZ"}`,
		`[{"id":0},{"id":1},{"id":"2"},{"id":2},{"id":null}]`,
		`[{"id":1e999},{"id":NaN},{"id":-Inf}]`,
	} {
		for _, path := range []string{"v", "#.id", "#(id>=2)#.id", "#(id>invalid)#.id", "#(id=~true)#.id"} {
			f.Add(doc, path)
		}
	}

	f.Fuzz(func(t *testing.T, doc, path string) {
		if len(doc) > 2048 || len(path) > 256 {
			t.Skip()
		}

		compare(t, doc, path)
	})
}
