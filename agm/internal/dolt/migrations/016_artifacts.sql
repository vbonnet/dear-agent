CREATE TABLE IF NOT EXISTS agm_artifacts (
    id          VARCHAR(255) PRIMARY KEY,
    session_id  VARCHAR(255) NOT NULL,
    type        VARCHAR(100) NOT NULL,
    path        VARCHAR(1024),
    size        BIGINT,
    metadata    JSON,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES agm_sessions(id),
    INDEX idx_artifacts_session_id (session_id)
);
