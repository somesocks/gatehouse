(begin
  ; Definitions, inherited modules, and lexical exports work in one import.
  (let ((temperature 18))
    (assert
      (import/define
        (@native:fixture/weather/v1
          (list (pair 'temperature (fn (city) temperature))))
        (@native:fixture/places/v1
          (list (pair 'cities (fn () (list "London" "Paris")))))
        (import
          (weather @native:fixture/weather/v1)
          (json @native:json/v1)
          (places @native:fixture/places/v1)
          (= (json/encode (json/number
                (if (= (list/map weather/temperature (places/cities)) (list 18 18)) "36" "0")))
             "36")))))

  ; Search merges, sorts, deduplicates overrides, and matches every term.
  (assert
    (import/restrict (@native:time/v1)
      (import/define
        (@native:time/v1 (list (pair 'now (fn () 42))))
        (@native:fixture/weather/v1 (list))
        (@native:fixture/places/v1 (list))
        (and
          (= (import/search)
             (list @native:fixture/places/v1 @native:fixture/weather/v1 @native:time/v1))
          (= (import/search "fixture" "weather") (list @native:fixture/weather/v1))
          (= (import/search "missing") null)
          (import (time @native:time/v1) (= (time/now) 42))))))

  ; Nested definitions override locally, and captured importers retain their scope.
  (assert
    (import/define
      (@native:fixture/value/v1 (list (pair 'value 1)))
      (let
        ((outer import)
         (inner
           (import/define
             (@native:fixture/value/v1 (list (pair 'value 2)))
             (fn ()
               (list
                 (import (value @native:fixture/value/v1) value/value)
                 (outer (value @native:fixture/value/v1) value/value)
                 (import/search "fixture"))))))
        (and
          (= (inner) (list 2 1 (list @native:fixture/value/v1)))
          (import (value @native:fixture/value/v1) (= value/value 1))))))

  ; Neither successful nor failed definition blocks change the outer importer.
  (assert (= (import/search "fixture") null))
  (assert
    (error? (error/catch
      (import/define
        (@native:fixture/value/v1 (list (pair 'value 1)))
        (error/throw "fixture failure")))))
  (assert (= (import/search "fixture") null))
  (assert (error? (error/catch (import (value @native:fixture/value/v1) value/value))))

  ; Restrictions can hide definitions, and fallback retains inherited restrictions.
  (assert
    (import/define
      (@native:fixture/value/v1 (list (pair 'value 1)))
      (import/restrict (@native:json/v1)
        (and
          (= (import/search) (list @native:json/v1))
          (error? (error/catch (import (value @native:fixture/value/v1) value/value)))
          (import (json @native:json/v1) (= (json/encode (json/null)) "null"))))))
  (assert
    (import/restrict ()
      (import/define
        (@native:fixture/value/v1 (list (pair 'value 1)))
        (and
          (= (import/search) (list @native:fixture/value/v1))
          (import (value @native:fixture/value/v1) (= value/value 1))
          (error? (error/catch (import (json @native:json/v1) (json/null))))
          (import/restrict (@native:json/v1 @native:fixture/value/v1)
            (= (import/search) (list @native:fixture/value/v1)))))))

  ; Empty modules and zero declarations are supported, including through fn/apply.
  (assert (= (import/define 42) 42))
  (assert
    (fn/apply import/define
      (list
        (list @native:fixture/empty/v1 '(list))
        '(import (empty @native:fixture/empty/v1) (= (help/env "empty/") null)))))

  ; Export documentation and taints survive local imports and delegated imports.
  (assert
    (import/define
      (@native:fixture/value/v1
        (list
          ; The documented answer.
          (pair 'answer 42)
          (pair 'secret (taint/secret/mark "token"))
          (pair 'read (help/document (fn () 42) "Fixture reader."))))
      (import/define
        (@native:fixture/empty/v1 (list))
        (import (value @native:fixture/value/v1)
          (and
            (= (help 'value/answer) "The documented answer.")
            (= (help 'value/read) "Fixture reader.")
            (= (value/read) 42)
            (taint/secret? value/secret))))))

  ; Definition helpers are discoverable through help.
  (assert (string/contains? (help 'import/define) "(@reference exports-expression) ... body"))
  null)
