CREATE TABLE IF NOT EXISTS positions (
    id            BIGSERIAL PRIMARY KEY,
    ticker        VARCHAR(10) NOT NULL,
    side          VARCHAR(4) NOT NULL CHECK (side IN ('LONG', 'SHORT')),
    quantity      INTEGER NOT NULL,
    entry_price   DOUBLE PRECISION NOT NULL,
    stop_loss     DOUBLE PRECISION NOT NULL,
    take_profit   DOUBLE PRECISION NOT NULL,
    sector        VARCHAR(20) NOT NULL,
    status        VARCHAR(12) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'CLOSED')),
    opened_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at     TIMESTAMPTZ,
    execution_ref VARCHAR(64) UNIQUE
);

CREATE TABLE IF NOT EXISTS sentiment_scores (
    id          BIGSERIAL PRIMARY KEY,
    ticker      VARCHAR(10) NOT NULL,
    confidence  DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    bias        VARCHAR(8) NOT NULL CHECK (bias IN ('BULLISH', 'BEARISH', 'NEUTRAL')),
    summary     TEXT,
    source      TEXT,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS trade_log (
    id            BIGSERIAL PRIMARY KEY,
    ticker        VARCHAR(10) NOT NULL,
    side          VARCHAR(4) NOT NULL,
    quantity      INTEGER NOT NULL,
    price         DOUBLE PRECISION NOT NULL,
    signal_reason TEXT,
    status        VARCHAR(12) NOT NULL,
    executed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_positions_status ON positions(status);
CREATE INDEX IF NOT EXISTS idx_sentiment_ticker ON sentiment_scores(ticker, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_trade_log_ticker ON trade_log(ticker, executed_at DESC);
