import {
  adminPath,
  buildParallelSafeRunID,
  execSQL,
  expect,
  probeEndpoint,
  sqlLiteral,
  test,
  waitForDashboard,
} from "../fixtures";

const GRAPHQL_PATH = "/api/graphql";

test.describe("GraphQL explorer", () => {
  let tableName: string | undefined;

  test.afterEach(async ({ adminToken, request }) => {
    if (tableName) {
      await execSQL(request, adminToken, `DROP TABLE IF EXISTS ${tableName}`).catch(() => {});
      tableName = undefined;
    }
  });

  test("queries seeded data and explores the live schema", async ({
    adminToken,
    page,
    request,
  }, testInfo) => {
    const graphqlStatus = await probeEndpoint(request, adminToken, GRAPHQL_PATH, {
      method: "POST",
      data: { query: "{ __typename }" },
    });
    if (graphqlStatus === 404 && process.env.AYB_EXPECT_GRAPHQL_ENABLED === undefined) {
      test.skip(true, "GraphQL is not enabled in this optional-backend runtime");
    }
    expect(
      graphqlStatus,
      `GraphQL precondition must be 200, received ${graphqlStatus}`,
    ).toBe(200);

    const runID = buildParallelSafeRunID(testInfo);
    tableName = `graphql_explorer_${runID}`;
    const seed = {
      id: 17,
      title: `Explorer row ${runID}`,
      score: 29,
    };
    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${tableName} (
        id integer PRIMARY KEY,
        title text NOT NULL,
        score integer NOT NULL
      );
      INSERT INTO ${tableName} (id, title, score)
      VALUES (${seed.id}, '${sqlLiteral(seed.title)}', ${seed.score});`,
    );

    const pageErrors: string[] = [];
    const consoleErrors: string[] = [];
    page.on("pageerror", (error) => pageErrors.push(error.message));
    page.on("console", (message) => {
      if (message.type() === "error") {
        consoleErrors.push(message.text());
      }
    });

    await page.goto(adminPath());
    await waitForDashboard(page);
    const sidebar = page.getByRole("complementary");
    await sidebar.getByRole("button", { name: "GraphQL", exact: true }).click();

    const query = `query ExplorerRow($title: String!) {
  ${tableName}(where: { title: { _eq: $title } }, limit: 1) {
    id
    title
    score
  }
}`;
    const variables = { title: seed.title };
    await page.getByLabel("GraphQL query").fill(query);
    await page.getByLabel("GraphQL variables").fill(JSON.stringify(variables));

    const responsePromise = page.waitForResponse((response) => {
      const observedRequest = response.request();
      return (
        observedRequest.method() === "POST" &&
        new URL(response.url()).pathname === GRAPHQL_PATH
      );
    });
    await page.getByRole("button", { name: "Send", exact: true }).click();
    const graphqlResponse = await responsePromise;
    expect(graphqlResponse.status()).toBe(200);

    const responseEnvelope = await graphqlResponse.json();
    const expectedEnvelope = { data: { [tableName]: [seed] } };
    expect(responseEnvelope).toEqual(expectedEnvelope);
    expect(responseEnvelope).not.toHaveProperty("errors");
    await testInfo.attach("graphql-response.json", {
      body: JSON.stringify(responseEnvelope, null, 2),
      contentType: "application/json",
    });

    await expect(page.getByText("200 OK", { exact: true })).toBeVisible();
    await expect(page.getByText(/^\d+ms$/)).toBeVisible();
    const renderedResponse = page.getByTestId("graphql-response-body");
    await expect(renderedResponse).toContainText(seed.title);
    expect(JSON.parse((await renderedResponse.textContent()) ?? "")).toEqual(expectedEnvelope);

    await page.getByRole("button", { name: "History (1)", exact: true }).click();
    await expect(page.getByText("Recent Queries", { exact: true })).toBeVisible();
    const historyEntry = page
      .getByRole("main")
      .getByRole("button", { name: /^query ExplorerRow/ });
    await expect(historyEntry).toContainText(query);
    await expect(historyEntry).toContainText("200");

    await page.getByRole("button", { name: "Load schema", exact: true }).click();
    const schemaRegion = page.getByRole("region", { name: "GraphQL schema" });
    await schemaRegion.getByText("Query", { exact: true }).click();
    const queryField = page.getByTestId(`graphql-schema-field-Query-${tableName}`);
    await expect(queryField).toBeVisible();
    await expect(
      page.getByTestId(`graphql-schema-argument-Query-${tableName}-limit`),
    ).toContainText("limit Int");

    // The generated schema summary has no unique accessible role or existing test ID.
    // eslint-disable-next-line playwright/no-raw-locators
    await schemaRegion.locator("summary").filter({ hasText: tableName }).click();
    const titleField = page.getByTestId(`graphql-schema-field-${tableName}-title`);
    await expect(titleField.getByText("title", { exact: true })).toBeVisible();
    await expect(titleField.getByText("String!", { exact: true })).toBeVisible();
    const scoreField = page.getByTestId(`graphql-schema-field-${tableName}-score`);
    await expect(scoreField.getByText("score", { exact: true })).toBeVisible();
    await expect(scoreField.getByText("Int!", { exact: true })).toBeVisible();

    await testInfo.attach("graphql-explorer-success.png", {
      body: await page.screenshot({ fullPage: true }),
      contentType: "image/png",
    });
    expect(pageErrors).toEqual([]);
    expect(consoleErrors).toEqual([]);
  });
});
