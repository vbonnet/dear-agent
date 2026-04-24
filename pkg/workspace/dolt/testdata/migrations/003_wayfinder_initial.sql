-- Wayfinder Migration 001: Initial Schema
-- Description: Create core Wayfinder tables (projects and tasks)
-- Author: Wayfinder Team
-- Date: 2026-02-19
-- Depends: None (initial migration)

CREATE TABLE IF NOT EXISTS wf_projects (
  id VARCHAR(255) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  status ENUM('active', 'completed', 'archived') DEFAULT 'active',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  metadata JSON,

  INDEX idx_status (status),
  INDEX idx_created_at (created_at),
  INDEX idx_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Wayfinder projects - SDLC project tracking';

CREATE TABLE IF NOT EXISTS wf_tasks (
  id VARCHAR(255) PRIMARY KEY,
  project_id VARCHAR(255) NOT NULL,
  title VARCHAR(255) NOT NULL,
  description TEXT,
  status ENUM('pending', 'in_progress', 'completed', 'blocked') DEFAULT 'pending',
  priority INT DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  metadata JSON,

  FOREIGN KEY (project_id) REFERENCES wf_projects(id) ON DELETE CASCADE,
  INDEX idx_project_id (project_id),
  INDEX idx_status (status),
  INDEX idx_priority (priority)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Wayfinder tasks - individual project tasks';
