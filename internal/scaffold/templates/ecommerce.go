// Package templates ecommerceTemplate provides an ecommerce domain scaffold with production-style schema, seed data, typed client SDK code, and documentation.
package templates

type ecommerceTemplate struct{}

func init() {
	Register(ecommerceTemplate{})
}

func (ecommerceTemplate) Name() string {
	return "ecommerce"
}

// Schema returns the SQL DDL for the ecommerce domain, creating tables for products, customers, orders, order items, and carts with row-level security policies enforcing user isolation.
func (ecommerceTemplate) Schema() string {
	return ecommerceSchemaPart1 + ecommerceSchemaPart2 + ecommerceSchemaPart3
}

// SeedData returns SQL statements that populate the ecommerce schema with sample users, customers, products with varying inventory levels, orders in different lifecycle states, order items, and shopping carts.
func (ecommerceTemplate) SeedData() string {
	return `-- Ecommerce domain seed data
-- Apply with: ayb sql < seed.sql

INSERT INTO _ayb_users (id, email, password_hash)
VALUES
    ('51111111-1111-1111-1111-111111111111', 'shopper.one@example.com', 'seeded-password-hash'),
    ('52222222-2222-2222-2222-222222222222', 'shopper.two@example.com', 'seeded-password-hash'),
    ('53333333-3333-3333-3333-333333333333', 'shopper.three@example.com', 'seeded-password-hash')
ON CONFLICT DO NOTHING;

INSERT INTO customers (id, user_id, name, email, shipping_address)
VALUES
    ('61000000-0000-0000-0000-000000000001', '51111111-1111-1111-1111-111111111111', 'Jordan Miles', 'shopper.one@example.com', '{"line1":"100 Market St","city":"New York","state":"NY","postal_code":"10001","country":"US"}'::jsonb),
    ('61000000-0000-0000-0000-000000000002', '52222222-2222-2222-2222-222222222222', 'Avery Chen', 'shopper.two@example.com', '{"line1":"250 Mission St","city":"San Francisco","state":"CA","postal_code":"94105","country":"US"}'::jsonb),
    ('61000000-0000-0000-0000-000000000003', '53333333-3333-3333-3333-333333333333', 'Riley Patel', 'shopper.three@example.com', '{"line1":"77 Wacker Dr","city":"Chicago","state":"IL","postal_code":"60601","country":"US"}'::jsonb)
ON CONFLICT DO NOTHING;

INSERT INTO products (id, name, description, price_cents, currency, sku, stock_count, image_url, active)
VALUES
    ('62000000-0000-0000-0000-000000000001', 'Mechanical Keyboard', 'Hot-swappable mechanical keyboard with RGB.', 12999, 'USD', 'KB-001', 42, 'https://example.com/images/keyboard.jpg', true),
    ('62000000-0000-0000-0000-000000000002', 'Ergonomic Mouse', 'Vertical ergonomic mouse for all-day comfort.', 6999, 'USD', 'MS-002', 55, 'https://example.com/images/mouse.jpg', true),
    ('62000000-0000-0000-0000-000000000003', '4K Monitor', '27-inch 4K monitor with USB-C.', 32999, 'USD', 'MN-003', 18, 'https://example.com/images/monitor.jpg', true),
    ('62000000-0000-0000-0000-000000000004', 'Laptop Stand', 'Aluminum stand for 13-16 inch laptops.', 3999, 'USD', 'ST-004', 73, 'https://example.com/images/stand.jpg', true),
    ('62000000-0000-0000-0000-000000000005', 'USB-C Dock', '10-in-1 USB-C docking station.', 15999, 'USD', 'DK-005', 30, 'https://example.com/images/dock.jpg', true),
    ('62000000-0000-0000-0000-000000000006', 'Noise-Canceling Headphones', 'Wireless over-ear ANC headphones.', 24999, 'USD', 'HP-006', 12, 'https://example.com/images/headphones.jpg', true),
    ('62000000-0000-0000-0000-000000000007', 'Webcam', '1080p webcam with stereo microphones.', 8999, 'USD', 'WC-007', 28, 'https://example.com/images/webcam.jpg', true),
    ('62000000-0000-0000-0000-000000000008', 'Desk Mat', 'Large anti-slip desk mat.', 2999, 'USD', 'DM-008', 120, 'https://example.com/images/deskmat.jpg', true),
    ('62000000-0000-0000-0000-000000000009', 'Portable SSD', '1TB USB-C portable SSD.', 10999, 'USD', 'SD-009', 0, 'https://example.com/images/ssd.jpg', false),
    ('62000000-0000-0000-0000-000000000010', 'LED Light Bar', 'Adjustable monitor light bar.', 4999, 'USD', 'LB-010', 44, 'https://example.com/images/lightbar.jpg', false)
ON CONFLICT DO NOTHING;

INSERT INTO orders (id, customer_id, status, total_cents)
VALUES
    ('63000000-0000-0000-0000-000000000001', '61000000-0000-0000-0000-000000000001', 'pending', 22997),
    ('63000000-0000-0000-0000-000000000002', '61000000-0000-0000-0000-000000000001', 'paid', 37998),
    ('63000000-0000-0000-0000-000000000003', '61000000-0000-0000-0000-000000000002', 'shipped', 37997),
    ('63000000-0000-0000-0000-000000000004', '61000000-0000-0000-0000-000000000003', 'delivered', 15997),
    ('63000000-0000-0000-0000-000000000005', '61000000-0000-0000-0000-000000000002', 'cancelled', 15999)
ON CONFLICT DO NOTHING;

INSERT INTO order_items (id, order_id, product_id, quantity, unit_price_cents)
VALUES
    ('64000000-0000-0000-0000-000000000001', '63000000-0000-0000-0000-000000000001', '62000000-0000-0000-0000-000000000001', 1, 12999),
    ('64000000-0000-0000-0000-000000000002', '63000000-0000-0000-0000-000000000001', '62000000-0000-0000-0000-000000000008', 1, 2999),
    ('64000000-0000-0000-0000-000000000003', '63000000-0000-0000-0000-000000000001', '62000000-0000-0000-0000-000000000002', 1, 6999),
    ('64000000-0000-0000-0000-000000000004', '63000000-0000-0000-0000-000000000002', '62000000-0000-0000-0000-000000000003', 1, 32999),
    ('64000000-0000-0000-0000-000000000005', '63000000-0000-0000-0000-000000000003', '62000000-0000-0000-0000-000000000006', 1, 24999),
    ('64000000-0000-0000-0000-000000000006', '63000000-0000-0000-0000-000000000003', '62000000-0000-0000-0000-000000000004', 1, 3999),
    ('64000000-0000-0000-0000-000000000007', '63000000-0000-0000-0000-000000000004', '62000000-0000-0000-0000-000000000007', 1, 8999),
    ('64000000-0000-0000-0000-000000000008', '63000000-0000-0000-0000-000000000004', '62000000-0000-0000-0000-000000000004', 1, 3999),
    ('64000000-0000-0000-0000-000000000009', '63000000-0000-0000-0000-000000000004', '62000000-0000-0000-0000-000000000008', 1, 2999),
    ('64000000-0000-0000-0000-000000000010', '63000000-0000-0000-0000-000000000005', '62000000-0000-0000-0000-000000000005', 1, 15999),
    ('64000000-0000-0000-0000-000000000011', '63000000-0000-0000-0000-000000000002', '62000000-0000-0000-0000-000000000010', 1, 4999),
    ('64000000-0000-0000-0000-000000000012', '63000000-0000-0000-0000-000000000003', '62000000-0000-0000-0000-000000000007', 1, 8999)
ON CONFLICT DO NOTHING;

INSERT INTO carts (id, user_id, items)
VALUES
    ('65000000-0000-0000-0000-000000000001', '51111111-1111-1111-1111-111111111111', '[{"product_id":"62000000-0000-0000-0000-000000000002","quantity":1},{"product_id":"62000000-0000-0000-0000-000000000008","quantity":2}]'::jsonb),
    ('65000000-0000-0000-0000-000000000002', '52222222-2222-2222-2222-222222222222', '[{"product_id":"62000000-0000-0000-0000-000000000006","quantity":1}]'::jsonb)
ON CONFLICT DO NOTHING;
`
}

