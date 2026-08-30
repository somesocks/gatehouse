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
		{`(import (json @native:json/v1) (list (json/boolean/value (json/boolean #t)) (json/string/value (json/string "text")) (json/number/value (json/number "-2.5e+3"))))`, `(#t "text" "-2.5e+3")`},
		{`(import (json @native:json/v1) (json/array/values (json/array (json/string "a") (json/null))))`, `((json/string "a") (json/null))`},
		{`(import (json @native:json/v1) (list (json/array/length (json/array)) (json/array/length (json/decode "[true,null]")) (json/object/length (json/object (pair "name" (json/string "Gatehouse")) (pair "name" (json/string "Dryad")) (pair "enabled" (json/boolean #t))))))`, `(0 2 3)`},
		{`(import (json @native:json/v1) (json/object/values (json/object (pair "name" (json/string "Gatehouse")) (pair "enabled" (json/boolean #t)))))`, `(("name" json/string "Gatehouse") ("enabled" json/boolean #t))`},
		{`(import (json @native:json/v1) (list (json/array/get (json/array (json/string "first") (json/number "2")) 0) (json/array/get (json/array (json/string "first") (json/number "2")) 1)))`, `((json/string "first") (json/number "2"))`},
		{`(import (json @native:json/v1) (list (json/object/get (json/object (pair "name" (json/string "Gatehouse")) (pair "enabled" (json/boolean #t))) "name") (json/object/get (json/object (pair "name" (json/string "Gatehouse"))) "missing") (json/object/get (json/object (pair "value" (json/null))) "value")))`, `((json/string "Gatehouse") null (json/null))`},
		{`(import (json @native:json/v1) (json/string/value (json/object/get (json/object (pair "name" (json/string "first")) (pair "name" (json/string "second"))) "name")))`, `"first"`},
		{`(import (json @native:json/v1) (json/array/map (fn (value index) (if (= index 0) (json/string "zero") (json/string "one"))) (json/array (json/null) (json/null))))`, `(json/array (json/string "zero") (json/string "one"))`},
		{`(import (json @native:json/v1) (json/array/filter (fn (value index) (= index 1)) (json/array (json/string "first") (json/string "second"))))`, `(json/array (json/string "second"))`},
		{`(import (json @native:json/v1) (json/encode (json/object/map (fn (value key) (if (= key "same") (json/string "updated") value)) (json/object (pair "first" (json/string "one")) (pair "same" (json/string "two")) (pair "same" (json/string "three"))))))`, `"{\"first\":\"one\",\"same\":\"updated\",\"same\":\"updated\"}"`},
		{`(import (json @native:json/v1) (json/object/filter (fn (value key) (= key "second")) (json/object (pair "first" (json/string "one")) (pair "second" (json/string "two")) (pair "third" (json/string "three")))))`, `(json/object ("second" json/string "two"))`},
		{`(import (json @native:json/v1) (list (taint/secret? (json/array/map (fn (value index) value) (json/decode (taint/secret/mark "[null]")))) (taint/secret? (json/object/filter (fn (value key) #f) (json/decode (taint/secret/mark "{\"token\":\"secret\"}"))))))`, `(#t #t)`},
		{`(import (json @native:json/v1) (list (taint/secret? (json/string/value (json/decode (taint/secret/mark "\"secret\"")))) (taint/secret? (json/array/values (json/decode (taint/secret/mark "[\"secret\"]")))) (taint/secret? (json/object/values (json/decode (taint/secret/mark "{\"token\":\"secret\"}"))))))`, `(#t #t #t)`},
		{`(import (json @native:json/v1) (list (taint/secret? (json/array/get (json/decode (taint/secret/mark "[\"secret\"]")) 0)) (taint/secret? (json/object/get (json/decode (taint/secret/mark "{\"token\":\"secret\"}")) "token")) (taint/secret? (json/object/get (json/decode (taint/secret/mark "{}")) "missing"))))`, `(#t #t #t)`},
		{`(import (json @native:json/v1) (list (error? (error/catch (json/boolean/value (json/null)))) (error? (error/catch (json/string/value (json/boolean #t)))) (error? (error/catch (json/number/value (json/string "1")))) (error? (error/catch (json/array/values (json/object)))) (error? (error/catch (json/object/values (json/array))))))`, `(#t #t #t #t #t)`},
		{`(import (json @native:json/v1) (list (error? (error/catch (json/array/get (json/object) 0))) (error? (error/catch (json/array/get (json/array) 0))) (error? (error/catch (json/array/get (json/array (json/null)) -1))) (error? (error/catch (json/array/get (json/array (json/null)) "0"))) (error? (error/catch (json/object/get (json/array) "key"))) (error? (error/catch (json/object/get (json/object) 0)))))`, `(#t #t #t #t #t #t)`},
		{`(import (json @native:json/v1) (list (error? (error/catch (json/array/length (json/object)))) (error? (error/catch (json/object/length (json/array))))))`, `(#t #t)`},
		{`(import (json @native:json/v1) (error? (error/catch (json/array/map (fn (value index) "not-json") (json/array (json/null))))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/array/filter (fn (value index) (json/null)) (json/array (json/null))))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/object/map (fn (value key) "not-json") (json/object (pair "key" (json/null)))))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/object/filter (fn (value key) (json/null)) (json/object (pair "key" (json/null)))))))`, `#t`},
		{`(import (json @native:json/v1) (json/encode (json/decode "{\"name\":\"Gatehouse\",\"items\":[1,true,null,-2.5e+3]}")))`, `"{\"name\":\"Gatehouse\",\"items\":[1,true,null,-2.5e+3]}"`},
		{`(import (json @native:json/v1) (json/encode (json/object (pair "name" (json/string "Gatehouse")) (pair "items" (json/array (json/number "1") (json/boolean #t) (json/null))))))`, `"{\"name\":\"Gatehouse\",\"items\":[1,true,null]}"`},
		{`(import (json @native:json/v1) (json/encode (fn/apply json/array (list/map json/string (list "first" "second")))))`, `"[\"first\",\"second\"]"`},
		{`(import (json @native:json/v1) (taint/secret? (json/decode (taint/secret/mark "{\"token\":\"secret\"}"))))`, `#t`},
		{`(import (json @native:json/v1) (taint/secret? (json/encode (taint/secret/mark (json/string "secret")))))`, `#t`},
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
