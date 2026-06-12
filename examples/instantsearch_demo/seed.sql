-- expected totals:
-- categories: Stationery=4, Lighting=4, Office=4, Kitchen=4, Travel=4
-- brands: Apex=4, Beacon=4, Cobalt=4, Drift=4, Ember=4
-- total_hits: 20
-- price_range price_cents_2000_6000: in_band=9, out_of_band=11
INSERT INTO instantsearch_products (slug, title, description, category, brand, price_cents) VALUES
  ('red-notebook', 'Red Notebook', 'Paper pages for research notes with the unique phrase crimson ledger', 'Stationery', 'Apex', 1299),
  ('blue-index-cards', 'Blue Index Cards', 'Study cards for spaced repetition with the unique phrase cobalt recall', 'Stationery', 'Cobalt', 899),
  ('graphite-pencil-set', 'Graphite Pencil Set', 'Sketch pencils for field diagrams with the unique phrase graphite orbit', 'Stationery', 'Drift', 999),
  ('archival-pen-case', 'Archival Pen Case', 'Ink case for durable notes with the unique phrase archive nib', 'Stationery', 'Ember', 2199),
  ('brass-desk-lamp', 'Brass Desk Lamp', 'Focused light for workspaces with the unique phrase amber beacon', 'Lighting', 'Beacon', 4599),
  ('linen-task-lamp', 'Linen Task Lamp', 'Soft desk lighting for reading with the unique phrase linen glow', 'Lighting', 'Apex', 3899),
  ('walnut-floor-lamp', 'Walnut Floor Lamp', 'Tall room lighting with the unique phrase walnut lantern', 'Lighting', 'Beacon', 7499),
  ('portable-reading-light', 'Portable Reading Light', 'Clip light for travel reading with the unique phrase pocket gleam', 'Lighting', 'Cobalt', 2499),
  ('steel-cable-tray', 'Steel Cable Tray', 'Under desk cable routing with the unique phrase tidy conduit', 'Office', 'Drift', 2499),
  ('oak-monitor-stand', 'Oak Monitor Stand', 'Raised display platform with the unique phrase oak horizon', 'Office', 'Apex', 5299),
  ('mesh-desk-organizer', 'Mesh Desk Organizer', 'Compartment storage with the unique phrase sorted lattice', 'Office', 'Cobalt', 1899),
  ('standing-desk-mat', 'Standing Desk Mat', 'Cushioned office mat with the unique phrase steady footing', 'Office', 'Ember', 6499),
  ('ceramic-coffee-mug', 'Ceramic Coffee Mug', 'Warm drink cup with the unique phrase morning kiln', 'Kitchen', 'Ember', 1599),
  ('glass-water-bottle', 'Glass Water Bottle', 'Reusable hydration bottle with the unique phrase clear spring', 'Kitchen', 'Drift', 2199),
  ('cotton-lunch-wrap', 'Cotton Lunch Wrap', 'Reusable meal wrap with the unique phrase folded harvest', 'Kitchen', 'Apex', 1399),
  ('countertop-air-filter', 'Countertop Air Filter', 'Compact kitchen air filter with the unique phrase fresh counter', 'Kitchen', 'Beacon', 11299),
  ('travel-tech-pouch', 'Travel Tech Pouch', 'Compact cord storage with the unique phrase roaming packet', 'Travel', 'Drift', 2999),
  ('canvas-weekender', 'Canvas Weekender', 'Overnight carry bag with the unique phrase canvas departure', 'Travel', 'Beacon', 8999),
  ('packing-cube-set', 'Packing Cube Set', 'Luggage organizers with the unique phrase folded itinerary', 'Travel', 'Cobalt', 3499),
  ('aluminum-carry-on', 'Aluminum Carry On', 'Rigid cabin suitcase with the unique phrase silver arrival', 'Travel', 'Ember', 12999)
ON CONFLICT (slug) DO UPDATE SET
  title = EXCLUDED.title,
  description = EXCLUDED.description,
  category = EXCLUDED.category,
  brand = EXCLUDED.brand,
  price_cents = EXCLUDED.price_cents;
