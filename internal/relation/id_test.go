package relation

import (
	"strings"
	"testing"
)

func TestNodeIDSerialization(t *testing.T) {
	cases := []struct {
		name string
		id   NodeID
		want string
	}{
		{
			"module-node",
			NodeID{Lang: "py", Member: "core", Module: "pkg.sub.mod"},
			"py:core:pkg%2Esub%2Emod:",
		},
		{
			"function-in-module",
			NodeID{Lang: "go", Member: "transport", Module: "internal/parser",
				Chain: []Segment{{Name: "Parse"}}},
			"go:transport:internal/parser:Parse",
		},
		{
			"method-in-type",
			NodeID{Lang: "py", Member: "_", Module: "svc",
				Chain: []Segment{{Name: "UserService"}, {Name: "save"}}},
			"py:_:svc:UserService.save",
		},
		{
			"overload-index",
			NodeID{Lang: "ts", Member: "app", Module: "src/util",
				Chain: []Segment{{Name: "parse", Overload: 1}}},
			"ts:app:src/util:parse#1",
		},
		{
			"overload-zero-omitted",
			NodeID{Lang: "ts", Member: "app", Module: "src/util",
				Chain: []Segment{{Name: "parse", Overload: 0}}},
			"ts:app:src/util:parse",
		},
		{
			"anonymous-with-hint",
			NodeID{Lang: "py", Member: "_", Module: "handlers",
				Chain: []Segment{{Name: "route"}, {Name: "callback", Anonymous: true, Ordinal: 2, Fingerprint: "ab12cd34"}}},
			"py:_:handlers:route.callback~2~ab12cd34",
		},
		{
			"anonymous-no-hint",
			NodeID{Lang: "py", Member: "_", Module: "m",
				Chain: []Segment{{Name: "anon", Anonymous: true, Ordinal: 0, Fingerprint: "00000000"}}},
			"py:_:m:anon~0~00000000",
		},
		{
			"escaping-reserved-chars",
			NodeID{Lang: "ts", Member: "app", Module: "src/x",
				Chain: []Segment{{Name: "a.b:c#d%e~f g"}}},
			"ts:app:src/x:a%2Eb%3Ac%23d%25e%7Ef%20g",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.id.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if got := c.id.String(); got != c.want {
				t.Fatalf("String() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestNodeIDValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		id      NodeID
		wantSub string
	}{
		{"bad-lang", NodeID{Lang: "rust", Member: "_", Module: "m"}, "invalid lang"},
		{"empty-member", NodeID{Lang: "py", Member: "", Module: "m"}, "empty member"},
		{"empty-module", NodeID{Lang: "py", Member: "_", Module: ""}, "empty module"},
		{"empty-segment-name", NodeID{Lang: "py", Member: "_", Module: "m",
			Chain: []Segment{{Name: ""}}}, "empty name"},
		{"negative-overload", NodeID{Lang: "py", Member: "_", Module: "m",
			Chain: []Segment{{Name: "f", Overload: -1}}}, "negative overload"},
		{"anonymous-bad-fingerprint", NodeID{Lang: "py", Member: "_", Module: "m",
			Chain: []Segment{{Name: "anon", Anonymous: true, Fingerprint: "XYZ"}}}, "not 8 lowercase hex"},
		{"anonymous-uppercase-fingerprint", NodeID{Lang: "py", Member: "_", Module: "m",
			Chain: []Segment{{Name: "anon", Anonymous: true, Fingerprint: "AB12CD34"}}}, "not 8 lowercase hex"},
		{"named-with-ordinal", NodeID{Lang: "py", Member: "_", Module: "m",
			Chain: []Segment{{Name: "f", Ordinal: 1}}}, "anonymous-only fields"},
		{"named-with-fingerprint", NodeID{Lang: "py", Member: "_", Module: "m",
			Chain: []Segment{{Name: "f", Fingerprint: "ab12cd34"}}}, "anonymous-only fields"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.id.Validate()
			if err == nil {
				t.Fatal("Validate accepted an invalid ID")
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Fatalf("error %q does not contain %q", err, c.wantSub)
			}
		})
	}
}

func TestDistinctIDsSerializeDistinctly(t *testing.T) {
	// The anonymous structure must not collide with crafted names: a named
	// segment whose literal name looks like an anonymous serialization is
	// escaped, an actual anonymous segment is not.
	named := NodeID{Lang: "ts", Member: "_", Module: "m",
		Chain: []Segment{{Name: "cb~0~ab12cd34"}}}
	anon := NodeID{Lang: "ts", Member: "_", Module: "m",
		Chain: []Segment{{Name: "cb", Anonymous: true, Ordinal: 0, Fingerprint: "ab12cd34"}}}
	if named.String() == anon.String() {
		t.Fatalf("crafted name and anonymous unit serialize identically: %q", named.String())
	}
}
