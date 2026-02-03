#!/usr/bin/env python3
"""
Helper module for testing astrocyte-daemon.py logging setup.

This module provides a standalone version of setup_logging that can be
imported and tested without running the full daemon.
"""

import sys
import logging
from logging.handlers import RotatingFileHandler
from pathlib import Path


def setup_logging_standalone(log_dir: Path, verbose: bool = False):
    """
    Standalone version of setup_logging from astrocyte-daemon.py.

    This is a copy of the fixed version used for testing.
    """
    log_dir.mkdir(parents=True, exist_ok=True)
    log_file = log_dir / "daemon.log"

    # Create logger
    logger = logging.getLogger("astrocyte_test")
    logger.setLevel(logging.DEBUG if verbose else logging.INFO)

    # Remove existing handlers to avoid duplicates
    # CRITICAL: Must close handlers before clearing to prevent file descriptor leaks
    # that would prevent logging after daemon restart
    for handler in logger.handlers[:]:  # Use slice to avoid modifying list during iteration
        try:
            handler.close()
        except Exception:
            pass  # Ignore errors closing old handlers
    logger.handlers.clear()

    # File handler with rotation (10MB max, keep 5 backups)
    file_handler = RotatingFileHandler(
        log_file,
        maxBytes=10 * 1024 * 1024,  # 10MB
        backupCount=5
    )
    file_handler.setLevel(logging.DEBUG)
    file_formatter = logging.Formatter(
        '%(asctime)s - %(name)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )
    file_handler.setFormatter(file_formatter)
    logger.addHandler(file_handler)

    # Console handler (only INFO and above to avoid clutter)
    console_handler = logging.StreamHandler(sys.stdout)
    console_handler.setLevel(logging.INFO)
    console_formatter = logging.Formatter('%(message)s')
    console_handler.setFormatter(console_formatter)
    logger.addHandler(console_handler)

    return logger
