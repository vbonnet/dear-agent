"""
BeadsCreate tool — appends a new bead to the beads JSONL database.
"""

import json
import logging
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

logger = logging.getLogger("engram-mcp-server.tools.beads_create")


class BeadsCreate:
    """Create beads (issues/tasks) in a JSONL database."""

    def __init__(self, beads_db: Path):
        self.beads_db = Path(beads_db)

    def create(
        self,
        title: str,
        description: str,
        priority: int = 1,
        labels: List[str] = None,
        estimated_minutes: int = 60,
    ) -> Dict[str, Any]:
        """Create a new bead entry.

        Args:
            title: Short imperative title.
            description: Detailed description.
            priority: 0 (highest) – 5 (lowest).
            labels: List of tag strings.
            estimated_minutes: Estimated effort in minutes.

        Returns:
            Dict with bead_id and metadata.

        Raises:
            ValueError: If priority is out of range or a duplicate title exists.
        """
        if not 0 <= priority <= 5:
            raise ValueError(f"priority must be 0–5, got {priority}")

        if labels is None:
            labels = []

        # Duplicate detection
        existing = self._load_all()
        for bead in existing:
            if bead.get("title", "").lower() == title.lower():
                raise ValueError(
                    f"Duplicate bead title: '{title}' (id={bead.get('id', '?')})"
                )

        bead_id = str(uuid.uuid4())[:8]
        now = datetime.now(timezone.utc).isoformat()

        bead = {
            "id": bead_id,
            "title": title,
            "description": description,
            "priority": priority,
            "labels": labels,
            "estimated_minutes": estimated_minutes,
            "status": "open",
            "created_at": now,
            "updated_at": now,
        }

        self._append(bead)
        logger.info("Created bead %s: %s", bead_id, title)

        return {
            "bead_id": bead_id,
            "title": title,
            "priority": priority,
            "labels": labels,
            "estimated_minutes": estimated_minutes,
            "created_at": now,
            "db_path": str(self.beads_db),
        }

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _load_all(self) -> List[Dict]:
        """Read all beads from the JSONL file."""
        if not self.beads_db.exists():
            return []
        beads = []
        for line in self.beads_db.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                beads.append(json.loads(line))
            except json.JSONDecodeError:
                logger.warning("Skipping malformed line in %s", self.beads_db)
        return beads

    def _append(self, bead: Dict) -> None:
        """Append a bead to the JSONL file, creating parent dirs as needed."""
        self.beads_db.parent.mkdir(parents=True, exist_ok=True)
        with self.beads_db.open("a", encoding="utf-8") as f:
            f.write(json.dumps(bead) + "\n")