// ClientCode returns a map of TypeScript files with typed helper functions for common ecommerce operations including product listing, cart management, and order creation.
func (ecommerceTemplate) ClientCode() map[string]string {
	return map[string]string{
		"src/lib/ecommerce.ts": ecommerceClientCodePart1 + ecommerceClientCodePart2,
	}
}

// Readme returns formatted markdown documentation for the ecommerce template, including schema overview, pricing conventions using integer cents, order status lifecycle, setup instructions, and SDK usage examples.
func (ecommerceTemplate) Readme() string {
	return `# Ecommerce Template

This scaffold provisions a production-style ecommerce schema and typed helper client code.

## Included schema

- ` + "`products`" + `: product catalog, inventory, active status, and SKU
- ` + "`customers`" + `: customer profile linked one-to-one with AYB auth users
- ` + "`orders`" + `: order header with lifecycle status and total in cents
- ` + "`order_items`" + `: per-product line items for each order
- ` + "`carts`" + `: per-user JSONB cart payload for pre-checkout state

## Pricing convention

All money values use integer cents (for example ` + "`12999`" + ` means $129.99 USD) to avoid floating-point rounding issues.

## Order status lifecycle

` + "`pending → paid → shipped → delivered`" + ` (or ` + "`cancelled`" + ` when an order does not complete).

## Apply schema and seed data

` + "```bash" + `
ayb sql < schema.sql && ayb sql < seed.sql
` + "```" + `

## SDK usage example

` + "```ts" + `
import { listProducts, updateCart, createOrder } from "./src/lib/ecommerce";

const { items: products } = await listProducts();
await updateCart([
  { product_id: products[0].id, quantity: 1 },
  { product_id: products[1].id, quantity: 2 },
]);

const order = await createOrder("<customer-id>", [
  { product_id: products[0].id, quantity: 1, unit_price_cents: products[0].price_cents },
  { product_id: products[1].id, quantity: 2, unit_price_cents: products[1].price_cents },
]);
console.log("created order", order.id);
` + "```" + `

## Quick start

1. Start AYB with ` + "`ayb start`" + `.
2. Apply schema and seed data.
3. Use ` + "`src/lib/ecommerce.ts`" + ` helpers to build catalog, cart, and order flows.
`
}
