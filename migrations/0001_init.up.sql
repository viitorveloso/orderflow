-- Initial schema: users, products, orders, order_items.

CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE products (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    price_cents BIGINT      NOT NULL CHECK (price_cents >= 0),
    stock       INTEGER     NOT NULL DEFAULT 0 CHECK (stock >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users (id),
    status      TEXT        NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'paid', 'shipped', 'cancelled')),
    total_cents BIGINT      NOT NULL CHECK (total_cents >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_user_id ON orders (user_id);

CREATE TABLE order_items (
    id               BIGSERIAL PRIMARY KEY,
    order_id         BIGINT  NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id       BIGINT  NOT NULL REFERENCES products (id),
    quantity         INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT  NOT NULL CHECK (unit_price_cents >= 0)
);

CREATE INDEX idx_order_items_order_id ON order_items (order_id);
