package gjson

import (
	"iter"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/tidwall/match"
	"github.com/tidwall/pretty"
)

type Type int

const (
	Null Type = iota
	False
	Number
	String
	True
	JSON
)

func (t Type) String() string {
	switch t {
	default:
		return ""
	case Null:
		return "Null"
	case False:
		return "False"
	case Number:
		return "Number"
	case String:
		return "String"
	case True:
		return "True"
	case JSON:
		return "JSON"
	}
}

type Result struct {
	Type    Type
	Raw     string
	Str     string
	Num     float64
	Index   int
	Indexes []int
}

func (t Result) String() string {
	switch t.Type {
	default:
		return ""
	case False:
		return "false"
	case Number:
		if len(t.Raw) == 0 {
			return strconv.FormatFloat(t.Num, 'f', -1, 64)
		}

		var i int
		if t.Raw[0] == '-' {
			i++
		}

		for ; i < len(t.Raw); i++ {
			if t.Raw[i] < '0' || t.Raw[i] > '9' {
				return strconv.FormatFloat(t.Num, 'f', -1, 64)
			}
		}

		return t.Raw
	case String:
		return t.Str
	case JSON:
		return t.Raw
	case True:
		return "true"
	}
}

func (t Result) Bool() bool {
	switch t.Type {
	default:
		return false
	case True:
		return true
	case String:
		switch len(t.Str) {
		case 1:
			return t.Str[0] == '1' || t.Str[0]|0x20 == 't'
		case 4:
			return t.Str[0]|0x20 == 't' && t.Str[1]|0x20 == 'r' &&
				t.Str[2]|0x20 == 'u' && t.Str[3]|0x20 == 'e'
		}

		return false
	case Number:
		return t.Num != 0
	}
}

func (t Result) Int() int64 {
	switch t.Type {
	default:
		return 0
	case True:
		return 1
	case String:
		n, ok := parseInt(t.Str)
		if !ok {
			f, err := strconv.ParseFloat(t.Str, 64)
			if err == nil {
				n = f2i(f)
			}
		}

		return n
	case Number:

		i, ok := safeInt(t.Num)
		if ok {
			return i
		}

		i, ok = parseInt(t.Raw)
		if ok {
			return i
		}

		return f2i(t.Num)
	}
}

func f2u(f float64) uint64 {
	if f >= math.MaxUint64 {
		return math.MaxUint64
	}

	if f < 0 {
		return 0
	}

	return uint64(f)
}

func f2i(f float64) int64 {
	if f >= math.MaxInt64 {
		return math.MaxInt64
	}

	if f < math.MinInt64 {
		return math.MinInt64
	}

	return int64(f)
}

func (t Result) Uint() uint64 {
	switch t.Type {
	default:
		return 0
	case True:
		return 1
	case String:
		n, ok := parseUint(t.Str)
		if !ok {
			f, err := strconv.ParseFloat(t.Str, 64)
			if err == nil {
				n = f2u(f)
			}
		}

		return n
	case Number:

		i, ok := safeInt(t.Num)
		if ok && i >= 0 {
			return uint64(i)
		}

		u, ok := parseUint(t.Raw)
		if ok {
			return u
		}

		return f2u(t.Num)
	}
}

func (t Result) Float() float64 {
	switch t.Type {
	default:
		return 0
	case True:
		return 1
	case String:
		n, _ := strconv.ParseFloat(t.Str, 64)
		return n
	case Number:
		return t.Num
	}
}

func (t Result) Time() time.Time {
	res, _ := time.Parse(time.RFC3339, t.String())
	return res
}

func (t Result) Array() []Result {
	if t.Type == Null {
		return []Result{}
	}

	if !t.IsArray() {
		return []Result{t}
	}

	r := t.arrayOrMap('[', false)
	return r.a
}

func (t Result) IsObject() bool {
	return t.Type == JSON && len(t.Raw) > 0 && t.Raw[0] == '{'
}

func (t Result) IsArray() bool {
	return t.Type == JSON && len(t.Raw) > 0 && t.Raw[0] == '['
}

func (t Result) IsBool() bool {
	return t.Type == True || t.Type == False
}

func (t Result) ForEach(iterator func(key, value Result) bool) {
	if !t.Exists() {
		return
	}

	if t.Type != JSON {
		iterator(Result{}, t)
		return
	}

	json := t.Raw
	var obj bool
	var i int
	var key, value Result
	for ; i < len(json); i++ {
		if json[i] == '{' {
			i++
			key.Type = String
			obj = true
			break
		}

		if json[i] == '[' {
			i++
			key.Type = Number
			key.Num = -1
			break
		}

		if json[i] > ' ' {
			return
		}
	}

	var str string
	var vesc bool
	var ok bool
	var idx int
	for ; i < len(json); i++ {
		if obj {
			if json[i] != '"' {
				continue
			}

			s := i
			i, str, vesc, ok = parseString(json, i+1)
			if !ok {
				return
			}

			if vesc {
				key.Str = unescape(str[1 : len(str)-1])
			} else {
				key.Str = str[1 : len(str)-1]
			}

			key.Raw = str
			key.Index = s + t.Index
		} else {
			key.Num += 1
		}

		for ; i < len(json); i++ {
			if json[i] <= ' ' || json[i] == ',' || json[i] == ':' {
				continue
			}

			break
		}

		s := i
		i, value, ok = parseAny(json, i, true)
		if !ok {
			return
		}

		if t.Indexes != nil {
			if idx < len(t.Indexes) {
				value.Index = t.Indexes[idx]
			}
		} else {
			value.Index = s + t.Index
		}

		if !iterator(key, value) {
			return
		}

		idx++
	}
}

func (t Result) Map() map[string]Result {
	if t.Type != JSON {
		return map[string]Result{}
	}

	r := t.arrayOrMap('{', false)
	return r.o
}

func (t Result) Get(path string) Result {
	r := Get(t.Raw, path)
	if r.Indexes == nil {
		r.Index += t.Index
		return r
	}

	for i := range r.Indexes {
		r.Indexes[i] += t.Index
	}

	return r
}

type arrayOrMapResult struct {
	o  map[string]Result
	oi map[string]any
	a  []Result
	ai []any
	vc byte
}

func (t Result) arrayOrMap(vc byte, valueize bool) (r arrayOrMapResult) {
	var json = t.Raw
	var i int
	var value Result
	var count int
	var key Result
	if vc == 0 {
		for ; i < len(json); i++ {
			if json[i] == '{' || json[i] == '[' {
				r.vc = json[i]
				i++
				break
			}

			if json[i] > ' ' {
				goto end
			}
		}
	} else {
		for ; i < len(json); i++ {
			if json[i] == vc {
				i++
				break
			}

			if json[i] > ' ' {
				goto end
			}
		}

		r.vc = vc
	}

	if r.vc == '{' {
		if valueize {
			r.oi = make(map[string]any)
		} else {
			r.o = make(map[string]Result)
		}
	} else {
		if valueize {
			r.ai = make([]any, 0)
		} else {
			r.a = make([]Result, 0)
		}
	}

	for ; i < len(json); i++ {
		if json[i] <= ' ' {
			continue
		}

		if json[i] == ']' || json[i] == '}' {
			break
		}

		switch json[i] {
		default:
			if !((json[i] >= '0' && json[i] <= '9') || json[i] == '-') {
				continue
			}

			value.Type = Number
			value.Raw, value.Num = tonum(json[i:])
			value.Str = ""
		case '{', '[':
			value.Type = JSON
			value.Raw = squash(json[i:])
			value.Str, value.Num = "", 0
		case 'n':
			value.Type = Null
			value.Raw = tolit(json[i:])
			value.Str, value.Num = "", 0
		case 't':
			value.Type = True
			value.Raw = tolit(json[i:])
			value.Str, value.Num = "", 0
		case 'f':
			value.Type = False
			value.Raw = tolit(json[i:])
			value.Str, value.Num = "", 0
		case '"':
			value.Type = String
			value.Raw, value.Str = tostr(json[i:])
			value.Num = 0
		}

		value.Index = i + t.Index

		i += len(value.Raw) - 1

		if r.vc == '{' {
			if count%2 == 0 {
				key = value
			} else {
				if valueize {
					if _, ok := r.oi[key.Str]; !ok {
						r.oi[key.Str] = value.Value()
					}
				} else {
					if _, ok := r.o[key.Str]; !ok {
						r.o[key.Str] = value
					}
				}
			}

			count++
		} else {
			if valueize {
				r.ai = append(r.ai, value.Value())
			} else {
				r.a = append(r.a, value)
			}
		}
	}

end:
	if t.Indexes == nil {
		return
	}

	if len(t.Indexes) != len(r.a) {
		for i := range r.a {
			r.a[i].Index = 0
		}

		return
	}

	for i := range r.a {
		r.a[i].Index = t.Indexes[i]
	}

	return
}

