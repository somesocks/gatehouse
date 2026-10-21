package lisp

import (
	_ "embed"
	"strings"
	"testing"
)

//go:embed native_compress_deflate_test.lisp
var nativeCompressDeflateTests string

func TestNativeCompressDeflateLisp(t *testing.T) {
	err, result := Run(nativeCompressDeflateTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Native DEFLATE test result = %s, want null", result)
	}
}

func TestNativeCompressDeflateFixedTreesMatchCanonicalBuilder(t *testing.T) {
	env := prelude()
	err, exports := (&moduleLoader{cache: newModuleCache()}).load(moduleReference{scheme: moduleReferenceNative, uri: nativeCompressDeflateID}, env)
	if err != nil {
		t.Fatal(err)
	}
	var moduleEnv *environment
	for _, export := range exports {
		if export.name == "decode" {
			base, _ := unwrap(export.value)
			decode, ok := base.(*closure)
			if !ok {
				t.Fatalf("DEFLATE decode is %T, want a Lisp closure", base)
			}
			moduleEnv = decode.env
		}
	}
	if moduleEnv == nil {
		t.Fatal("DEFLATE decode export is missing")
	}
	err, builder := moduleEnv.lookup("h/build")
	if err != nil {
		t.Fatal(err)
	}
	err, vectorFrom := moduleEnv.lookup("vector/from")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, binding string
		lengths       []int
	}{
		{"literal-length", "fixed-literal", fixedDeflateLiteralLengths()},
		{"distance", "fixed-distance", []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5}},
	} {
		t.Run(test.name, func(t *testing.T) {
			evaluator := newEvaluator(nil)
			lengths := make([]Expr, len(test.lengths))
			for index, width := range test.lengths {
				lengths[index] = integer(int64(width))
			}
			err, vector := evaluator.call(vectorFrom, moduleEnv, lengths)
			if err != nil {
				t.Fatal(err)
			}
			err, expected := evaluator.call(builder, moduleEnv, []Expr{vector})
			if err != nil {
				t.Fatal(err)
			}
			err, actual := moduleEnv.lookup(test.binding)
			if err != nil {
				t.Fatal(err)
			}
			// Compare every branch and leaf, including reserved fixed-code symbols.
			if !equal(actual, expected) {
				t.Fatal("direct fixed tree differs from the canonical Huffman builder")
			}
		})
	}
}

func fixedDeflateLiteralLengths() []int {
	lengths := make([]int, 288)
	for symbol := range lengths {
		switch {
		case symbol < 144:
			lengths[symbol] = 8
		case symbol < 256:
			lengths[symbol] = 9
		case symbol < 280:
			lengths[symbol] = 7
		default:
			lengths[symbol] = 8
		}
	}
	return lengths
}

func BenchmarkNativeModuleImports(b *testing.B) {
	for _, test := range []struct{ name, source string }{
		{"bare", "(int/add 1 2)"},
		{"deflate", "(import (deflate @native:compress/deflate/v1) null)"},
		{"zip", "(import (zip @native:archive/zip/v1) null)"},
		{"zip-and-deflate", "(import (zip @native:archive/zip/v1) (deflate @native:compress/deflate/v1) null)"},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err, _ := Run(test.source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestNativeCompressDeflateDocumentationExamples(t *testing.T) {
	cache := newModuleCache()
	err, exports := (&moduleLoader{cache: cache}).load(moduleReference{scheme: moduleReferenceNative, uri: nativeCompressDeflateID}, prelude())
	if err != nil {
		t.Fatal(err)
	}
	for _, export := range exports {
		t.Run(export.name, func(t *testing.T) {
			lines := strings.Split(helpOf(export.value), "\n")
			if len(lines) != 3 || !strings.HasPrefix(lines[2], "Example: ") {
				t.Fatalf("help = %q, want standard documentation", helpOf(export.value))
			}
			example, expected, found := strings.Cut(strings.TrimPrefix(lines[2], "Example: "), " => ")
			if !found || !strings.HasSuffix(expected, ".") {
				t.Fatalf("example = %q, want source and result", lines[2])
			}
			err, result := Run(example)
			if err != nil {
				t.Fatalf("example %s: %v", example, err)
			}
			if got, want := result.String(), strings.TrimSuffix(expected, "."); got != want {
				t.Fatalf("example %s = %s, want %s", example, got, want)
			}
		})
	}
}
