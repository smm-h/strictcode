package testctx

import "testing"

// The shared test-context predicate (DESIGN.md section 6.4). One predicate,
// computed on the path relative to the project root, shared by every check.
// Lessons 6, 7, 8 of the register are encoded here.
func TestIsTestContext(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// Lesson 6: root-relative classification, not substring — a
		// production src/test/ is NOT test code.
		{"src/test/handler.py", false},
		{"src/tests/util.go", false},
		{"lib/example/render.ts", false},

		// Lesson 8: root-level first components.
		{"test/handler.py", true},
		{"tests/util.py", true},
		{"example/demo.py", true},
		{"examples/demo.ts", true},
		{"integration_test/full.go", true},
		{"tests/deep/nested/x.py", true},

		// Lesson 7: unconditional directories at any depth.
		{"src/components/__tests__/button.ts", true},
		{"__tests__/a.js", true},
		{"internal/parser/testdata/sample.go", true},
		{"testdata/x.py", true},
		{"a/b/c/testdata/d/e.py", true},

		// File-name patterns.
		{"src/test_util.py", true},
		{"src/util_test.py", true},
		{"src/conftest.py", true},
		{"conftest.py", true},
		{"internal/parser/parse_test.go", true},
		{"src/app.test.ts", true},
		{"src/app.test.tsx", true},
		{"src/app.test.js", true},
		{"src/app.test.jsx", true},
		{"src/app.spec.ts", true},
		{"src/app.spec.jsx", true},

		// Non-test files.
		{"src/app.py", false},
		{"main.go", false},
		{"src/index.ts", false},
		// Pattern lookalikes that must NOT match.
		{"src/attest.py", false},        // not test_* nor *_test
		{"src/latest.go", false},        // not *_test.go
		{"src/contest.py", false},       // not conftest.py
		{"src/app.testx.ts", false},     // .test. must be a full dot segment
		{"src/protest_tools.py", false}, // *_test only as suffix before .py
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if got := IsTestContext(c.path); got != c.want {
				t.Fatalf("IsTestContext(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestWindowsStyleSeparatorsRejected(t *testing.T) {
	// The predicate is defined over slash-separated root-relative paths; the
	// walker produces those. Backslashes are never path separators here.
	if IsTestContext(`src\test_x.py`) {
		t.Fatal("backslash path treated as separated components")
	}
}
