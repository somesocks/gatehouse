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
		{`(import (json @native:json/v1) (json/decode "{\"name\":\"Gatehouse\",\"items\":[1,true,null,-2.5e+3]}"))`, `(json/object ("name" (json/string "Gatehouse")) ("items" (json/array (json/number "1") (json/boolean #t) (json/null) (json/number "-2.5e+3"))))`},
		{`(import (json @native:json/v1) (json/encode '(json/object ("name" (json/string "Gatehouse")) ("items" (json/array (json/number "1") (json/boolean #t) (json/null))))))`, `"{\"name\":\"Gatehouse\",\"items\":[1,true,null]}"`},
		{`(import (json @native:json/v1) (secret? (json/decode (secret/mark "{\"token\":\"secret\"}"))))`, `#t`},
		{`(import (json @native:json/v1) (secret? (json/encode (secret/mark '(json/string "secret")))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/decode "["))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/decode "true false"))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/encode '(json/number "NaN")))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/encode '(json/number "1 2")))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/encode '(json/object ("key" (json/string "value") "extra"))))))`, `#t`},
		{`(import (json @native:json/v1) (error? (error/catch (json/encode '(json/array "not-json")))))`, `#t`},
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
