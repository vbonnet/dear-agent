#!/usr/bin/env python3
"""Parallel Execution Implementation
Enables concurrent execution of independent skills
"""

import os
import time
import subprocess
from concurrent.futures import ThreadPoolExecutor, as_completed, Future
from typing import Any, Callable, Dict, List, Optional, Tuple
from dataclasses import dataclass, field


@dataclass
class JobResult:
    """Result of parallel job execution"""
    job_id: str
    exit_code: int
    output: str
    error: str
    duration_ms: int


class ParallelExecutor:
    """Parallel execution manager for skills"""

    def __init__(self, max_workers: Optional[int] = None, timeout: int = 300):
        """Initialize parallel executor

        Args:
            max_workers: Maximum number of parallel workers (default: CPU count)
            timeout: Timeout in seconds (default: 5 minutes)
        """
        self.max_workers = max_workers or int(
            os.getenv('PARALLEL_MAX_JOBS', str(os.cpu_count() or 4))
        )
        self.timeout = int(os.getenv('PARALLEL_TIMEOUT', str(timeout)))

        self._jobs: Dict[str, Future] = {}
        self._results: Dict[str, JobResult] = {}

    def execute(
        self,
        job_id: str,
        func: Callable,
        *args,
        **kwargs
    ) -> Future:
        """Execute function in background

        Args:
            job_id: Unique job identifier
            func: Function to execute
            *args: Positional arguments
            **kwargs: Keyword arguments

        Returns:
            Future object
        """
        executor = ThreadPoolExecutor(max_workers=1)
        future = executor.submit(func, *args, **kwargs)
        self._jobs[job_id] = future

        return future

    def execute_command(
        self,
        job_id: str,
        command: List[str]
    ) -> Future:
        """Execute shell command in background

        Args:
            job_id: Unique job identifier
            command: Command to execute (list of strings)

        Returns:
            Future object
        """
        def run_command():
            start_time = time.time()

            try:
                result = subprocess.run(
                    command,
                    capture_output=True,
                    text=True,
                    timeout=self.timeout
                )

                end_time = time.time()
                duration_ms = int((end_time - start_time) * 1000)

                return JobResult(
                    job_id=job_id,
                    exit_code=result.returncode,
                    output=result.stdout,
                    error=result.stderr,
                    duration_ms=duration_ms
                )

            except subprocess.TimeoutExpired:
                end_time = time.time()
                duration_ms = int((end_time - start_time) * 1000)

                return JobResult(
                    job_id=job_id,
                    exit_code=124,  # Timeout exit code
                    output="",
                    error=f"Command timed out after {self.timeout}s",
                    duration_ms=duration_ms
                )

            except Exception as e:
                end_time = time.time()
                duration_ms = int((end_time - start_time) * 1000)

                return JobResult(
                    job_id=job_id,
                    exit_code=1,
                    output="",
                    error=str(e),
                    duration_ms=duration_ms
                )

        return self.execute(job_id, run_command)

    def wait(self, job_id: str) -> JobResult:
        """Wait for specific job to complete

        Args:
            job_id: Job identifier

        Returns:
            JobResult object

        Raises:
            KeyError: If job not found
        """
        if job_id not in self._jobs:
            raise KeyError(f"Job not found: {job_id}")

        future = self._jobs[job_id]

        try:
            result = future.result(timeout=self.timeout)
            self._results[job_id] = result
            return result

        except Exception as e:
            # Create error result
            result = JobResult(
                job_id=job_id,
                exit_code=1,
                output="",
                error=str(e),
                duration_ms=0
            )
            self._results[job_id] = result
            return result

    def wait_all(self) -> Dict[str, JobResult]:
        """Wait for all jobs to complete

        Returns:
            Dictionary mapping job_id to JobResult
        """
        results = {}

        for job_id in list(self._jobs.keys()):
            results[job_id] = self.wait(job_id)

        return results

    def get_result(self, job_id: str) -> Optional[JobResult]:
        """Get job result (non-blocking)

        Args:
            job_id: Job identifier

        Returns:
            JobResult if available, None otherwise
        """
        return self._results.get(job_id)

    def execute_batch(
        self,
        commands: List[Tuple[str, List[str]]]
    ) -> Dict[str, JobResult]:
        """Execute multiple commands in parallel

        Args:
            commands: List of (job_id, command) tuples

        Returns:
            Dictionary mapping job_id to JobResult
        """
        # Start all commands
        for job_id, command in commands:
            self.execute_command(job_id, command)

        # Wait for all to complete
        return self.wait_all()

    def run_skills(
        self,
        skills: List[Callable],
        job_prefix: str = "skill"
    ) -> Dict[str, Any]:
        """Run multiple skills in parallel

        Args:
            skills: List of skill functions to execute
            job_prefix: Prefix for job IDs

        Returns:
            Dictionary mapping job_id to result
        """
        results = {}

        # Execute in batches if needed
        batch_size = self.max_workers
        skill_count = len(skills)

        if skill_count <= batch_size:
            # Run all in parallel
            with ThreadPoolExecutor(max_workers=batch_size) as executor:
                futures = {
                    executor.submit(skill): f"{job_prefix}-{i}"
                    for i, skill in enumerate(skills)
                }

                for future in as_completed(futures):
                    job_id = futures[future]
                    try:
                        results[job_id] = future.result(timeout=self.timeout)
                    except Exception as e:
                        results[job_id] = {
                            "error": str(e),
                            "exit_code": 1
                        }

        else:
            # Run in batches
            for batch_num, i in enumerate(range(0, skill_count, batch_size)):
                batch_end = min(i + batch_size, skill_count)
                batch_skills = skills[i:batch_end]

                with ThreadPoolExecutor(max_workers=batch_size) as executor:
                    futures = {
                        executor.submit(skill): f"{job_prefix}-batch{batch_num}-{j}"
                        for j, skill in enumerate(batch_skills)
                    }

                    for future in as_completed(futures):
                        job_id = futures[future]
                        try:
                            results[job_id] = future.result(timeout=self.timeout)
                        except Exception as e:
                            results[job_id] = {
                                "error": str(e),
                                "exit_code": 1
                            }

        return results

    def cleanup(self) -> None:
        """Cleanup parallel execution resources"""
        # Cancel any pending futures
        for future in self._jobs.values():
            if not future.done():
                future.cancel()

        self._jobs.clear()
        self._results.clear()

    def stats(self) -> Dict[str, Any]:
        """Get parallel execution statistics

        Returns:
            Dictionary of statistics
        """
        total_jobs = len(self._results)
        successful_jobs = sum(
            1 for r in self._results.values()
            if r.exit_code == 0
        )
        failed_jobs = total_jobs - successful_jobs

        return {
            "max_parallel_jobs": self.max_workers,
            "timeout_seconds": self.timeout,
            "total_jobs_executed": total_jobs,
            "successful_jobs": successful_jobs,
            "failed_jobs": failed_jobs
        }

    def print_stats(self) -> None:
        """Print parallel execution statistics"""
        stats = self.stats()

        print("=== Parallel Execution Statistics ===")
        for key, value in stats.items():
            label = key.replace('_', ' ').title()
            print(f"{label:<25}: {value}")


# Global executor instance
_global_executor: Optional[ParallelExecutor] = None


def get_executor() -> ParallelExecutor:
    """Get global parallel executor instance

    Returns:
        Global ParallelExecutor instance
    """
    global _global_executor

    if _global_executor is None:
        _global_executor = ParallelExecutor()

    return _global_executor
