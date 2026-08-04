(begin
  (assert (string? "text"))
  (assert (not (string? 1)))
  (assert (= (string/length "aλb") 3))
  (assert (= (string/concat) ""))
  (assert (= (string/concat "gate" "house") "gatehouse"))
  (assert (= (string/slice "aλbc" 1 3) "λb"))
  (assert (= (string/trim " text ") "text"))
  (assert (= (string/lower "Straße") "straße"))
  (assert (= (string/upper "Straße") "STRAßE"))
  (assert (string/contains? "gatehouse" "house"))
  (assert (string/contains? "gatehouse" ""))
  (assert (string/prefix? "gatehouse" "gate"))
  (assert (string/suffix? "gatehouse" "house"))
  (assert
    (= (string/split "a,,b," ",") (list "a" "" "b" "")))
  (assert
    (= (string/split "aλb" "") (list "a" "λ" "b")))
  (assert (= (string/split "" "") null))
  (assert
    (= (string/join (list "one" "two" "three") ", ") "one, two, three"))
  (assert (= (string/join null ",") ""))
  (assert (= (string/replace "a-b-c" "-" "") "abc"))
  (assert (= (string/replace "one two one" "one" "1") "1 two 1"))
  (assert
    (string/contains?
      (error/value (error/catch (string/length 1)))
      "expected a string"))
  (assert
    (string/contains?
      (error/value (error/catch (string/slice "abc" -1 2)))
      "indices are out of range"))
  (assert
    (string/contains?
      (error/value (error/catch (string/slice "abc" 2 1)))
      "indices are out of range"))
  (assert
    (string/contains?
      (error/value (error/catch (string/join (list "a" 1) ",")))
      "expected a string"))
  (assert
    (string/contains?
      (error/value (error/catch (string/replace "abc" "" "x")))
      "non-empty old text"))
  null)
