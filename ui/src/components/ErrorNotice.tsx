import { AlertCircle } from "lucide-react";
import { docsUrl, type GuidePath } from "../lib/docs_url";

interface ErrorNoticeProps {
  message: string;
  docsPath: GuidePath;
  actionLabel?: string;
  onAction?: () => void;
  variant?: "inline" | "page";
}

// Centralizes the RF-013-RF-018 message, action, and docs layout.
export function ErrorNotice({
  message,
  docsPath,
  actionLabel = "Retry",
  onAction,
  variant = "inline",
}: ErrorNoticeProps) {
  const MessageElement = variant === "page" ? "h1" : "p";

  return (
    <div
      role="alert"
      className={
        variant === "page"
          ? "flex flex-col items-center gap-3 text-center"
          : "flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-900/60 dark:bg-red-950/30"
      }
    >
      <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-500" />
      <div className={variant === "page" ? "flex flex-col items-center gap-3" : "min-w-0"}>
        <MessageElement
          className={
            variant === "page"
              ? "text-lg font-semibold text-red-700 dark:text-red-300"
              : "whitespace-pre-wrap break-words text-sm text-red-700 dark:text-red-300"
          }
        >
          {message}
        </MessageElement>
        <div className="mt-2 flex flex-wrap items-center gap-3 text-xs">
          {onAction && (
            <button
              type="button"
              onClick={onAction}
              className="font-medium text-red-700 underline hover:text-red-900 dark:text-red-300 dark:hover:text-red-100"
            >
              {actionLabel}
            </button>
          )}
          <a
            href={docsUrl(docsPath)}
            target="_blank"
            rel="noreferrer"
            className="font-medium text-blue-700 underline hover:text-blue-900 dark:text-blue-300 dark:hover:text-blue-100"
          >
            View guide
          </a>
        </div>
      </div>
    </div>
  );
}
