import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfirmDialog } from "../shared/ConfirmDialog";
import { StatusBadge } from "../shared/StatusBadge";
import { AdminTable, type Column } from "../shared/AdminTable";
import { FilterBar, type FilterField } from "../shared/FilterBar";
import { expectWcagContrastToken } from "../../test-utils";

describe("ConfirmDialog", () => {
  it("renders title and message", () => {
    render(
      <ConfirmDialog
        open
        title="Delete Item"
        message="Are you sure you want to delete this item?"
        confirmLabel="Delete"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByText("Delete Item")).toBeInTheDocument();
    expect(
      screen.getByText("Are you sure you want to delete this item?"),
    ).toBeInTheDocument();
  });

  it("does not render when open is false", () => {
    render(
      <ConfirmDialog
        open={false}
        title="Delete Item"
        message="Gone"
        confirmLabel="Delete"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.queryByText("Delete Item")).not.toBeInTheDocument();
  });

  it("calls onCancel when Cancel is clicked", () => {
    const onCancel = vi.fn();
    render(
      <ConfirmDialog
        open
        title="Delete"
        message="Sure?"
        confirmLabel="Delete"
        onConfirm={vi.fn()}
        onCancel={onCancel}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("calls onConfirm when confirm button is clicked", () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog
        open
        title="Delete"
        message="Sure?"
        confirmLabel="Delete"
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("shows loading state and disables both dialog actions", () => {
    render(
      <ConfirmDialog
        open
        title="Delete"
        message="Sure?"
        confirmLabel="Delete"
        loading
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: /delete/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  });

  it("renders custom content and respects confirmDisabled", () => {
    render(
      <ConfirmDialog
        open
        title="Confirm"
        message="Provide input"
        confirmLabel="Continue"
        confirmDisabled
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      >
        <input aria-label="Extra input" />
      </ConfirmDialog>,
    );
    expect(screen.getByLabelText("Extra input")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
  });

  it("applies destructive styling variant", () => {
    render(
      <ConfirmDialog
        open
        title="Danger"
        message="Bad things"
        confirmLabel="Do it"
        destructive
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    const btn = screen.getByRole("button", { name: "Do it" });
    expect(btn.className).toContain("bg-red-600");
  });

  it("uses default (non-destructive) styling", () => {
    render(
      <ConfirmDialog
        open
        title="Confirm"
        message="Proceed?"
        confirmLabel="OK"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    const btn = screen.getByRole("button", { name: "OK" });
    expect(btn.className).toContain("bg-blue-600");
  });

  it("marks both dialog actions as non-submit buttons", () => {
    render(
      <ConfirmDialog
        open
        title="Confirm"
        message="Proceed?"
        confirmLabel="OK"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Cancel" })).toHaveAttribute("type", "button");
    expect(screen.getByRole("button", { name: "OK" })).toHaveAttribute("type", "button");
  });
});

describe("StatusBadge", () => {
  it("renders the status text", () => {
    render(<StatusBadge status="completed" />);
    expect(screen.getByText("completed")).toBeInTheDocument();
  });

  it("maps success statuses to green", () => {
    const { container } = render(<StatusBadge status="completed" />);
    const badge = container.firstChild as HTMLElement;
    expect(badge.className).toContain("bg-green-100");
    expect(badge.className).toContain("text-green-700");
  });

  it("maps error statuses to red", () => {
    const { container } = render(<StatusBadge status="failed" />);
    const badge = container.firstChild as HTMLElement;
    expect(badge.className).toContain("bg-red-100");
    expect(badge.className).toContain("text-red-700");
  });

  it("maps running/pending statuses to appropriate colors", () => {
    const { container: c1 } = render(<StatusBadge status="running" />);
    expect((c1.firstChild as HTMLElement).className).toContain("bg-yellow-100");

    const { container: c2 } = render(<StatusBadge status="queued" />);
    expect((c2.firstChild as HTMLElement).className).toContain("bg-blue-100");
  });

  it("maps unknown statuses to gray", () => {
    const { container } = render(<StatusBadge status="something-else" />);
    const badge = container.firstChild as HTMLElement;
    expect(badge.className).toContain("bg-gray-100");
  });

  it("supports custom status-to-variant mapping", () => {
    const { container } = render(
      <StatusBadge status="active" variantMap={{ active: "success" }} />,
    );
    const badge = container.firstChild as HTMLElement;
    expect(badge.className).toContain("bg-green-100");
  });
});

describe("AdminTable", () => {
  interface Row {
    id: string;
    name: string;
    count: number;
  }

  const columns: Column<Row>[] = [
    { key: "name", header: "Name" },
    { key: "count", header: "Count" },
  ];

  const rows: Row[] = [
    { id: "1", name: "Alpha", count: 10 },
    { id: "2", name: "Beta", count: 20 },
  ];

  it("renders header row with column names", () => {
    render(
      <AdminTable columns={columns} rows={rows} rowKey="id" />,
    );
    expect(screen.getByText("Name")).toBeInTheDocument();
    expect(screen.getByText("Count")).toBeInTheDocument();
  });

  it("renders data rows", () => {
    render(
      <AdminTable columns={columns} rows={rows} rowKey="id" />,
    );
    expect(screen.getByText("Alpha")).toBeInTheDocument();
    expect(screen.getByText("Beta")).toBeInTheDocument();
    expect(screen.getByText("10")).toBeInTheDocument();
    expect(screen.getByText("20")).toBeInTheDocument();
  });

  it("shows empty state when no rows", () => {
    render(
      <AdminTable
        columns={columns}
        rows={[]}
        rowKey="id"
        emptyMessage="No data available"
      />,
    );
    expect(screen.getByText("No data available")).toBeInTheDocument();
  });

  it("renders pagination controls", () => {
    const onPageChange = vi.fn();
    render(
      <AdminTable
        columns={columns}
        rows={rows}
        rowKey="id"
        page={2}
        totalPages={5}
        onPageChange={onPageChange}
      />,
    );
    expect(screen.getByText("2 / 5")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Previous page"));
    expect(onPageChange).toHaveBeenCalledWith(1);

    fireEvent.click(screen.getByLabelText("Next page"));
    expect(onPageChange).toHaveBeenCalledWith(3);
  });

  it("disables prev button on first page", () => {
    render(
      <AdminTable
        columns={columns}
        rows={rows}
        rowKey="id"
        page={1}
        totalPages={3}
        onPageChange={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Previous page")).toBeDisabled();
  });

  it("disables next button on last page", () => {
    render(
      <AdminTable
        columns={columns}
        rows={rows}
        rowKey="id"
        page={3}
        totalPages={3}
        onPageChange={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Next page")).toBeDisabled();
  });

  it("supports custom cell renderer", () => {
    const singleRow = [rows[0]];
    const cols: Column<Row>[] = [
      {
        key: "name",
        header: "Name",
        render: (row: Row) => <strong data-testid="custom">{row.name}!</strong>,
      },
    ];
    render(<AdminTable columns={cols} rows={singleRow} rowKey="id" />);
    expect(screen.getByTestId("custom")).toHaveTextContent("Alpha!");
  });

  // The 16 screens that render their own loading/error blocks outside the table
  // pass none of the degraded-state props, so omitting them must keep today's
  // render byte-for-byte until Stage 3 migrates each caller.
  it("renders rows and pagination unchanged when no degraded-state props are supplied", () => {
    render(
      <AdminTable
        columns={columns}
        rows={rows}
        rowKey="id"
        page={2}
        totalPages={5}
        onPageChange={vi.fn()}
      />,
    );

    expect(screen.getByText("Alpha")).toBeInTheDocument();
    expect(screen.getByText("2 / 5")).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("renders the supplied loading message instead of rows while loading", () => {
    render(
      <AdminTable
        columns={columns}
        rows={rows}
        rowKey="id"
        loading
        loadingMessage="Loading widgets..."
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("Loading widgets...");
    expect(screen.queryByText("Alpha")).not.toBeInTheDocument();
    expect(screen.queryByText("Name")).not.toBeInTheDocument();
  });

  it("falls back to a default loading message", () => {
    render(<AdminTable columns={columns} rows={[]} rowKey="id" loading />);

    const loadingState = screen.getByRole("status");
    expect(loadingState).toHaveTextContent("Loading...");
    expectWcagContrastToken(loadingState.className);
  });

  it("renders the exact empty message after a settled empty result", () => {
    render(
      <AdminTable
        columns={columns}
        rows={[]}
        rowKey="id"
        loading={false}
        error={null}
        docsPath="/guide/patterns"
        emptyMessage="No widgets yet"
      />,
    );

    const emptyState = screen.getByText("No widgets yet");
    expect(emptyState).toBeInTheDocument();
    expectWcagContrastToken(emptyState.className);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("renders the returned error text with a Retry action", () => {
    render(
      <AdminTable
        columns={columns}
        rows={[]}
        rowKey="id"
        error="Widget service unavailable"
        docsPath="/guide/patterns"
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("Widget service unavailable");
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("invokes the supplied retry callback", async () => {
    const onRetry = vi.fn();
    render(
      <AdminTable
        columns={columns}
        rows={[]}
        rowKey="id"
        error="Widget service unavailable"
        docsPath="/guide/patterns"
        onRetry={onRetry}
      />,
    );

    await userEvent.setup().click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  // ErrorNotice requires docsPath, and useAdminResource does not produce one, so
  // callers must thread it through AdminTable for the error branch to render.
  it("routes the error branch through the supplied docs path", () => {
    render(
      <AdminTable
        columns={columns}
        rows={[]}
        rowKey="id"
        error="Widget service unavailable"
        docsPath="/guide/file-storage"
      />,
    );

    expect(screen.getByRole("link", { name: "View guide" })).toHaveAttribute(
      "href",
      "https://allyourbase.io/guide/file-storage",
    );
  });

  it("prefers loading over error, error over empty, and empty over rows", () => {
    const degraded = {
      columns,
      rowKey: "id" as const,
      docsPath: "/guide/patterns" as const,
      emptyMessage: "No widgets yet",
      loadingMessage: "Loading widgets...",
    };

    const { rerender } = render(
      <AdminTable {...degraded} rows={rows} loading error="Widget service unavailable" />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Loading widgets...");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    rerender(
      <AdminTable {...degraded} rows={rows} loading={false} error="Widget service unavailable" />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Widget service unavailable");
    expect(screen.queryByText("No widgets yet")).not.toBeInTheDocument();
    expect(screen.queryByText("Alpha")).not.toBeInTheDocument();

    rerender(<AdminTable {...degraded} rows={[]} loading={false} error={null} />);
    expect(screen.getByText("No widgets yet")).toBeInTheDocument();

    rerender(<AdminTable {...degraded} rows={rows} loading={false} error={null} />);
    expect(screen.getByText("Alpha")).toBeInTheDocument();
    expect(screen.queryByText("No widgets yet")).not.toBeInTheDocument();
  });
});

describe("FilterBar", () => {
  const fields: FilterField[] = [
    {
      name: "status",
      label: "Status",
      type: "select",
      options: [
        { value: "", label: "All" },
        { value: "active", label: "Active" },
        { value: "inactive", label: "Inactive" },
      ],
    },
    {
      name: "search",
      label: "Search",
      type: "text",
      placeholder: "Search...",
    },
  ];

  it("renders filter inputs with labels", () => {
    render(
      <FilterBar
        fields={fields}
        values={{ status: "", search: "" }}
        onApply={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Status")).toBeInTheDocument();
    expect(screen.getByLabelText("Search")).toBeInTheDocument();
  });

  it("calls onApply with current values on submit", () => {
    const onApply = vi.fn();
    render(
      <FilterBar
        fields={fields}
        values={{ status: "active", search: "test" }}
        onApply={onApply}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Apply Filters" }));
    expect(onApply).toHaveBeenCalledWith({ status: "active", search: "test" });
  });

  it("calls onReset when Reset is clicked", () => {
    const onReset = vi.fn();
    render(
      <FilterBar
        fields={fields}
        values={{ status: "active", search: "test" }}
        onApply={vi.fn()}
        onReset={onReset}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(onReset).toHaveBeenCalledOnce();
  });

  it("hides Reset button when onReset is not provided", () => {
    render(
      <FilterBar
        fields={fields}
        values={{ status: "", search: "" }}
        onApply={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: "Reset" })).not.toBeInTheDocument();
  });

  it("fires onChange for individual field changes", () => {
    const onChange = vi.fn();
    render(
      <FilterBar
        fields={fields}
        values={{ status: "", search: "" }}
        onApply={vi.fn()}
        onChange={onChange}
      />,
    );
    fireEvent.change(screen.getByLabelText("Status"), {
      target: { value: "active" },
    });
    expect(onChange).toHaveBeenCalledWith("status", "active");
  });
});
