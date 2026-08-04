(begin
  (assert (string? "text") null)
  (assert (not (string? 1)) null)
  (assert (= (string/length "aλb") 3) null)
  (assert (= (string/concat) "") null)
  (assert (= (string/concat "gate" "house") "gatehouse") null)
  (assert (= (string/slice "aλbc" 1 3) "λb") null)
  (assert (= (string/trim " text ") "text") null)
  (assert (= (string/lower "Straße") "straße") null)
  (assert (= (string/upper "Straße") "STRAßE") null)
  (assert (string/contains? "gatehouse" "house") null)
  (assert (string/contains? "gatehouse" "") null)
  (assert (string/prefix? "gatehouse" "gate") null)
  (assert (string/suffix? "gatehouse" "house") null)
  (assert
    (= (string/split "a,,b," ",") (list "a" "" "b" ""))
    null)
  (assert
    (= (string/split "aλb" "") (list "a" "λ" "b"))
    null)
  (assert (= (string/split "" "") null) null)
  (assert
    (= (string/join (list "one" "two" "three") ", ") "one, two, three")
    null)
  (assert (= (string/join null ",") "") null)
  (assert (= (string/replace "a-b-c" "-" "") "abc") null)
  (assert (= (string/replace "one two one" "one" "1") "1 two 1") null)
  (assert
    (string/contains?
      (error/value (error/catch (string/length 1)))
      "expected a string")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (string/slice "abc" -1 2)))
      "indices are out of range")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (string/slice "abc" 2 1)))
      "indices are out of range")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (string/join (list "a" 1) ",")))
      "expected a string")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (string/replace "abc" "" "x")))
      "non-empty old text")
    null)
  null)
