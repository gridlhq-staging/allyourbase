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

  it("renders one OAuth button per provider when oauthProviders is provided", async () => {
    const onOAuthProvider = vi.fn(async () => {});

    render(
      <AybLoginBar
        methods={{ password: true, oauth: true, anonymous: false, canUpgradeAnonymous: false }}
        loading={false}
        email=""
        password=""
        error={null}
        demoSuggestions={[]}
        oauthProviders={["github", "google"]}
        onEmailChange={() => {}}
        onPasswordChange={() => {}}
        onSubmit={async () => {}}
        onOAuth={async () => {}}
        onOAuthProvider={onOAuthProvider}
        onAnonymous={async () => {}}
      />,
    );

    const githubBtn = screen.getByRole("button", { name: /github/i });
    const googleBtn = screen.getByRole("button", { name: /google/i });
    expect(screen.queryByRole("button", { name: "Continue with OAuth" })).toBeNull();

    fireEvent.click(githubBtn);
    fireEvent.click(googleBtn);
    expect(onOAuthProvider).toHaveBeenNthCalledWith(1, "github");
    expect(onOAuthProvider).toHaveBeenNthCalledWith(2, "google");
  });

  it("falls back to single OAuth button when oauthProviders is absent (back-compat)", () => {
    render(
      <AybLoginBar
        methods={{ password: true, oauth: true, anonymous: false, canUpgradeAnonymous: false }}
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
    expect(screen.getByRole("button", { name: "Continue with OAuth" })).toBeTruthy();
  });

  it("renders a magic-link trigger when methods.magicLink is true and calls onRequestMagicLink", async () => {
    const onRequestMagicLink = vi.fn(async () => {});

    render(
      <AybLoginBar
        methods={{ password: true, oauth: false, anonymous: false, canUpgradeAnonymous: false, magicLink: true }}
        loading={false}
        email="alice@demo.test"
        password=""
        error={null}
        demoSuggestions={[]}
        onEmailChange={() => {}}
        onPasswordChange={() => {}}
        onSubmit={async () => {}}
        onOAuth={async () => {}}
        onAnonymous={async () => {}}
        onRequestMagicLink={onRequestMagicLink}
      />,
    );

    const magicBtn = screen.getByRole("button", { name: /magic link/i });
    fireEvent.click(magicBtn);
    expect(onRequestMagicLink).toHaveBeenCalledWith("alice@demo.test");
  });

  it("does not render a magic-link trigger when methods.magicLink is falsy", () => {
    render(
      <AybLoginBar
        methods={{ password: true, oauth: false, anonymous: false, canUpgradeAnonymous: false }}
        loading={false}
        email="alice@demo.test"
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
    expect(screen.queryByRole("button", { name: /magic link/i })).toBeNull();
  });

  it("renders passkey CTA copy and calls onPasskey with the current email", () => {
    const onPasskey = vi.fn(async () => {});
    render(
      <AybLoginBar
        methods={{ password: true, oauth: false, anonymous: false, canUpgradeAnonymous: false, passkey: true }}
        loading={false}
        email="passkey@example.com"
        password=""
        error={null}
        demoSuggestions={[]}
        onEmailChange={() => {}}
        onPasswordChange={() => {}}
        onSubmit={async () => {}}
        onOAuth={async () => {}}
        onAnonymous={async () => {}}
        onPasskey={onPasskey}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Sign in with a passkey" }));
    expect(onPasskey).toHaveBeenCalledWith("passkey@example.com");
  });

  it("shows email input in passkey-only mode and enables passkey submit after email entry", () => {
    const onPasskey = vi.fn(async () => {});

    function PasskeyOnlyHarness() {
      const [email, setEmail] = React.useState("");
      return (
        <AybLoginBar
          methods={{ password: false, oauth: false, anonymous: false, canUpgradeAnonymous: false, passkey: true }}
          loading={false}
          email={email}
          password=""
          error={null}
          demoSuggestions={[]}
          onEmailChange={setEmail}
          onPasswordChange={() => {}}
          onSubmit={async () => {}}
          onOAuth={async () => {}}
          onAnonymous={async () => {}}
          onPasskey={onPasskey}
        />
      );
    }

    render(<PasskeyOnlyHarness />);

    const emailInput = screen.getByLabelText("Email");
    const passkeyButton = screen.getByRole("button", { name: "Sign in with a passkey" });
    expect(passkeyButton.getAttribute("disabled")).not.toBeNull();

    fireEvent.change(emailInput, { target: { value: "passkey-only@example.com" } });
    expect(passkeyButton.getAttribute("disabled")).toBeNull();

    fireEvent.click(passkeyButton);
    expect(onPasskey).toHaveBeenCalledWith("passkey-only@example.com");
  });

  it("renders a clickable guest-upgrade button when canUpgradeAnonymous and calls onUpgradeAnonymous", async () => {
    const onUpgradeAnonymous = vi.fn(async () => {});

    render(
      <AybLoginBar
        methods={{ password: true, oauth: false, anonymous: false, canUpgradeAnonymous: true }}
        loading={false}
        email="alice@demo.test"
        password="password123"
        error={null}
        demoSuggestions={[]}
        onEmailChange={() => {}}
        onPasswordChange={() => {}}
        onSubmit={async () => {}}
        onOAuth={async () => {}}
        onAnonymous={async () => {}}
        onUpgradeAnonymous={onUpgradeAnonymous}
      />,
    );

    const upgradeBtn = screen.getByRole("button", { name: /upgrade account/i });
    fireEvent.click(upgradeBtn);
    expect(onUpgradeAnonymous).toHaveBeenCalledTimes(1);
  });
});
