#!/usr/bin/env python3
"""
Integration tests for Engram MCP Server

Tests all tools from Task 3.2 and Task 3.5.
"""

import json
import sys
import subprocess
from pathlib import Path
from datetime import datetime

# Test cases
TEST_CASES = [
    {
        "name": "tools/list - List available tools",
        "request": {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "tools/list"
        },
        "expected_tools": ["engram_retrieve", "engram_plugins_list", "wayfinder_phase_status", "beads_create"]
    },
    {
        "name": "engram_retrieve - Basic query",
        "request": {
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/call",
            "params": {
                "name": "engram_retrieve",
                "arguments": {
                    "query": "error handling",
                    "top_k": 3
                }
            }
        },
        "expect_success": True
    },
    {
        "name": "engram_retrieve - Type filter (ai)",
        "request": {
            "jsonrpc": "2.0",
            "id": 3,
            "method": "tools/call",
            "params": {
                "name": "engram_retrieve",
                "arguments": {
                    "query": "testing patterns",
                    "type_filter": "ai",
                    "top_k": 5
                }
            }
        },
        "expect_success": True
    },
    {
        "name": "engram_plugins_list - List plugins",
        "request": {
            "jsonrpc": "2.0",
            "id": 4,
            "method": "tools/call",
            "params": {
                "name": "engram_plugins_list",
                "arguments": {}
            }
        },
        "expect_success": True
    },
    {
        "name": "wayfinder_phase_status - Valid project (if exists)",
        "request": {
            "jsonrpc": "2.0",
            "id": 5,
            "method": "tools/call",
            "params": {
                "name": "wayfinder_phase_status",
                "arguments": {
                    "project_path": "~/src/ws/oss/wf/workflow-improvements-batch-edit"
                }
            }
        },
        "expect_success": True,
        "allow_failure": True  # May not exist on all systems
    },
    {
        "name": "beads_create - Create new bead",
        "request": {
            "jsonrpc": "2.0",
            "id": 6,
            "method": "tools/call",
            "params": {
                "name": "beads_create",
                "arguments": {
                    "title": f"MCP Test Bead {datetime.now().isoformat()}",
                    "description": "Test bead created by MCP server integration tests. Safe to delete.",
                    "priority": 2,
                    "labels": ["test", "mcp-server"],
                    "estimated_minutes": 15
                }
            }
        },
        "expect_success": True,
        "unique_title": True  # Will be populated at runtime
    },
    {
        "name": "beads_create - Duplicate detection",
        "request": {
            "jsonrpc": "2.0",
            "id": 7,
            "method": "tools/call",
            "params": {
                "name": "beads_create",
                "arguments": {
                    "title": "DUPLICATE_TITLE_PLACEHOLDER",
                    "description": "Duplicate test - should fail",
                    "priority": 1
                }
            }
        },
        "expect_error": True,  # Should detect duplicate
        "use_previous_title": True  # Will use title from previous test
    },
    {
        "name": "beads_create - Invalid priority",
        "request": {
            "jsonrpc": "2.0",
            "id": 8,
            "method": "tools/call",
            "params": {
                "name": "beads_create",
                "arguments": {
                    "title": "Invalid Priority Test",
                    "description": "Test with invalid priority",
                    "priority": 10  # Invalid (max is 5)
                }
            }
        },
        "expect_error": True
    }
]


def run_test(test_case: dict, server_process):
    """Run a single test case."""
    print(f"\nTest: {test_case['name']}")
    print(f"Request: {json.dumps(test_case['request'], indent=2)}")

    # Send request
    request_json = json.dumps(test_case['request']) + '\n'
    server_process.stdin.write(request_json)
    server_process.stdin.flush()

    # Read response
    response_line = server_process.stdout.readline()
    response = json.loads(response_line)

    print(f"Response: {json.dumps(response, indent=2)}")

    # Validate response
    if test_case.get('expect_error'):
        if 'error' in response:
            print("✅ PASS: Error response as expected")
            return True
        else:
            print("❌ FAIL: Expected error response, got success")
            return False

    if test_case.get('expect_success'):
        if 'result' in response:
            print("✅ PASS: Success response")
            return True
        elif test_case.get('allow_failure'):
            print("⚠️  SKIP: Optional test (resource may not exist)")
            return True
        else:
            print(f"❌ FAIL: Expected success, got error: {response.get('error')}")
            return False

    # Check for expected tools (tools/list)
    if test_case.get('expected_tools'):
        if 'result' not in response:
            print(f"❌ FAIL: No result in response")
            return False

        tools = response['result'].get('tools', [])
        tool_names = [tool['name'] for tool in tools]

        for expected_tool in test_case['expected_tools']:
            if expected_tool in tool_names:
                print(f"✅ Found tool: {expected_tool}")
            else:
                print(f"❌ FAIL: Missing expected tool: {expected_tool}")
                return False

        print("✅ PASS: All expected tools found")
        return True

    return True


def main():
    """Run all tests."""
    print("="  * 60)
    print("Engram MCP Server Integration Tests")
    print("=" * 60)

    # Start MCP server
    server_script = Path(__file__).parent / "engram_mcp_server.py"

    if not server_script.exists():
        print(f"❌ ERROR: Server script not found: {server_script}")
        sys.exit(1)

    print(f"\nStarting MCP server: {server_script}")

    # Use virtual environment Python if available
    venv_python = Path(__file__).parent / ".venv/bin/python3"
    python_executable = str(venv_python) if venv_python.exists() else sys.executable

    print(f"Using Python: {python_executable}")

    server_process = subprocess.Popen(
        [python_executable, str(server_script)],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        bufsize=1
    )

    try:
        # Run initialize handshake
        init_request = {
            "jsonrpc": "2.0",
            "id": 0,
            "method": "initialize",
            "params": {}
        }

        print(f"\nInitializing MCP server...")
        server_process.stdin.write(json.dumps(init_request) + '\n')
        server_process.stdin.flush()

        init_response = server_process.stdout.readline()
        init_data = json.loads(init_response)

        if 'result' in init_data:
            print(f"✅ Server initialized: {init_data['result']['serverInfo']}")
        else:
            print(f"❌ Initialization failed: {init_data}")
            sys.exit(1)

        # Run test cases
        passed = 0
        failed = 0
        skipped = 0
        last_created_title = None

        for test_case in TEST_CASES:
            try:
                # Handle unique title generation for beads_create tests
                if test_case.get('unique_title'):
                    # Generate unique title with timestamp
                    unique_title = f"MCP Test Bead {datetime.now().isoformat()}"
                    test_case['request']['params']['arguments']['title'] = unique_title
                    last_created_title = unique_title
                elif test_case.get('use_previous_title') and last_created_title:
                    # Use title from previous test for duplicate detection
                    test_case['request']['params']['arguments']['title'] = last_created_title

                result = run_test(test_case, server_process)
                if result:
                    if test_case.get('allow_failure'):
                        skipped += 1
                    else:
                        passed += 1
                else:
                    failed += 1
            except Exception as e:
                print(f"❌ ERROR running test: {e}")
                failed += 1

        # Summary
        print("\n" + "=" * 60)
        print("Test Summary")
        print("=" * 60)
        print(f"Passed:  {passed}")
        print(f"Failed:  {failed}")
        print(f"Skipped: {skipped}")
        print(f"Total:   {passed + failed + skipped}")

        if failed == 0:
            print("\n✅ All tests passed!")
            return 0
        else:
            print(f"\n❌ {failed} test(s) failed")
            return 1

    finally:
        # Cleanup
        server_process.terminate()
        server_process.wait(timeout=5)


if __name__ == '__main__':
    sys.exit(main())