func Parse(json string) Result {
	var value Result
	i := 0
	for ; i < len(json); i++ {
		if json[i] == '{' || json[i] == '[' {
			value.Type = JSON
			value.Raw = json[i:]
			break
		}

		if json[i] <= ' ' {
			continue
		}

		switch json[i] {
		case '+', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'i', 'I', 'N':
			value.Type = Number
			value.Raw, value.Num = tonum(json[i:])
		case 'n':
			if i+1 < len(json) && json[i+1] != 'u' {
				value.Type = Number
				value.Raw, value.Num = tonum(json[i:])
			} else {
				value.Type = Null
				value.Raw = tolit(json[i:])
			}
		case 't':
			value.Type = True
			value.Raw = tolit(json[i:])
		case 'f':
			value.Type = False
			value.Raw = tolit(json[i:])
		case '"':
			value.Type = String
			value.Raw, value.Str = tostr(json[i:])
		default:
			return Result{}
		}

		break
	}

	if value.Exists() {
		value.Index = i
	}

	return value
}

func ParseBytes(json []byte) Result {
	return Parse(string(json))
}

func squash(json string) string {
	var i, depth int
	if json[0] != '"' {
		i, depth = 1, 1
	}

	for ; i < len(json); i++ {
		if json[i] >= '"' && json[i] <= '}' {
			switch json[i] {
			case '"':
				i++
				s2 := i
				for ; i < len(json); i++ {
					if json[i] > '\\' {
						continue
					}

					if json[i] == '"' {
						if json[i-1] == '\\' {
							n := 0
							for j := i - 2; j > s2-1; j-- {
								if json[j] != '\\' {
									break
								}

								n++
							}

							if n%2 == 0 {
								continue
							}
						}

						break
					}
				}

				if depth == 0 {
					if i >= len(json) {
						return json
					}

					return json[:i+1]
				}
			case '{', '[', '(':
				depth++
			case '}', ']', ')':
				depth--
				if depth == 0 {
					return json[:i+1]
				}
			}
		}
	}

	return json
}

func tonum(json string) (raw string, num float64) {
	for i := 1; i < len(json); i++ {
		if json[i] <= '-' {
			if json[i] <= ' ' || json[i] == ',' {
				raw = json[:i]
				num, _ = strconv.ParseFloat(raw, 64)
				return
			}
		} else if json[i] == ']' || json[i] == '}' {
			raw = json[:i]
			num, _ = strconv.ParseFloat(raw, 64)
			return
		}
	}

	raw = json
	num, _ = strconv.ParseFloat(raw, 64)
	return
}

func tolit(json string) (raw string) {
	for i := 1; i < len(json); i++ {
		if json[i] < 'a' || json[i] > 'z' {
			return json[:i]
		}
	}

	return json
}

func tostr(json string) (raw string, str string) {
	for i := 1; i < len(json); i++ {
		if json[i] > '\\' {
			continue
		}

		if json[i] == '"' {
			return json[:i+1], json[1:i]
		}

		if json[i] == '\\' {
			i++
			for ; i < len(json); i++ {
				if json[i] > '\\' {
					continue
				}

				if json[i] == '"' {
					if json[i-1] == '\\' {
						n := 0
						for j := i - 2; j > 0; j-- {
							if json[j] != '\\' {
								break
							}

							n++
						}

						if n%2 == 0 {
							continue
						}
					}

					return json[:i+1], unescape(json[1:i])
				}
			}

			var ret string
			if i+1 < len(json) {
				ret = json[:i+1]
			} else {
				ret = json[:i]
			}

			return ret, unescape(json[1:i])
		}
	}

	return json, json[1:]
}

func (t Result) Exists() bool {
	return t.Type != Null || len(t.Raw) != 0
}

func (t Result) Value() any {
	if t.Type == String {
		return t.Str
	}

	switch t.Type {
	default:
		return nil
	case False:
		return false
	case Number:
		return t.Num
	case JSON:
		r := t.arrayOrMap(0, true)
		switch r.vc {
		case '{':
			return r.oi
		case '[':
			return r.ai
		}

		return nil
	case True:
		return true
	}
}

func parseString(json string, i int) (int, string, bool, bool) {
	var s = i
	for ; i < len(json); i++ {
		if json[i] > '\\' {
			continue
		}

		if json[i] == '"' {
			return i + 1, json[s-1 : i+1], false, true
		}

		if json[i] == '\\' {
			i++
			for ; i < len(json); i++ {
				if json[i] > '\\' {
					continue
				}

				if json[i] == '"' {
					if json[i-1] == '\\' {
						n := 0
						for j := i - 2; j > 0; j-- {
							if json[j] != '\\' {
								break
							}

							n++
						}

						if n%2 == 0 {
							continue
						}
					}

					return i + 1, json[s-1 : i+1], true, true
				}
			}

			break
		}
	}

	return i, json[s-1:], false, false
}

func parseNumber(json string, i int) (int, string) {
	var s = i
	i++
	for ; i < len(json); i++ {
		if json[i] <= ' ' || json[i] == ',' || json[i] == ']' ||
			json[i] == '}' {
			return i, json[s:i]
		}
	}

	return i, json[s:]
}

func parseLiteral(json string, i int) (int, string) {
	var s = i
	i++
	for ; i < len(json); i++ {
		if json[i] < 'a' || json[i] > 'z' {
			return i, json[s:i]
		}
	}

	return i, json[s:]
}

type arrayPathResult struct {
	query struct {
		num       float64
		numParsed bool
		on        bool
		all       bool
		path      string
		op        string
		value     string
	}
	part    string
	path    string
	pipe    string
	alogkey string
	piped   bool
	more    bool
	alogok  bool
	arrch   bool
}

func parseArrayPath(path string) (r arrayPathResult) {
	for i := 0; i < len(path); i++ {
		if path[i] == '|' {
			r.part = path[:i]
			r.pipe = path[i+1:]
			r.piped = true
			return
		}

		if path[i] == '.' {
			r.part = path[:i]
			if !r.arrch && i < len(path)-1 && isDotPiperChar(path[i+1:]) {
				r.pipe = path[i+1:]
				r.piped = true
				return
			}

			r.path = path[i+1:]
			r.more = true
			return
		}

		if path[i] == '#' {
			r.arrch = true
			if i == 0 && len(path) > 1 {
				if path[1] == '.' {
					r.alogok = true
					r.alogkey = path[2:]
					r.path = path[:1]
				} else if path[1] == '[' || path[1] == '(' {
					r.query.on = true
					qpath, op, value, _, fi, vesc, ok :=
						parseQuery(path[i:])
					if !ok {
						break
					}

					if len(value) >= 2 && value[0] == '"' &&
						value[len(value)-1] == '"' {
						value = value[1 : len(value)-1]
						if vesc {
							value = unescape(value)
						}
					}

					r.query.path = qpath
					r.query.op = op
					r.query.value = value

					i = fi - 1
					if i+1 < len(path) && path[i+1] == '#' {
						r.query.all = true
					}
				}
			}

			continue
		}
	}

	r.part = path
	r.path = ""
	return
}

func parseQuery(query string) (
	path, op, value, remain string, i int, vesc, ok bool,
) {
	if len(query) < 2 || query[0] != '#' ||
		(query[1] != '(' && query[1] != '[') {
		return "", "", "", "", i, false, false
	}

	i = 2
	j := 0
	depth := 1
	for ; i < len(query); i++ {
		if depth == 1 && j == 0 {
			switch query[i] {
			case '!', '=', '<', '>', '%':

				j = i
				continue
			}
		}

		if query[i] == '\\' {
			i++
		} else if query[i] == '[' || query[i] == '(' {
			depth++
		} else if query[i] == ']' || query[i] == ')' {
			depth--
			if depth == 0 {
				break
			}
		} else if query[i] == '"' {
			i++
			for ; i < len(query); i++ {
				if query[i] == '\\' {
					vesc = true
					i++
				} else if query[i] == '"' {
					break
				}
			}
		}
	}

	if depth > 0 {
		return "", "", "", "", i, false, false
	}

	if j > 0 {
		path = trim(query[2:j])
		value = trim(query[j:i])
		remain = query[i+1:]

		var opsz int
		switch {
		case len(value) == 1:
			opsz = 1
		case value[0] == '!' && value[1] == '=':
			opsz = 2
		case value[0] == '!' && value[1] == '%':
			opsz = 2
		case value[0] == '<' && value[1] == '=':
			opsz = 2
		case value[0] == '>' && value[1] == '=':
			opsz = 2
		case value[0] == '=' && value[1] == '=':
			value = value[1:]
			opsz = 1
		case value[0] == '<':
			opsz = 1
		case value[0] == '>':
			opsz = 1
		case value[0] == '=':
			opsz = 1
		case value[0] == '%':
			opsz = 1
		}

		op = value[:opsz]
		value = trim(value[opsz:])
	} else {
		path = trim(query[2:i])
		remain = query[i+1:]
	}

	return path, op, value, remain, i + 1, vesc, true
}

func trim(s string) string {
left:
	if len(s) > 0 && s[0] <= ' ' {
		s = s[1:]
		goto left
	}
right:
	if len(s) > 0 && s[len(s)-1] <= ' ' {
		s = s[:len(s)-1]
		goto right
	}
	return s
}

