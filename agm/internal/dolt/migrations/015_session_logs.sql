CREATE TABLE IF NOT EXISTS agm_session_logs (
    id          VARCHAR(255) PRIMARY KEY,
    session_id  VARCHAR(255) NOT NULL,
    timestamp   TIMESTAMP NOT NULL,
    level       VARCHAR(20) NOT NULL,
    source      VARCHAR(255),
    message     TEXT,
    data        JSON,
    FOREIGN KEY (session_id) REFERENCES agm_sessions(id),
    INDEX idx_logs_session_id (session_id),
    INDEX idx_logs_timestamp (timestamp)
);
