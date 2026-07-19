DROP TABLE IF EXISTS processed_events;
DROP TABLE IF EXISTS idempotency_keys;
DROP INDEX IF EXISTS idx_outbox_unpublished;
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS order_status_history;
DROP TABLE IF EXISTS orders;