func isDotPiperChar(s string) bool {
	if DisableModifiers {
		return false
	}

	c := s[0]
	if c == '@' {
		i := 1
		for ; i < len(s); i++ {
			if s[i] == '.' || s[i] == '|' || s[i] == ':' {
				break
			}
		}

		_, ok := modifiers[s[1:i]]
		return ok
	}

	return c == '[' || c == '{'
}

type objectPathResult struct {
	part  string
	path  string
	pipe  string
	piped bool
	wild  bool
	more  bool
}

func parseObjectPath(path string) (r objectPathResult) {
	for i := 0; i < len(path); i++ {
		if path[i] == '|' {
			r.part = path[:i]
			r.pipe = path[i+1:]
			r.piped = true
			return
		}

		if path[i] == '.' {
			r.part = path[:i]
			if i < len(path)-1 && isDotPiperChar(path[i+1:]) {
				r.pipe = path[i+1:]
				r.piped = true
				return
			}

			r.path = path[i+1:]
			r.more = true
			return
		}

		if path[i] == '*' || path[i] == '?' {
			r.wild = true
			continue
		}

		if path[i] == '\\' {
			epart := []byte(path[:i])
			i++
			if i < len(path) {
				epart = append(epart, path[i])
				i++
				for ; i < len(path); i++ {
					if path[i] == '\\' {
						i++
						if i < len(path) {
							epart = append(epart, path[i])
						}

						continue
					}

					if path[i] == '.' {
						r.part = string(epart)
						if i < len(path)-1 && isDotPiperChar(path[i+1:]) {
							r.pipe = path[i+1:]
							r.piped = true
							return
						}

						r.path = path[i+1:]
						r.more = true
						return
					}

					if path[i] == '|' {
						r.part = string(epart)
						r.pipe = path[i+1:]
						r.piped = true
						return
					}

					if path[i] == '*' || path[i] == '?' {
						r.wild = true
					}

					epart = append(epart, path[i])
				}
			}

			r.part = string(epart)
			return
		}
	}

	r.part = path
	return
}

var vchars = [256]byte{
	'"': 2, '{': 3, '(': 3, '[': 3, '}': 1, ')': 1, ']': 1,
}

func parseSquash(json string, i int) (int, string) {
	s := i
	i++
	depth := 1
	var c byte
	for i < len(json) {
		for i < len(json)-8 {
			jslice := json[i : i+8]
			c = vchars[jslice[0]]
			if c != 0 {
				i += 0
				goto token
			}

			c = vchars[jslice[1]]
			if c != 0 {
				i += 1
				goto token
			}

			c = vchars[jslice[2]]
			if c != 0 {
				i += 2
				goto token
			}

			c = vchars[jslice[3]]
			if c != 0 {
				i += 3
				goto token
			}

			c = vchars[jslice[4]]
			if c != 0 {
				i += 4
				goto token
			}

			c = vchars[jslice[5]]
			if c != 0 {
				i += 5
				goto token
			}

			c = vchars[jslice[6]]
			if c != 0 {
				i += 6
				goto token
			}

			c = vchars[jslice[7]]
			if c != 0 {
				i += 7
				goto token
			}

			i += 8
		}

		c = vchars[json[i]]
		if c == 0 {
			i++
			continue
		}

	token:
		if c == 2 {
			i++
			s2 := i
		nextquote:
			for i < len(json)-8 {
				jslice := json[i : i+8]
				if jslice[0] == '"' {
					i += 0
					goto strchkesc
				}

				if jslice[1] == '"' {
					i += 1
					goto strchkesc
				}

				if jslice[2] == '"' {
					i += 2
					goto strchkesc
				}

				if jslice[3] == '"' {
					i += 3
					goto strchkesc
				}

				if jslice[4] == '"' {
					i += 4
					goto strchkesc
				}

				if jslice[5] == '"' {
					i += 5
					goto strchkesc
				}

				if jslice[6] == '"' {
					i += 6
					goto strchkesc
				}

				if jslice[7] == '"' {
					i += 7
					goto strchkesc
				}

				i += 8
			}
			goto strchkstd
		strchkesc:
			if json[i-1] != '\\' {
				i++
				continue
			}
		strchkstd:
			for i < len(json) {
				if json[i] > '\\' || json[i] != '"' {
					i++
					continue
				}

				if json[i-1] == '\\' {
					n := 0
					for j := i - 2; j > s2-1; j-- {
						if json[j] != '\\' {
							break
						}

						n++
					}

					if n%2 == 0 {
						i++
						goto nextquote
					}
				}

				break
			}
		} else {
			depth += int(c) - 2
			if depth == 0 {
				i++
				return i, json[s:i]
			}
		}
		i++
	}

	return i, json[s:]
}

func parseObject(c *parseContext, i int, path string) (int, bool) {
	var pmatch, kesc, vesc, ok, hit bool
	var key, val string
	rp := parseObjectPath(path)
	if !rp.more && rp.piped {
		c.pipe = rp.pipe
		c.piped = true
	}

	for i < len(c.json) {
		for ; i < len(c.json); i++ {
			if c.json[i] == '"' {
				i++
				var s = i
				for ; i < len(c.json); i++ {
					if c.json[i] > '\\' {
						continue
					}

					if c.json[i] == '"' {
						i, key, kesc, ok = i+1, c.json[s:i], false, true
						goto parse_key_string_done
					}

					if c.json[i] == '\\' {
						i++
						for ; i < len(c.json); i++ {
							if c.json[i] > '\\' {
								continue
							}

							if c.json[i] == '"' {
								if c.json[i-1] == '\\' {
									n := 0
									for j := i - 2; j > 0; j-- {
										if c.json[j] != '\\' {
											break
										}

										n++
									}

									if n%2 == 0 {
										continue
									}
								}

								i, key, kesc, ok = i+1, c.json[s:i], true, true
								goto parse_key_string_done
							}
						}

						break
					}
				}

				key, kesc, ok = c.json[s:], false, false
			parse_key_string_done:
				break
			}

			if c.json[i] == '}' {
				return i + 1, false
			}
		}

		if !ok {
			return i, false
		}

		if rp.wild {
			if kesc {
				pmatch = matchLimit(unescape(key), rp.part)
			} else {
				pmatch = matchLimit(key, rp.part)
			}
		} else {
			if kesc {
				pmatch = rp.part == unescape(key)
			} else {
				pmatch = rp.part == key
			}
		}

		hit = pmatch && !rp.more
		for ; i < len(c.json); i++ {
			var num bool
			switch c.json[i] {
			default:
				continue
			case '"':
				i++
				i, val, vesc, ok = parseString(c.json, i)
				if !ok {
					return i, false
				}

				if hit {
					if vesc {
						c.value.Str = unescape(val[1 : len(val)-1])
					} else {
						c.value.Str = val[1 : len(val)-1]
					}

					c.value.Raw = val
					c.value.Type = String
					return i, true
				}
			case '{':
				if pmatch && !hit {
					i, hit = parseObject(c, i+1, rp.path)
					if hit {
						return i, true
					}
				} else {
					i, val = parseSquash(c.json, i)
					if hit {
						c.value.Raw = val
						c.value.Type = JSON
						return i, true
					}
				}
			case '[':
				if pmatch && !hit {
					i, hit = parseArray(c, i+1, rp.path)
					if hit {
						return i, true
					}
				} else {
					i, val = parseSquash(c.json, i)
					if hit {
						c.value.Raw = val
						c.value.Type = JSON
						return i, true
					}
				}
			case 'n':
				if i+1 < len(c.json) && c.json[i+1] != 'u' {
					num = true
					break
				}

				fallthrough
			case 't', 'f':
				vc := c.json[i]
				i, val = parseLiteral(c.json, i)
				if hit {
					c.value.Raw = val
					switch vc {
					case 't':
						c.value.Type = True
					case 'f':
						c.value.Type = False
					}

					return i, true
				}
			case '+', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
				'i', 'I', 'N':
				num = true
			}

			if num {
				i, val = parseNumber(c.json, i)
				if hit {
					c.value.Raw = val
					c.value.Type = Number
					c.value.Num, _ = strconv.ParseFloat(val, 64)
					return i, true
				}
			}

			break
		}
	}

	return i, false
}

func matchLimit(str, pattern string) bool {
	matched, _ := match.MatchLimit(str, pattern, 10000)
	return matched
}

func falseish(t Result) bool {
	switch t.Type {
	case Null:
		return true
	case False:
		return true
	case String:
		b, err := strconv.ParseBool(strings.ToLower(t.Str))
		if err != nil {
			return false
		}

		return !b
	case Number:
		return t.Num == 0
	default:
		return false
	}
}

func trueish(t Result) bool {
	switch t.Type {
	case True:
		return true
	case String:
		b, err := strconv.ParseBool(strings.ToLower(t.Str))
		if err != nil {
			return false
		}

		return b
	case Number:
		return t.Num != 0
	default:
		return false
	}
}

