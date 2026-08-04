-- AGM Migration 019: Session Name Reservations
-- Serializes new active-name claims without rewriting or rejecting historical
-- duplicate session rows. Expired operation leases are safe to reclaim.

CREATE TABLE IF NOT EXISTS agm_session_name_reservations (
  workspace VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  session_id VARCHAR(255) NOT NULL,
  created_at TIMESTAMP NOT NULL,
  expires_at TIMESTAMP NOT NULL,

  PRIMARY KEY (workspace, name),
  UNIQUE KEY uq_agm_session_name_reservation_owner (workspace, session_id),
  INDEX idx_agm_session_name_reservation_expiry (expires_at)
);
