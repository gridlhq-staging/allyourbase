package templates

const ecommerceSchemaPart1 = `-- Ecommerce domain schema
-- Apply with: ayb sql < schema.sql

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    sku TEXT UNIQUE,
    stock_count INTEGER NOT NULL DEFAULT 0 CHECK (stock_count >= 0),
    image_url TEXT,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES _ayb_users(id),
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    shipping_address JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'shipped', 'delivered', 'cancelled')),
    total_cents INTEGER NOT NULL DEFAULT 0 CHECK (total_cents >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_cents INTEGER NOT NULL CHECK (unit_price_cents >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS carts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES _ayb_users(id),
    items JSONB NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE products ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE carts ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS products_select ON products;
CREATE POLICY products_select ON products FOR SELECT
    USING (active = true);

DROP POLICY IF EXISTS products_insert ON products;
CREATE POLICY products_insert ON products FOR INSERT
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS products_update ON products;
CREATE POLICY products_update ON products FOR UPDATE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL)
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS products_delete ON products;
CREATE POLICY products_delete ON products FOR DELETE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS customers_select ON customers;
CREATE POLICY customers_select ON customers FOR SELECT
    USING (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS customers_insert ON customers;
CREATE POLICY customers_insert ON customers FOR INSERT
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);
`

const ecommerceSchemaPart2 = `
DROP POLICY IF EXISTS customers_update ON customers;
CREATE POLICY customers_update ON customers FOR UPDATE
    USING (user_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS customers_delete ON customers;
CREATE POLICY customers_delete ON customers FOR DELETE
    USING (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS orders_select ON orders;
CREATE POLICY orders_select ON orders FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS orders_insert ON orders;
CREATE POLICY orders_insert ON orders FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS orders_update ON orders;
CREATE POLICY orders_update ON orders FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS orders_delete ON orders;
CREATE POLICY orders_delete ON orders FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS order_items_select ON order_items;
CREATE POLICY order_items_select ON order_items FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS order_items_insert ON order_items;
CREATE POLICY order_items_insert ON order_items FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );
`

const ecommerceSchemaPart3 = `
DROP POLICY IF EXISTS order_items_update ON order_items;
CREATE POLICY order_items_update ON order_items FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS order_items_delete ON order_items;
CREATE POLICY order_items_delete ON order_items FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS carts_select ON carts;
CREATE POLICY carts_select ON carts FOR SELECT
    USING (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS carts_insert ON carts;
CREATE POLICY carts_insert ON carts FOR INSERT
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS carts_update ON carts;
CREATE POLICY carts_update ON carts FOR UPDATE
    USING (user_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS carts_delete ON carts;
CREATE POLICY carts_delete ON carts FOR DELETE
    USING (user_id = current_setting('ayb.user_id', true)::uuid);
`

const ecommerceClientCodePart1 = `import { ayb } from "./ayb";

export interface Product {
  id: string;
  name: string;
  description: string;
  price_cents: number;
  currency: string;
  sku: string | null;
  stock_count: number;
  image_url: string | null;
  active: boolean;
  created_at: string;
}

export interface Customer {
  id: string;
  user_id: string;
  name: string;
  email: string;
  shipping_address: Record<string, unknown> | null;
  created_at: string;
}

export type OrderStatus =
  | "pending"
  | "paid"
  | "shipped"
  | "delivered"
  | "cancelled";

export interface Order {
  id: string;
  customer_id: string;
  status: OrderStatus;
  total_cents: number;
  created_at: string;
}

export interface OrderItem {
  id: string;
  order_id: string;
  product_id: string;
  quantity: number;
  unit_price_cents: number;
  created_at: string;
}

export interface CartItem {
  product_id: string;
  quantity: number;
}

export interface CreateOrderItemInput {
  product_id: string;
  quantity: number;
  unit_price_cents: number;
}

export function listProducts(filter?: string) {
  if (filter) {
    return ayb.records.list("products", { filter, sort: "name" });
  }
  return ayb.records.list("products", { sort: "name" });
}

export function getProduct(id: string) {
  return ayb.records.get("products", id);
}

export async function getCart() {
  const result = await ayb.records.list("carts", { limit: 1 });
  return result.items?.[0] ?? null;
}
`

const ecommerceClientCodePart2 = `
export async function updateCart(items: CartItem[]) {
  const existing = await getCart();
  if (existing) {
    return ayb.records.update("carts", existing.id, {
      items,
      updated_at: new Date().toISOString(),
    });
  }

  const me = await ayb.auth.me();
  const userId = (me as { id?: string; user?: { id?: string } }).id
    ?? (me as { id?: string; user?: { id?: string } }).user?.id;
  if (!userId) {
    throw new Error("Cannot update cart without an authenticated user");
  }

  return ayb.records.create("carts", {
    user_id: userId,
    items,
  });
}

export async function createOrder(customerId: string, items: CreateOrderItemInput[]) {
  const totalCents = items.reduce(
    (sum, item) => sum + item.unit_price_cents * item.quantity,
    0
  );

  const order = await ayb.records.create("orders", {
    customer_id: customerId,
    status: "pending",
    total_cents: totalCents,
  });

  await Promise.all(
    items.map((item) =>
      ayb.records.create("order_items", {
        order_id: order.id,
        product_id: item.product_id,
        quantity: item.quantity,
        unit_price_cents: item.unit_price_cents,
      })
    )
  );

  return order;
}

export function listOrders(customerId?: string) {
  if (customerId) {
    return ayb.records.list("orders", {
      filter: "customer_id='" + customerId + "'",
      sort: "-created_at",
    });
  }
  return ayb.records.list("orders", { sort: "-created_at" });
}

export function getOrder(id: string) {
  return ayb.records.get("orders", id);
}
`
