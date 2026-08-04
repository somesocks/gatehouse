(begin
  ; Strings satisfy the string predicate.
  (assert (string? "text"))

  ; Non-strings do not satisfy the string predicate.
  (assert (not (string? 1)))

  ; String length counts Unicode code points.
  (assert (= (string/length "aλb") 3))

  ; Concatenating no strings produces an empty string.
  (assert (= (string/concat) ""))

  ; Concatenation preserves input order.
  (assert (= (string/concat "gate" "house") "gatehouse"))

  ; Slicing uses Unicode code-point indexes.
  (assert (= (string/slice "aλbc" 1 3) "λb"))

  ; Trimming removes Unicode whitespace.
  (assert (= (string/trim " text ") "text"))

  ; Lowercasing is Unicode-aware.
  (assert (= (string/lower "Straße") "straße"))

  ; Uppercasing is Unicode-aware.
  (assert (= (string/upper "Straße") "STRAßE"))

  ; Contains recognizes an internal substring.
  (assert (string/contains? "gatehouse" "house"))

  ; Every string contains the empty string.
  (assert (string/contains? "gatehouse" ""))

  ; Prefix checks recognize matching text.
  (assert (string/prefix? "gatehouse" "gate"))

  ; Suffix checks recognize matching text.
  (assert (string/suffix? "gatehouse" "house"))

  ; Splitting preserves empty fields and a trailing field.
  (assert
    (= (string/split "a,,b," ",") (list "a" "" "b" "")))

  ; Empty-separator splitting produces Unicode code points.
  (assert
    (= (string/split "aλb" "") (list "a" "λ" "b")))

  ; Splitting an empty string on an empty separator produces an empty list.
  (assert (= (string/split "" "") null))

  ; Joining inserts the separator between every element.
  (assert
    (= (string/join (list "one" "two" "three") ", ") "one, two, three"))

  ; Joining an empty list produces an empty string.
  (assert (= (string/join null ",") ""))

  ; Replacing with an empty string removes every occurrence.
  (assert (= (string/replace "a-b-c" "-" "") "abc"))

  ; Replacing updates every non-overlapping occurrence.
  (assert (= (string/replace "one two one" "one" "1") "1 two 1"))

  ; String length rejects a non-string value.
  (assert
    (string/contains?
      (error/value (error/catch (string/length 1)))
      "expected a string"))

  ; String slicing rejects a negative index.
  (assert
    (string/contains?
      (error/value (error/catch (string/slice "abc" -1 2)))
      "indices are out of range"))

  ; String slicing rejects a reversed range.
  (assert
    (string/contains?
      (error/value (error/catch (string/slice "abc" 2 1)))
      "indices are out of range"))

  ; Joining rejects a list containing a non-string value.
  (assert
    (string/contains?
      (error/value (error/catch (string/join (list "a" 1) ",")))
      "expected a string"))

  ; Replacing requires a non-empty old string.
  (assert
    (string/contains?
      (error/value (error/catch (string/replace "abc" "" "x")))
      "non-empty old text"))
  null)
