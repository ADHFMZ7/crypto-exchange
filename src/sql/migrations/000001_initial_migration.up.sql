CREATE TABLE users (
  id              SERIAL PRIMARY KEY,
  fullname        TEXT NOT NULL,
  email           TEXT UNIQUE NOT NULL,
  hashed_password TEXT NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Orders placed by users for a currency pair
CREATE TABLE orders (
  id           BIGSERIAL PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id),
  quantity     NUMERIC NOT NULL,
  price_each   NUMERIC NOT NULL,
  side         TEXT NOT NULL,     -- buy or sell
  market       TEXT NOT NULL,     -- e.g. BTC-USD
  status       TEXT NOT NULL,     -- open, filled, cancelled
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Trades executed between users
CREATE TABLE trades (
  id            BIGSERIAL PRIMARY KEY,
  buy_order_id  BIGINT NOT NULL REFERENCES orders(id),
  sell_order_id BIGINT NOT NULL REFERENCES orders(id),
  quantity      NUMERIC NOT NULL,
  price_each    NUMERIC NOT NULL,
  executed_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- User balances for different currencies
CREATE TABLE balances (
  id         BIGSERIAL PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id),
  currency   TEXT NOT NULL,  -- e.g. BTC, USD
  balance    NUMERIC NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for performance
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_trades_buy_order_id ON trades(buy_order_id);
CREATE INDEX idx_trades_sell_order_id ON trades(sell_order_id);
CREATE INDEX idx_balances_user_id ON balances(user_id);
CREATE INDEX idx_balances_currency ON balances(currency);

