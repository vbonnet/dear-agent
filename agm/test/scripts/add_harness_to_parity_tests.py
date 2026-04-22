#!/usr/bin/env python3
"""
Add a new harness to all AGM parity tests.

This script automates adding a new harness (Codex, OpenCode, Gemini, etc.) to:
- Integration parity tests (BeforeEach + Entry statements)
- BDD feature files (Examples tables)
- Adapter recreation logic (if/else-if patterns)

Usage:
    python3 add_harness_to_parity_tests.py --harness codex
    python3 add_harness_to_parity_tests.py --harness gemini --dry-run

Requirements:
    - Run from claude-session-manager/ directory
    - Harness adapter must exist (e.g., codex_adapter.go)
    - Unit tests should exist (e.g., codex_adapter_test.go)
"""

import argparse
import re
import os
import sys
from pathlib import Path
from typing import List, Tuple


class HarnessParity:
    """Add harness to AGM parity tests."""

    def __init__(self, harness: str, dry_run: bool = False):
        self.harness = harness
        self.harness_title = harness.capitalize()
        self.dry_run = dry_run
        self.changes_made = []

    def add_to_integration_tests(self) -> None:
        """Add harness to integration parity test files."""
        test_files = [
            "test/integration/agent_parity_integration_test.go",
            "test/integration/agent_parity_session_management_test.go",
            "test/integration/agent_parity_messaging_test.go",
            "test/integration/agent_parity_data_exchange_test.go",
            "test/integration/agent_parity_capabilities_test.go",
            "test/integration/agent_parity_commands_test.go",
        ]

        for test_file in test_files:
            if not os.path.exists(test_file):
                print(f"⚠️  Warning: {test_file} not found, skipping")
                continue

            self._update_test_file(test_file)

    def _update_test_file(self, filepath: str) -> None:
        """Update a single test file with harness entries."""
        with open(filepath, 'r') as f:
            content = f.read()

        original_content = content

        # 1. Add to BeforeEach if not present
        if f'{self.harness}Adapter' not in content:
            content = self._add_to_before_each(content)

        # 2. Add Entry statements
        content = self._add_entry_statements(content)

        # 3. Update adapter recreation logic
        content = self._update_adapter_recreation(content)

        if content != original_content:
            if not self.dry_run:
                with open(filepath, 'w') as f:
                    f.write(content)
            self.changes_made.append(f"✓ Updated {filepath}")
        else:
            print(f"  No changes needed for {filepath}")

    def _add_to_before_each(self, content: str) -> str:
        """Add harness adapter to BeforeEach block."""
        # Find the last adapter initialization
        pattern = r'(adapters\["(?:claude|gemini|opencode|codex)"\] = \w+Adapter)\n(\t}\))'

        harness_init = f"""
\t\t// {self.harness_title} adapter
\t\t{self.harness}Adapter, err := agent.New{self.harness_title}Adapter(nil)
\t\tExpect(err).ToNot(HaveOccurred())
\t\tadapters["{self.harness}"] = {self.harness}Adapter"""

        def replacer(match):
            return match.group(1) + harness_init + "\n" + match.group(2)

        return re.sub(pattern, replacer, content)

    def _add_entry_statements(self, content: str) -> str:
        """Add Entry statements after existing entries."""
        # Pattern: Add after last Entry("...", "...")
        pattern = r'(Entry\("(?:claude|gemini|opencode|codex) agent", "(?:claude|gemini|opencode|codex)"\),)\n(\t\t\))'

        new_entry = f'\t\t\tEntry("{self.harness} agent", "{self.harness}"),'

        def replacer(match):
            # Only add if not already present
            existing_entries = match.group(0)
            if f'Entry("{self.harness} agent"' in existing_entries:
                return match.group(0)
            return match.group(1) + '\n' + new_entry + '\n' + match.group(2)

        return re.sub(pattern, replacer, content)

    def _update_adapter_recreation(self, content: str) -> str:
        """Update adapter recreation if/else-if logic."""
        # Find adapter recreation block
        pattern = r'(if agentName == "claude" \{[^}]+\} else if agentName == "(?:gemini|opencode|codex)" \{[^}]+\})'

        # Add new else-if clause
        new_clause = f''' else if agentName == "{self.harness}" {{
\t\t\t\tnewAdapter, err = agent.New{self.harness_title}Adapter(nil)
\t\t\t}}'''

        def replacer(match):
            existing = match.group(0)
            if f'agentName == "{self.harness}"' in existing:
                return existing
            return existing + new_clause

        return re.sub(pattern, replacer, content)

    def add_to_bdd_features(self) -> None:
        """Add harness to BDD feature Examples tables."""
        feature_files = list(Path("test/bdd/features").glob("*.feature"))

        for feature_file in feature_files:
            content = feature_file.read_text()
            original_content = content

            # Find Examples tables with | agent | header
            if '| agent' in content and f'| {self.harness}' not in content:
                content = self._add_to_examples_table(content)

            if content != original_content:
                if not self.dry_run:
                    feature_file.write_text(content)
                self.changes_made.append(f"✓ Updated {feature_file}")

    def _add_to_examples_table(self, content: str) -> str:
        """Add harness to Examples tables."""
        lines = content.split('\n')
        updated_lines = []
        in_examples = False
        added_harness = False

        for line in lines:
            updated_lines.append(line)

            if 'Examples:' in line:
                in_examples = True
                added_harness = False
            elif in_examples and '| codex' in line and not added_harness:
                # Add after codex
                updated_lines.append(f'      | {self.harness:<8} |')
                added_harness = True
            elif in_examples and '| gemini' in line and '| codex' not in content:
                # If no codex, add after gemini
                if not added_harness:
                    updated_lines.append(f'      | {self.harness:<8} |')
                    added_harness = True
            elif in_examples and line.strip() == '':
                in_examples = False

        return '\n'.join(updated_lines)

    def create_contract_tests(self) -> None:
        """Create contract test file for harness."""
        contract_file = f"test/contract/{self.harness}_contract_test.go"

        if os.path.exists(contract_file):
            print(f"  Contract test {contract_file} already exists, skipping")
            return

        # Use gemini_contract_test.go as template
        template_file = "test/contract/gemini_contract_test.go"
        if not os.path.exists(template_file):
            print(f"⚠️  Warning: Template {template_file} not found, skipping contract test creation")
            return

        with open(template_file, 'r') as f:
            content = f.read()

        # Replace gemini with harness name
        content = content.replace('Gemini', self.harness_title)
        content = content.replace('gemini', self.harness)
        content = content.replace('GEMINI', self.harness.upper())

        # Update API key references (harness-specific)
        if self.harness == 'codex':
            content = content.replace('GOOGLE_API_KEY', 'OPENAI_API_KEY')

        if not self.dry_run:
            with open(contract_file, 'w') as f:
                f.write(content)

        self.changes_made.append(f"✓ Created {contract_file}")

    def validate_harness_exists(self) -> bool:
        """Validate that harness adapter code exists."""
        adapter_file = f"internal/agent/{self.harness}_adapter.go"
        test_file = f"internal/agent/{self.harness}_adapter_test.go"

        if not os.path.exists(adapter_file):
            print(f"❌ Error: {adapter_file} not found")
            print(f"   Please implement the {self.harness} adapter first")
            return False

        if not os.path.exists(test_file):
            print(f"⚠️  Warning: {test_file} not found")
            print(f"   Consider creating unit tests first")

        return True

    def run(self) -> bool:
        """Execute all parity updates."""
        if not self.validate_harness_exists():
            return False

        print(f"\n{'DRY RUN: ' if self.dry_run else ''}Adding {self.harness} to AGM parity tests...\n")

        print("1. Updating integration parity tests...")
        self.add_to_integration_tests()

        print("\n2. Updating BDD feature files...")
        self.add_to_bdd_features()

        print("\n3. Creating contract tests...")
        self.create_contract_tests()

        print("\n" + "="*60)
        print(f"\n{'DRY RUN ' if self.dry_run else ''}Summary:")
        if self.changes_made:
            for change in self.changes_made:
                print(f"  {change}")
            print(f"\nTotal changes: {len(self.changes_made)}")
        else:
            print("  No changes needed - harness already added to all tests")

        if self.dry_run:
            print("\nℹ️  This was a dry run. Run without --dry-run to apply changes.")
        else:
            print("\n✅ Harness successfully added to parity tests!")
            print("\nNext steps:")
            print(f"  1. Review changes: git diff")
            print(f"  2. Run tests: go test -tags=integration ./test/integration/...")
            print(f"  3. Update BDD tests manually if needed")
            print(f"  4. Commit: git add -A && git commit -m 'test: Add {self.harness} to parity tests'")

        return True


def main():
    parser = argparse.ArgumentParser(
        description="Add new harness to AGM parity tests",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python3 add_harness_to_parity_tests.py --harness codex
  python3 add_harness_to_parity_tests.py --harness gemini --dry-run
        """
    )
    parser.add_argument('--harness', required=True,
                       help='Harness name (e.g., codex, gemini, opencode)')
    parser.add_argument('--dry-run', action='store_true',
                       help='Show what would be changed without modifying files')

    args = parser.parse_args()

    # Validate we're in the right directory
    if not os.path.exists('test/integration'):
        print("❌ Error: Must run from claude-session-manager/ directory")
        sys.exit(1)

    parity = HarnessParity(args.harness.lower(), args.dry_run)
    success = parity.run()

    sys.exit(0 if success else 1)


if __name__ == '__main__':
    main()
