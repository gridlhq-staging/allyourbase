import { useState, useEffect } from "react";
import { ayb, quoteRecordFilterLiteral } from "../lib/ayb";
import type { Attachment, Card } from "../types";

const ATTACHMENT_BUCKET = "card-attachments";

function sanitizeAttachmentObjectComponent(fileName: string): string {
  const normalized = fileName
    .replace(/[\\/]/g, "_")
    .replace(/[\u0000-\u001f\u007f]/g, "")
    .trim();
  return normalized || "attachment";
}

interface Props {
  card: Card;
  onClose: () => void;
  onUpdate: (card: Card) => void;
  onDelete: (cardId: string) => void;
}

export default function CardModal({ card, onClose, onUpdate, onDelete }: Props) {
  const [title, setTitle] = useState(card.title);
  const [description, setDescription] = useState(card.description);
  const [saving, setSaving] = useState(false);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [loadingAttachments, setLoadingAttachments] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [deletingAttachmentId, setDeletingAttachmentId] = useState<string | null>(null);
  const [attachmentError, setAttachmentError] = useState<string | null>(null);
  const [cardError, setCardError] = useState<string | null>(null);

  useEffect(() => {
    function handleEsc(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handleEsc);
    return () => window.removeEventListener("keydown", handleEsc);
  }, [onClose]);

  useEffect(() => {
    let active = true;
    setAttachments([]);
    setLoadingAttachments(true);
    setAttachmentError(null);

    ayb.records.list<Attachment>("attachments", {
      filter: `card_id=${quoteRecordFilterLiteral(card.id)}`,
      perPage: 100,
      sort: "created_at",
    }).then((response) => {
      if (active) setAttachments(response.items);
    }).catch((err) => {
      if (!active) return;
      setAttachmentError(
        err instanceof Error ? err.message : "Failed to load attachments",
      );
    }).finally(() => {
      if (active) setLoadingAttachments(false);
    });

    return () => {
      active = false;
    };
  }, [card.id]);

  async function handleSave() {
    if (!title.trim()) return;
    setSaving(true);
    setCardError(null);
    try {
      const updated = await ayb.records.update<Card>("cards", card.id, {
        title: title.trim(),
        description: description.trim(),
      });
      onUpdate(updated);
      onClose();
    } catch (err) {
      console.error("Failed to update card:", err);
      setCardError(err instanceof Error ? err.message : "Failed to update card");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!confirm("Delete this card?")) return;
    setCardError(null);
    try {
      await ayb.records.delete("cards", card.id);
      onDelete(card.id);
      onClose();
    } catch (err) {
      console.error("Failed to delete card:", err);
      setCardError(err instanceof Error ? err.message : "Failed to delete card");
    }
  }

  async function handleAttachmentUpload(file: File) {
    setUploading(true);
    setAttachmentError(null);
    const objectName = `${card.id}/${crypto.randomUUID()}-${sanitizeAttachmentObjectComponent(file.name)}`;
    let objectUploaded = false;

    try {
      const user = await ayb.auth.me();
      await ayb.storage.upload(ATTACHMENT_BUCKET, file, objectName);
      objectUploaded = true;
      const attachment = await ayb.records.create<Attachment>("attachments", {
        card_id: card.id,
        bucket: ATTACHMENT_BUCKET,
        object_name: objectName,
        file_name: file.name,
        content_type: file.type || "application/octet-stream",
        size: file.size,
        user_id: user.id,
      });
      setAttachments((current) => [...current, attachment]);
    } catch (err) {
      if (objectUploaded) {
        try {
          await ayb.storage.delete(ATTACHMENT_BUCKET, objectName);
        } catch (cleanupError) {
          console.error("Failed to clean up attachment after metadata error:", cleanupError);
        }
      }
      setAttachmentError(
        err instanceof Error ? err.message : "Failed to upload attachment",
      );
    } finally {
      setUploading(false);
    }
  }

  async function handleAttachmentDelete(attachment: Attachment) {
    setDeletingAttachmentId(attachment.id);
    setAttachmentError(null);
    let metadataDeleted = false;
    try {
      await ayb.records.delete("attachments", attachment.id);
      metadataDeleted = true;
      setAttachments((current) => current.filter(({ id }) => id !== attachment.id));
      await ayb.storage.delete(attachment.bucket, attachment.object_name);
    } catch (err) {
      if (metadataDeleted) {
        console.error("Failed to clean up attachment file after metadata delete:", err);
        setAttachmentError(
          err instanceof Error
            ? `Attachment removed from card, but file cleanup failed: ${err.message}`
            : "Attachment removed from card, but file cleanup failed",
        );
      } else {
        setAttachmentError(
          err instanceof Error ? err.message : "Failed to delete attachment",
        );
      }
    } finally {
      setDeletingAttachmentId(null);
    }
  }

  async function handleAttachmentDownload(attachment: Attachment) {
    setAttachmentError(null);
    try {
      const file = await ayb.storage.download(attachment.bucket, attachment.object_name);
      const objectURL = URL.createObjectURL(file);
      const downloadLink = document.createElement("a");
      downloadLink.href = objectURL;
      downloadLink.download = attachment.file_name;
      downloadLink.click();
      URL.revokeObjectURL(objectURL);
    } catch (err) {
      setAttachmentError(
        err instanceof Error ? err.message : "Failed to download attachment",
      );
    }
  }

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="card-modal-title"
        className="bg-white rounded-xl shadow-2xl p-6 w-full max-w-lg mx-4 max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between mb-4">
          <h3 id="card-modal-title" className="text-lg font-semibold text-gray-900">Edit Card</h3>
          <button
            onClick={onClose}
            aria-label="Close"
            className="text-gray-400 hover:text-gray-600"
          >
            <svg
              className="w-5 h-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>

        <div className="space-y-4">
          {cardError && (
            <p role="alert" className="text-sm text-red-600">
              {cardError}
            </p>
          )}

          <div>
            <label htmlFor="card-title" className="block text-sm font-medium text-gray-700 mb-1">
              Title
            </label>
            <input
              id="card-title"
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>

          <div>
            <label htmlFor="card-description" className="block text-sm font-medium text-gray-700 mb-1">
              Description
            </label>
            <textarea
              id="card-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={4}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
              placeholder="Add a description..."
            />
          </div>

          <section aria-labelledby="attachments-title" className="space-y-3">
            <div className="flex items-center justify-between gap-3">
              <h4 id="attachments-title" className="text-sm font-medium text-gray-700">
                Attachments
              </h4>
              <label
                htmlFor="card-attachment"
                className="cursor-pointer text-sm font-medium text-blue-600 hover:text-blue-700"
              >
                Attach file
              </label>
              <input
                id="card-attachment"
                type="file"
                className="sr-only"
                disabled={loadingAttachments || uploading}
                onChange={(event) => {
                  const input = event.currentTarget;
                  const file = input.files?.[0];
                  if (file) void handleAttachmentUpload(file).finally(() => {
                    input.value = "";
                  });
                }}
              />
            </div>

            {loadingAttachments && (
              <p className="text-sm text-gray-500">Loading attachments...</p>
            )}
            {uploading && (
              <p className="text-sm text-gray-500">Uploading attachment...</p>
            )}
            {!loadingAttachments && attachments.length === 0 && !uploading && (
              <p className="text-sm text-gray-500">No attachments</p>
            )}
            {attachments.length > 0 && (
              <ul className="divide-y divide-gray-200 rounded-lg border border-gray-200">
                {attachments.map((attachment) => {
                  const deleting = deletingAttachmentId === attachment.id;
                  return (
                    <li
                      key={attachment.id}
                      className="flex items-center justify-between gap-3 px-3 py-2"
                    >
                      <div className="min-w-0">
                        <a
                          href={ayb.storage.downloadURL(attachment.bucket, attachment.object_name)}
                          download={attachment.file_name}
                          onClick={(event) => {
                            event.preventDefault();
                            void handleAttachmentDownload(attachment);
                          }}
                          className="block truncate text-sm text-blue-600 hover:underline"
                        >
                          {attachment.file_name}
                        </a>
                        <p className="text-xs text-gray-500">
                          {attachment.content_type} · {attachment.size} bytes
                        </p>
                      </div>
                      <button
                        type="button"
                        aria-label={`Delete ${attachment.file_name}`}
                        disabled={deletingAttachmentId !== null}
                        onClick={() => void handleAttachmentDelete(attachment)}
                        className="shrink-0 text-sm font-medium text-red-600 hover:text-red-700 disabled:opacity-50"
                      >
                        {deleting ? "Deleting..." : "Delete"}
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
            {attachmentError && (
              <p role="alert" className="text-sm text-red-600">{attachmentError}</p>
            )}
          </section>
        </div>

        <div className="flex items-center justify-between mt-6">
          <button
            onClick={handleDelete}
            className="text-red-600 hover:text-red-700 text-sm font-medium"
          >
            Delete card
          </button>
          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 rounded-lg transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleSave}
              disabled={saving || !title.trim()}
              className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
