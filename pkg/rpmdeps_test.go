package roci

import (
	"testing"

	"github.com/google/rpmpack"
)

func TestParseRpmdepsOutput(t *testing.T) {
	input := `  0 /usr/lib64/libabrt.so.0.1.0
    R libgobject-2.0.so.0()(64bit)
    R libc.so.6(GLIBC_2.2.5)(64bit)
    R libc.so.6(GLIBC_2.3.4)(64bit)
    R libc.so.6(GLIBC_2.33)(64bit)
    R libc.so.6(GLIBC_2.3)(64bit)
    R libc.so.6(GLIBC_2.4)(64bit)
    R libc.so.6(GLIBC_2.38)(64bit)
    R libc.so.6(GLIBC_ABI_DT_RELR)(64bit)
    R libgcc_s.so.1(GCC_3.3.1)(64bit)
    P libabrt.so.0(LIBRABRT_2.14.5)(64bit)
    R libreport.so.2(LIBREPORT_2.13.1)(64bit)
    P libabrt.so.0()(64bit)
    R libgcc_s.so.1(GCC_3.0)(64bit)
    R libreport.so.2()(64bit)
    R libglib-2.0.so.0()(64bit)
    R libsatyr.so.4()(64bit)
    R libjson-c.so.5()(64bit)
    R libgcc_s.so.1()(64bit)
    R libc.so.6()(64bit)
    R rtld(GNU_HASH)
    R libgio-2.0.so.0()(64bit)
    S libsupplement.so()(64bit)
    e libenhance.so()(64bit)
  1 /usr/lib64/libacl.so.1.1.2302
    P libacl.so.1(ACL_1.2)(64bit)
    R libc.so.6(GLIBC_2.38)(64bit)
    R libc.so.6(GLIBC_ABI_DT_RELR)(64bit)
    P libacl.so.1()(64bit)
    R libc.so.6(GLIBC_2.4)(64bit)
    P libacl.so.1(ACL_1.2)(64bit)
    P libacl.so.1(ACL_1.1)(64bit)
    P libacl.so.1(ACL_1.0)(64bit)
    R libc.so.6(GLIBC_2.33)(64bit)
    R libc.so.6(GLIBC_2.3.4)(64bit)
    R rtld(GNU_HASH)
    R libc.so.6()(64bit)
    R libattr.so.1()(64bit)
    R libc.so.6(GLIBC_2.2.5)(64bit)
    R libc.so.6(GLIBC_2.3)(64bit)
    o liborder.so()(64bit)`

	meta, err := ParseRpmdepsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Multiple file aggregation + deduplication: 20 unique requires from both files
	if len(meta.Requires) != 20 {
		t.Errorf("expected 20 unique requires, got %d", len(meta.Requires))
	}

	// Multiple file aggregation + deduplication: 6 unique provides from both files
	if len(meta.Provides) != 6 {
		t.Errorf("expected 6 unique provides, got %d", len(meta.Provides))
	}

	// Unsupported types (S, e, o) should be skipped
	if meta.Recommends != nil || meta.Suggests != nil || meta.Conflicts != nil || meta.Obsoletes != nil {
		t.Error("expected unsupported types and unused types to be nil")
	}

	// Verify specific relations exist
	hasLibc238 := false
	hasRtld := false
	hasLibacl := false

	for _, r := range meta.Requires {
		if r.Name == "libc.so.6(GLIBC_2.38)(64bit)" {
			hasLibc238 = true
		}
		if r.Name == "rtld(GNU_HASH)" {
			hasRtld = true
		}
	}

	for _, p := range meta.Provides {
		if p.Name == "libacl.so.1(ACL_1.2)(64bit)" {
			hasLibacl = true
		}
	}

	if !hasLibc238 {
		t.Error("expected libc.so.6(GLIBC_2.38)(64bit) in requires")
	}
	if !hasRtld {
		t.Error("expected rtld(GNU_HASH) in requires")
	}
	if !hasLibacl {
		t.Error("expected libacl.so.1(ACL_1.2)(64bit) in provides")
	}
}

func TestParseRpmdepsOutput_AllTypes(t *testing.T) {
	input := `0 /usr/lib64/test.so
  R required.so()(64bit)
  r recommended.so()(64bit)
  P provided.so()(64bit)
  C conflicted.so()(64bit)
  O itself < 1.0
  s some-symbol`

	meta, err := ParseRpmdepsOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name     string
		got      []*rpmpack.Relation
		expected string
	}{
		{"Requires", meta.Requires, "required.so()(64bit)"},
		{"Recommends", meta.Recommends, "recommended.so()(64bit)"},
		{"Provides", meta.Provides, "provided.so()(64bit)"},
		{"Conflicts", meta.Conflicts, "conflicted.so()(64bit)"},
		{"Obsoletes", meta.Obsoletes, "itself"},
		{"Suggests", meta.Suggests, "some-symbol"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.got) != 1 {
				t.Errorf("expected 1 %s, got %d", tt.name, len(tt.got))
				return
			}
			if tt.got[0].Name != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.got[0].Name)
			}
		})
	}
}
