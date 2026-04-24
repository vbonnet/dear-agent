#!/usr/bin/env python3
"""Skill Caching Implementation
Provides in-memory caching to avoid redundant skill loads
"""

import os
import time
from typing import Any, Dict, Optional, Tuple
from dataclasses import dataclass, field
from datetime import datetime


@dataclass
class CacheEntry:
    """Cache entry with metadata"""
    data: Any
    load_time: float
    invocation_count: int = 0


class SkillCache:
    """In-memory cache for skill data"""

    def __init__(self, max_size: int = 50, ttl: int = 3600):
        """Initialize skill cache

        Args:
            max_size: Maximum number of entries
            ttl: Time-to-live in seconds (default 1 hour)
        """
        self.enabled = os.getenv('SKILL_CACHE_ENABLED', '1') == '1'
        self.max_size = int(os.getenv('SKILL_CACHE_MAX_SIZE', str(max_size)))
        self.ttl = int(os.getenv('SKILL_CACHE_TTL', str(ttl)))

        self._cache: Dict[str, CacheEntry] = {}
        self._hits = 0
        self._misses = 0

    def key(self, skill_name: str, skill_version: str = "latest") -> str:
        """Generate cache key

        Args:
            skill_name: Name of skill
            skill_version: Version of skill

        Returns:
            Cache key string
        """
        return f"{skill_name}:{skill_version}"

    def is_cached(self, cache_key: str) -> bool:
        """Check if skill is cached and not expired

        Args:
            cache_key: Cache key

        Returns:
            True if cached and valid
        """
        if not self.enabled:
            return False

        if cache_key not in self._cache:
            return False

        # Check TTL
        entry = self._cache[cache_key]
        age = time.time() - entry.load_time

        if age >= self.ttl:
            # Expired - remove from cache
            del self._cache[cache_key]
            return False

        return True

    def get(self, cache_key: str) -> Optional[Any]:
        """Get cached skill data

        Args:
            cache_key: Cache key

        Returns:
            Cached data or None if not found
        """
        if not self.is_cached(cache_key):
            self._misses += 1
            return None

        entry = self._cache[cache_key]
        entry.invocation_count += 1
        self._hits += 1

        return entry.data

    def set(self, cache_key: str, data: Any) -> None:
        """Cache skill data

        Args:
            cache_key: Cache key
            data: Data to cache
        """
        if not self.enabled:
            return

        # Check size limit
        if len(self._cache) >= self.max_size:
            # Evict oldest entry (LRU-like)
            oldest_key = min(
                self._cache.keys(),
                key=lambda k: self._cache[k].load_time
            )
            del self._cache[oldest_key]

        # Store in cache
        self._cache[cache_key] = CacheEntry(
            data=data,
            load_time=time.time(),
            invocation_count=0
        )

    def clear(self, cache_key: Optional[str] = None) -> None:
        """Clear cache

        Args:
            cache_key: Specific key to clear, or None to clear all
        """
        if cache_key:
            if cache_key in self._cache:
                del self._cache[cache_key]
        else:
            self._cache.clear()
            self._hits = 0
            self._misses = 0

    def stats(self) -> Dict[str, Any]:
        """Get cache statistics

        Returns:
            Dictionary of statistics
        """
        total_requests = self._hits + self._misses
        hit_rate = (self._hits / total_requests * 100) if total_requests > 0 else 0

        return {
            "cache_enabled": self.enabled,
            "cache_size": len(self._cache),
            "cache_max_size": self.max_size,
            "cache_ttl": self.ttl,
            "cache_hits": self._hits,
            "cache_misses": self._misses,
            "total_requests": total_requests,
            "hit_rate_percent": round(hit_rate, 2)
        }

    def print_stats(self) -> None:
        """Print cache statistics (human-readable)"""
        stats = self.stats()

        print("=== Skill Cache Statistics ===")
        for key, value in stats.items():
            label = key.replace('_', ' ').title()
            print(f"{label:<25}: {value}")

    def export_stats(self, output_file: str) -> None:
        """Export cache statistics to JSON file

        Args:
            output_file: Path to output file
        """
        import json

        with open(output_file, 'w') as f:
            json.dump(self.stats(), f, indent=2)


# Global cache instance
_global_cache: Optional[SkillCache] = None


def get_cache() -> SkillCache:
    """Get global cache instance

    Returns:
        Global SkillCache instance
    """
    global _global_cache

    if _global_cache is None:
        _global_cache = SkillCache()

    return _global_cache
