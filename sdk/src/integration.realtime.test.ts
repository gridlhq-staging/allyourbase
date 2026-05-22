import { EventSource as NodeEventSource } from "eventsource";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { AYBClient } from "./client";
import type { RealtimeEvent } from "./types";
import {
  AUTH_TEST_PASSWORD,
  INTEGRATION_RUN_ID,
  adminSql,
  cleanupTrackedAuthUsers,
  createTestClient,
  dropTableAndAssertRemoved,
  expectRealtimeEventShape,
  makeUniqueAuthEmail,
  primeIntegrationSuite,
  sleep,
  trackAuthUser,
  waitForCondition,
  waitForCollectionSchemaCache,
} from "./integration-helpers";

(globalThis as typeof globalThis & { EventSource?: typeof NodeEventSource }).EventSource ??=
  NodeEventSource as unknown as typeof globalThis.EventSource;

describe("SDK realtime integration suite", () => {
  const tableName = `sdk_rt_${INTEGRATION_RUN_ID}`;
  let client: AYBClient;

  async function openSubscription(): Promise<{
    receivedEvents: RealtimeEvent[];
    unsubscribe: () => void;
  }> {
    const receivedEvents: RealtimeEvent[] = [];
    const unsubscribe = client.realtime.subscribe([tableName], (event) => {
      receivedEvents.push(event);
    });
    await sleep(500);
    return { receivedEvents, unsubscribe };
  }

  async function waitForRealtimeAction(
    receivedEvents: RealtimeEvent[],
    action: RealtimeEvent["action"],
  ): Promise<void> {
    await waitForCondition({
      description: `${action} realtime event`,
      timeoutMs: 10_000,
      intervalMs: 200,
      check: async () => receivedEvents.some((event) => event.action === action),
    });
  }

  beforeAll(async () => {
    await primeIntegrationSuite();

    client = createTestClient();
    const email = makeUniqueAuthEmail("realtime");
    const registered = await client.auth.register(email, AUTH_TEST_PASSWORD);
    trackAuthUser(registered.user.id);
    await client.auth.logout();
    await client.auth.login(email, AUTH_TEST_PASSWORD);

    await adminSql(
      `CREATE TABLE ${tableName} (
        id serial PRIMARY KEY,
        title text NOT NULL,
        created_at timestamptz DEFAULT now()
      )`,
    );
    await adminSql(`ALTER TABLE ${tableName} ENABLE ROW LEVEL SECURITY`);
    await adminSql(
      `CREATE POLICY sdk_rt_all ON ${tableName} FOR ALL USING (true) WITH CHECK (true)`,
    );

    await waitForCollectionSchemaCache(client, tableName, "realtime");
  }, 60_000);

  afterAll(async () => {
    await dropTableAndAssertRemoved(tableName);
    await cleanupTrackedAuthUsers();
  }, 35_000);

  it("opens and closes realtime subscription without throwing", async () => {
    const unsubscribe = client.realtime.subscribe([tableName], () => {});
    await sleep(500);
    unsubscribe();
  });

  it("receives create events with expected shape", async () => {
    const { receivedEvents, unsubscribe } = await openSubscription();
    await client.records.create(tableName, { title: "rt-test" });

    await waitForRealtimeAction(receivedEvents, "create");
    unsubscribe();

    const createEvent = receivedEvents.find((event) => event.action === "create");
    expect(createEvent).toBeDefined();
    expect(createEvent?.table).toBe(tableName);
    expect(createEvent?.record.title).toBe("rt-test");
    expectRealtimeEventShape(receivedEvents);
  });

  it("receives update events with expected shape", async () => {
    const created = await client.records.create<{ id: number }>(tableName, {
      title: "before-update",
    });
    const { receivedEvents, unsubscribe } = await openSubscription();
    await client.records.update(tableName, String(created.id), {
      title: "updated",
    });

    await waitForRealtimeAction(receivedEvents, "update");
    unsubscribe();

    const updateEvent = receivedEvents.find((event) => event.action === "update");
    expect(updateEvent).toBeDefined();
    expect(updateEvent?.table).toBe(tableName);
    expect(updateEvent?.record.title).toBe("updated");
    expectRealtimeEventShape(receivedEvents);
  });

  it("receives delete events with expected shape", async () => {
    const created = await client.records.create<{ id: number }>(tableName, {
      title: "before-delete",
    });
    const { receivedEvents, unsubscribe } = await openSubscription();
    await client.records.delete(tableName, String(created.id));

    await waitForRealtimeAction(receivedEvents, "delete");
    unsubscribe();

    const deleteEvent = receivedEvents.find((event) => event.action === "delete");
    expect(deleteEvent).toBeDefined();
    expect(deleteEvent?.table).toBe(tableName);
    // Delete events carry PK values as strings (extracted from URL path
    // in internal/api/handler_crud.go:extractPK -> []string).
    expect(String(deleteEvent?.record.id)).toBe(String(created.id));
    expectRealtimeEventShape(receivedEvents);
  });

  it("stops receiving events after unsubscribe", async () => {
    const { receivedEvents, unsubscribe } = await openSubscription();
    unsubscribe();
    const eventCountBeforeCreate = receivedEvents.length;

    await client.records.create(tableName, { title: "after-unsubscribe" });
    await sleep(2_000);
    expect(receivedEvents).toHaveLength(eventCountBeforeCreate);
  });
});

