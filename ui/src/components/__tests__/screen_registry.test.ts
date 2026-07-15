import { describe, expect, it } from "vitest";
import {
  ADMIN_VIEWS,
  SCREEN_REGISTRY,
  filterScreenRegistry,
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

describe("screen registry", () => {
  it("owns the complete unique admin-view inventory", () => {
    const registryScreens = SCREEN_REGISTRY.sections.flatMap((section) => section.screens);
    const registryIds = registryScreens.map((screen) => screen.id);

    expect(ADMIN_VIEWS).toHaveLength(49);
    expect(new Set(ADMIN_VIEWS)).toHaveLength(49);
    expect(registryIds).toHaveLength(49);
    expect(new Set(registryIds)).toHaveLength(49);
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
});
