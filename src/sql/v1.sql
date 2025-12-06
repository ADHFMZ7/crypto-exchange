
create table users (
  id serial primary key 
  fname text not null,
  lname text not null,

  email text unique not null,
  hashed_password text not null,
  created_at timestamptz default now()
)

-- Orders placed by users for a currency pair
create table orders (
  id bigserial primary key
  user_id integer references users(id),
  quantity numeric,
  price_each numeric, 
  side text, -- buy or sell
  market text, -- e.g. BTC-USD
  status text, -- open, filled, cancelled

  created_at timestamptz default now()
)

-- Trades executed between users
create table trades (
  id bigserial primary key
  buy_order_id bigint references orders(id),
  sell_order_id bigint references orders(id),
  quantity numeric,
  price_each numeric,
  executed_at timestamptz default now()
)

-- User balances for different currencies
create table balance (
  id bigserial primary key,
  user_id integer references users(id),
  currency text, -- e.g. BTC, USD
  balance numeric,

  updated_at timestamptz default now()
)

-- Indexes for performance
create index idx_orders_user_id on orders(user_id);
create index idx_orders_status on orders(status);
create index idx_trades_buy_order_id on trades(buy_order_id);
create index idx_trades_sell_order_id on trades(sell_order_id);
create index idx_balance_user_id on balance(user_id);
create index idx_balance_currency on balance(currency);