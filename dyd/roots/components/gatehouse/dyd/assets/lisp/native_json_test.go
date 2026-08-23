package lisp

import "testing"

func TestNativeJSON(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{`(import (json @native:json/v1) (json/decode "null"))`, `(json/null)`},
		{`(import (json @native:json/v1) (json/decode "false"))`, `(json/boolean #f)`},
		{`(import (json @native:json/v1) (json/decode "\"\\u2603\\n\""))`, `(json/string "☃\n")`},
		{`(import (json @native:json/v1) (json/decode "[]"))`, `(json/array)`},
		{`(import (json @native:json/v1) (json/decode "{}"))`, `(json/object)`},
		{`(import (json @native:json/v1) (list (json/null? (json/null)) (json/boolean? (json/boolean #t)) (json/string? (json/string "text")) (json/number? (json/number "1")) (json/array? (json/array)) (json/object? (json/object))))`, `(#t #t #t #t #t #t)`},
		{`(import (json @native:json/v1) (list (json/null? (json/boolean #t)) (json/boolean? (json/string "text")) (json/string? (json/number "1")) (json/number? (json/array)) (json/array? (json/object)) (json/object? (json/null))))`, `(#f #f #f #f #f #f)`},
		{`(import (json @native:json/v1) (list (json/null? null) (json/boolean? #t) (json/string? "text") (json/number? 1) (json/array? (list)) (json/object? (pair "key" "value"))))`, `(#f #f #f #f #f #f)`},
		{`(import (json @native:json/v1) (list (json/null? '(json/null extra)) (json/boolean? '(json/boolean "not-a-boolean")) (json/string? '(json/string "text" extra)) (json/number? '(json/number "NaN")) (json/array? '(json/array "not-json")) (json/object? '(json/object ("key" . "not-json")))))`, `(#f #f #f #f #f #f)`},
		{`(import (json @native:json/v1) (json/encode (json/decode "{\"name\":\"Gatehouse\",\"items\":[1,true,null,-2.5e+3]}")))`, `"{\"name\":\"Gatehouse\",\"items\":[1,true,null,-2.5e+3]}"`},
		{`(import (json @native:json/v1) (json/encode (json/object (pair "name" (json/string "Gatehouse")) (pair "items" (json/array (json/number "1") (json/boolean #t) (json/null))))))`, `"{\"name\":\"Gatehouse\",\"items\":[1,true,null]}"`},
		{`(import (json @native:json/v1) (json/encode (fn/apply json/array (list/map json/string (list "first" "second")))))`, `"[\"first\",\"second\"]"`},
		{`(import (json @native:json/v1) (secret? (json/decode (secret/mark "{\"token\":\"secret\"}"))))`, `#t`},
		{`(import (json @native:json/v1) (secret? (json/encode (secret/mark (json/string "secret")))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/decode "["))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/decode "true false"))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/number "NaN"))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/number "1 2"))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/object (list "key" (json/string "value"))))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/array "not-json"))))`, `#t`},
	} {
		err, result := Run(test.source)
		if err != nil {
			t.Fatalf("Run(%s): %v", test.source, err)
		}
		if got := result.String(); got != test.want {
			t.Fatalf("Run(%s) = %s, want %s", test.source, got, test.want)
		}
	}
}