describe("SDK realtime WS integration suite", () => {
  const tableName = `sdk_rtws_${INTEGRATION_RUN_ID}`;
  let client: AYBClient;

  async function openSubscriptionWS(): Promise<{
    receivedEvents: RealtimeEvent[];
    unsubscribe: () => void;
  }> {
    const receivedEvents: RealtimeEvent[] = [];
    const unsubscribe = await client.realtime.subscribeWS([tableName], (event) => {
      receivedEvents.push(event);
    });
    return { receivedEvents, unsubscribe };
  }

  async function waitForRealtimeAction(
    receivedEvents: RealtimeEvent[],
    action: RealtimeEvent["action"],
  ): Promise<void> {
    await waitForCondition({
      description: `WS ${action} realtime event`,
      timeoutMs: 10_000,
      intervalMs: 200,
      check: async () => receivedEvents.some((event) => event.action === action),
    });
  }

  beforeAll(async () => {
    await primeIntegrationSuite();

    client = createTestClient();
    const email = makeUniqueAuthEmail("realtime_ws");
    const registered = await client.auth.register(email, AUTH_TEST_PASSWORD);
    trackAuthUser(registered.user.id);
    await client.auth.logout();
    await client.auth.login(email, AUTH_TEST_PASSWORD);

    await adminSql(
      `CREATE TABLE ${tableName} (
        id serial PRIMARY KEY,
        title text NOT NULL,
        created_at timestamptz DEFAULT now()
      )`,
    );
    await adminSql(`ALTER TABLE ${tableName} ENABLE ROW LEVEL SECURITY`);
    await adminSql(
      `CREATE POLICY sdk_rtws_all ON ${tableName} FOR ALL USING (true) WITH CHECK (true)`,
    );

    await waitForCollectionSchemaCache(client, tableName, "realtime_ws");
  }, 60_000);

  afterAll(async () => {
    await dropTableAndAssertRemoved(tableName);
    await cleanupTrackedAuthUsers();
  }, 35_000);

  it("completes connected -> auth -> subscribe handshake", async () => {
    const { unsubscribe } = await openSubscriptionWS();
    unsubscribe();
  });

  it("receives create events via WS", async () => {
    const { receivedEvents, unsubscribe } = await openSubscriptionWS();
    await client.records.create(tableName, { title: "ws-create" });

    await waitForRealtimeAction(receivedEvents, "create");
    unsubscribe();

    const createEvent = receivedEvents.find((event) => event.action === "create");
    expect(createEvent).toBeDefined();
    expect(createEvent?.table).toBe(tableName);
    expect(createEvent?.record.title).toBe("ws-create");
    expectRealtimeEventShape(receivedEvents);
  });

  it("receives update events via WS", async () => {
    const created = await client.records.create<{ id: number }>(tableName, {
      title: "ws-before-update",
    });
    const { receivedEvents, unsubscribe } = await openSubscriptionWS();
    await client.records.update(tableName, String(created.id), {
      title: "ws-updated",
    });

    await waitForRealtimeAction(receivedEvents, "update");
    unsubscribe();

    const updateEvent = receivedEvents.find((event) => event.action === "update");
    expect(updateEvent).toBeDefined();
    expect(updateEvent?.table).toBe(tableName);
    expect(updateEvent?.record.title).toBe("ws-updated");
    expectRealtimeEventShape(receivedEvents);
  });

  it("receives delete events via WS", async () => {
    const created = await client.records.create<{ id: number }>(tableName, {
      title: "ws-before-delete",
    });
    const { receivedEvents, unsubscribe } = await openSubscriptionWS();
    await client.records.delete(tableName, String(created.id));

    await waitForRealtimeAction(receivedEvents, "delete");
    unsubscribe();

    const deleteEvent = receivedEvents.find((event) => event.action === "delete");
    expect(deleteEvent).toBeDefined();
    expect(deleteEvent?.table).toBe(tableName);
    expect(String(deleteEvent?.record.id)).toBe(String(created.id));
    expectRealtimeEventShape(receivedEvents);
  });

  it("stops receiving events after WS unsubscribe", async () => {
    const { receivedEvents, unsubscribe } = await openSubscriptionWS();
    unsubscribe();
    const eventCountBeforeCreate = receivedEvents.length;

    await client.records.create(tableName, { title: "ws-after-unsub" });
    await sleep(2_000);
    expect(receivedEvents).toHaveLength(eventCountBeforeCreate);
  });
});
