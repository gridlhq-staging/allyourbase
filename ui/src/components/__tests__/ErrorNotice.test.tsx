import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { docsUrl } from "../../lib/docs_url";
import { ErrorNotice } from "../ErrorNotice";

describe("ErrorNotice", () => {
  it("renders the failure message and exact guide URL", () => {
    render(
      <ErrorNotice
        message="Database rejected the query"
        docsPath="/guide/patterns"
      />,
    );

    expect(screen.getByText("Database rejected the query")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View guide" })).toHaveAttribute(
      "href",
      "https://allyourbase.io/guide/patterns",
    );
  });

  it("renders the supplied recovery label and invokes its handler", async () => {
    const onAction = vi.fn();
    render(
      <ErrorNotice
        message="Policy loading failed"
        docsPath="/guide/authentication#row-level-security-rls"
        actionLabel="Retry"
        onAction={onAction}
      />,
    );

    await userEvent.setup().click(screen.getByRole("button", { name: "Retry" }));
    expect(onAction).toHaveBeenCalledOnce();
  });

  it("does not render an action without a handler", () => {
    render(
      <ErrorNotice
        message="Invalid password"
        docsPath="/guide/authentication"
        actionLabel="Sign in"
      />,
    );

    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});

describe("docsUrl", () => {
  it("resolves a guide path against the public documentation origin", () => {
    expect(docsUrl("/guide/file-storage")).toBe(
      "https://allyourbase.io/guide/file-storage",
    );
  });
});
