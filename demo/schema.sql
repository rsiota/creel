-- creel demo database — a small e-commerce schema with a rich foreign-key
-- graph, sized for screenshots: enough tables and rows to look substantial,
-- not so many that the ERD becomes cluttered.
--
-- Build it with:
--   sqlite3 demo/creel-demo.db < demo/schema.sql
-- Then explore:
--   ./creel -database demo/creel-demo.db
--
-- Key relationships (what makes the ERD / relationship explorer interesting):
--   users ← addresses, orders, reviews           (users is a hub)
--   categories ← categories (self-ref), products
--   products  ← order_items, reviews
--   orders    ← order_items, payments

PRAGMA foreign_keys = ON;

DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS addresses;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS users;

CREATE TABLE users (
    id         INTEGER PRIMARY KEY,
    email      TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL,
    role       TEXT    NOT NULL DEFAULT 'customer',
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE categories (
    id        INTEGER PRIMARY KEY,
    name      TEXT NOT NULL,
    parent_id INTEGER,
    FOREIGN KEY (parent_id) REFERENCES categories(id)
);

CREATE TABLE products (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL,
    description TEXT,
    price       REAL    NOT NULL CHECK (price >= 0),
    stock       INTEGER NOT NULL DEFAULT 0,
    category_id INTEGER NOT NULL,
    FOREIGN KEY (category_id) REFERENCES categories(id)
);

CREATE TABLE addresses (
    id      INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    line1   TEXT    NOT NULL,
    city    TEXT    NOT NULL,
    country TEXT    NOT NULL,
    postal  TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE orders (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    status     TEXT    NOT NULL DEFAULT 'pending',
    total      REAL    NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE order_items (
    id         INTEGER PRIMARY KEY,
    order_id   INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    quantity   INTEGER NOT NULL CHECK (quantity > 0),
    unit_price REAL    NOT NULL,
    FOREIGN KEY (order_id)   REFERENCES orders(id),
    FOREIGN KEY (product_id) REFERENCES products(id)
);

CREATE TABLE reviews (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    rating     INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body       TEXT,
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (user_id)    REFERENCES users(id),
    FOREIGN KEY (product_id) REFERENCES products(id)
);

CREATE TABLE payments (
    id        INTEGER PRIMARY KEY,
    order_id  INTEGER NOT NULL,
    method    TEXT    NOT NULL,
    amount    REAL    NOT NULL,
    status    TEXT    NOT NULL DEFAULT 'captured',
    FOREIGN KEY (order_id) REFERENCES orders(id)
);

-- --- seed data -----------------------------------------------------------

INSERT INTO users (id, email, name, role) VALUES
    (1, 'ada@example.com',   'Ada Lovelace',      'admin'),
    (2, 'alan@example.com',  'Alan Turing',       'customer'),
    (3, 'grace@example.com', 'Grace Hopper',      'customer'),
    (4, 'linus@example.com', 'Linus Torvalds',    'customer'),
    (5, 'ken@example.com',   'Ken Thompson',      'customer'),
    (6, 'barb@example.com',  'Barbara Liskov',    'customer'),
    (7, 'dijk@example.com',  'Edsger Dijkstra',   'customer'),
    (8, 'margaret@example.com', 'Margaret Hamilton', 'admin');

INSERT INTO categories (id, name, parent_id) VALUES
    (1, 'Electronics',  NULL),
    (2, 'Computers',    1),
    (3, 'Peripherals',  1),
    (4, 'Audio',        1),
    (5, 'Books',        NULL),
    (6, 'Software',     NULL);

INSERT INTO products (id, name, description, price, stock, category_id) VALUES
    (1,  'Mechanical Keyboard', 'Hot-swappable, 75% layout',             129.99,  42, 3),
    (2,  '4K Monitor',          '27-inch IPS, 144Hz',                    399.00,  17, 2),
    (3,  'USB-C Hub',           '8-in-1 with power delivery',             59.50,  88, 3),
    (4,  'Laptop Stand',        'Aluminium, adjustable',                  45.00, 120, 3),
    (5,  'Noise-Cancelling Headphones', 'ANC, 30h battery',              249.99,  23, 4),
    (6,  'Desk Microphone',     'Cardioid, USB',                          89.00,  51, 4),
    (7,  'The Pragmatic Programmer', 'Classic dev wisdom',                39.99, 200, 5),
    (8,  'Database Internals',  'Deep dive into storage engines',         49.99,  76, 5),
    (9,  'Code Editor Pro',     'License, yearly',                        99.00, 999, 6),
    (10, 'Terminal Theme Pack', '12 curated palettes',                     12.00, 999, 6);

INSERT INTO addresses (id, user_id, line1, city, country, postal) VALUES
    (1, 1, '1 Analytical Engine Ln',  'London',     'UK', 'SW1A 1AA'),
    (2, 2, '42 Turing Way',           'Cambridge',  'UK', 'CB2 1TN'),
    (3, 3, '9 Compiler Court',        'New York',   'US', '10001'),
    (4, 4, '12 Penguin Plaza',        'Helsinki',   'FI', '00100'),
    (5, 5, '7 Bell Labs Rd',          'Murray Hill','US', '07974'),
    (6, 6, '3 Abstraction Ave',       'Boston',     'US', '02139');

INSERT INTO orders (id, user_id, status, total) VALUES
    (1, 2, 'delivered',  529.98),
    (2, 3, 'delivered',  249.99),
    (3, 4, 'shipped',    189.00),
    (4, 5, 'pending',    159.49),
    (5, 1, 'delivered',   99.00),
    (6, 6, 'shipped',    489.98),
    (7, 2, 'pending',    139.99),
    (8, 8, 'delivered',   89.00),
    (9, 4, 'delivered',  438.99),
    (10,7, 'cancelled',   49.99),
    (11,3, 'shipped',    109.00),
    (12,1, 'pending',     45.00);

INSERT INTO order_items (id, order_id, product_id, quantity, unit_price) VALUES
    (1,  1, 2, 1, 399.00),
    (2,  1, 1, 1, 129.99),
    (3,  2, 5, 1, 249.99),
    (4,  3, 1, 1, 129.99),
    (5,  3, 4, 1,  45.00),
    (6,  3, 3, 1,  59.50),
    (7,  4, 4, 1,  45.00),
    (8,  4, 3, 1,  59.50),
    (9,  4, 7, 1,  39.99),
    (10, 4, 10,1,  12.00),
    (11, 5, 9, 1,  99.00),
    (12, 6, 2, 1, 399.00),
    (13, 6, 6, 1,  89.00),
    (14, 7, 1, 1, 129.99),
    (15, 8, 6, 1,  89.00),
    (16, 9, 2, 1, 399.00),
    (17, 9, 7, 1,  39.99),
    (18, 11,6, 1,  89.00),
    (19, 11,10,1,  12.00),
    (20, 11,7, 1,  39.99),
    (21, 12,4, 1,  45.00),
    (22, 10,8, 1,  49.99),
    (23, 3, 8, 1,  49.99);

INSERT INTO reviews (id, user_id, product_id, rating, body) VALUES
    (1, 2, 1, 5, 'Best keyboard I have owned. The switches are buttery smooth.'),
    (2, 3, 5, 4, 'Great ANC, slightly tight on the head after a few hours.'),
    (3, 4, 2, 5, 'Crisp panel, no dead pixels, the 144Hz is glorious.'),
    (4, 1, 7, 5, 'Timeless advice. Re-read it every year.'),
    (5, 6, 9, 4, 'Fast and polished. Wish it had better Vim defaults.'),
    (6, 5, 3, 3, 'Works, but runs warm under load.'),
    (7, 8, 6, 5, 'Crystal clear audio for calls and streaming.'),
    (8, 2, 8, 4, 'Dense but rewarding. The B-tree chapter alone is worth it.');

INSERT INTO payments (id, order_id, method, amount, status) VALUES
    (1,  1, 'card',   529.98, 'captured'),
    (2,  2, 'card',   249.99, 'captured'),
    (3,  3, 'card',   189.00, 'captured'),
    (4,  5, 'card',    99.00, 'captured'),
    (5,  6, 'card',   489.98, 'captured'),
    (6,  8, 'card',    89.00, 'captured'),
    (7,  9, 'card',   438.99, 'captured'),
    (8,  11,'card',   109.00, 'captured'),
    (9,  7, 'card',   139.99, 'authorized'),
    (10, 4, 'card',   159.49, 'authorized'),
    (11, 12,'card',    45.00, 'authorized'),
    (12, 10,'card',    49.99, 'refunded');
