#!/usr/bin/env sh
set -eu

ASSETS=$1
GATEHOUSE=${2:-gatehouse}

check() {
    condition=$1
    program=$2
    # Quote the program, then evaluate the prelude around it.
    result=$(
        {
            printf "(let ((program '\n"
            cat "$program"
            printf '))\n'
            cat "$ASSETS/preludes/$condition.lisp"
            printf ')\n'
        } | "$GATEHOUSE" lisp eval --from-stdin
    )
    [ "$result" = null ]
}

for condition in pre-imported self-import; do
    check "$condition" "$ASSETS/checks/$condition.lisp"
    check "$condition" "$ASSETS/checks/fixtures.lisp"
    # A second, fresh evaluation must reconstruct the same environment.
    check "$condition" "$ASSETS/checks/$condition.lisp"
    check "$condition" "$ASSETS/checks/fixtures.lisp"
    printf 'offline fixture/prelude checks passed: %s\n' "$condition"
done
