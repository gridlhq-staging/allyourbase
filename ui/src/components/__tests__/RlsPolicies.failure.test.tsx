import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithProviders } from "../../test-utils";
import { RlsPolicies } from "../RlsPolicies";
import {
  getRlsStatus,
  listRlsPolicies,
} from "../../api";
import { docsUrl } from "../../lib/docs_url";
import { makeRlsSchema } from "./rls-test-fixtures";

vi.mock("../../api", async () => {
  const actual = await vi.importActual<typeof import("../../api")>("../../api");
  return {
    ...actual,
    listRlsPolicies: vi.fn(),
    getRlsStatus: vi.fn(),
    createRlsPolicy: vi.fn(),
    deleteRlsPolicy: vi.fn(),
    enableRls: vi.fn(),
    disableRls: vi.fn(),
  };
});

const mockGetStatus = vi.mocked(getRlsStatus);
const mockListPolicies = vi.mocked(listRlsPolicies);

function makeStatus() {
  return { rlsEnabled: true, forceRls: false };
}

describe("RlsPolicies failure guidance", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("retries malformed policy loads and links to the RLS guide", async () => {
    mockListPolicies.mockRejectedValueOnce(new Error("malformed policy response"));
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    renderWithProviders(<RlsPolicies screenLabel="RLS Policies" schema={makeRlsSchema()} />);

    await waitFor(() => {
      expect(screen.getByText("malformed policy response")).toBeInTheDocument();
    });
    expect(screen.getByRole("link", { name: "View guide" })).toHaveAttribute(
      "href",
      docsUrl("/guide/authentication#row-level-security-rls"),
    );

    mockListPolicies.mockResolvedValueOnce([]);
    mockGetStatus.mockResolvedValueOnce(makeStatus());
    const user = userEvent.setup();
    await user.click(screen.getByText("Retry"));

    await waitFor(() => {
      expect(screen.getByText("No policies on this table")).toBeInTheDocument();
    });
  });
});
