(begin
  ; Restrict permits listed module references without quoting the allowlist.
  (assert
    (import/restrict
      (@native:time/v1)
      (import
        (time @native:time/v1)
        (int? (time/now)))))

  ; fn/apply accepts a data-built declaration list for special builtin imports.
  (assert
    (fn/apply import
      (list
        (list 'time @native:time/v1)
        '(int? (time/now)))))

  ; Restrict rejects available modules that are not on the allowlist.
  (assert
    (string/contains?
      (error/value
        (error/catch
          (import/restrict
            (@native:time/v1)
            (import
              (random @native:random/v1)
              (random/bool)))))
      "is not allowed"))

  ; Restriction does not change the surrounding importer.
  (assert
    (import
      (random @native:random/v1)
      (bool? (random/bool))))

  ; Search lists and filters the current allowed module references.
  (assert
    (import/restrict
      (@native:time/v1 @native:random/v1)
      (and
        (= (import/search) (list @native:random/v1 @native:time/v1))
        (= (import/search "random") (list @native:random/v1))
        (string/contains? (help import/search) "(import/search term...) -> List"))))

  ; Nested restrictions cannot widen the visible importer.
  (assert
    (string/contains?
      (error/value
        (error/catch
          (import/restrict
            (@native:random/v1 @native:time/v1)
            (import/restrict
              (@native:time/v1)
              (import
                (random @native:random/v1)
                (random/bool))))))
      "is not allowed"))

  ; Nested search returns the effective intersection.
  (assert
    (import/restrict
      (@native:random/v1 @native:time/v1)
      (import/restrict
        (@native:time/v1)
        (= (import/search) (list @native:time/v1)))))

  ; Closures retain their restricted importer after the body exits.
  (let
    ((blocked
       (import/restrict
         (@native:time/v1)
         (fn ()
           (import
             (random @native:random/v1)
             (random/bool))))))
    (assert
      (string/contains?
        (error/value (error/catch (blocked)))
        "is not allowed")))

  ; Closures retain their scoped module search.
  (let
    ((search
      (import/restrict
        (@native:time/v1)
        (fn () (import/search)))))
    (assert (= (search) (list @native:time/v1))))

  ; Restrict validates arity, allowlist shape, entries, duplicates, and import binding.
  (assert
    (and
      (string/contains?
        (error/value (error/catch (import/restrict () null null)))
        "requires an allowlist and body expression")
      (string/contains?
        (error/value (error/catch (import/restrict 1 null)))
        "allowlist must be a proper list")
      (string/contains?
        (error/value (error/catch (import/restrict (1) null)))
        "allowlist entries must be module references")
      (string/contains?
        (error/value (error/catch (import/restrict (@native:time/v1 @native:time/v1) null)))
        "contains duplicate module reference")
      (string/contains?
        (error/value (error/catch (let ((import null)) (import/restrict () null))))
        "requires an import binding")
      (import/restrict
        (@native:time/v1)
        (string/contains?
          (error/value (error/catch (import/search 1)))
          "expected a string"))))

  ; Restrict reports its literal allowlist syntax.
  (assert (string/contains? (help import/restrict) "(import/restrict (@reference...) body) -> Value"))
  null)
