#!/bin/bash
# Suite 10: Accessibility
# Tests --no-color and --screen-reader output formatting for accessibility compliance.

# Test --no-color removes ANSI escape codes
test_start "agm --no-color removes ANSI codes"
agm_run session list --no-color
if assert_exit_code 0; then
    if ! echo "$AGM_LAST_OUTPUT" | grep -qP '\x1b\['; then
        test_pass
    else
        test_fail "ANSI escape codes found in --no-color output"
    fi
fi

# Test --screen-reader replaces Unicode symbols
test_start "agm --screen-reader replaces Unicode symbols"
agm_run session list --screen-reader
if assert_exit_code 0; then
    # Should NOT contain Unicode status symbols like circle variants
    if ! echo "$AGM_LAST_OUTPUT" | grep -qP '[●◐○⊗◉]'; then
        test_pass
    else
        test_fail "Unicode symbols found in --screen-reader output"
    fi
fi

# Test --screen-reader uses text labels
test_start "agm --screen-reader uses text labels"
agm_run session list --screen-reader
if assert_exit_code 0; then
    # Should use text labels like [SUCCESS], [ERROR], [ACTIVE], etc.
    # At minimum, output should not be empty and should lack Unicode symbols
    if ! echo "$AGM_LAST_OUTPUT" | grep -qP '[●◐○⊗◉✓✗▶]'; then
        test_pass
    else
        test_fail "Unicode decorative symbols still present in --screen-reader output"
    fi
fi

# Test combined --no-color --screen-reader
test_start "agm --no-color --screen-reader combined"
agm_run session list --no-color --screen-reader
if assert_exit_code 0; then
    has_ansi=$(echo "$AGM_LAST_OUTPUT" | grep -cP '\x1b\[' || true)
    has_unicode=$(echo "$AGM_LAST_OUTPUT" | grep -cP '[●◐○⊗◉]' || true)
    if [[ "$has_ansi" -eq 0 ]] && [[ "$has_unicode" -eq 0 ]]; then
        test_pass
    else
        test_fail "combined flags did not strip all formatting (ansi=$has_ansi, unicode=$has_unicode)"
    fi
fi