func nullish(t Result) bool {
	return t.Type == Null
}

func queryMatches(rp *arrayPathResult, value Result) bool {
	rpv := rp.query.value
	if len(rpv) > 0 {
		if rpv[0] == '~' {
			rpv = rpv[1:]
			var ish, ok bool
			switch rpv {
			case "*":
				ish, ok = value.Exists(), true
			case "null":
				ish, ok = nullish(value), true
			case "true":
				ish, ok = trueish(value), true
			case "false":
				ish, ok = falseish(value), true
			}

			if ok {
				rpv = "true"
				if ish {
					value = Result{Type: True}
				} else {
					value = Result{Type: False}
				}
			} else {
				rpv = ""
				value = Result{}
			}
		}
	}

	if !value.Exists() {
		return false
	}

	if rp.query.op == "" {
		return true
	}

	switch value.Type {
	case String:
		switch rp.query.op {
		case "=":
			return value.Str == rpv
		case "!=":
			return value.Str != rpv
		case "<":
			return value.Str < rpv
		case "<=":
			return value.Str <= rpv
		case ">":
			return value.Str > rpv
		case ">=":
			return value.Str >= rpv
		case "%":
			return matchLimit(value.Str, rpv)
		case "!%":
			return !matchLimit(value.Str, rpv)
		}
	case Number:
		if !rp.query.numParsed {
			rp.query.num, _ = strconv.ParseFloat(rpv, 64)
			rp.query.numParsed = true
		}

		rpvn := rp.query.num
		switch rp.query.op {
		case "=":
			return value.Num == rpvn
		case "!=":
			return value.Num != rpvn
		case "<":
			return value.Num < rpvn
		case "<=":
			return value.Num <= rpvn
		case ">":
			return value.Num > rpvn
		case ">=":
			return value.Num >= rpvn
		}
	case True:
		switch rp.query.op {
		case "=":
			return rpv == "true"
		case "!=":
			return rpv != "true"
		case ">":
			return rpv == "false"
		case ">=":
			return true
		}
	case False:
		switch rp.query.op {
		case "=":
			return rpv == "false"
		case "!=":
			return rpv != "false"
		case "<":
			return rpv == "true"
		case "<=":
			return true
		}
	}

	return false
}
func parseArray(c *parseContext, i int, path string) (int, bool) {
	var pmatch, vesc, ok, hit bool
	var val string
	var h int
	var alog []int
	var partidx int
	var multires []byte
	var queryIndexes []int
	rp := parseArrayPath(path)
	if !rp.arrch {
		n, ok := parseUint(rp.part)
		if !ok {
			partidx = -1
		} else {
			partidx = int(n)
		}
	}

	if !rp.more && rp.piped {
		c.pipe = rp.pipe
		c.piped = true
	}

	procQuery := func(qval Result) bool {
		if rp.query.all {
			if len(multires) == 0 {
				multires = append(multires, '[')
			}
		}

		var tmp parseContext
		tmp.value = qval
		fillIndex(c.json, &tmp)
		parentIndex := tmp.value.Index
		var res Result
		if qval.Type == JSON {
			res = qval.Get(rp.query.path)
		} else {
			if rp.query.path != "" {
				return false
			}

			res = qval
		}

		if !queryMatches(&rp, res) {
			return false
		}

		if rp.more {
			left, right, ok := splitPossiblePipe(rp.path)
			if ok {
				rp.path = left
				c.pipe = right
				c.piped = true
			}

			res = qval.Get(rp.path)
		} else {
			res = qval
		}

		if !rp.query.all {
			c.value = res
			return true
		}

		raw := res.Raw
		if len(raw) == 0 {
			raw = res.String()
		}

		if raw != "" {
			if len(multires) > 1 {
				multires = append(multires, ',')
			}

			multires = append(multires, raw...)
			queryIndexes = append(queryIndexes, res.Index+parentIndex)
		}

		return false
	}
	for i < len(c.json)+1 {
		if !rp.arrch {
			pmatch = partidx == h
			hit = pmatch && !rp.more
		}

		h++
		if rp.alogok {
			alog = append(alog, i)
		}

		for ; ; i++ {
			var ch byte
			if i > len(c.json) {
				break
			}

			if i == len(c.json) {
				ch = ']'
			} else {
				ch = c.json[i]
			}

			var num bool
			switch ch {
			default:
				continue
			case '"':
				i++
				i, val, vesc, ok = parseString(c.json, i)
				if !ok {
					return i, false
				}

				if rp.query.on {
					var qval Result
					if vesc {
						qval.Str = unescape(val[1 : len(val)-1])
					} else {
						qval.Str = val[1 : len(val)-1]
					}

					qval.Raw = val
					qval.Type = String
					if procQuery(qval) {
						return i, true
					}
				} else if hit {
					if rp.alogok {
						break
					}

					if vesc {
						c.value.Str = unescape(val[1 : len(val)-1])
					} else {
						c.value.Str = val[1 : len(val)-1]
					}

					c.value.Raw = val
					c.value.Type = String
					return i, true
				}
			case '{':
				if pmatch && !hit {
					i, hit = parseObject(c, i+1, rp.path)
					if hit {
						if rp.alogok {
							break
						}

						return i, true
					}
				} else {
					i, val = parseSquash(c.json, i)
					if rp.query.on {
						if procQuery(Result{Raw: val, Type: JSON}) {
							return i, true
						}
					} else if hit {
						if rp.alogok {
							break
						}

						c.value.Raw = val
						c.value.Type = JSON
						return i, true
					}
				}
			case '[':
				if pmatch && !hit {
					i, hit = parseArray(c, i+1, rp.path)
					if hit {
						if rp.alogok {
							break
						}

						return i, true
					}
				} else {
					i, val = parseSquash(c.json, i)
					if rp.query.on {
						if procQuery(Result{Raw: val, Type: JSON}) {
							return i, true
						}
					} else if hit {
						if rp.alogok {
							break
						}

						c.value.Raw = val
						c.value.Type = JSON
						return i, true
					}
				}
			case 'n':
				if i+1 < len(c.json) && c.json[i+1] != 'u' {
					num = true
					break
				}

				fallthrough
			case 't', 'f':
				vc := c.json[i]
				i, val = parseLiteral(c.json, i)
				if rp.query.on {
					var qval Result
					qval.Raw = val
					switch vc {
					case 't':
						qval.Type = True
					case 'f':
						qval.Type = False
					}

					if procQuery(qval) {
						return i, true
					}
				} else if hit {
					if rp.alogok {
						break
					}

					c.value.Raw = val
					switch vc {
					case 't':
						c.value.Type = True
					case 'f':
						c.value.Type = False
					}

					return i, true
				}
			case '+', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
				'i', 'I', 'N':
				num = true
			case ']':
				if rp.arrch && rp.part == "#" {
					if rp.alogok {
						left, right, ok := splitPossiblePipe(rp.alogkey)
						if ok {
							rp.alogkey = left
							c.pipe = right
							c.piped = true
						}

						var indexes = make([]int, 0, min(len(alog), 64))
						var jsons = make([]byte, 0, 64)
						jsons = append(jsons, '[')
						for j, k := 0, 0; j < len(alog); j++ {
							idx := alog[j]
							for idx < len(c.json) {
								switch c.json[idx] {
								case ' ', '\t', '\r', '\n':
									idx++
									continue
								}

								break
							}

							if idx < len(c.json) && c.json[idx] != ']' {
								_, res, ok := parseAny(c.json, idx, true)
								if ok {
									res := res.Get(rp.alogkey)
									if res.Exists() {
										if k > 0 {
											jsons = append(jsons, ',')
										}

										raw := res.Raw
										if len(raw) == 0 {
											raw = res.String()
										}

										jsons = append(jsons, []byte(raw)...)
										indexes = append(indexes, res.Index)
										k++
									}
								}
							}
						}

						jsons = append(jsons, ']')
						c.value.Type = JSON
						c.value.Raw = string(jsons)
						c.value.Indexes = indexes
						return i + 1, true
					}

					if rp.alogok {
						break
					}

					c.value.Type = Number
					c.value.Num = float64(h - 1)
					c.value.Raw = strconv.Itoa(h - 1)
					c.calcd = true
					return i + 1, true
				}

				if !c.value.Exists() {
					if len(multires) > 0 {
						c.value = Result{
							Raw:     string(append(multires, ']')),
							Type:    JSON,
							Indexes: queryIndexes,
						}
					} else if rp.query.all {
						c.value = Result{
							Raw:  "[]",
							Type: JSON,
						}
					}
				}

				return i + 1, false
			}

			if num {
				i, val = parseNumber(c.json, i)
				if rp.query.on {
					var qval Result
					qval.Raw = val
					qval.Type = Number
					qval.Num, _ = strconv.ParseFloat(val, 64)
					if procQuery(qval) {
						return i, true
					}
				} else if hit {
					if rp.alogok {
						break
					}

					c.value.Raw = val
					c.value.Type = Number
					c.value.Num, _ = strconv.ParseFloat(val, 64)
					return i, true
				}
			}

			break
		}
	}

	return i, false
}

