import { render, screen, waitForElementToBeRemoved } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Component, createElement, type ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import {
  ADMIN_VIEWS,
  SCREEN_REGISTRY,
  createLazyScreenRender,
  findAdminScreen,
  filterScreenRegistry,
  type ScreenProps,
  type ScreenRegistry,
} from "../../screens/registry";

const EXPECTED_SECTION_TITLES = [
  "Database",
  "Services",
  "Messaging",
  "Admin",
  "AI",
  "Auth",
] as const;

const EMPTY_SCHEMA = {
  schemas: [],
  tables: {},
  builtAt: "2026-07-15T00:00:00Z",
};
const SCREEN_LABEL_PROBE_TEST_ID = "screen-label-probe";

vi.mock("../FunctionBrowser", async () => {
  const { createElement } = await import("react");
  return {
    FunctionBrowser: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

vi.mock("../RlsPolicies", async () => {
  const { createElement } = await import("react");
  return {
    RlsPolicies: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

vi.mock("../MatviewsAdmin", async () => {
  const { createElement } = await import("react");
  return {
    MatviewsAdmin: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

vi.mock("../FDWManagement", async () => {
  const { createElement } = await import("react");
  return {
    FDWManagement: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

vi.mock("../Apps", async () => {
  const { createElement } = await import("react");
  return {
    Apps: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

vi.mock("../Backups", async () => {
  const { createElement } = await import("react");
  return {
    Backups: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

vi.mock("../UsageMetering", async () => {
  const { createElement } = await import("react");
  return {
    UsageMetering: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

vi.mock("../MFAEnrollment", async () => {
  const { createElement } = await import("react");
  return {
    MFAEnrollment: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

vi.mock("../AccountLinking", async () => {
  const { createElement } = await import("react");
  return {
    AccountLinking: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

vi.mock("../SAMLConfig", async () => {
  const { createElement } = await import("react");
  return {
    SAMLConfig: ({ screenLabel }: { screenLabel: string }) =>
      createElement("div", { "data-testid": "screen-label-probe" }, screenLabel),
  };
});

class TestErrorBoundary extends Component<
  { children: ReactNode },
  { hasError: boolean; resetKey: number }
> {
  state = { hasError: false, resetKey: 0 };

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  private handleRetry = () => {
    this.setState((state) => ({
      hasError: false,
      resetKey: state.resetKey + 1,
    }));
  };

  render() {
    if (this.state.hasError) {
      return createElement("button", { onClick: this.handleRetry }, "Retry");
    }

    return createElement("div", { key: this.state.resetKey }, this.props.children);
  }
}

function renderScreen(renderScreen: (props: ScreenProps) => ReactNode) {
  const onRefresh = vi.fn();
  render(
    createElement(
      TestErrorBoundary,
      null,
      renderScreen({ schema: EMPTY_SCHEMA, onRefresh, screenLabel: "Test Screen" }),
    ),
  );
  return { onRefresh };
}

function rejectedRetryURLCases() {
  const screenAssetURL = new URL("../../screens/SqlEditor-test.js", import.meta.url);
  const crossOriginURL = new URL(screenAssetURL.toString());
  const protocolMismatchedURL = new URL(screenAssetURL.toString());

  crossOriginURL.hostname = "attacker.example";
  protocolMismatchedURL.protocol = screenAssetURL.protocol === "https:" ? "http:" : "https:";

  return [
    {
      name: "cross-origin URL",
      url: crossOriginURL.toString(),
      assertRejectedShape: (rejectedURL: URL) => {
        expect(rejectedURL.protocol).toBe(screenAssetURL.protocol);
        expect(rejectedURL.pathname).toBe(screenAssetURL.pathname);
        expect(rejectedURL.origin).not.toBe(screenAssetURL.origin);
      },
    },
    {
      name: "protocol-mismatched URL",
      url: protocolMismatchedURL.toString(),
      assertRejectedShape: (rejectedURL: URL) => {
        expect(rejectedURL.protocol).not.toBe(screenAssetURL.protocol);
        expect(rejectedURL.hostname).toBe(screenAssetURL.hostname);
        expect(rejectedURL.pathname).toBe(screenAssetURL.pathname);
      },
    },
    {
      name: "non-JavaScript path",
      url: new URL("../../screens/SqlEditor-test.css", import.meta.url).toString(),
      assertRejectedShape: (rejectedURL: URL) => {
        expect(rejectedURL.origin).toBe(screenAssetURL.origin);
        expect(rejectedURL.pathname).not.toMatch(/\.js$/);
      },
    },
    {
      name: "path outside the lazy-screen asset base",
      url: new URL("../SqlEditor-test.js", import.meta.url).toString(),
      assertRejectedShape: (rejectedURL: URL) => {
        expect(rejectedURL.origin).toBe(screenAssetURL.origin);
        expect(rejectedURL.pathname).toMatch(/\/src\/components\/SqlEditor-test\.js$/);
      },
    },
  ] as const;
}

describe("screen registry", () => {
  it("owns the complete unique admin-view inventory", () => {
    const registryScreens = SCREEN_REGISTRY.sections.flatMap((section) => section.screens);
    const registryIds = registryScreens.map((screen) => screen.id);

    expect(ADMIN_VIEWS).toHaveLength(50);
    expect(new Set(ADMIN_VIEWS)).toHaveLength(50);
    expect(registryIds).toHaveLength(50);
    expect(new Set(registryIds)).toHaveLength(50);
    expect(new Set(registryIds)).toEqual(new Set(ADMIN_VIEWS));
  });

  it("provides ordered sidebar and command-palette metadata for every screen", () => {
    expect(SCREEN_REGISTRY.sections.map((section) => section.title)).toEqual(
      EXPECTED_SECTION_TITLES,
    );

    for (const section of SCREEN_REGISTRY.sections) {
      expect(EXPECTED_SECTION_TITLES).toContain(section.title);
      expect(section.screens.length).toBeGreaterThan(0);

      for (const screen of section.screens) {
        expect(screen.label.trim()).not.toBe("");
        expect(screen.icon).toBeTypeOf("object");
        expect(screen.render).toBeTypeOf("function");
      }
    }
  });

  it("registers users explicitly", () => {
    const usersEntries = SCREEN_REGISTRY.sections
      .flatMap((section) => section.screens)
      .filter((screen) => screen.id === "users");

    expect(usersEntries).toHaveLength(1);
    expect(usersEntries[0].label).toBe("Users");
  });

  it("registers GraphQL once immediately after SQL Editor in Database", () => {
    const adminGraphqlEntries = ADMIN_VIEWS.filter((view) => view === "graphql");
    const registryGraphqlEntries = SCREEN_REGISTRY.sections
      .flatMap((section) => section.screens)
      .filter((screen) => screen.id === "graphql");
    const databaseScreens = SCREEN_REGISTRY.sections.find(
      (section) => section.title === "Database",
    )?.screens;
    const databaseIds = databaseScreens?.map((screen) => screen.id);

    expect(adminGraphqlEntries).toHaveLength(1);
    expect(registryGraphqlEntries).toHaveLength(1);
    expect(registryGraphqlEntries[0].label).toBe("GraphQL");
    expect(databaseIds?.slice(0, 2)).toEqual(["sql-editor", "graphql"]);
  });

  it("filters opt-in capability screens without reordering survivors", () => {
    const storageScreen = {
      id: "storage",
      label: "Storage",
      icon: {} as never,
      requires: "storage",
      render: () => null,
    };
    const usersScreen = {
      id: "users",
      label: "Users",
      icon: {} as never,
      render: () => null,
    };
    const supportScreen = {
      id: "support-tickets",
      label: "Support Tickets",
      icon: {} as never,
      requires: "support",
      render: () => null,
    };
    const registry = {
      sections: [
        {
          title: "Services",
          screens: [storageScreen],
        },
        {
          title: "Admin",
          screens: [usersScreen, supportScreen],
        },
      ],
    } satisfies ScreenRegistry;

    expect(filterScreenRegistry(registry, () => true)).toEqual(registry);
    expect(
      filterScreenRegistry(registry, (capability) => capability !== "storage"),
    ).toEqual({
      sections: [
        {
          title: "Admin",
          screens: [usersScreen, supportScreen],
        },
      ],
    });
  });

  it("keeps screen prop normalization behind the lazy registry render seam", async () => {
    const user = userEvent.setup();
    const loader = vi.fn().mockResolvedValue({
      default: ({ onSchemaChange }: { onSchemaChange: () => void }) =>
        createElement("button", { onClick: onSchemaChange }, "Normalized lazy screen"),
    });
    const renderLazyScreen = createLazyScreenRender(
      loader,
      ({ onRefresh }) => ({ onSchemaChange: onRefresh }),
    );

    const { onRefresh } = renderScreen(renderLazyScreen);

    expect(screen.getByRole("status", { name: "Loading screen" })).toBeVisible();
    await user.click(await screen.findByRole("button", { name: "Normalized lazy screen" }));

    expect(loader).toHaveBeenCalledTimes(1);
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });

  it.each([
    "functions",
    "rls",
    "matviews",
    "fdw",
    "apps",
    "backups",
    "usage",
    "mfa-management",
    "account-linking",
    "saml",
  ] as const)("maps the registry label into the %s lazy screen props", async (screenId) => {
    const registeredScreen = findAdminScreen(screenId);

    expect(registeredScreen).toBeDefined();
    renderScreen((props) =>
      registeredScreen!.render({
        ...props,
        screenLabel: registeredScreen!.label,
      }),
    );

    expect(await screen.findByTestId(SCREEN_LABEL_PROBE_TEST_ID)).toHaveTextContent(
      registeredScreen!.label,
    );
  });

  it("can recover from a rejected lazy import with a fresh loader request after retry", async () => {
    const user = userEvent.setup();
    const loader = vi
      .fn()
      .mockRejectedValueOnce(new Error("chunk failed"))
      .mockResolvedValueOnce({
        default: () => createElement("div", null, "Recovered lazy screen"),
      });
    const renderLazyScreen = createLazyScreenRender(loader);

    renderScreen(renderLazyScreen);

    await waitForElementToBeRemoved(() =>
      screen.queryByRole("status", { name: "Loading screen" }),
    );
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("Recovered lazy screen")).toBeVisible();
    expect(loader).toHaveBeenCalledTimes(2);
  });

  it("uses the explicit named export when a SQL chunk retry returns multiple function exports", async () => {
    const user = userEvent.setup();
    const retryURL = new URL("../../screens/SqlEditor-test.js", import.meta.url).toString();
    const retryImport = vi.fn().mockResolvedValue({
      AnotherExport: () => createElement("div", null, "Wrong first export"),
      SqlEditor: () => createElement("div", null, "Recovered SqlEditor export"),
    });
    const loader = vi
      .fn()
      .mockRejectedValueOnce(new Error(retryURL));
    const renderLazyScreen = createLazyScreenRender(loader, undefined, {
      exportName: "SqlEditor",
      importRejectedChunk: retryImport,
    });

    renderScreen(renderLazyScreen);

    await waitForElementToBeRemoved(() =>
      screen.queryByRole("status", { name: "Loading screen" }),
    );
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("Recovered SqlEditor export")).toBeVisible();
    expect(screen.queryByText("Wrong first export")).not.toBeInTheDocument();
    expect(retryImport).toHaveBeenCalledWith(
      expect.stringMatching(/\/src\/screens\/SqlEditor-test\.js\?ayb_retry=\d+$/),
    );
  });

  it.each(rejectedRetryURLCases())(
    "rejects $name before the retry importer runs",
    async ({ assertRejectedShape, url }) => {
      const user = userEvent.setup();
      const retryImport = vi.fn().mockResolvedValue({
        default: () => createElement("div", null, "Unsafe retry import"),
      });
      const loader = vi
        .fn()
        .mockRejectedValueOnce(new Error(url))
        .mockResolvedValueOnce({
          default: () => createElement("div", null, "Recovered through primary loader"),
        });
      const renderLazyScreen = createLazyScreenRender(loader, undefined, {
        importRejectedChunk: retryImport,
      });

      renderScreen(renderLazyScreen);

      await waitForElementToBeRemoved(() =>
        screen.queryByRole("status", { name: "Loading screen" }),
      );
      await user.click(screen.getByRole("button", { name: "Retry" }));

      expect(await screen.findByText("Recovered through primary loader")).toBeVisible();
      expect(screen.queryByText("Unsafe retry import")).not.toBeInTheDocument();
      expect(retryImport).not.toHaveBeenCalled();
      expect(loader).toHaveBeenCalledTimes(2);
      assertRejectedShape(new URL(url));
    },
  );
});
