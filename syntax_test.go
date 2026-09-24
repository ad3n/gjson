package gjson_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ad3n/gjson"
)

type syntaxBlock struct {
	section  string
	language string
	body     string
	line     int
}

func syntaxBlocks(t *testing.T) []syntaxBlock {
	t.Helper()
	data, err := os.ReadFile("SYNTAX.md")

	if err != nil {
		t.Fatal(err)
	}

	var blocks []syntaxBlock
	var section string
	var block syntaxBlock
	var inside bool

	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "```") {
			if inside {
				blocks = append(blocks, block)
				inside = false
				continue
			}

			block = syntaxBlock{section: section, language: strings.TrimPrefix(line, "```"), line: i + 2}
			inside = true
			continue
		}

		if inside {
			block.body += line + "\n"
			continue
		}

		if strings.HasPrefix(line, "#") {
			section = strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}

	if inside {
		t.Fatal("unterminated code block in SYNTAX.md")
	}

	return blocks
}

func syntaxFixture(t *testing.T, blocks []syntaxBlock, section string) string {
	t.Helper()

	for _, block := range blocks {
		if block.section == section && block.language == "json" {
			return strings.TrimSpace(block.body)
		}
	}

	t.Fatalf("missing JSON fixture for %s", section)
	return ""
}

func checkSyntaxResult(t *testing.T, doc, path, want string) {
	t.Helper()
	readers := map[string]func() gjson.Result{
		"Get": func() gjson.Result { return gjson.Get(doc, path) },
		"GetBytes": func() gjson.Result {
			data := []byte(doc)
			result := gjson.GetBytes(data, path)
			clear(data)
			return result
		},
		"Result.Get": func() gjson.Result { return gjson.Parse(doc).Get(path) },
	}

	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			got := read()

			if want == "<non-existent>" {
				if got.Exists() {
					t.Fatalf("%s: got %#v, want missing result", path, got)
				}

				return
			}

			var expected bytes.Buffer

			if err := json.Compact(&expected, []byte(want)); err != nil {
				t.Fatalf("invalid expected JSON %q: %v", want, err)
			}

			var actual bytes.Buffer

			if err := json.Compact(&actual, []byte(got.Raw)); err != nil {
				t.Fatalf("%s returned invalid JSON %q: %v", path, got.Raw, err)
			}

			if !got.Exists() || actual.String() != expected.String() {
				t.Fatalf("%s: got %s, want %s", path, actual.String(), expected.String())
			}

			var value any

			if err := json.Unmarshal([]byte(want), &value); err != nil {
				t.Fatal(err)
			}

			switch value := value.(type) {
			case string:
				if got.Type != gjson.String || got.Str != value {
					t.Fatalf("string result differs: %#v", got)
				}
			case float64:
				if got.Type != gjson.Number || got.Num != value {
					t.Fatalf("numeric result differs: %#v", got)
				}
			case bool:
				if !got.IsBool() || got.Bool() != value {
					t.Fatalf("boolean result differs: %#v", got)
				}
			case nil:
				if got.Type != gjson.Null {
					t.Fatalf("null result differs: %#v", got)
				}
			default:
				if got.Type != gjson.JSON {
					t.Fatalf("container result differs: %#v", got)
				}
			}
		})
	}
}