func splitPossiblePipe(path string) (left, right string, ok bool) {
	var possible bool
	for i := 0; i < len(path); i++ {
		if path[i] == '|' {
			possible = true
			break
		}
	}

	if !possible {
		return
	}

	if len(path) > 0 && path[0] == '{' {
		squashed := squash(path[1:])
		if len(squashed) < len(path)-1 {
			squashed = path[:len(squashed)+1]
			remain := path[len(squashed):]
			if remain[0] == '|' {
				return squashed, remain[1:], true
			}
		}

		return
	}

	for i := 0; i < len(path); i++ {
		if path[i] == '\\' {
			i++
		} else if path[i] == '.' {
			if i == len(path)-1 {
				return
			}

			if path[i+1] == '#' {
				i += 2
				if i == len(path) {
					return
				}

				if path[i] == '[' || path[i] == '(' {
					var start, end byte
					if path[i] == '[' {
						start, end = '[', ']'
					} else {
						start, end = '(', ')'
					}

					i++
					depth := 1
					for ; i < len(path); i++ {
						if path[i] == '\\' {
							i++
						} else if path[i] == start {
							depth++
						} else if path[i] == end {
							depth--
							if depth == 0 {
								break
							}
						} else if path[i] == '"' {
							i++
							for ; i < len(path); i++ {
								if path[i] == '\\' {
									i++
								} else if path[i] == '"' {
									break
								}
							}
						}
					}
				}
			}
		} else if path[i] == '|' {
			return path[:i], path[i+1:], true
		}
	}

	return
}

func ForEachLine(json string, iterator func(line Result) bool) {
	var res Result
	var i int
	for {
		i, res, _ = parseAny(json, i, true)
		if !res.Exists() {
			break
		}

		if !iterator(res) {
			return
		}
	}
}

type subSelector struct {
	name string
	path string
}

func parseSubSelectors(path string) (sels []subSelector, out string, ok bool) {
	modifier := 0
	depth := 1
	colon := 0
	start := 1
	i := 1
	pushSel := func() {
		var sel subSelector
		if colon == 0 {
			sel.path = path[start:i]
		} else {
			sel.name = path[start:colon]
			sel.path = path[colon+1 : i]
		}

		sels = append(sels, sel)
		colon = 0
		modifier = 0
		start = i + 1
	}
	for ; i < len(path); i++ {
		switch path[i] {
		case '\\':
			i++
		case '@':
			if modifier == 0 && i > 0 && (path[i-1] == '.' || path[i-1] == '|') {
				modifier = i
			}
		case ':':
			if modifier == 0 && colon == 0 && depth == 1 {
				colon = i
			}
		case ',':
			if depth == 1 {
				pushSel()
			}
		case '"':
			i++
		loop:
			for ; i < len(path); i++ {
				switch path[i] {
				case '\\':
					i++
				case '"':
					break loop
				}
			}
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			depth--
			if depth == 0 {
				pushSel()
				path = path[i+1:]
				return sels, path, true
			}
		}
	}

	return
}

func nameOfLast(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '|' || path[i] == '.' {
			if i > 0 {
				if path[i-1] == '\\' {
					continue
				}
			}

			return path[i+1:]
		}
	}

	return path
}

func isSimpleName(component string) bool {
	for i := 0; i < len(component); i++ {
		if component[i] < ' ' {
			return false
		}

		switch component[i] {
		case '[', ']', '{', '}', '(', ')', '#', '|', '!':
			return false
		}
	}

	return true
}

var hexchars = [...]byte{
	'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
	'a', 'b', 'c', 'd', 'e', 'f',
}

func appendHex16(dst []byte, x uint16) []byte {
	return append(dst,
		hexchars[x>>12&0xF], hexchars[x>>8&0xF],
		hexchars[x>>4&0xF], hexchars[x>>0&0xF],
	)
}

var DisableEscapeHTML = false

func AppendJSONString(dst []byte, s string) []byte {
	dst = append(dst, make([]byte, len(s)+2)...)
	dst = append(dst[:len(dst)-len(s)-2], '"')
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' {
			dst = append(dst, '\\')
			switch s[i] {
			case '\b':
				dst = append(dst, 'b')
			case '\f':
				dst = append(dst, 'f')
			case '\n':
				dst = append(dst, 'n')
			case '\r':
				dst = append(dst, 'r')
			case '\t':
				dst = append(dst, 't')
			default:
				dst = append(dst, 'u')
				dst = appendHex16(dst, uint16(s[i]))
			}
		} else if !DisableEscapeHTML &&
			(s[i] == '>' || s[i] == '<' || s[i] == '&') {
			dst = append(dst, '\\', 'u')
			dst = appendHex16(dst, uint16(s[i]))
		} else if s[i] == '\\' {
			dst = append(dst, '\\', '\\')
		} else if s[i] == '"' {
			dst = append(dst, '\\', '"')
		} else if s[i] > 127 {
			r, n := utf8.DecodeRuneInString(s[i:])
			if n == 0 {
				break
			}

			if r == utf8.RuneError && n == 1 {
				dst = append(dst, "\xef\xbf\xbd"...)
			} else if r == '\u2028' || r == '\u2029' {
				dst = append(dst, `\u202`...)
				dst = append(dst, hexchars[r&0xF])
			} else {
				dst = append(dst, s[i:i+n]...)
			}

			i = i + n - 1
		} else {
			dst = append(dst, s[i])
		}
	}

	return append(dst, '"')
}

type parseContext struct {
	json  string
	pipe  string
	value Result
	piped bool
	calcd bool
	lines bool
}

func Get(json, path string) Result {
	if len(path) > 1 {
		if (path[0] == '@' && !DisableModifiers) || path[0] == '!' {
			var ok bool
			var npath string
			var rjson string
			if path[0] == '@' && !DisableModifiers {
				npath, rjson, ok = execModifier(json, path)
			} else if path[0] == '!' {
				npath, rjson, ok = execStatic(json, path)
			}

			if ok {
				path = npath
				if len(path) > 0 && (path[0] == '|' || path[0] == '.') {
					res := Get(rjson, path[1:])
					res.Index = 0
					res.Indexes = nil
					return res
				}

				return Parse(rjson)
			}
		}

		if path[0] == '[' || path[0] == '{' {
			kind := path[0]
			var ok bool
			var subs []subSelector
			subs, path, ok = parseSubSelectors(path)
			if ok {
				if len(path) == 0 || (path[0] == '|' || path[0] == '.') {
					var b []byte
					b = append(b, kind)
					var i int
					for _, sub := range subs {
						res := Get(json, sub.path)
						if res.Exists() {
							if i > 0 {
								b = append(b, ',')
							}

							if kind == '{' {
								if len(sub.name) > 0 {
									if sub.name[0] == '"' && Valid(sub.name) {
										b = append(b, sub.name...)
									} else {
										b = AppendJSONString(b, sub.name)
									}
								} else {
									last := nameOfLast(sub.path)
									if isSimpleName(last) {
										b = AppendJSONString(b, last)
									} else {
										b = AppendJSONString(b, "_")
									}
								}

								b = append(b, ':')
							}

							var raw string
							if len(res.Raw) == 0 {
								raw = res.String()
								if len(raw) == 0 {
									raw = "null"
								}
							} else {
								raw = res.Raw
							}

							b = append(b, raw...)
							i++
						}
					}

					b = append(b, kind+2)
					var res Result
					res.Raw = string(b)
					res.Type = JSON
					if len(path) > 0 {
						res = res.Get(path[1:])
					}

					res.Index = 0
					return res
				}
			}
		}
	}

	var i int
	var c = &parseContext{json: json}
	if len(path) >= 2 && path[0] == '.' && path[1] == '.' {
		c.lines = true
		parseArray(c, 0, path[2:])
	} else {
		for ; i < len(c.json); i++ {
			if c.json[i] == '{' {
				i++
				parseObject(c, i, path)
				break
			}

			if c.json[i] == '[' {
				i++
				parseArray(c, i, path)
				break
			}
		}
	}

	if c.piped {
		res := c.value.Get(c.pipe)
		res.Index = 0
		return res
	}

	fillIndex(json, c)
	return c.value
}

func GetBytes(json []byte, path string) Result {
	return getBytes(json, path)
}

func runeit(json string) rune {
	n, _ := strconv.ParseUint(json[:4], 16, 64)
	return rune(n)
}

func unescape(json string) string {
	if len(json) == 2 && json[0] == '\\' {
		switch json[1] {
		case '\\':
			return "\\"
		case '/':
			return "/"
		case '"':
			return "\""
		case 'b':
			return "\b"
		case 'f':
			return "\f"
		case 'n':
			return "\n"
		case 'r':
			return "\r"
		case 't':
			return "\t"
		default:
			return ""
		}
	}

	if len(json) <= 32 {
		var buf [32]byte
		return string(appendUnescaped(buf[:0], json))
	}

	str := appendUnescaped(make([]byte, 0, len(json)), json)

	if len(str) <= 1 || len(str) < len(json)/2 {
		return string(str)
	}

	return bytesString(str)
}

