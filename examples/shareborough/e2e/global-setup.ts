/**
 * Global setup: clean test data before each Playwright run.
 * Since all libraries are public (visible to all users), stale data from
 * previous runs causes name collisions and strict mode violations.
 */
export default async function globalSetup() {
  const baseURL = process.env.AYB_URL ?? "http://localhost:8090";
  const adminPassword = process.env.AYB_ADMIN_PASSWORD ?? "";

  // Authenticate as admin to get a token for cleanup queries.
  let adminToken = "";
  if (adminPassword) {
    const authRes = await fetch(`${baseURL}/api/admin/auth`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: adminPassword }),
    });
    if (authRes.ok) {
      const data = (await authRes.json()) as { token: string };
      adminToken = data.token;
      console.log("Global setup: admin auth succeeded");
    } else {
      console.warn(`Global setup: admin auth failed (${authRes.status}), cleanup will be skipped`);
      return;
    }
  } else {
    console.warn("Global setup: AYB_ADMIN_PASSWORD not set, skipping cleanup");
    return;
  }

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Authorization: `Bearer ${adminToken}`,
  };

  // Use the admin SQL endpoint to clean test data.
  // Tables are deleted in dependency order (children first).
  const tables = [
    "reminders",
    "loans",
    "borrow_requests",
    "borrowers",
    "item_facets",
    "items",
    "facet_definitions",
    "libraries",
  ];

  for (const table of tables) {
    const res = await fetch(`${baseURL}/api/admin/sql`, {
      method: "POST",
      headers,
      body: JSON.stringify({ query: `DELETE FROM ${table}` }),
    });
    if (res.ok) {
      const data = (await res.json()) as { rowCount: number };
      if (data.rowCount > 0) {
        console.log(`  Cleaned ${data.rowCount} rows from ${table}`);
      }
    } else {
      console.warn(`  Warning: could not clean table ${table}: ${res.status}`);
    }
  }

  // Also clean test users (emails matching test pattern)
  const res = await fetch(`${baseURL}/api/admin/sql`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      query: `DELETE FROM _ayb_users WHERE email LIKE 'test-%@example.com'`,
    }),
  });
  if (res.ok) {
    const data = (await res.json()) as { rowCount: number };
    console.log(`  Cleaned ${data.rowCount} test users`);
  } else {
    console.warn(`  Warning: could not clean test users: ${res.status}`);
  }
}
