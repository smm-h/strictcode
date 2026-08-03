package relation

import (
	"fmt"
	"strings"
)

// NodeID is the hierarchical qualified name of a node (schema/SPEC.md
// section 2). Location is never part of identity. The serialized form is
//
//	<lang>:<member>:<module>:<container-chain>
//
// Serialized IDs are opaque strings for consumers; the structured form is
// authoritative.
type NodeID struct {
	// Lang is the profile prefix: "py" | "go" | "ts".
	Lang string
	// Member is the workspace member name, or "_" for a single-project
	// (non-workspace) scan.
	Member string
	// Module is the module's logical name (SPEC.md section 2.2).
	Module string
	// Chain is the container chain from module scope inward; empty for
	// module-kind nodes.
	Chain []Segment
}

// Segment is one element of the container chain.
type Segment struct {
	// Name is the unit's name; for anonymous units it is the name hint, or
	// "anon" when no hint is syntactically derivable.
	Name string
	// Overload is the 0-based source-order index among same-name siblings in
	// the same container (SPEC.md section 2.4). Serialized as #<n>, omitted
	// for 0.
	Overload int
	// Anonymous marks a synthesized segment (SPEC.md section 2.3):
	// <name-hint|anon>~<ordinal>~<fp8>.
	Anonymous bool
	// Ordinal is the 0-based source-order index among same-hint anonymous
	// siblings in the same parent. Anonymous only.
	Ordinal int
	// Fingerprint is the first 8 hex chars of SHA-256 over the unit's
	// normalized signature text. Anonymous only. It exists so ordinal drift
	// is detectable, never silently misattributed.
	Fingerprint string
}

// validLangs is the closed set of profile prefixes (one profile, one prefix;
// TS and JS share "ts").
var validLangs = map[string]bool{"py": true, "go": true, "ts": true}

// Validate checks the structural invariants of an ID. Extractors construct
// IDs; the relation builder rejects invalid ones as hard errors.
func (id NodeID) Validate() error {
	if !validLangs[id.Lang] {
		return fmt.Errorf("node ID: invalid lang %q (want py, go, or ts)", id.Lang)
	}
	if id.Member == "" {
		return fmt.Errorf("node ID: empty member (use %q for non-workspace scans)", "_")
	}
	if id.Module == "" {
		return fmt.Errorf("node ID: empty module")
	}
	for i, seg := range id.Chain {
		if seg.Name == "" {
			return fmt.Errorf("node ID: chain segment %d has empty name", i)
		}
		if seg.Overload < 0 {
			return fmt.Errorf("node ID: chain segment %d has negative overload index", i)
		}
		if seg.Anonymous {
			if seg.Ordinal < 0 {
				return fmt.Errorf("node ID: anonymous segment %d has negative ordinal", i)
			}
			if !isFP8(seg.Fingerprint) {
				return fmt.Errorf("node ID: anonymous segment %d fingerprint %q is not 8 lowercase hex chars", i, seg.Fingerprint)
			}
		} else {
			if seg.Ordinal != 0 || seg.Fingerprint != "" {
				return fmt.Errorf("node ID: named segment %d carries anonymous-only fields", i)
			}
		}
	}
	return nil
}

func isFP8(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// String returns the serialized ID. Reserved characters inside a segment are
// percent-encoded (SPEC.md section 2.1 lists %, :, ., #, whitespace; the
// tilde is additionally escaped so the anonymous-segment structure
// <hint>~<ordinal>~<fp8> is unambiguous — see BUILDLOG.md).
func (id NodeID) String() string {
	var b strings.Builder
	b.WriteString(escapeSegment(id.Lang))
	b.WriteByte(':')
	b.WriteString(escapeSegment(id.Member))
	b.WriteByte(':')
	b.WriteString(escapeSegment(id.Module))
	b.WriteByte(':')
	for i, seg := range id.Chain {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(escapeSegment(seg.Name))
		if seg.Anonymous {
			fmt.Fprintf(&b, "~%d~%s", seg.Ordinal, seg.Fingerprint)
		}
		if seg.Overload > 0 {
			fmt.Fprintf(&b, "#%d", seg.Overload)
		}
	}
	return b.String()
}

// escapeSegment percent-encodes the reserved characters within one segment.
func escapeSegment(s string) string {
	if !strings.ContainsAny(s, "%:.#~ \t\n\r") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '%', ':', '.', '#', '~', ' ', '\t', '\n', '\r':
			fmt.Fprintf(&b, "%%%02X", c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
