#!/usr/bin/env python3
"""
Engram MCP Server - Enhanced

Provides MCP tools for:
- Task 3.2 (Basic): engram.retrieve(), engram.plugins.list(), wayfinder.phase.status()
- Task 3.5 (Enhanced): beads.create(), advanced ecphory, performance profiling

Part of workflow-improvements-2026 Phase 3.
"""

import sys
import json
import logging
from pathlib import Path
from typing import Dict, Any, List

# Ensure the script's directory is on sys.path so that the tools/ package
# and performance.py are importable regardless of the working directory
# when this script is launched (e.g. by the MCP host via absolute path).
sys.path.insert(0, str(Path(__file__).parent))

# Import tool implementations
from tools.engram_retrieve import EngramRetrieve
from tools.beads_create import BeadsCreate
from tools.plugins_list import PluginsList
from tools.wayfinder_status import WayfinderStatus
from performance import PerformanceProfiler

# Configure logging (stderr only for stdio transport)
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    stream=sys.stderr
)
logger = logging.getLogger('engram-mcp-server')


class EngramMCPServer:
    """MCP server exposing Engram tools."""

    def __init__(self, engram_root: Path = None, beads_db: Path = None):
        """Initialize Engram MCP server.

        Args:
            engram_root: Root directory of Engram repository
                        (defaults to ~/src/ws/oss/repos/engram)
            beads_db: Path to beads database
                     (defaults to ~/.beads/issues.jsonl)
        """
        if engram_root is None:
            engram_root = Path.home() / "src/engram"

        if beads_db is None:
            beads_db = Path.home() / ".beads/issues.jsonl"

        self.engram_root = Path(engram_root)
        self.beads_db = Path(beads_db)

        # Initialize tools
        self.engram_retrieve = EngramRetrieve(self.engram_root)
        self.beads_create = BeadsCreate(self.beads_db)
        self.plugins_list = PluginsList(self.engram_root)
        self.wayfinder_status = WayfinderStatus()

        # Performance profiling (Task 3.5)
        self.profiler = PerformanceProfiler()

        # Server metadata
        self.name = "engram-mcp-server"
        self.version = "1.1.0"  # 1.0 = Task 3.2, 1.1 = Task 3.5 enhancements

        logger.info(f"Engram MCP Server initialized: {self.name} v{self.version}")
        logger.info(f"Engram root: {self.engram_root}")
        logger.info(f"Beads database: {self.beads_db}")

    def handle_request(self, request: Dict) -> Dict:
        """Handle MCP JSON-RPC request.

        Args:
            request: JSON-RPC request object

        Returns:
            JSON-RPC response object
        """
        method = request.get('method')
        params = request.get('params', {})
        request_id = request.get('id')

        logger.debug(f"Request: method={method}, params={params}")

        # MCP protocol methods
        if method == 'initialize':
            return self._handle_initialize(request_id, params)
        elif method == 'tools/list':
            return self._handle_list_tools(request_id)
        elif method == 'tools/call':
            return self._handle_call_tool(request_id, params)
        else:
            return self._error_response(request_id, -32601, f"Method not found: {method}")

    def _handle_initialize(self, request_id: int, params: Dict) -> Dict:
        """Handle initialize request."""
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {
                "protocolVersion": "0.1.0",
                "serverInfo": {
                    "name": self.name,
                    "version": self.version
                },
                "capabilities": {
                    "tools": {}
                }
            }
        }

    def _handle_list_tools(self, request_id: int) -> Dict:
        """Handle tools/list request."""
        tools = [
            # Task 3.2: Basic tools
            {
                "name": "engram_retrieve",
                "description": "Retrieve relevant engrams using semantic search. Supports .ai.md (actionable instructions) and .why.md (rationale) engrams.",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "query": {
                            "type": "string",
                            "description": "Search query (e.g., 'error handling patterns', 'testing best practices')"
                        },
                        "type_filter": {
                            "type": "string",
                            "enum": ["ai", "why", "all"],
                            "description": "Filter by engram type: 'ai' (.ai.md), 'why' (.why.md), or 'all' (default: all)",
                            "default": "all"
                        },
                        "top_k": {
                            "type": "integer",
                            "description": "Number of results to return (default: 5, max: 20)",
                            "minimum": 1,
                            "maximum": 20,
                            "default": 5
                        }
                    },
                    "required": ["query"]
                }
            },
            {
                "name": "engram_plugins_list",
                "description": "List installed Engram plugins with metadata (name, version, status, description).",
                "inputSchema": {
                    "type": "object",
                    "properties": {},
                    "required": []
                }
            },
            {
                "name": "wayfinder_phase_status",
                "description": "Get current phase status of a Wayfinder project (phase number, completion, deliverables).",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "project_path": {
                            "type": "string",
                            "description": "Path to Wayfinder project directory (e.g., ~/src/ws/oss/wf/my-project)"
                        }
                    },
                    "required": ["project_path"]
                }
            },
            # Task 3.5: Enhanced tools
            {
                "name": "beads_create",
                "description": "Create a new bead (issue/task) in the beads database. Validates against duplicates and returns bead ID.",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "title": {
                            "type": "string",
                            "description": "Bead title (brief, imperative form, e.g., 'Fix authentication bug')"
                        },
                        "description": {
                            "type": "string",
                            "description": "Detailed description with context, acceptance criteria, and deliverables"
                        },
                        "priority": {
                            "type": "integer",
                            "description": "Priority level (0=P0/highest, 1=P1, 2=P2, etc.)",
                            "minimum": 0,
                            "maximum": 5,
                            "default": 1
                        },
                        "labels": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Labels/tags for categorization (e.g., ['bug', 'authentication', 'p0'])",
                            "default": []
                        },
                        "estimated_minutes": {
                            "type": "integer",
                            "description": "Estimated time in minutes (e.g., 60 for 1 hour, 480 for 1 day)",
                            "minimum": 1,
                            "default": 60
                        }
                    },
                    "required": ["title", "description"]
                }
            }
        ]

        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {
                "tools": tools
            }
        }

    def _handle_call_tool(self, request_id: int, params: Dict) -> Dict:
        """Handle tools/call request with performance profiling."""
        tool_name = params.get('name')
        arguments = params.get('arguments', {})

        logger.info(f"Calling tool: {tool_name}")

        try:
            # Start performance profiling (Task 3.5)
            with self.profiler.profile(tool_name):
                # Route to appropriate tool
                if tool_name == 'engram_retrieve':
                    result = self.engram_retrieve.retrieve(
                        query=arguments['query'],
                        type_filter=arguments.get('type_filter', 'all'),
                        top_k=arguments.get('top_k', 5)
                    )
                    content = json.dumps(result, indent=2)

                elif tool_name == 'engram_plugins_list':
                    result = self.plugins_list.list_plugins()
                    content = json.dumps(result, indent=2)

                elif tool_name == 'wayfinder_phase_status':
                    result = self.wayfinder_status.get_status(
                        project_path=arguments['project_path']
                    )
                    content = json.dumps(result, indent=2)

                elif tool_name == 'beads_create':
                    result = self.beads_create.create(
                        title=arguments['title'],
                        description=arguments['description'],
                        priority=arguments.get('priority', 1),
                        labels=arguments.get('labels', []),
                        estimated_minutes=arguments.get('estimated_minutes', 60)
                    )
                    content = json.dumps(result, indent=2)

                else:
                    return self._error_response(request_id, -32601, f"Unknown tool: {tool_name}")

            # Log performance stats
            stats = self.profiler.get_stats()
            if tool_name in stats:
                logger.info(f"Tool {tool_name} latency: {stats[tool_name]['avg_ms']:.2f}ms (avg)")

            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "content": [
                        {
                            "type": "text",
                            "text": content
                        }
                    ]
                }
            }

        except ValueError as e:
            logger.error(f"Validation error: {e}")
            return self._error_response(request_id, -32602, str(e))

        except Exception as e:
            logger.error(f"Tool execution error: {e}", exc_info=True)
            return self._error_response(request_id, -32603, f"Tool execution failed: {e}")

    def _error_response(self, request_id: int, code: int, message: str) -> Dict:
        """Create JSON-RPC error response."""
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {
                "code": code,
                "message": message
            }
        }

    def run(self):
        """Run MCP server with stdio transport."""
        logger.info("Starting Engram MCP Server (stdio transport)")

        try:
            for line in sys.stdin:
                line = line.strip()

                if not line:
                    continue

                try:
                    request = json.loads(line)
                    response = self.handle_request(request)
                    print(json.dumps(response), flush=True)

                except json.JSONDecodeError as e:
                    logger.error(f"JSON decode error: {e}")
                    error_response = self._error_response(None, -32700, "Parse error")
                    print(json.dumps(error_response), flush=True)

                except Exception as e:
                    logger.error(f"Request handling error: {e}", exc_info=True)
                    error_response = self._error_response(None, -32603, str(e))
                    print(json.dumps(error_response), flush=True)

        except KeyboardInterrupt:
            logger.info("Server shutdown requested")

        except Exception as e:
            logger.error(f"Fatal error: {e}", exc_info=True)
            sys.exit(1)

        logger.info("Server stopped")


def main():
    """Main entry point."""
    import argparse

    parser = argparse.ArgumentParser(description="Engram MCP Server")
    parser.add_argument(
        '--engram-root',
        type=Path,
        default=Path.home() / "src/engram",
        help='Engram repository root directory'
    )
    parser.add_argument(
        '--beads-db',
        type=Path,
        default=Path.home() / ".beads/issues.jsonl",
        help='Beads database path'
    )

    args = parser.parse_args()

    server = EngramMCPServer(
        engram_root=args.engram_root,
        beads_db=args.beads_db
    )
    server.run()


if __name__ == '__main__':
    main()
