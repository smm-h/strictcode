package relation

import (
	"fmt"
	"strconv"
	"strings"
)

// ValueKind is the type of an attribute value.
type ValueKind int

const (
	ValueString ValueKind = iota
	ValueInt
	ValueBool
)

// Value is one typed attribute value. Enum-typed attributes carry their
// value as a string; legality against the vocabulary enum is checked by the
// relation builder.
type Value struct {
	kind ValueKind
	s    string
	i    int64
	b    bool
}

// StringValue constructs a string (or enum) value.
func StringValue(s string) Value { return Value{kind: ValueString, s: s} }

// IntValue constructs an integer value.
func IntValue(i int64) Value { return Value{kind: ValueInt, i: i} }

// BoolValue constructs a boolean value.
func BoolValue(b bool) Value { return Value{kind: ValueBool, b: b} }

// Kind returns the value's type.
func (v Value) Kind() ValueKind { return v.kind }

// AsString returns the string content; the boolean is false for non-strings.
func (v Value) AsString() (string, bool) { return v.s, v.kind == ValueString }

// AsInt returns the integer content; the boolean is false for non-integers.
func (v Value) AsInt() (int64, bool) { return v.i, v.kind == ValueInt }

// AsBool returns the boolean content; the boolean is false for non-booleans.
func (v Value) AsBool() (bool, bool) { return v.b, v.kind == ValueBool }

// canonical renders the value for the canonical form: a type tag plus the
// escaped payload. The encoding is injective across kinds and values.
func (v Value) canonical() string {
	switch v.kind {
	case ValueString:
		return "s:" + escapeCanonical(v.s)
	case ValueInt:
		return "i:" + strconv.FormatInt(v.i, 10)
	case ValueBool:
		if v.b {
			return "b:true"
		}
		return "b:false"
	}
	panic(fmt.Sprintf("relation: undefined value kind %d", int(v.kind)))
}

// escapeCanonical percent-encodes the characters the canonical line format
// reserves (%, space, tab, LF, CR), keeping one line per record and one
// spelling per value.
func escapeCanonical(s string) string {
	if !strings.ContainsAny(s, "% \t\n\r") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '%', ' ', '\t', '\n', '\r':
			fmt.Fprintf(&b, "%%%02X", c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
