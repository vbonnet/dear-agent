#!/usr/bin/env python3
"""Lazy Loading Implementation
Only loads skills when first invoked, not at marketplace startup
"""

import importlib.util
import sys
import time
from pathlib import Path
from typing import Any, Callable, Dict, Optional
from dataclasses import dataclass
from enum import Enum


class LoadStatus(str, Enum):
    """Skill load status"""
    DEFERRED = "deferred"
    LOADED = "loaded"
    FAILED = "failed"


@dataclass
class LazySkill:
    """Lazy-loaded skill metadata"""
    name: str
    path: Path
    status: LoadStatus = LoadStatus.DEFERRED
    load_time_ms: int = 0
    module: Optional[Any] = None


class LazyLoader:
    """Lazy loading manager for skills"""

    def __init__(self):
        self._registry: Dict[str, LazySkill] = {}
        self._deferred_count = 0
        self._executed_count = 0

    def register(self, skill_name: str, skill_path: str) -> bool:
        """Register skill for lazy loading

        Args:
            skill_name: Name of skill
            skill_path: Path to skill module

        Returns:
            True if registered successfully
        """
        path = Path(skill_path)

        if not path.exists():
            print(f"Error: Skill path does not exist: {skill_path}", file=sys.stderr)
            return False

        self._registry[skill_name] = LazySkill(
            name=skill_name,
            path=path
        )
        self._deferred_count += 1

        return True

    def load(self, skill_name: str) -> bool:
        """Load skill on-demand

        Args:
            skill_name: Name of skill to load

        Returns:
            True if loaded successfully
        """
        # Check if already loaded
        if skill_name in self._registry:
            skill = self._registry[skill_name]

            if skill.status == LoadStatus.LOADED:
                return True

            # Load the skill
            start_time = time.time()

            try:
                # Import module
                spec = importlib.util.spec_from_file_location(
                    skill_name,
                    skill.path
                )

                if spec is None or spec.loader is None:
                    raise ImportError(f"Cannot load spec for {skill_name}")

                module = importlib.util.module_from_spec(spec)
                sys.modules[skill_name] = module
                spec.loader.exec_module(module)

                # Update skill metadata
                end_time = time.time()
                duration_ms = int((end_time - start_time) * 1000)

                skill.module = module
                skill.status = LoadStatus.LOADED
                skill.load_time_ms = duration_ms
                self._executed_count += 1

                return True

            except Exception as e:
                print(f"Error: Failed to load skill {skill_name}: {e}", file=sys.stderr)
                skill.status = LoadStatus.FAILED
                return False

        else:
            print(f"Error: Skill not registered: {skill_name}", file=sys.stderr)
            return False

    def invoke(self, skill_name: str, function_name: str, *args, **kwargs) -> Any:
        """Invoke skill function with lazy loading

        Args:
            skill_name: Name of skill
            function_name: Function to call
            *args: Positional arguments
            **kwargs: Keyword arguments

        Returns:
            Function result
        """
        # Lazy load if needed
        if skill_name not in self._registry or \
           self._registry[skill_name].status != LoadStatus.LOADED:
            if not self.load(skill_name):
                raise RuntimeError(f"Failed to load skill: {skill_name}")

        # Get module
        skill = self._registry[skill_name]
        module = skill.module

        # Check if function exists
        if not hasattr(module, function_name):
            raise AttributeError(
                f"Skill {skill_name} has no function {function_name}"
            )

        # Invoke function
        func = getattr(module, function_name)
        return func(*args, **kwargs)

    def preload(self, *skill_names: str) -> None:
        """Preload critical skills

        Args:
            *skill_names: Names of skills to preload
        """
        for skill_name in skill_names:
            if skill_name in self._registry:
                skill = self._registry[skill_name]
                if skill.status == LoadStatus.DEFERRED:
                    self.load(skill_name)

    def unload(self, skill_name: str) -> None:
        """Unload skill (free memory)

        Args:
            skill_name: Name of skill to unload
        """
        if skill_name in self._registry:
            skill = self._registry[skill_name]

            if skill.status == LoadStatus.LOADED:
                # Remove from sys.modules
                if skill_name in sys.modules:
                    del sys.modules[skill_name]

                # Reset status
                skill.status = LoadStatus.DEFERRED
                skill.module = None

    def stats(self) -> Dict[str, Any]:
        """Get lazy load statistics

        Returns:
            Dictionary of statistics
        """
        total_registered = len(self._registry)
        currently_loaded = sum(
            1 for s in self._registry.values()
            if s.status == LoadStatus.LOADED
        )
        deferred_count = total_registered - currently_loaded

        # Calculate average load time
        load_times = [
            s.load_time_ms for s in self._registry.values()
            if s.load_time_ms > 0
        ]
        avg_load_time = (
            sum(load_times) // len(load_times)
            if load_times else 0
        )

        deferred_percent = (
            round((deferred_count / total_registered) * 100, 2)
            if total_registered > 0 else 0
        )

        return {
            "total_registered": total_registered,
            "currently_loaded": currently_loaded,
            "deferred_count": deferred_count,
            "deferred_percent": deferred_percent,
            "lazy_loads_executed": self._executed_count,
            "avg_load_time_ms": avg_load_time
        }

    def print_stats(self) -> None:
        """Print lazy load statistics"""
        stats = self.stats()

        print("=== Lazy Load Statistics ===")
        for key, value in stats.items():
            label = key.replace('_', ' ').title()
            print(f"{label:<25}: {value}")


# Global lazy loader instance
_global_loader: Optional[LazyLoader] = None


def get_loader() -> LazyLoader:
    """Get global lazy loader instance

    Returns:
        Global LazyLoader instance
    """
    global _global_loader

    if _global_loader is None:
        _global_loader = LazyLoader()

    return _global_loader