func TestSyntaxDocumentExamples(t *testing.T) {
	blocks := syntaxBlocks(t)
	doc := syntaxFixture(t, blocks, "Example")
	queryDoc := syntaxFixture(t, blocks, "Queries")
	row := regexp.MustCompile(`^(\S+)\s{2,}(?:>>\s*)?(.+)$`)
	counts := make(map[string]int)
	gjson.AddModifier("case", func(doc, arg string) string {
		switch arg {
		case "upper":
			return strings.ToUpper(doc)
		case "lower":
			return strings.ToLower(doc)
		default:
			return doc
		}
	})

	for _, block := range blocks {
		if block.language != "go" && block.language != "" {
			continue
		}

		for i, line := range strings.Split(block.body, "\n") {
			match := row.FindStringSubmatch(strings.TrimSpace(line))

			if match == nil {
				continue
			}

			path, want := match[1], match[2]

			if want != "<non-existent>" && !json.Valid([]byte(want)) {
				continue
			}

			if strings.HasPrefix(path, `"`) {
				var err error
				path, err = strconv.Unquote(path)

				if err != nil {
					t.Fatal(err)
				}
			}

			input := doc

			if strings.HasPrefix(path, "vals.") {
				input = queryDoc
			}

			counts[block.section]++
			t.Run(fmt.Sprintf("%s/line-%d", block.section, block.line+i), func(t *testing.T) {
				checkSyntaxResult(t, input, path, want)
			})
		}
	}

	for section, minimum := range map[string]int{
		"Basic": 8, "Wildcards": 2, "Escape character": 1, "Arrays": 2,
		"Queries": 13, "Dot vs Pipe": 13, "Modifiers": 2, "Custom modifiers": 2,
	} {
		if counts[section] < minimum {
			t.Errorf("%s: parsed %d examples, want at least %d", section, counts[section], minimum)
		}
	}

	for _, section := range []string{"Modifier arguments", "Multipaths", "Literals"} {
		var path string

		for _, block := range blocks {
			if block.section == section && block.language == "" {
				path = strings.TrimSpace(block.body)
				break
			}
		}

		if path == "" {
			t.Fatalf("missing path example for %s", section)
		}

		t.Run(section, func(t *testing.T) {
			checkSyntaxResult(t, doc, path, syntaxFixture(t, blocks, section))
		})
	}

	t.Logf("document example counts: %v, plus 3 block examples", counts)
}

func TestSyntaxOperatorsAndBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, doc, path, want string }{
		{"equal", `[1,2,3]`, `#(==2)#`, `[2]`},
		{"equal-alias", `[1,2,3]`, `#(=2)#`, `[2]`},
		{"not-equal", `[1,2,3]`, `#(!=2)#`, `[1,3]`},
		{"less", `[1,2,3]`, `#(<2)#`, `[1]`},
		{"less-equal", `[1,2,3]`, `#(<=2)#`, `[1,2]`},
		{"greater", `[1,2,3]`, `#(>2)#`, `[3]`},
		{"greater-equal", `[1,2,3]`, `#(>=2)#`, `[2,3]`},
		{"legacy-first", `[1,2,3]`, `#[>=2]`, `2`},
		{"legacy-all", `[1,2,3]`, `#[>=2]#`, `[2,3]`},
		{"no-first-match", `[1,2,3]`, `#(>3)`, `<non-existent>`},
		{"no-all-matches", `[1,2,3]`, `#(>3)#`, `[]`},
		{"empty-count", `[]`, `#`, `0`},
		{"missing-index", `[1,2,3]`, `3`, `<non-existent>`},
		{"projection-missing", `[{"x":1},{},{"x":null}]`, `#.x`, `[1,null]`},
		{"wildcard-empty", `{"ab":1}`, `a*b`, `1`},
		{"wildcard-one", `{"acb":1}`, `a?b`, `1`},
		{"wildcard-one-missing", `{"ab":1}`, `a?b`, `<non-existent>`},
		{"escaped-star", `{"a*b":1}`, `a\*b`, `1`},
		{"escaped-question", `{"a?b":1}`, `a\?b`, `1`},
		{"escaped-pipe", `{"a|b":1}`, `a\|b`, `1`},
		{"escaped-slash", `{"a\\b":1}`, `a\\b`, `1`},
		{"escaped-hash", `{"#":1}`, `\#`, `1`},
		{"escaped-at", `{"@this":1}`, `\@this`, `1`},
		{"escaped-bang", `{"!true":1}`, `\!true`, `1`},
		{"multipath-array", `{"a":1,"b":2}`, `[a,b]`, `[1,2]`},
		{"multipath-missing", `{"a":1}`, `[missing,a]`, `[1]`},
		{"multipath-default-name", `{}`, `{!true}`, `{"_":true}`},
		{"literal-string", `{}`, `!"hello"`, `"hello"`},
		{"literal-number", `{}`, `!42`, `42`},
		{"literal-true", `{}`, `!true`, `true`},
		{"literal-false", `{}`, `!false`, `false`},
		{"literal-null", `{}`, `!null`, `null`},
		{"literal-object", `{}`, `!{"x":1}.x`, `1`},
		{"literal-array", `{}`, `![1,2]|1`, `2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkSyntaxResult(t, tc.doc, tc.path, tc.want)
		})
	}
}

func TestSyntaxBuiltInModifiers(t *testing.T) {
	cases := []struct{ name, doc, path, want string }{
		{"reverse", `[1,2,3]`, `@reverse`, `[3,2,1]`},
		{"reverse-object", `{"a":1,"b":2}`, `@reverse`, `{"b":2,"a":1}`},
		{"ugly", "{\n  \"x\": 1\n}", `@ugly`, `{"x":1}`},
		{"pretty", `{"x":1}`, `@pretty`, `{"x":1}`},
		{"this", `{"x":1}`, `@this`, `{"x":1}`},
		{"valid", `{"x":1}`, `@valid`, `{"x":1}`},
		{"valid-reject", `{"x":`, `@valid`, `<non-existent>`},
		{"flatten", `[1,[2,[3]]]`, `@flatten`, `[1,2,[3]]`},
		{"flatten-deep", `[1,[2,[3]]]`, `@flatten:{"deep":true}`, `[1,2,3]`},
		{"join", `[{"a":1,"b":2},{"a":3}]`, `@join`, `{"a":3,"b":2}`},
		{"join-preserve", `[{"a":1},{"a":2}]`, `@join:{"preserve":true}`, `{"a":1,"a":2}`},
		{"keys", `{"a":1,"b":2}`, `@keys`, `["a","b"]`},
		{"values", `{"a":1,"b":2}`, `@values`, `[1,2]`},
		{"tostr", `{"x":1}`, `@tostr`, `"{\"x\":1}"`},
		{"fromstr", `"{\"x\":1}"`, `@fromstr`, `{"x":1}`},
		{"roundtrip", `{"x":1}`, `@tostr|@fromstr|x`, `1`},
		{"group", `{"id":[1,2],"name":["a"]}`, `@group`, `[{"id":1,"name":"a"},{"id":2}]`},
		{"dig", `{"a":{"x":1},"b":[{"x":2}]}`, `@dig:x`, `[1,2]`},
	}
	covered := make(map[string]bool)

	for _, tc := range cases {
		covered[tc.name] = true
		t.Run(tc.name, func(t *testing.T) {
			checkSyntaxResult(t, tc.doc, tc.path, tc.want)
		})
	}

	doc, err := os.ReadFile("SYNTAX.md")

	if err != nil {
		t.Fatal(err)
	}

	for _, match := range regexp.MustCompile("(?m)^- `@([a-z]+)`:").FindAllStringSubmatch(string(doc), -1) {
		if !covered[match[1]] {
			t.Errorf("documented modifier @%s has no explicit test", match[1])
		}

		if !gjson.ModifierExists(match[1], nil) {
			t.Errorf("documented modifier @%s is not registered", match[1])
		}
	}
}

func TestSyntaxPrettyOptions(t *testing.T) {
	for _, tc := range []struct{ name, path, want string }{
		{"default", `@pretty`, "{\n  \"b\": [1, 2],\n  \"a\": 3\n}"},
		{"sortKeys", `@pretty:{"sortKeys":true}`, "{\n  \"a\": 3,\n  \"b\": [1, 2]\n}"},
		{"indent", `@pretty:{"indent":"\t"}`, "{\n\t\"b\": [1, 2],\n\t\"a\": 3\n}"},
		{"prefix", `@pretty:{"prefix":"\t"}`, "{\n\t  \"b\": [1, 2],\n\t  \"a\": 3\n\t}"},
		{"width", `@pretty:{"width":0}`, "{\n  \"b\": [\n    1,\n    2\n  ],\n  \"a\": 3\n}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := gjson.Get(`{"b":[1,2],"a":3}`, tc.path)

			if got.Raw != tc.want+"\n" {
				t.Fatalf("got %q, want %q", got.Raw, tc.want+"\n")
			}
		})
	}
}

func TestSyntaxCustomModifierArguments(t *testing.T) {
	gjson.AddModifier("syntax_arg", func(_, arg string) string { return arg })

	for _, tc := range []struct{ path, want string }{
		{`@syntax_arg:{"x":[1,2]}|x.1`, `2`},
		{`@syntax_arg:[1,2]|@reverse`, `[2,1]`},
		{`@syntax_arg:"a|b.c"`, `"a|b.c"`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			checkSyntaxResult(t, `{}`, tc.path, tc.want)
		})
	}
}
