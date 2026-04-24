"""CLI Abstraction Library for Spec Review Marketplace"""

from .cli_detector import detect_cli, get_cli_version, cli_supports_feature, CLI_TYPE
from .cli_abstraction import CLIAbstraction, cli

__all__ = [
    "detect_cli",
    "get_cli_version",
    "cli_supports_feature",
    "CLI_TYPE",
    "CLIAbstraction",
    "cli",
]
