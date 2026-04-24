#!/bin/bash
# Suite 05: Workspace Commands
# Tests agm workspace list and show functionality.

# Test agm workspace list
test_start "agm workspace list"
agm_run workspace list
if assert_exit_code 0; then
    test_pass
fi

# Test agm workspace show
test_start "agm workspace show oss"
agm_run workspace show oss
if assert_exit_code 0; then
    test_pass
fi
