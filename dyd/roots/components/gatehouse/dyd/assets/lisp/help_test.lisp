; Root leading help.
(begin
  (let
    ((leading
       ; Leading first line.
       ; Leading second line.
       1)
     (trailing
       2 ; Trailing first line.
       ; Trailing second line.
       )
     (separated
       ; First block.

       ; Second block.
       3)
     (nested
       (list
         ; First list value.
         4
         5 ; Second list value.
         ))
     (documented
       ; replacement-marker
       +))
    (begin
      (assert (= (help 'leading) "Leading first line.\nLeading second line."))
      (assert (= (help 'trailing) "Trailing first line.\nTrailing second line."))
      (assert (= (help 'separated) "First block.\nSecond block."))
      (assert (= (help (head nested)) "First list value."))
      (assert (= (help (head (tail nested))) "Second list value."))
      (assert (= (help 'documented) "replacement-marker"))
      (assert (= (help/search "replacement-marker") (list 'documented)))))

  ; Help identifies builtins, values, functions, and symbols.
  (assert
    (and
      (string/contains? (help string/slice) "(string/slice text start end) -> String")
      (= (help (int/min 1 2)) "int")
      (string/contains? (help 'string/slice) "Example: (string/slice \"hello\" 1 4) => \"ell\".")
       (= (help 'unbound) "symbol")
       (= (help 1) "int")
       (= (help (fn (value) value)) "function (value)")
       (= (help (fn (value . rest) value)) "function (value . rest)")
       (= (help (fn rest rest)) "function rest")))

  ; Evaluator forms are discoverable and documented without bindings.
  (assert
    (and
       (string/contains? (help 'fn) "(fn parameters body) -> Function")
      (string/contains? (help 'let) "(let ((name value) ...) body) -> Value")
      (list? (help/search "recursive lexical bindings"))))

  ; Help follows lexical bindings and shadowing.
  (assert
    (and
      (= (let ((string/slice 1)) (help 'string/slice)) "int")
      (= (let ((local (fn (value) value))) (help 'local)) "function (value)")))

  ; Help requires exactly one argument.
  (assert
    (and
      (string/contains? (error/value (error/catch (help))) "help requires one argument")
      (string/contains? (error/value (error/catch (help 1 2))) "help requires one argument")))

  ; Help search matches documented signatures case-insensitively.
  (assert
    (and
      (= (help/search "STRING/SLICE") (list 'string/slice))
      (= (help/search "list/fold") (list 'list/fold))
      (= (help/search "BOOL?") (list 'bool?))))

  ; Help search includes documented aliases and respects shadowing.
  (assert
    (and
       (= (let ((slice string/slice)) (help/search "string/slice")) (list 'slice 'string/slice))
       (string/contains? (head (list/map help (help/search "string/slice"))) "(string/slice text start end) -> String")
       (= (let ((string/slice 1)) (help/search "string/slice")) null)
       (= ((fn (string/slice) (help/search "string/slice")) 1) null)))

  ; Help search requires string terms.
  (assert
    (string/contains?
      (error/value (error/catch (help/search 1)))
      "expected a string"))

  ; Help environment search matches visible binding names.
  (assert
    (and
      (= (help/env "BOOL?") (list 'bool?))
      (= (let ((alias +)) (help/env "ALIAS")) (list 'alias))
      (= (let ((string/slice 1)) (help/env "STRING/SLICE")) (list 'string/slice))
      (= ((fn (parameter) (help/env "PARAMETER")) 1) (list 'parameter))))

  ; Documented values retain their value and expose their text through help.
  (assert
    (and
      (= (let ((pi (help/document 3 "Circle constant."))) (help 'pi)) "Circle constant.")
      (= (let ((double (help/document (fn (value) (+ value value)) "Doubles an integer."))) (help 'double)) "Doubles an integer.")
      (= (let ((double (help/document (fn (value) (+ value value)) "Doubles an integer."))) (double 3)) 6)
      (= (let ((pi (help/document 3 "Circle constant."))) (help/search "circle")) (list 'pi))
      (= (let ((sum (help/document + "Sums integer values."))) (help 'sum)) "Sums integer values.")
      (= + (help/document + "Sums integer values."))))

  ; Documenting values validates its public text argument.
  (assert
    (and
      (string/contains? (error/value (error/catch (help/document))) "requires a value and help text")
      (string/contains? (error/value (error/catch (help/document 1))) "requires a value and help text")
      (string/contains? (error/value (error/catch (help/document 1 "text" "extra"))) "requires a value and help text")
      (string/contains? (error/value (error/catch (help/document 1 2))) "expected a string")
      (string/contains? (error/value (error/catch (help/document 1 (taint/secret/mark "secret")))) "requires public help text")))

  ; An empty help search has the same results as an omitted search term.
  (assert (= (help/search) (help/search "")))
  null) ; Root trailing help.
