INSERT INTO instantsearch_products (slug, title, description, category, price_cents) VALUES
  ('red-notebook', 'Red Notebook', 'Paper pages for research notes with the unique phrase crimson ledger', 'Stationery', 1299),
  ('blue-index-cards', 'Blue Index Cards', 'Study cards for spaced repetition with the unique phrase cobalt recall', 'Stationery', 799),
  ('graphite-pencil-set', 'Graphite Pencil Set', 'Sketch pencils for field diagrams with the unique phrase graphite orbit', 'Stationery', 999),
  ('brass-desk-lamp', 'Brass Desk Lamp', 'Focused light for workspaces with the unique phrase amber beacon', 'Lighting', 4599),
  ('linen-task-lamp', 'Linen Task Lamp', 'Soft desk lighting for reading with the unique phrase linen glow', 'Lighting', 3899),
  ('walnut-floor-lamp', 'Walnut Floor Lamp', 'Tall room lighting with the unique phrase walnut lantern', 'Lighting', 7499),
  ('steel-cable-tray', 'Steel Cable Tray', 'Under desk cable routing with the unique phrase tidy conduit', 'Office', 2499),
  ('oak-monitor-stand', 'Oak Monitor Stand', 'Raised display platform with the unique phrase oak horizon', 'Office', 5299),
  ('mesh-desk-organizer', 'Mesh Desk Organizer', 'Compartment storage with the unique phrase sorted lattice', 'Office', 1899),
  ('ceramic-coffee-mug', 'Ceramic Coffee Mug', 'Warm drink cup with the unique phrase morning kiln', 'Kitchen', 1599),
  ('glass-water-bottle', 'Glass Water Bottle', 'Reusable hydration bottle with the unique phrase clear spring', 'Kitchen', 2199),
  ('cotton-lunch-wrap', 'Cotton Lunch Wrap', 'Reusable meal wrap with the unique phrase folded harvest', 'Kitchen', 1399),
  ('travel-tech-pouch', 'Travel Tech Pouch', 'Compact cord storage with the unique phrase roaming packet', 'Travel', 2999),
  ('canvas-weekender', 'Canvas Weekender', 'Overnight carry bag with the unique phrase canvas departure', 'Travel', 8999)
ON CONFLICT (slug) DO UPDATE SET
  title = EXCLUDED.title,
  description = EXCLUDED.description,
  category = EXCLUDED.category,
  price_cents = EXCLUDED.price_cents;
