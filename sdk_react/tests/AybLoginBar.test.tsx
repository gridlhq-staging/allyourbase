import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AybLoginBar } from "../src/AybLoginBar";

const demoSuggestions = [
  { label: "Alice", email: "alice@demo.test", password: "password123" },
  { label: "Bob", email: "bob@demo.test", password: "password123" },
];

describe("AybLoginBar", () => {
  it("shows only enabled methods and supports suggestion click + submit", async () => {
    const onSubmit = vi.fn(async () => {});
    const onOAuth = vi.fn(async () => {});
    const onAnonymous = vi.fn(async () => {});

    render(
      <AybLoginBar
        methods={{ password: true, oauth: true, anonymous: false, canUpgradeAnonymous: false }}
        loading={false}
        email="alice@demo.test"
        password="password123"
        error={null}
        demoSuggestions={demoSuggestions}
        onEmailChange={() => {}}
        onPasswordChange={() => {}}
        onSubmit={onSubmit}
        onOAuth={onOAuth}
        onAnonymous={onAnonymous}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Alice" }));
    fireEvent.click(screen.getByRole("button", { name: "Sign In" }));

    expect(onSubmit).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Continue with OAuth" }));
    expect(onOAuth).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("button", { name: "Continue as Guest" })).toBeNull();
  });

  it("renders guest-upgrade CTA only when canUpgradeAnonymous is true", () => {
    render(
      <AybLoginBar
        methods={{ password: true, oauth: false, anonymous: false, canUpgradeAnonymous: true }}
        loading={false}
        email=""
        password=""
        error={null}
        demoSuggestions={[]}
        onEmailChange={() => {}}
        onPasswordChange={() => {}}
        onSubmit={async () => {}}
        onOAuth={async () => {}}
        onAnonymous={async () => {}}
      />,
    );

    expect(screen.getByText("Upgrade your guest account")).toBeTruthy();
  });

  it("renders a secure password input field", () => {
    render(
      <AybLoginBar
        methods={{ password: true, oauth: false, anonymous: false, canUpgradeAnonymous: false }}
        loading={false}
        email="user@demo.test"
        password="secret"
        error={null}
        demoSuggestions={[]}
        onEmailChange={() => {}}
        onPasswordChange={() => {}}
        onSubmit={async () => {}}
        onOAuth={async () => {}}
        onAnonymous={async () => {}}
      />,
    );

    expect(screen.getByLabelText("Password").getAttribute("type")).toBe("password");
  });
});