func appendUnescaped(str []byte, json string) []byte {
	for i := 0; i < len(json); i++ {
		if json[i] != '\\' {
			start := i

			for i < len(json) && json[i] >= ' ' && json[i] != '\\' {
				i++
			}

			str = append(str, json[start:i]...)

			if i == len(json) || json[i] < ' ' {
				return str
			}
		}

		i++

		if i >= len(json) {
			return str
		}

		switch json[i] {
		default:
			return str
		case '\\', '/', '"':
			str = append(str, json[i])
		case 'b':
			str = append(str, '\b')
		case 'f':
			str = append(str, '\f')
		case 'n':
			str = append(str, '\n')
		case 'r':
			str = append(str, '\r')
		case 't':
			str = append(str, '\t')
		case 'u':
			if i+5 > len(json) {
				return str
			}

			r := runeit(json[i+1:])
			i += 5

			if utf16.IsSurrogate(r) && len(json[i:]) >= 6 &&
				json[i] == '\\' && json[i+1] == 'u' {
				r = utf16.DecodeRune(r, runeit(json[i+2:]))
				i += 6
			}

			str = utf8.AppendRune(str, r)
			i--
		}
	}

	return str
}

func (t Result) Less(token Result, caseSensitive bool) bool {
	if t.Type < token.Type {
		return true
	}

	if t.Type > token.Type {
		return false
	}

	if t.Type == String {
		if caseSensitive {
			return t.Str < token.Str
		}

		return stringLessInsensitive(t.Str, token.Str)
	}

	if t.Type == Number {
		return t.Num < token.Num
	}

	return t.Raw < token.Raw
}

func stringLessInsensitive(a, b string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] >= 'A' && a[i] <= 'Z' {
			if b[i] >= 'A' && b[i] <= 'Z' {
				if a[i] < b[i] {
					return true
				}

				if a[i] > b[i] {
					return false
				}
			} else {
				if a[i]+32 < b[i] {
					return true
				}

				if a[i]+32 > b[i] {
					return false
				}
			}
		} else if b[i] >= 'A' && b[i] <= 'Z' {
			if a[i] < b[i]+32 {
				return true
			}

			if a[i] > b[i]+32 {
				return false
			}
		} else {
			if a[i] < b[i] {
				return true
			}

			if a[i] > b[i] {
				return false
			}
		}
	}

	return len(a) < len(b)
}

func parseAny(json string, i int, hit bool) (int, Result, bool) {
	var res Result
	var val string
	for ; i < len(json); i++ {
		if json[i] == '{' || json[i] == '[' {
			i, val = parseSquash(json, i)
			if hit {
				res.Raw = val
				res.Type = JSON
			}

			var tmp parseContext
			tmp.value = res
			fillIndex(json, &tmp)
			return i, tmp.value, true
		}

		if json[i] <= ' ' {
			continue
		}

		var num bool
		switch json[i] {
		case '"':
			i++
			var vesc bool
			var ok bool
			i, val, vesc, ok = parseString(json, i)
			if !ok {
				return i, res, false
			}

			if hit {
				res.Type = String
				res.Raw = val
				if vesc {
					res.Str = unescape(val[1 : len(val)-1])
				} else {
					res.Str = val[1 : len(val)-1]
				}
			}

			return i, res, true
		case 'n':
			if i+1 < len(json) && json[i+1] != 'u' {
				num = true
				break
			}

			fallthrough
		case 't', 'f':
			vc := json[i]
			i, val = parseLiteral(json, i)
			if hit {
				res.Raw = val
				switch vc {
				case 't':
					res.Type = True
				case 'f':
					res.Type = False
				}

				return i, res, true
			}
		case '+', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'i', 'I', 'N':
			num = true
		}

		if num {
			i, val = parseNumber(json, i)
			if hit {
				res.Raw = val
				res.Type = Number
				res.Num, _ = strconv.ParseFloat(val, 64)
			}

			return i, res, true
		}
	}

	return i, res, false
}

func GetMany(json string, path ...string) []Result {
	res := make([]Result, len(path))
	for i, path := range path {
		res[i] = Get(json, path)
	}

	return res
}

func GetManyBytes(json []byte, path ...string) []Result {
	res := make([]Result, len(path))
	for i, path := range path {
		res[i] = GetBytes(json, path)
	}

	return res
}

func validpayload(data []byte, i int) (outi int, ok bool) {
	for ; i < len(data); i++ {
		switch data[i] {
		default:
			i, ok = validany(data, i)
			if !ok {
				return i, false
			}

			for ; i < len(data); i++ {
				switch data[i] {
				default:
					return i, false
				case ' ', '\t', '\n', '\r':
					continue
				}
			}

			return i, true
		case ' ', '\t', '\n', '\r':
			continue
		}
	}

	return i, false
}
func validany(data []byte, i int) (outi int, ok bool) {
	for ; i < len(data); i++ {
		switch data[i] {
		default:
			return i, false
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return validobject(data, i+1)
		case '[':
			return validarray(data, i+1)
		case '"':
			return validstring(data, i+1)
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return validnumber(data, i+1)
		case 't':
			return validtrue(data, i+1)
		case 'f':
			return validfalse(data, i+1)
		case 'n':
			return validnull(data, i+1)
		}
	}

	return i, false
}
func validobject(data []byte, i int) (outi int, ok bool) {
	for ; i < len(data); i++ {
		switch data[i] {
		default:
			return i, false
		case ' ', '\t', '\n', '\r':
			continue
		case '}':
			return i + 1, true
		case '"':
		key:
			if i, ok = validstring(data, i+1); !ok {
				return i, false
			}
			if i, ok = validcolon(data, i); !ok {
				return i, false
			}

			if i, ok = validany(data, i); !ok {
				return i, false
			}

			if i, ok = validcomma(data, i, '}'); !ok {
				return i, false
			}

			if data[i] == '}' {
				return i + 1, true
			}

			i++
			for ; i < len(data); i++ {
				switch data[i] {
				default:
					return i, false
				case ' ', '\t', '\n', '\r':
					continue
				case '"':
					goto key
				}
			}

			return i, false
		}
	}

	return i, false
}
func validcolon(data []byte, i int) (outi int, ok bool) {
	for ; i < len(data); i++ {
		switch data[i] {
		default:
			return i, false
		case ' ', '\t', '\n', '\r':
			continue
		case ':':
			return i + 1, true
		}
	}

	return i, false
}
func validcomma(data []byte, i int, end byte) (outi int, ok bool) {
	for ; i < len(data); i++ {
		switch data[i] {
		default:
			return i, false
		case ' ', '\t', '\n', '\r':
			continue
		case ',':
			return i, true
		case end:
			return i, true
		}
	}

	return i, false
}
func validarray(data []byte, i int) (outi int, ok bool) {
	for ; i < len(data); i++ {
		switch data[i] {
		default:
			for ; i < len(data); i++ {
				if i, ok = validany(data, i); !ok {
					return i, false
				}

				if i, ok = validcomma(data, i, ']'); !ok {
					return i, false
				}

				if data[i] == ']' {
					return i + 1, true
				}
			}
		case ' ', '\t', '\n', '\r':
			continue
		case ']':
			return i + 1, true
		}
	}

	return i, false
}
func validstring(data []byte, i int) (outi int, ok bool) {
	for ; i < len(data); i++ {
		if data[i] < ' ' {
			return i, false
		}

		if data[i] == '\\' {
			i++
			if i == len(data) {
				return i, false
			}

			switch data[i] {
			default:
				return i, false
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			case 'u':
				for range 4 {
					i++
					if i >= len(data) {
						return i, false
					}

					if !((data[i] >= '0' && data[i] <= '9') ||
						(data[i] >= 'a' && data[i] <= 'f') ||
						(data[i] >= 'A' && data[i] <= 'F')) {
						return i, false
					}
				}
			}
		} else if data[i] == '"' {
			return i + 1, true
		}
	}

	return i, false
}
func validnumber(data []byte, i int) (outi int, ok bool) {
	i--

	if data[i] == '-' {
		i++
		if i == len(data) {
			return i, false
		}

		if data[i] < '0' || data[i] > '9' {
			return i, false
		}
	}

	if i == len(data) {
		return i, false
	}

	if data[i] == '0' {
		i++
	} else {
		for ; i < len(data); i++ {
			if data[i] >= '0' && data[i] <= '9' {
				continue
			}

			break
		}
	}

	if i == len(data) {
		return i, true
	}

	if data[i] == '.' {
		i++
		if i == len(data) {
			return i, false
		}

		if data[i] < '0' || data[i] > '9' {
			return i, false
		}

		i++
		for ; i < len(data); i++ {
			if data[i] >= '0' && data[i] <= '9' {
				continue
			}

			break
		}
	}

	if i == len(data) {
		return i, true
	}

	if data[i] == 'e' || data[i] == 'E' {
		i++
		if i == len(data) {
			return i, false
		}

		if data[i] == '+' || data[i] == '-' {
			i++
		}

		if i == len(data) {
			return i, false
		}

		if data[i] < '0' || data[i] > '9' {
			return i, false
		}

		i++
		for ; i < len(data); i++ {
			if data[i] >= '0' && data[i] <= '9' {
				continue
			}

			break
		}
	}

	return i, true
}

