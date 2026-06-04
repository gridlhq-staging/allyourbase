import { mkdtempSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { AYBClient } from "./client";
import type { RecordsFixture, SeedInput } from "./integration-helpers";
import {
  AUTH_TEST_PASSWORD,
  BASE_URL,
  INTEGRATION_RUN_ID,
  adminSql,
  captureAuthEvents,
  cleanupTrackedAuthUsers,
  createTestClient,
  dropTableAndAssertRemoved,
  expectAYBError,
  getAdminToken,
  getCachedAdminToken,
  makeUniqueAuthEmail,
  primeIntegrationSuite,
  sqlStringLiteral,
  toCount,
  trackAuthUser,
  trackedAuthUserIDs,
  trackedUserIDSQLList,
  waitForCollectionSchemaCache,
} from "./integration-helpers";

describe("SDK integration smoke + auth suite", () => {
  beforeAll(async () => {
    await primeIntegrationSuite();
  }, 35_000);

  afterAll(async () => {
    await cleanupTrackedAuthUsers();
  }, 35_000);

  it("GET /health returns 200", async () => {
    const response = await fetch(`${BASE_URL}/health`);
    expect(response.status).toBe(200);
  });

  it("resolves a non-empty admin token", async () => {
    const cachedAdminToken = getCachedAdminToken();
    expect(cachedAdminToken).toBeTypeOf("string");
    expect((cachedAdminToken ?? "").trim().length).toBeGreaterThan(0);
  });

  it("uses the admin-token file directly instead of reposting it as a password", async () => {
    const adminToken = await getAdminToken();
    const tempHome = mkdtempSync(path.join(tmpdir(), "ayb-admin-token-"));
    const tokenPath = path.join(tempHome, ".ayb", "admin-token");
    mkdirSync(path.dirname(tokenPath), { recursive: true });
    writeFileSync(tokenPath, `${adminToken}\n`);

    const originalFetch = globalThis.fetch;
    const originalEnv = {
      AYB_ADMIN_TOKEN: process.env.AYB_ADMIN_TOKEN,
      AYB_ADMIN_PASSWORD: process.env.AYB_ADMIN_PASSWORD,
      AYB_ADMIN_TOKEN_PATH: process.env.AYB_ADMIN_TOKEN_PATH,
    };
    const unexpectedFetch = vi.fn(async () => {
      throw new Error("getAdminToken should not call fetch when admin-token file exists");
    });

    delete process.env.AYB_ADMIN_TOKEN;
    delete process.env.AYB_ADMIN_PASSWORD;
    process.env.AYB_ADMIN_TOKEN_PATH = tokenPath;
    globalThis.fetch = unexpectedFetch as typeof globalThis.fetch;
    vi.resetModules();

    try {
      const freshHelpers = await import("./integration-helpers");
      expect(await freshHelpers.getAdminToken()).toBe(adminToken);
      expect(unexpectedFetch).not.toHaveBeenCalled();
    } finally {
      globalThis.fetch = originalFetch;
      for (const [key, value] of Object.entries(originalEnv)) {
        if (value === undefined) {
          delete process.env[key];
        } else {
          process.env[key] = value;
        }
      }
      vi.resetModules();
    }
  });

  it("runs SELECT 1 through admin SQL endpoint", async () => {
    const result = await adminSql("SELECT 1 AS ok");
    expect(result.columns).toEqual(["ok"]);
    expect(result.rows).toEqual([[1]]);
    expect(result.rowCount).toBe(1);
  });

  it("creates unique auth emails and reusable AYB clients", () => {
    const emailOne = makeUniqueAuthEmail("fixture");
    const emailTwo = makeUniqueAuthEmail("fixture");
    expect(emailOne).not.toBe(emailTwo);
    expect(emailOne).toContain(`sdk-${INTEGRATION_RUN_ID}-fixture-`);

    const client = createTestClient();
    expect(client).toBeInstanceOf(AYBClient);
  });

  it("covers register/login/me/refresh/logout lifecycle with real /api/auth endpoints", async () => {
    const client = createTestClient();
    const email = makeUniqueAuthEmail("happy-path");
    const password = AUTH_TEST_PASSWORD;

    const registered = await client.auth.register(email, password);
    trackAuthUser(registered.user.id);
    expect(registered.user.email).toBe(email);
    expect(registered.token.length).toBeGreaterThan(0);
    expect(registered.refreshToken.length).toBeGreaterThan(0);
    expect(client.token).toBe(registered.token);
    expect(client.refreshToken).toBe(registered.refreshToken);

    await client.auth.logout();
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();

    const loggedIn = await client.auth.login(email, password);
    expect(loggedIn.user.id).toBe(registered.user.id);
    expect(loggedIn.user.email).toBe(email);
    const refreshTokenBeforeRotation = loggedIn.refreshToken;

    const me = await client.auth.me();
    expect(me.id).toBe(registered.user.id);
    expect(me.email).toBe(email);

    const refreshed = await client.auth.refresh();
    expect(refreshed.user.id).toBe(registered.user.id);
    expect(refreshed.refreshToken).not.toBe(refreshTokenBeforeRotation);
    expect(client.refreshToken).toBe(refreshed.refreshToken);

    const refreshRotationProbe = createTestClient();
    refreshRotationProbe.setTokens("stale-access-token", refreshTokenBeforeRotation);
    await expectAYBError(
      () => refreshRotationProbe.auth.refresh(),
      401,
      "invalid or expired refresh token",
    );

    const refreshTokenBeforeLogout = refreshed.refreshToken;
    await client.auth.logout();
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();

    const logoutProbe = createTestClient();
    logoutProbe.setTokens("stale-access-token", refreshTokenBeforeLogout);
    await expectAYBError(
      () => logoutProbe.auth.refresh(),
      401,
      "invalid or expired refresh token",
    );
  });

  it("returns 409 'email already registered' for duplicate registration", async () => {
    const client = createTestClient();
    const email = makeUniqueAuthEmail("duplicate-register");
    const password = AUTH_TEST_PASSWORD;
    const authEvents = captureAuthEvents(client);
    const initial = await client.auth.register(email, password);
    trackAuthUser(initial.user.id);
    expect(authEvents.events).toHaveLength(1);

    const baselineSession = {
      token: client.token,
      refreshToken: client.refreshToken,
    };

    await expectAYBError(
      () => client.auth.register(email, password),
      409,
      "email already registered",
    );
    expect(client.token).toBe(baselineSession.token);
    expect(client.refreshToken).toBe(baselineSession.refreshToken);
    expect(authEvents.events).toHaveLength(1);
    authEvents.unsubscribe();
  });

  it("returns 401 'invalid email or password' for wrong-password login", async () => {
    const seedClient = createTestClient();
    const email = makeUniqueAuthEmail("wrong-password");
    const password = AUTH_TEST_PASSWORD;
    const registered = await seedClient.auth.register(email, password);
    trackAuthUser(registered.user.id);
    await seedClient.auth.logout();

    const failingClient = createTestClient();
    const authEvents = captureAuthEvents(failingClient);
    await expectAYBError(
      () => failingClient.auth.login(email, "definitely-wrong-password"),
      401,
      "invalid email or password",
    );
    expect(failingClient.token).toBeNull();
    expect(failingClient.refreshToken).toBeNull();
    expect(authEvents.events).toEqual([]);
    authEvents.unsubscribe();
  });

  it("emits SIGNED_IN, TOKEN_REFRESHED, SIGNED_OUT with post-action session payloads", async () => {
    const client = createTestClient();
    const email = makeUniqueAuthEmail("events");
    const password = AUTH_TEST_PASSWORD;
    const authEvents = captureAuthEvents(client);

    const registered = await client.auth.register(email, password);
    trackAuthUser(registered.user.id);
    const refreshed = await client.auth.refresh();
    await client.auth.logout();

    authEvents.unsubscribe();
    expect(authEvents.events).toEqual([
      {
        event: "SIGNED_IN",
        session: {
          token: registered.token,
          refreshToken: registered.refreshToken,
        },
      },
      {
        event: "TOKEN_REFRESHED",
        session: {
          token: refreshed.token,
          refreshToken: refreshed.refreshToken,
        },
      },
      {
        event: "SIGNED_OUT",
        session: null,
      },
    ]);
  });

  describe("Records CRUD", () => {
    const tableName = `sdk_records_${INTEGRATION_RUN_ID}`;
    const seedFixtures: SeedInput[] = [
      { title: "alpha-seed", priority: 1 },
      { title: "beta-seed", priority: 2 },
      { title: "gamma", priority: 3 },
      { title: "delta-seed", priority: 4 },
      { title: "epsilon", priority: 5 },
      { title: "zeta-seed", priority: 6 },
    ];

    let client: AYBClient;
    let createdRecord: RecordsFixture | null = null;
    let seededRecords: RecordsFixture[] = [];

    function getCreatedRecord(): RecordsFixture {
      expect(createdRecord).not.toBeNull();
      return createdRecord as RecordsFixture;
    }

    async function resetAndSeedRecords(): Promise<RecordsFixture[]> {
      await adminSql(`TRUNCATE TABLE ${tableName} RESTART IDENTITY`);
      const records: RecordsFixture[] = [];
      for (const fixture of seedFixtures) {
        const created = await client.records.create<RecordsFixture>(tableName, fixture);
        records.push(created);
      }
      return records;
    }

    beforeAll(async () => {
      client = createTestClient();
      const email = makeUniqueAuthEmail("records-crud");
      const registered = await client.auth.register(email, AUTH_TEST_PASSWORD);
      trackAuthUser(registered.user.id);
      await client.auth.logout();
      await client.auth.login(email, AUTH_TEST_PASSWORD);

      await adminSql(
        `CREATE TABLE ${tableName} (
          id serial PRIMARY KEY,
          title text NOT NULL,
          priority int DEFAULT 0,
          created_at timestamptz DEFAULT now()
        )`,
      );
      await adminSql(`ALTER TABLE ${tableName} ENABLE ROW LEVEL SECURITY`);
      await adminSql(`CREATE POLICY sdk_test_all ON ${tableName} FOR ALL USING (true) WITH CHECK (true)`);

      await waitForCollectionSchemaCache(client, tableName, "records");
    }, 60_000);

    afterAll(async () => {
      await dropTableAndAssertRemoved(tableName);
    }, 35_000);

    it("creates a record", async () => {
      const created = await client.records.create<RecordsFixture>(tableName, {
        title: "hello",
        priority: 1,
      });
      createdRecord = created;

      expect(Number.isInteger(created.id)).toBe(true);
      expect(created.title).toBe("hello");
      expect(created.priority).toBe(1);
      expect(Number.isNaN(Date.parse(created.created_at))).toBe(false);
    });

    it("reads the created record by id", async () => {
      const record = getCreatedRecord();
      const recordID = String(record.id);
      const fetched = await client.records.get<RecordsFixture>(tableName, recordID);

      expect(fetched.id).toBe(record.id);
      expect(fetched.title).toBe(record.title);
      expect(fetched.priority).toBe(record.priority);
      expect(fetched.created_at).toBe(record.created_at);
    });

    it("updates the created record and persists the change", async () => {
      const record = getCreatedRecord();
      const recordID = String(record.id);
      const updated = await client.records.update<RecordsFixture>(tableName, recordID, {
        title: "updated",
        priority: 99,
      });

      expect(updated.id).toBe(record.id);
      expect(updated.title).toBe("updated");
      expect(updated.priority).toBe(99);

      const fetched = await client.records.get<RecordsFixture>(tableName, recordID);
      expect(fetched.title).toBe("updated");
      expect(fetched.priority).toBe(99);
      createdRecord = updated;
    });

    it("deletes the created record and get returns 404", async () => {
      const recordID = String(getCreatedRecord().id);
      const deleteResult = await client.records.delete(tableName, recordID);
      expect(deleteResult).toBeUndefined();

      await expectAYBError(
        () => client.records.get(tableName, recordID),
        404,
        "record not found",
      );
      createdRecord = null;
    });

    describe("query, batch, and error paths", () => {
      beforeEach(async () => {
        seededRecords = await resetAndSeedRecords();
      });

      it("lists seeded rows with the default pagination envelope", async () => {
        const listed = await client.records.list<RecordsFixture>(tableName);

        expect(listed.items).toHaveLength(seededRecords.length);
        expect(listed.page).toBe(1);
        expect(listed.perPage).toBe(20);
        expect(listed.totalItems).toBe(seededRecords.length);
        expect(listed.totalPages).toBe(1);
      });

      it("filters rows by numeric and LIKE operators", async () => {
        const byPriority = await client.records.list<RecordsFixture>(tableName, {
          filter: "priority>2",
        });
        expect(byPriority.items.length).toBeGreaterThan(0);
        expect(byPriority.items.every((item) => item.priority > 2)).toBe(true);

        const byTitle = await client.records.list<RecordsFixture>(tableName, {
          filter: "title~'%seed%'",
        });
        expect(byTitle.items.length).toBeGreaterThan(0);
        expect(byTitle.items.every((item) => item.title.includes("seed"))).toBe(true);
      });

      it("sorts rows descending and ascending", async () => {
        const byPriorityDesc = await client.records.list<RecordsFixture>(tableName, {
          sort: "-priority",
        });
        for (let i = 1; i < byPriorityDesc.items.length; i += 1) {
          expect(byPriorityDesc.items[i - 1].priority).toBeGreaterThanOrEqual(
            byPriorityDesc.items[i].priority,
          );
        }

        const byTitleAsc = await client.records.list<RecordsFixture>(tableName, {
          sort: "+title",
        });
        for (let i = 1; i < byTitleAsc.items.length; i += 1) {
          expect(byTitleAsc.items[i - 1].title.localeCompare(byTitleAsc.items[i].title)).toBeLessThanOrEqual(0);
        }
      });

      it("paginates rows without overlap", async () => {
        const firstPage = await client.records.list<RecordsFixture>(tableName, {
          page: 1,
          perPage: 2,
          sort: "+id",
        });
        const secondPage = await client.records.list<RecordsFixture>(tableName, {
          page: 2,
          perPage: 2,
          sort: "+id",
        });

        expect(firstPage.items).toHaveLength(2);
        expect(firstPage.totalPages).toBe(Math.ceil(firstPage.totalItems / 2));
        const firstPageIDs = new Set(firstPage.items.map((item) => item.id));
        const secondPageIDs = new Set(secondPage.items.map((item) => item.id));
        for (const secondPageID of secondPageIDs) {
          expect(firstPageIDs.has(secondPageID)).toBe(false);
        }
      });

      it("handles batch create, update, and delete", async () => {
        const created = await client.records.batch<RecordsFixture>(tableName, [
          { method: "create", body: { title: "b1", priority: 10 } },
          { method: "create", body: { title: "b2", priority: 20 } },
        ]);
        expect(created).toHaveLength(2);
        expect(created[0].status).toBe(201);
        expect(created[1].status).toBe(201);
        expect(created[0].body?.id).toBeTypeOf("number");
        expect(created[1].body?.id).toBeTypeOf("number");

        const firstID = String(created[0].body?.id);
        const secondID = String(created[1].body?.id);
        const updated = await client.records.batch<RecordsFixture>(tableName, [
          { method: "update", id: firstID, body: { title: "b1-updated", priority: 11 } },
          { method: "update", id: secondID, body: { title: "b2-updated", priority: 21 } },
        ]);
        expect(updated[0].status).toBe(200);
        expect(updated[1].status).toBe(200);

        const deleted = await client.records.batch<RecordsFixture>(tableName, [
          { method: "delete", id: firstID },
          { method: "delete", id: secondID },
        ]);
        expect(deleted[0].status).toBe(204);
        expect(deleted[1].status).toBe(204);

        await expectAYBError(
          () => client.records.get(tableName, firstID),
          404,
          "record not found",
        );
        await expectAYBError(
          () => client.records.get(tableName, secondID),
          404,
          "record not found",
        );
      });

      it("returns expected 404 errors for missing collection and missing record", async () => {
        await expectAYBError(
          () => client.records.list("nonexistent_table_xyz"),
          404,
          "collection not found: nonexistent_table_xyz",
        );
        await expectAYBError(
          () => client.records.get(tableName, "999999999"),
          404,
          "record not found",
        );
      });
    });

    describe("records search (fuzzy + facets)", () => {
      // Dedicated table because the records list handler 500s on the first list
      // request after ALTER TABLE (pgx cached prepared statement returns stale
      // result type; SQLSTATE 0A000). Fixing that connection-pool behavior is a
      // Go backend change explicitly out of scope for this stage; the contract
      // we need to prove (facet counts + RLS scoping + unknown-column reject)
      // is fully exercised on a fresh table that includes `category` from the
      // start.
      const facetsTableName = `${tableName}_facets`;
      type SearchFixture = { id: number; title: string; category: string };
      let pgTrgmInstalled = false;
      let searchFixtures: SearchFixture[] = [];

      function fixtureID(title: string): number {
        const fixture = searchFixtures.find((row) => row.title === title);
        expect(fixture).toBeDefined();
        return fixture!.id;
      }

      beforeAll(async () => {
        await adminSql(
          `CREATE TABLE ${facetsTableName} (
            id serial PRIMARY KEY,
            title text NOT NULL,
            category text
          )`,
        );
        await adminSql(`ALTER TABLE ${facetsTableName} ENABLE ROW LEVEL SECURITY`);
        await adminSql(
          `CREATE POLICY sdk_test_all ON ${facetsTableName} FOR ALL USING (true) WITH CHECK (true)`,
        );
        const extensionProbe = await adminSql(
          `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')`,
        );
        pgTrgmInstalled = extensionProbe.rows[0]?.[0] === true;
        await waitForCollectionSchemaCache(client, facetsTableName, "facets records");
      }, 35_000);

      afterAll(async () => {
        await dropTableAndAssertRemoved(facetsTableName);
      }, 35_000);

      beforeEach(async () => {
        await adminSql(`TRUNCATE TABLE ${facetsTableName} RESTART IDENTITY`);
        const seeded = await adminSql(
          `INSERT INTO ${facetsTableName} (title, category) VALUES ` +
            `('banana split','dessert'), ('banna bread','dessert'), ` +
            `('apple pie','fruit'), ('appel tart','fruit'), ` +
            `('celery sticks','other'), ('carrot soup','other') ` +
            `RETURNING id, title, category`,
        );
        searchFixtures = seeded.rows.map(([id, title, category]) => ({
          id: Number(id),
          title: String(title),
          category: String(category),
        }));
      });

      it("returns fuzzy typo matches when fuzzy: true is enabled", async (ctx) => {
        if (!pgTrgmInstalled) {
          ctx.skip("pg_trgm extension not installed on test server — fuzzy search unavailable");
        }

        const result = await client.records.list<{ id: number; title: string; category: string }>(
          facetsTableName,
          { search: "banan", fuzzy: true },
        );
        const returnedIDs = new Set(result.items.map((item) => item.id));

        expect(returnedIDs.has(fixtureID("banana split"))).toBe(true);
        expect(returnedIDs.has(fixtureID("banna bread"))).toBe(true);
        expect(returnedIDs.has(fixtureID("apple pie"))).toBe(false);
      });

      it("returns balanced facet counts alongside the page items", async () => {
        const result = await client.records.list<{ id: number; title: string; category: string }>(
          facetsTableName,
          { facets: ["category"] },
        );
        const categoryCounts = Object.fromEntries(
          (result.facets?.category ?? []).map(({ value, count }) => [String(value), count]),
        );

        expect(result.facets).toBeDefined();
        expect(categoryCounts).toEqual({ dessert: 2, fruit: 2, other: 2 });
        expect(result.facets!.category).toHaveLength(3);
        expect(result.items).toHaveLength(6);
      });

      it("rejects unknown facet columns with a 400 error", async () => {
        await expectAYBError(
          () => client.records.list(facetsTableName, { facets: ["nonexistent_col"] }),
          400,
          /unknown column .*nonexistent_col/i,
        );
      });
    });
  });

  describe("auth webauthn MFA (HTTP contract)", () => {
    let sessionToken = "";
    let pendingToken = "";

    async function expectErrorEnvelope(
      response: Response,
      status: number,
      message?: string | RegExp,
    ): Promise<void> {
      expect(response.status).toBe(status);
      const body = (await response.json()) as { code?: unknown; message?: unknown };
      expect(body.code).toBe(status);
      expect(typeof body.message).toBe("string");
      if (typeof message === "string") {
        expect(body.message).toBe(message);
      } else if (message) {
        expect(String(body.message)).toMatch(message);
      }
    }

    beforeAll(async () => {
      const enrollClient = createTestClient();
      const enrollEmail = makeUniqueAuthEmail("webauthn-enroll");
      const enrollRegistered = await enrollClient.auth.register(enrollEmail, AUTH_TEST_PASSWORD);
      trackAuthUser(enrollRegistered.user.id);
      sessionToken = enrollRegistered.token;

      const pendingClient = createTestClient();
      const pendingEmail = makeUniqueAuthEmail("webauthn-pending");
      const pendingRegistered = await pendingClient.auth.register(pendingEmail, AUTH_TEST_PASSWORD);
      trackAuthUser(pendingRegistered.user.id);

      // A pending-MFA token only requires one enabled MFA row. Insert a minimal
      // WebAuthn factor so password login returns mfa_pending and the malformed
      // /verify request reaches WebAuthn handler validation instead of failing
      // the auth gate first.
      await adminSql(
        `INSERT INTO _ayb_user_mfa (user_id, method, enabled, enrolled_at, webauthn_display_name)
         VALUES (${sqlStringLiteral(pendingRegistered.user.id)}, 'webauthn', true, NOW(), 'sdk contract passkey')`,
      );
      await pendingClient.auth.logout();

      const loginResponse = await fetch(`${BASE_URL}/api/auth/login`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: pendingEmail, password: AUTH_TEST_PASSWORD }),
      });
      expect(loginResponse.status).toBe(200);
      const loginBody = (await loginResponse.json()) as {
        mfa_pending?: unknown;
        mfa_token?: unknown;
      };
      expect(loginBody.mfa_pending).toBe(true);
      pendingToken = String(loginBody.mfa_token ?? "");
      expect(pendingToken.length).toBeGreaterThan(0);
    }, 60_000);

    it("returns enroll options with challenge, rp.id, user.id, and pubKeyCredParams", async () => {
      const response = await fetch(`${BASE_URL}/api/auth/mfa/webauthn/enroll`, {
        method: "POST",
        headers: { Authorization: `Bearer ${sessionToken}` },
      });
      expect(response.status).toBe(200);
      const body = (await response.json()) as {
        challenge?: unknown;
        rp?: { id?: unknown };
        user?: { id?: unknown };
        pubKeyCredParams?: unknown[];
      };

      expect(typeof body.challenge).toBe("string");
      expect(String(body.challenge ?? "").length).toBeGreaterThan(0);
      expect(typeof body.rp?.id).toBe("string");
      expect(String(body.rp?.id ?? "").length).toBeGreaterThan(0);
      expect(typeof body.user?.id).toBe("string");
      expect(String(body.user?.id ?? "").length).toBeGreaterThan(0);
      expect(Array.isArray(body.pubKeyCredParams)).toBe(true);
      expect((body.pubKeyCredParams ?? []).length).toBeGreaterThan(0);
    });

    it("rejects malformed enroll confirmations with the AYB error envelope", async () => {
      const response = await fetch(`${BASE_URL}/api/auth/mfa/webauthn/enroll/confirm`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${sessionToken}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({}),
      });

      await expectErrorEnvelope(response, 400, "attestation_response is required");
    });

    it("requires an MFA-pending bearer for the WebAuthn challenge endpoint", async () => {
      const missingAuth = await fetch(`${BASE_URL}/api/auth/mfa/webauthn/challenge`, {
        method: "POST",
      });
      await expectErrorEnvelope(missingAuth, 401, "no MFA challenge pending");

      const plainSession = await fetch(`${BASE_URL}/api/auth/mfa/webauthn/challenge`, {
        method: "POST",
        headers: { Authorization: `Bearer ${sessionToken}` },
      });
      await expectErrorEnvelope(plainSession, 401, "no MFA challenge pending");
    });

    it("requires an MFA-pending bearer for the WebAuthn verify endpoint", async () => {
      const missingAuth = await fetch(`${BASE_URL}/api/auth/mfa/webauthn/verify`, {
        method: "POST",
      });
      await expectErrorEnvelope(missingAuth, 401, "no MFA challenge pending");

      const plainSession = await fetch(`${BASE_URL}/api/auth/mfa/webauthn/verify`, {
        method: "POST",
        headers: { Authorization: `Bearer ${sessionToken}` },
      });
      await expectErrorEnvelope(plainSession, 401, "no MFA challenge pending");
    });

    it("rejects malformed WebAuthn verify payloads with the AYB error envelope", async () => {
      const missingChallenge = await fetch(`${BASE_URL}/api/auth/mfa/webauthn/verify`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${pendingToken}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ assertion: null }),
      });
      await expectErrorEnvelope(missingChallenge, 400, "challenge_id is required");

      const missingAssertion = await fetch(`${BASE_URL}/api/auth/mfa/webauthn/verify`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${pendingToken}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ challenge_id: "sdk-contract-challenge" }),
      });
      await expectErrorEnvelope(missingAssertion, 400, "assertion_response is required");
    });
  });

  describe("Storage", () => {
    const bucketName = `sdk-test-${INTEGRATION_RUN_ID}`;
    let client: AYBClient;

    beforeAll(async () => {
      // Register and login a dedicated user for storage tests.
      client = createTestClient();
      const email = makeUniqueAuthEmail("storage");
      const registered = await client.auth.register(email, AUTH_TEST_PASSWORD);
      trackAuthUser(registered.user.id);

      // Create a public test bucket (admin-only endpoint).
      const adminToken = await getAdminToken();
      const createRes = await fetch(`${BASE_URL}/api/storage/buckets/`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${adminToken}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ name: bucketName, public: true }),
      });
      expect(createRes.status).toBe(201);
    }, 35_000);

    afterAll(async () => {
      const adminToken = await getAdminToken();

      // Force-delete bucket (also deletes all objects inside).
      const deleteRes = await fetch(
        `${BASE_URL}/api/storage/buckets/${encodeURIComponent(bucketName)}?force=true`,
        {
          method: "DELETE",
          headers: { Authorization: `Bearer ${adminToken}` },
        },
      );
      expect(deleteRes.status).toBe(204);

      // Verify bucket no longer appears in list.
      const listRes = await fetch(`${BASE_URL}/api/storage/buckets/`, {
        headers: { Authorization: `Bearer ${adminToken}` },
      });
      expect(listRes.ok).toBe(true);
      const body = (await listRes.json()) as { items: Array<{ name: string }> };
      const bucketNames = body.items.map((b) => b.name);
      expect(bucketNames).not.toContain(bucketName);
    }, 35_000);

    it("uploads a text blob and returns valid StorageObject metadata", async () => {
      const blob = new Blob(["hello world"], { type: "text/plain" });
      const result = await client.storage.upload(bucketName, blob, "test.txt");

      expect(result.id).toBeTruthy();
      expect(result.bucket).toBe(bucketName);
      expect(result.name).toBe("test.txt");
      expect(result.contentType).toContain("text/plain");
      expect(result.size).toBeGreaterThan(0);
      expect(Number.isNaN(Date.parse(result.createdAt))).toBe(false);
    });

    it("uploads a binary blob and returns correct metadata", async () => {
      const bytes = new Uint8Array([0x89, 0x50, 0x4e, 0x47]);
      const blob = new Blob([bytes], { type: "image/png" });
      const result = await client.storage.upload(bucketName, blob, "binary.png");

      expect(result.id).toBeTruthy();
      expect(result.bucket).toBe(bucketName);
      expect(result.name).toBe("binary.png");
      expect(result.contentType).toContain("image/png");
      expect(result.size).toBe(4);
    });

    it("downloads text file content via downloadURL", async () => {
      // downloadURL is synchronous — returns a URL string.
      const url = client.storage.downloadURL(bucketName, "test.txt");
      expect(url).toContain(`/api/storage/${bucketName}/test.txt`);

      const response = await fetch(url);
      expect(response.ok).toBe(true);
      expect(await response.text()).toBe("hello world");
    });

    it("downloads binary file content and verifies bytes", async () => {
      const url = client.storage.downloadURL(bucketName, "binary.png");
      const response = await fetch(url);
      expect(response.ok).toBe(true);

      const buffer = await response.arrayBuffer();
      const actual = new Uint8Array(buffer);
      expect(actual).toEqual(new Uint8Array([0x89, 0x50, 0x4e, 0x47]));
    });

    it("lists objects with prefix filtering", async () => {
      // Upload 3 files with shared and distinct prefixes.
      await client.storage.upload(
        bucketName,
        new Blob(["a"], { type: "text/plain" }),
        "prefix/a.txt",
      );
      await client.storage.upload(
        bucketName,
        new Blob(["b"], { type: "text/plain" }),
        "prefix/b.txt",
      );
      await client.storage.upload(
        bucketName,
        new Blob(["c"], { type: "text/plain" }),
        "other/c.txt",
      );

      const listed = await client.storage.list(bucketName, { prefix: "prefix/" });
      expect(listed.items).toHaveLength(2);
      expect(listed.totalItems).toBe(2);
      expect(listed.items.every((item) => item.name.startsWith("prefix/"))).toBe(true);
    });

    it("generates a signed URL that serves content without auth", async () => {
      // test.txt was uploaded earlier in this describe block.
      const signed = await client.storage.getSignedURL(bucketName, "test.txt", 3600);
      expect(signed.url).toContain("/api/storage/");

      // Signed URL is relative — prepend BASE_URL; fetch without auth headers.
      const response = await fetch(`${BASE_URL}${signed.url}`);
      expect(response.ok).toBe(true);
      expect(await response.text()).toBe("hello world");
    });

    it("deletes a file and confirms 404 on subsequent download", async () => {
      // Must run after signed URL test since both reference test.txt.
      const deleteResult = await client.storage.delete(bucketName, "test.txt");
      expect(deleteResult).toBeUndefined();

      const response = await fetch(
        client.storage.downloadURL(bucketName, "test.txt"),
      );
      expect(response.status).toBe(404);
    });

    it("returns errors for invalid bucket name and nonexistent file signing", async () => {
      // Upload with invalid bucket name (uppercase chars) → 400 ErrInvalidBucket.
      await expectAYBError(
        () =>
          client.storage.upload(
            "INVALID!Bucket",
            new Blob(["data"], { type: "text/plain" }),
            "file.txt",
          ),
        400,
        "invalid bucket name: bucket name must contain only lowercase letters, digits, hyphens, underscores",
      );

      // Sign URL for nonexistent file → expect 404 "file not found".
      await expectAYBError(
        () => client.storage.getSignedURL(bucketName, "nonexistent-file.txt", 3600),
        404,
        "file not found",
      );
    });
  });

  it("post-suite audit seeds tracked identities for deterministic afterAll cleanup", async () => {
    expect(trackedAuthUserIDs.size).toBeGreaterThan(0);
    const userIDSQLList = trackedUserIDSQLList();
    const trackedCount = await adminSql(
      `SELECT COUNT(*) AS count FROM _ayb_users WHERE id IN (${userIDSQLList})`,
    );
    const trackedRowCount = toCount(trackedCount.rows[0]?.[0]);
    expect(trackedRowCount).toBeGreaterThan(0);
    expect(trackedRowCount).toBeLessThanOrEqual(trackedAuthUserIDs.size);
  });
});
