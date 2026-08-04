(begin
  ; Quoted names are symbols.
  (assert (symbol? 'name))

  ; Strings are not symbols.
  (assert (not (symbol? "name")))

  ; Symbol predicate requires one argument.
  (assert
    (string/contains?
      (error/value (error/catch (symbol? 'one 'two)))
      "symbol? requires one argument"))
  null)
