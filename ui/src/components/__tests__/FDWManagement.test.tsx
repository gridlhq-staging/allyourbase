import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { FDWManagement } from "../FDWManagement";

vi.mock("../../api_fdw", () => ({
  listServers: vi.fn(),
  createServer: vi.fn(),
  dropServer: vi.fn(),
  listTables: vi.fn(),
  importTables: vi.fn(),
  dropTable: vi.fn(),
}));

import * as api from "../../api_fdw";

const mockServers = [
  {
    name: "remote_pg",
    fdw_type: "postgres_fdw",
    options: { host: "db.example.com", port: "5432", dbname: "prod" },
    created_at: "2026-03-10T10:00:00Z",
  },
];

const mockTables = [
  {
    schema: "public",
    name: "users",
    server_name: "remote_pg",
    columns: [
      { name: "id", type: "integer" },
      { name: "email", type: "text" },
    ],
    options: {},
  },
];

beforeEach(() => {
  vi.clearAllMocks();
  (api.listServers as ReturnType<typeof vi.fn>).mockResolvedValue(mockServers);
  (api.listTables as ReturnType<typeof vi.fn>).mockResolvedValue(mockTables);
  (api.createServer as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.dropServer as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.importTables as ReturnType<typeof vi.fn>).mockResolvedValue(mockTables);
  (api.dropTable as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
});

describe("FDWManagement", () => {
  it("uses the injected registry label as the primary heading", () => {
    renderWithProviders(<FDWManagement screenLabel="Registry FDW Label" />);

    expect(
      screen.getByRole("heading", { name: "Registry FDW Label" }),
    ).toBeInTheDocument();
  });

  it("renders server list with name/type/created_at", async () => {
    renderWithProviders(<FDWManagement screenLabel="FDW Management" />);
    await waitFor(() => {
      expect(screen.getByText("postgres_fdw")).toBeInTheDocument();
    });
    // remote_pg appears in both servers and tables sections
    expect(screen.getAllByText("remote_pg").length).toBeGreaterThanOrEqual(1);
  });

  it("renders table list with schema/name/server_name", async () => {
    renderWithProviders(<FDWManagement screenLabel="FDW Management" />);
    await waitFor(() => {
      expect(screen.getByText("users")).toBeInTheDocument();
    });
    expect(screen.getByText("public")).toBeInTheDocument();
  });

  it("create-server form validates required fields", async () => {
    renderWithProviders(<FDWManagement screenLabel="FDW Management" />);
    await waitFor(() => {
      expect(screen.getByText("postgres_fdw")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /add server/i }));
    const createBtn = screen.getByRole("button", { name: /^create$/i });
    expect(createBtn).toBeDisabled();
  });

  it("renders file_fdw-specific fields and submits filename option", async () => {
    renderWithProviders(<FDWManagement screenLabel="FDW Management" />);
    await waitFor(() => {
      expect(screen.getByText("postgres_fdw")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /add server/i }));
    fireEvent.change(screen.getByPlaceholderText("Server name"), {
      target: { value: "csv_fdw" },
    });
    fireEvent.change(screen.getByDisplayValue("postgres_fdw"), {
      target: { value: "file_fdw" },
    });

    expect(screen.queryByPlaceholderText("Host")).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText("User mapping user")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Filename"), {
      target: { value: "/tmp/data.csv" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(api.createServer).toHaveBeenCalledWith({
        name: "csv_fdw",
        fdw_type: "file_fdw",
        options: { filename: "/tmp/data.csv" },
      });
    });
  });

  it("drop-server fires destructive ConfirmDialog", async () => {
    renderWithProviders(<FDWManagement screenLabel="FDW Management" />);
    await waitFor(() => {
      expect(screen.getByText("postgres_fdw")).toBeInTheDocument();
    });

    const dropButtons = screen.getAllByRole("button", { name: /drop/i });
    fireEvent.click(dropButtons[0]);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: /drop server/i }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^drop$/i }));
    await waitFor(() => {
      expect(api.dropServer).toHaveBeenCalledWith("remote_pg", false);
    });
  });

  it("import-tables form validates server and remote_schema", async () => {
    renderWithProviders(<FDWManagement screenLabel="FDW Management" />);
    await waitFor(() => {
      expect(screen.getByText("postgres_fdw")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /import tables/i }));
    const importBtn = screen.getByRole("button", { name: /^import$/i });
    expect(importBtn).toBeDisabled();
  });

  it("drop-table fires ConfirmDialog", async () => {
    renderWithProviders(<FDWManagement screenLabel="FDW Management" />);
    await waitFor(() => {
      expect(screen.getByText("users")).toBeInTheDocument();
    });

    // Use aria-label to target the table-section drop button specifically
    fireEvent.click(screen.getByLabelText("Drop public.users"));

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: /drop table/i }),
      ).toBeInTheDocument();
    });
  });

  it("keeps table state mounted and retries an exact server fetch failure", async () => {
    (api.listServers as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("Connection refused"),
    );
    renderWithProviders(<FDWManagement screenLabel="FDW Management" />);
    await waitFor(() => {
      expect(screen.getByText("Connection refused")).toBeInTheDocument();
    });
    expect(screen.getByText("users")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(screen.getByText("postgres_fdw")).toBeInTheDocument();
    });
    expect(api.listServers).toHaveBeenCalledTimes(2);
    expect(api.listTables).toHaveBeenCalledTimes(1);
  });

  it("keeps server state mounted and retries an exact table fetch failure", async () => {
    (api.listTables as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("Tables unavailable"),
    );
    renderWithProviders(<FDWManagement screenLabel="FDW Management" />);
    await waitFor(() => {
      expect(screen.getByText("Tables unavailable")).toBeInTheDocument();
    });
    expect(screen.getByText("postgres_fdw")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(screen.getByText("users")).toBeInTheDocument();
    });
    expect(api.listTables).toHaveBeenCalledTimes(2);
    expect(api.listServers).toHaveBeenCalledTimes(1);
  });
});
