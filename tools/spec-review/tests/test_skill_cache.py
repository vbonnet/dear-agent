#!/usr/bin/env python3
"""Tests for skill_cache module"""

import sys
import time
from pathlib import Path

# Add lib to path
sys.path.insert(0, str(Path(__file__).parent.parent / "lib"))

from skill_cache import SkillCache


def test_cache_initialization():
    """Test cache initialization"""
    cache = SkillCache(max_size=10, ttl=60)

    assert cache.max_size == 10
    assert cache.ttl == 60
    assert len(cache._cache) == 0
    print("✅ PASS: Cache initialization")


def test_cache_key_generation():
    """Test cache key generation"""
    cache = SkillCache()

    key1 = cache.key("test-skill", "1.0.0")
    assert key1 == "test-skill:1.0.0"

    key2 = cache.key("test-skill")
    assert key2 == "test-skill:latest"

    print("✅ PASS: Cache key generation")


def test_cache_operations():
    """Test basic cache operations"""
    cache = SkillCache()
    cache.clear()

    key = cache.key("test-skill")

    # Initially not cached
    assert not cache.is_cached(key)
    assert cache.get(key) is None

    # Cache data
    cache.set(key, {"data": "test"})

    # Now cached
    assert cache.is_cached(key)
    data = cache.get(key)
    assert data == {"data": "test"}

    print("✅ PASS: Cache operations")


def test_cache_hit_miss_tracking():
    """Test cache hit/miss tracking"""
    cache = SkillCache()
    cache.clear()

    key = cache.key("test")

    # Cache miss
    cache.get(key)
    assert cache._misses == 1
    assert cache._hits == 0

    # Cache and hit
    cache.set(key, "data")
    cache.get(key)
    assert cache._hits == 1

    print("✅ PASS: Cache hit/miss tracking")


def test_cache_eviction():
    """Test cache eviction (LRU)"""
    cache = SkillCache(max_size=3)
    cache.clear()

    # Fill cache
    for i in range(3):
        cache.set(cache.key(f"skill-{i}"), f"data-{i}")
        time.sleep(0.01)  # Ensure different timestamps

    assert len(cache._cache) == 3

    # Add one more (should evict oldest)
    cache.set(cache.key("skill-3"), "data-3")

    assert len(cache._cache) == 3
    # Oldest (skill-0) should be evicted
    assert not cache.is_cached(cache.key("skill-0"))
    # Newest should be present
    assert cache.is_cached(cache.key("skill-3"))

    print("✅ PASS: Cache eviction")


def test_cache_ttl():
    """Test TTL expiration"""
    cache = SkillCache(ttl=1)  # 1 second TTL
    cache.clear()

    key = cache.key("ttl-test")
    cache.set(key, "data")

    # Initially cached
    assert cache.is_cached(key)

    # Wait for expiration
    time.sleep(1.5)

    # Should be expired
    assert not cache.is_cached(key)

    print("✅ PASS: Cache TTL")


def test_clear_cache():
    """Test cache clearing"""
    cache = SkillCache()
    cache.clear()

    # Add multiple entries
    cache.set(cache.key("skill-a"), "data-a")
    cache.set(cache.key("skill-b"), "data-b")

    assert len(cache._cache) == 2

    # Clear specific entry
    cache.clear(cache.key("skill-a"))
    assert len(cache._cache) == 1

    # Clear all
    cache.clear()
    assert len(cache._cache) == 0

    print("✅ PASS: Clear cache")


def test_cache_statistics():
    """Test cache statistics"""
    cache = SkillCache()
    cache.clear()

    # Generate some hits and misses
    cache.set(cache.key("test"), "data")
    cache.get(cache.key("test"))  # Hit
    cache.get(cache.key("other"))  # Miss

    stats = cache.stats()

    assert "cache_enabled" in stats
    assert "cache_size" in stats
    assert "cache_hits" in stats
    assert "cache_misses" in stats
    assert "hit_rate_percent" in stats

    assert stats["cache_hits"] == 1
    assert stats["cache_misses"] == 1
    assert stats["hit_rate_percent"] == 50.0

    print("✅ PASS: Cache statistics")


def test_cache_disabled():
    """Test caching when disabled"""
    import os

    os.environ['SKILL_CACHE_ENABLED'] = '0'
    cache = SkillCache()

    assert not cache.enabled

    key = cache.key("test")
    cache.set(key, "data")

    # Should not cache when disabled
    assert len(cache._cache) == 0
    assert cache.get(key) is None

    # Restore
    os.environ['SKILL_CACHE_ENABLED'] = '1'

    print("✅ PASS: Cache disabled")


def run_all_tests():
    """Run all tests"""
    print("=" * 50)
    print("Skill Cache Tests (Python)")
    print("=" * 50)
    print()

    tests = [
        test_cache_initialization,
        test_cache_key_generation,
        test_cache_operations,
        test_cache_hit_miss_tracking,
        test_cache_eviction,
        test_cache_ttl,
        test_clear_cache,
        test_cache_statistics,
        test_cache_disabled,
    ]

    failed = 0
    for test in tests:
        try:
            test()
        except AssertionError as e:
            print(f"❌ FAIL: {test.__name__}")
            print(f"  Error: {e}")
            failed += 1
        except Exception as e:
            print(f"❌ ERROR: {test.__name__}")
            print(f"  Error: {e}")
            failed += 1

    print()
    print("=" * 50)
    print(f"Tests run: {len(tests)}")
    print(f"Passed: {len(tests) - failed}")
    print(f"Failed: {failed}")

    if failed == 0:
        print("✅ All tests passed!")
        return 0
    else:
        print("❌ Some tests failed")
        return 1


if __name__ == "__main__":
    sys.exit(run_all_tests())
