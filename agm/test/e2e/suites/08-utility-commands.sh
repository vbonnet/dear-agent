#!/bin/bash
# Suite 08: Utility Commands
# Tests agm version and output formatting flags.

# Test agm version
test_start "agm version outputs version number"
agm_run version
if assert_exit_code 0; then
    if assert_output_contains "[0-9]+\.[0-9]+"; then
        test_pass
    else
        test_fail "version output does not contain a version number"
    fi
fi

# Test --no-color removes ANSI escape codes
test_start "agm session list --no-color has no ANSI codes"
agm_run session list --no-color
if assert_exit_code 0; then
    if ! echo "$AGM_LAST_OUTPUT" | grep -qP '\x1b\['; then
        test_pass
    else
        test_fail "ANSI escape codes found in --no-color output"
    fi
fi

# Test --screen-reader replaces Unicode symbols with text labels
test_start "agm session list --screen-reader uses text labels"
agm_run session list --screen-reader
if assert_exit_code 0; then
    # Should NOT contain Unicode status symbols
    if ! echo "$AGM_LAST_OUTPUT" | grep -qP '[●◐○⊗◉]'; then
        test_pass
    else
        test_fail "Unicode symbols found in --screen-reader output"
    fi
fi
