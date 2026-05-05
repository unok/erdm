DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;


CREATE TABLE customers (
    id int NOT NULL,
    email varchar(255) UNIQUE NOT NULL,
    status varchar(16) DEFAULT 'active',

    PRIMARY KEY (id)
);
CREATE UNIQUE INDEX uk_customers_email ON customers (email);

CREATE TABLE order_items (
    order_id int NOT NULL,
    line_no int NOT NULL,
    qty int NOT NULL DEFAULT 1,

    PRIMARY KEY (order_id, line_no)
);
CREATE INDEX ix_order_items_order ON order_items (order_id);
CREATE INDEX ix_order_items_order_line ON order_items (order_id, line_no);

CREATE TABLE orders (
    id int NOT NULL,
    customer_id int NOT NULL,

    PRIMARY KEY (id)
);