func validtrue(data []byte, i int) (outi int, ok bool) {
	if i+3 <= len(data) && data[i] == 'r' && data[i+1] == 'u' &&
		data[i+2] == 'e' {
		return i + 3, true
	}

	return i, false
}
func validfalse(data []byte, i int) (outi int, ok bool) {
	if i+4 <= len(data) && data[i] == 'a' && data[i+1] == 'l' &&
		data[i+2] == 's' && data[i+3] == 'e' {
		return i + 4, true
	}

	return i, false
}
func validnull(data []byte, i int) (outi int, ok bool) {
	if i+3 <= len(data) && data[i] == 'u' && data[i+1] == 'l' &&
		data[i+2] == 'l' {
		return i + 3, true
	}

	return i, false
}

func Valid(json string) bool {
	_, ok := validpayload(stringBytes(json), 0)
	return ok
}

func ValidBytes(json []byte) bool {
	_, ok := validpayload(json, 0)
	return ok
}

func parseUint(s string) (uint64, bool) {
	var i int
	if i == len(s) {
		return 0, false
	}

	var n uint64
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}

		next := n*10 + uint64(s[i]-'0')
		if next < n {
			goto overflow
		}

		n = next
	}

	return n, true
overflow:

	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	return 18446744073709551615, true
}

func parseInt(s string) (int64, bool) {
	var sign bool
	if len(s) > 0 && s[0] == '-' {
		sign = true
		s = s[1:]
	}

	n, ok := parseUint(s)
	if !ok {
		return 0, false
	}

	if sign {
		if n > 9223372036854775808 {
			return -9223372036854775808, true
		}

		return -int64(n), true
	}

	if n > 9223372036854775807 {
		return 9223372036854775807, true
	}

	return int64(n), true
}

func safeInt(f float64) (n int64, ok bool) {
	if f < -9007199254740991 || f > 9007199254740991 {
		return 0, false
	}

	return int64(f), true
}

func execStatic(json, path string) (pathOut, res string, ok bool) {
	name := path[1:]
	if len(name) > 0 {
		switch name[0] {
		case '{', '[', '"', '+', '-', '0', '1', '2', '3', '4', '5', '6', '7',
			'8', '9':
			_, res = parseSquash(name, 0)
			pathOut = name[len(res):]
			return pathOut, res, true
		}
	}

	for i := 1; i < len(path); i++ {
		if path[i] == '|' {
			pathOut = path[i:]
			name = path[1:i]
			break
		}

		if path[i] == '.' {
			pathOut = path[i:]
			name = path[1:i]
			break
		}
	}

	switch strings.ToLower(name) {
	case "true", "false", "null", "nan", "inf":
		return pathOut, name, true
	}

	return pathOut, res, false
}

func execModifier(json, path string) (pathOut, res string, ok bool) {
	name := path[1:]
	var hasArgs bool
	for i := 1; i < len(path); i++ {
		if path[i] == ':' {
			pathOut = path[i+1:]
			name = path[1:i]
			hasArgs = len(pathOut) > 0
			break
		}

		if path[i] == '|' {
			pathOut = path[i:]
			name = path[1:i]
			break
		}

		if path[i] == '.' {
			pathOut = path[i:]
			name = path[1:i]
			break
		}
	}

	if fn, ok := modifiers[name]; ok {
		var args string
		if hasArgs {
			var parsedArgs bool
			switch pathOut[0] {
			case '{', '[', '"':

				res := Parse(pathOut)
				if res.Exists() {
					args = squash(pathOut)
					pathOut = pathOut[len(args):]
					parsedArgs = true
				}
			}

			if !parsedArgs {
				i := 0
				for ; i < len(pathOut); i++ {
					if pathOut[i] == '|' {
						break
					}

					switch pathOut[i] {
					case '{', '[', '"', '(':
						s := squash(pathOut[i:])
						i += len(s) - 1
					}
				}

				args = pathOut[:i]
				pathOut = pathOut[i:]
			}
		}

		return pathOut, fn(json, args), true
	}

	return pathOut, res, false
}

func unwrap(json string) string {
	json = trim(json)
	if len(json) >= 2 && (json[0] == '[' || json[0] == '{') {
		json = json[1 : len(json)-1]
	}

	return json
}

var DisableModifiers = false

var modifiers map[string]func(json, arg string) string

func init() {
	modifiers = map[string]func(json, arg string) string{
		"pretty":  modPretty,
		"ugly":    modUgly,
		"reverse": modReverse,
		"this":    modThis,
		"flatten": modFlatten,
		"join":    modJoin,
		"valid":   modValid,
		"keys":    modKeys,
		"values":  modValues,
		"tostr":   modToStr,
		"fromstr": modFromStr,
		"group":   modGroup,
		"dig":     modDig,
	}
}

func AddModifier(name string, fn func(json, arg string) string) {
	modifiers[name] = fn
}

func ModifierExists(name string, fn func(json, arg string) string) bool {
	_, ok := modifiers[name]
	return ok
}

func cleanWS(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			var s2 []byte
			for i := 0; i < len(s); i++ {
				switch s[i] {
				case ' ', '\t', '\n', '\r':
					s2 = append(s2, s[i])
				}
			}

			return string(s2)
		}
	}

	return s
}

func modPretty(json, arg string) string {
	if len(arg) > 0 {
		opts := *pretty.DefaultOptions
		Parse(arg).ForEach(func(key, value Result) bool {
			switch key.String() {
			case "sortKeys":
				opts.SortKeys = value.Bool()
			case "indent":
				opts.Indent = cleanWS(value.String())
			case "prefix":
				opts.Prefix = cleanWS(value.String())
			case "width":
				opts.Width = int(value.Int())
			}

			return true
		})
		return bytesString(pretty.PrettyOptions(stringBytes(json), &opts))
	}

	return bytesString(pretty.Pretty(stringBytes(json)))
}

func modThis(json, arg string) string {
	return json
}

func modUgly(json, arg string) string {
	return bytesString(pretty.Ugly(stringBytes(json)))
}

func modReverse(json, arg string) string {
	res := Parse(json)
	if res.IsArray() {
		var values []Result
		res.ForEach(func(_, value Result) bool {
			values = append(values, value)
			return true
		})
		out := make([]byte, 0, len(json))
		out = append(out, '[')
		for i, j := len(values)-1, 0; i >= 0; i, j = i-1, j+1 {
			if j > 0 {
				out = append(out, ',')
			}

			out = append(out, values[i].Raw...)
		}

		out = append(out, ']')
		return bytesString(out)
	}

	if res.IsObject() {
		var keyValues []Result
		res.ForEach(func(key, value Result) bool {
			keyValues = append(keyValues, key, value)
			return true
		})
		out := make([]byte, 0, len(json))
		out = append(out, '{')
		for i, j := len(keyValues)-2, 0; i >= 0; i, j = i-2, j+1 {
			if j > 0 {
				out = append(out, ',')
			}

			out = append(out, keyValues[i+0].Raw...)
			out = append(out, ':')
			out = append(out, keyValues[i+1].Raw...)
		}

		out = append(out, '}')
		return bytesString(out)
	}

	return json
}

func modFlatten(json, arg string) string {
	res := Parse(json)
	if !res.IsArray() {
		return json
	}

	var deep bool
	if arg != "" {
		Parse(arg).ForEach(func(key, value Result) bool {
			if key.String() == "deep" {
				deep = value.Bool()
			}

			return true
		})
	}

	var out []byte
	out = append(out, '[')
	var idx int
	res.ForEach(func(_, value Result) bool {
		var raw string
		if value.IsArray() {
			if deep {
				raw = unwrap(modFlatten(value.Raw, arg))
			} else {
				raw = unwrap(value.Raw)
			}
		} else {
			raw = value.Raw
		}

		raw = strings.TrimSpace(raw)
		if len(raw) == 0 {
			return true
		}

		if idx > 0 {
			out = append(out, ',')
		}

		out = append(out, raw...)
		idx++
		return true
	})
	out = append(out, ']')
	return bytesString(out)
}

func modKeys(json, arg string) string {
	v := Parse(json)
	if !v.Exists() {
		return "[]"
	}

	obj := v.IsObject()
	var out strings.Builder
	out.WriteByte('[')
	var i int
	v.ForEach(func(key, _ Result) bool {
		if i > 0 {
			out.WriteByte(',')
		}

		if obj {
			out.WriteString(key.Raw)
		} else {
			out.WriteString("null")
		}

		i++
		return true
	})
	out.WriteByte(']')
	return out.String()
}

func modValues(json, arg string) string {
	v := Parse(json)
	if !v.Exists() {
		return "[]"
	}

	if v.IsArray() {
		return json
	}

	var out strings.Builder
	out.WriteByte('[')
	var i int
	v.ForEach(func(_, value Result) bool {
		if i > 0 {
			out.WriteByte(',')
		}

		out.WriteString(value.Raw)
		i++
		return true
	})
	out.WriteByte(']')
	return out.String()
}

