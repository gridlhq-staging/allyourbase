import { test, expect } from "@playwright/test";
import { Buffer } from "node:buffer";
import {
  addCard,
  addColumn,
  createBoard,
  deleteStorageObject,
  installFailingAttachmentDeleteRoute,
  openBoard,
  registerUser,
  storageBucketStatus,
  uniqueName,
} from "./helpers";

const attachment = {
  fileName: "kanban-attachment-proof.txt",
  mimeType: "text/plain",
  bytes: Buffer.from("AYB attachment proof\n", "utf8"),
};

test.describe("Attachments", () => {
  test("storage bucket route is reachable for authenticated kanban users", async ({ page }) => {
    await registerUser(page);
    await expect.poll(() => storageBucketStatus(page)).toBe(200);
  });

  test("can upload, download, persist, and delete a card attachment", async ({ page }) => {
    const boardName = uniqueName("AttachmentTest");
    await registerUser(page);
    await createBoard(page, boardName);
    await openBoard(page, boardName);
    await addColumn(page, "To Do");
    await addCard(page, "To Do", "Attachment Card");
    await page.getByText("Attachment Card").click();

    const modal = page.getByRole("dialog");
    await expect(page.getByText("Edit Card")).toBeVisible();

    await modal.getByLabel("Attach file").setInputFiles({
      name: attachment.fileName,
      mimeType: attachment.mimeType,
      buffer: attachment.bytes,
    });
    await expect(modal.getByText(attachment.fileName)).toBeVisible();
    await expect(modal.getByText(`${attachment.bytes.length} bytes`)).toBeVisible();

    await page.reload();
    await openBoard(page, boardName);
    await page.getByText("Attachment Card").click();
    const reopenedModal = page.getByRole("dialog");
    await expect(reopenedModal.getByText(attachment.fileName)).toBeVisible();

    const downloadPromise = page.waitForEvent("download");
    await reopenedModal.getByRole("link", { name: attachment.fileName }).click();
    const download = await downloadPromise;
    const stream = await download.createReadStream();
    expect(stream).not.toBeNull();

    const chunks: Buffer[] = [];
    for await (const chunk of stream!) {
      chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
    }
    expect(Buffer.concat(chunks)).toEqual(attachment.bytes);

    await reopenedModal.getByRole("button", { name: `Delete ${attachment.fileName}` }).click();
    await expect(reopenedModal.getByText(attachment.fileName)).toBeHidden();

    await page.reload();
    await openBoard(page, boardName);
    await page.getByText("Attachment Card").click();
    await expect(page.getByRole("dialog").getByText(attachment.fileName)).toBeHidden();
  });

  test("keeps the card consistent when storage cleanup fails during attachment delete", async ({ page }) => {
    const boardName = uniqueName("AttachmentDeleteCleanup");
    await registerUser(page);
    await createBoard(page, boardName);
    await openBoard(page, boardName);
    await addColumn(page, "To Do");
    await addCard(page, "To Do", "Cleanup Failure Card");
    await page.getByText("Cleanup Failure Card").click();

    const modal = page.getByRole("dialog");
    await modal.getByLabel("Attach file").setInputFiles({
      name: attachment.fileName,
      mimeType: attachment.mimeType,
      buffer: attachment.bytes,
    });
    await expect(modal.getByText(attachment.fileName)).toBeVisible();

    const failingDeleteRoute = await installFailingAttachmentDeleteRoute(page);

    await modal.getByRole("button", { name: `Delete ${attachment.fileName}` }).click();
    await expect(modal.getByText(attachment.fileName)).toBeHidden();
    await expect(modal.getByRole("alert")).toContainText(
      "Attachment removed from card, but file cleanup failed: storage cleanup failed",
    );

    await failingDeleteRoute.uninstall();
    const interceptedObjectName = failingDeleteRoute.interceptedObjectName();
    expect(interceptedObjectName).not.toBeNull();
    await deleteStorageObject(page, "card-attachments", interceptedObjectName!);

    await page.reload();
    await openBoard(page, boardName);
    await page.getByText("Cleanup Failure Card").click();
    await expect(page.getByRole("dialog").getByText(attachment.fileName)).toBeHidden();
  });
});