func modJoin(json, arg string) string {
	res := Parse(json)
	if !res.IsArray() {
		return json
	}

	var preserve bool
	if arg != "" {
		Parse(arg).ForEach(func(key, value Result) bool {
			if key.String() == "preserve" {
				preserve = value.Bool()
			}

			return true
		})
	}

	var out []byte
	out = append(out, '{')
	if preserve {
		var idx int
		res.ForEach(func(_, value Result) bool {
			if !value.IsObject() {
				return true
			}

			if idx > 0 {
				out = append(out, ',')
			}

			out = append(out, unwrap(value.Raw)...)
			idx++
			return true
		})
		out = append(out, '}')
		return bytesString(out)
	}

	var keys []Result
	kvals := make(map[string]Result)
	res.ForEach(func(_, value Result) bool {
		if !value.IsObject() {
			return true
		}

		value.ForEach(func(key, value Result) bool {
			k := key.String()
			if _, ok := kvals[k]; !ok {
				keys = append(keys, key)
			}

			kvals[k] = value
			return true
		})
		return true
	})
	for i := range keys {
		if i > 0 {
			out = append(out, ',')
		}

		out = append(out, keys[i].Raw...)
		out = append(out, ':')
		out = append(out, kvals[keys[i].String()].Raw...)
	}

	out = append(out, '}')
	return bytesString(out)
}

func modValid(json, arg string) string {
	if !Valid(json) {
		return ""
	}

	return json
}

func modFromStr(json, arg string) string {
	if !Valid(json) {
		return ""
	}

	return Parse(json).String()
}

func modToStr(str, arg string) string {
	return string(AppendJSONString(nil, str))
}

func modGroup(json, arg string) string {
	res := Parse(json)
	if !res.IsObject() {
		return ""
	}

	var all [][]byte
	res.ForEach(func(key, value Result) bool {
		if !value.IsArray() {
			return true
		}

		var idx int
		value.ForEach(func(_, value Result) bool {
			if idx == len(all) {
				all = append(all, []byte{})
			}

			all[idx] = append(all[idx], ("," + key.Raw + ":" + value.Raw)...)
			idx++
			return true
		})
		return true
	})
	var data []byte
	data = append(data, '[')
	for i, item := range all {
		if i > 0 {
			data = append(data, ',')
		}

		data = append(data, '{')
		data = append(data, item[1:]...)
		data = append(data, '}')
	}

	data = append(data, ']')
	return string(data)
}

func getBytes(json []byte, path string) Result {
	if json == nil {
		return Result{}
	}

	result := Get(bytesString(json), path)
	rawData := unsafe.StringData(result.Raw)
	strData := unsafe.StringData(result.Str)

	if len(result.Str) == 0 {
		result.Raw = string(stringBytes(result.Raw))
		result.Str = ""
		return result
	}

	if len(result.Raw) == 0 {
		result.Raw = ""
		result.Str = string(stringBytes(result.Str))
		return result
	}

	if uintptr(unsafe.Pointer(strData)) >= uintptr(unsafe.Pointer(rawData)) &&
		uintptr(unsafe.Pointer(strData))+uintptr(len(result.Str)) <=
			uintptr(unsafe.Pointer(rawData))+uintptr(len(result.Raw)) {
		start := uintptr(unsafe.Pointer(strData)) - uintptr(unsafe.Pointer(rawData))
		result.Raw = string(stringBytes(result.Raw))
		result.Str = result.Raw[start : start+uintptr(len(result.Str))]
		return result
	}

	result.Raw = string(stringBytes(result.Raw))
	result.Str = string(stringBytes(result.Str))
	return result
}

func fillIndex(json string, c *parseContext) {
	if len(c.value.Raw) == 0 || c.calcd {
		return
	}

	jsonData := unsafe.StringData(json)
	rawData := unsafe.StringData(c.value.Raw)
	c.value.Index = int(uintptr(unsafe.Pointer(rawData)) - uintptr(unsafe.Pointer(jsonData)))
	if c.value.Index < 0 || c.value.Index >= len(json) {
		c.value.Index = 0
	}
}

func stringBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

func bytesString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func revSquash(json string) string {
	i := len(json) - 1
	var depth int
	if json[i] != '"' {
		depth++
	}

	if json[i] == '}' || json[i] == ']' || json[i] == ')' {
		i--
	}

	for ; i >= 0; i-- {
		switch json[i] {
		case '"':
			i--
			for ; i >= 0; i-- {
				if json[i] == '"' {
					esc := 0
					for i > 0 && json[i-1] == '\\' {
						i--
						esc++
					}

					if esc%2 == 1 {
						continue
					}

					i += esc
					break
				}
			}

			if depth == 0 {
				if i < 0 {
					i = 0
				}

				return json[i:]
			}
		case '}', ']', ')':
			depth++
		case '{', '[', '(':
			depth--
			if depth == 0 {
				return json[i:]
			}
		}
	}

	return json
}

func (t Result) Paths(json string) []string {
	if t.Indexes == nil {
		return nil
	}

	paths := make([]string, 0, len(t.Indexes))
	t.ForEach(func(_, value Result) bool {
		paths = append(paths, value.Path(json))
		return true
	})
	if len(paths) != len(t.Indexes) {
		return nil
	}

	return paths
}

func (t Result) Path(json string) string {
	var path []byte
	var comps []string
	i := t.Index - 1
	if t.Index+len(t.Raw) > len(json) {
		goto fail
	}

	if !strings.HasPrefix(json[t.Index:], t.Raw) {
		goto fail
	}

	for ; i >= 0; i-- {
		if json[i] <= ' ' {
			continue
		}

		if json[i] == ':' {
			for ; i >= 0; i-- {
				if json[i] != '"' {
					continue
				}

				break
			}

			raw := revSquash(json[:i+1])
			i = i - len(raw)
			comps = append(comps, raw)

			raw = revSquash(json[:i+1])
			i = i - len(raw)
			i++
		} else if json[i] == '{' {
			goto fail
		} else if json[i] == ',' || json[i] == '[' {
			var arrIdx int
			if json[i] == ',' {
				arrIdx++
				i--
			}

			for ; i >= 0; i-- {
				if json[i] == ':' {
					goto fail
				} else if json[i] == ',' {
					arrIdx++
				} else if json[i] == '[' {
					comps = append(comps, strconv.Itoa(arrIdx))
					break
				}

				if json[i] == ']' || json[i] == '}' || json[i] == '"' {
					raw := revSquash(json[:i+1])
					i = i - len(raw) + 1
				}
			}
		}
	}

	if len(comps) == 0 {
		if DisableModifiers {
			goto fail
		}

		return "@this"
	}

	for i := len(comps) - 1; i >= 0; i-- {
		rcomp := Parse(comps[i])
		if !rcomp.Exists() {
			goto fail
		}

		comp := Escape(rcomp.String())
		path = append(path, '.')
		path = append(path, comp...)
	}

	if len(path) > 0 {
		path = path[1:]
	}

	return string(path)
fail:
	return ""
}

func isSafePathKeyChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c <= ' ' || c > '~' || c == '_' ||
		c == '-' || c == ':'
}

func Escape(comp string) string {
	for i := 0; i < len(comp); i++ {
		if !isSafePathKeyChar(comp[i]) {
			ncomp := make([]byte, len(comp)+1)
			copy(ncomp, comp[:i])
			ncomp = ncomp[:i]
			for ; i < len(comp); i++ {
				if !isSafePathKeyChar(comp[i]) {
					ncomp = append(ncomp, '\\')
				}

				ncomp = append(ncomp, comp[i])
			}

			return string(ncomp)
		}
	}

	return comp
}

func parseRecursiveDescent(all []Result, parent Result, path string) []Result {
	if res := parent.Get(path); res.Exists() {
		all = append(all, res)
	}

	if parent.IsArray() || parent.IsObject() {
		parent.ForEach(func(_, val Result) bool {
			all = parseRecursiveDescent(all, val, path)
			return true
		})
	}

	return all
}

func modDig(json, arg string) string {
	all := parseRecursiveDescent(nil, Parse(json), arg)
	var out []byte
	out = append(out, '[')
	for i, res := range all {
		if i > 0 {
			out = append(out, ',')
		}

		out = append(out, res.Raw...)
	}

	out = append(out, ']')
	return string(out)
}

func (t Result) All() iter.Seq2[Result, Result] {
	return func(yield func(Result, Result) bool) {
		t.ForEach(yield)
	}
}

func (t Result) Keys() iter.Seq[Result] {
	return func(yield func(Result) bool) {
		t.ForEach(func(key, _ Result) bool {
			return yield(key)
		})
	}
}

func (t Result) Values() iter.Seq[Result] {
	return func(yield func(Result) bool) {
		t.ForEach(func(_, value Result) bool {
			return yield(value)
		})
	}
}
