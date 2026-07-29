# Storage

## Task

Browse a storage bucket, upload files, copy file URLs, preview images, purge CDN paths, and delete stored objects.

## Layout

1. Toolbar with the `Storage` title, bucket name field, file count, `Upload` action, and `CDN Purge` toggle.
2. Shared recoverable error notice when file loading or upload fails, with `View guide` linking to `https://allyourbase.io/guide/file-storage`.
3. Drag-and-drop overlay while files are dragged over the screen.
4. Main file area showing loading, blank-bucket, empty-bucket, or populated table state.
5. Optional CDN purge section below the file area when the toggle is enabled.
6. Preview dialog for image files.
7. Delete confirmation dialog for the selected file.

## State contract

### Loading
- The file area shows a spinner with `Loading files...` while `fetchFiles` is waiting for `listStorageFiles`.
- The toolbar remains visible so the user can see the current bucket and upload controls.

### Error
- Non-404 `fetchFiles` failures show an inline red alert containing the returned error message, or `Failed to load files` when the thrown value is not an `Error`.
- The fetch error alert includes a `Retry` action that reruns `fetchFiles` for the current bucket without removing the toolbar context.
- A 404 response from `fetchFiles` is treated as an empty bucket: the error alert is not shown, the file list is cleared, and the total is set to zero.
- Upload failures show the returned message in the shared recoverable error notice while preserving the file list and `Upload` action; the notice links to `https://allyourbase.io/guide/file-storage`.
- Signed URL and delete failures surface as toast errors rather than replacing the file list.

### Bucket selection
- The bucket field is labeled `Bucket name` and is initialized from local storage key `ayb_storage_bucket`, falling back to `default`.
- Each bucket field change persists the new value to `ayb_storage_bucket` and reloads files for the trimmed bucket value.
- When the bucket value is blank or whitespace, the file area shows `Enter a bucket name to browse`, the file list is empty, the total is zero, and the `Upload` button is disabled.

### Empty bucket
- When a non-blank bucket has no files, the file area shows `No files in "<bucket>"` and an `Upload your first file` action.
- The empty-state upload action opens the same hidden multi-file input used by the toolbar `Upload` action.

### Populated file table
- The table columns are `Name`, `Type`, `Size`, `Created`, and `Actions`.
- Each file row shows the object name, content type, formatted size, localized creation time, and action buttons.
- The name cell includes a copy-name action that copies the object name and shows a success toast.
- Image rows show a `Preview` action; all rows show `Download`, `Copy signed URL`, `Copy download URL`, and `Delete` actions.

### Upload
- The hidden file input accepts multiple files and uploads them to the selected bucket through `uploadStorageFile`.
- Dropping files anywhere on the screen uploads the dropped files after showing the drag overlay.
- While uploading, the toolbar button reads `Uploading...` and remains disabled.
- A successful upload shows a success toast with the uploaded file count, clears the file input value, and reloads the current bucket.
- A failed upload clears the file input value and leaves `Upload` available beside the shared recoverable error notice.

### Preview
- Image files are identified by content types beginning with `image/`.
- The preview dialog is labeled with the file name, displays the image from `storageDownloadURL(bucket, name)`, and shows content type, formatted size, and creation time.
- The close button returns to the file table without changing the bucket or file list.

### URL actions
- `Download` opens `storageDownloadURL(bucket, name)` in a new tab.
- `Copy signed URL` requests a one-hour signed URL and copies the returned URL to the clipboard.
- `Copy download URL` copies `storageDownloadURL(bucket, name)` to the clipboard.
- Successful copy actions show success toasts naming the copied value.

### CDN purge
- The `CDN Purge` toolbar action toggles the CDN purge section.
- The active toggle state is visible through the selected button styling while the section is displayed.

### Delete confirmation
- Clicking a row `Delete` action opens a dialog titled `Delete File`.
- The dialog states `Are you sure? This cannot be undone.` and displays the exact target file name.
- `Cancel` closes the dialog without deleting the file.
- Confirming `Delete` calls `deleteStorageFile(bucket, name)`, closes the dialog, removes the row from the current table, decrements the total, and shows a success toast.

## Navigation

- Route: `/admin/` with the `Storage` sidebar item selected.
- Entry: Select `Storage` from the admin sidebar.
- Back: Browser back follows the admin app history.
- Download: opens the selected file URL in a new browser tab.

## Acceptance criteria

- Given the admin sidebar is visible, when the user selects `Storage`, then the `Upload` button is visible before file assertions run. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`.
- Given a file has been seeded into the `default` bucket, when the user opens `Storage`, then the seeded file name appears in the list. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`.
- Given the storage screen is loaded, when the user uploads a text file through the file input, then the uploaded file name appears in the table. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`.
- Given an uploaded file row is visible, when the user clicks the row `Delete` action, then the delete confirmation is displayed before deletion. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`.
- Given the delete confirmation is displayed for an uploaded file, when the user confirms deletion, then the matching file row is no longer visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`.
- Given the storage screen is loaded, when the user inspects the toolbar, then the bucket field, file count, upload action, and CDN purge toggle are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`.
- Given a seeded file row is visible, when the user inspects the row, then the `Name`, `Type`, `Size`, `Created`, and `Actions` columns and copy/download/delete row actions are visible. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`.
- Given a file delete confirmation is open, when the user inspects the dialog, then the exact target file name is shown before confirmation. Evidence owner: existing assertion in `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`.
- Given an oversized upload is rejected, when the request completes, then the returned error, `Upload` recovery action, and `https://allyourbase.io/guide/file-storage` guide link are visible. Evidence owner: `ui/src/components/__tests__/StorageBrowser.test.tsx`.

## Edge cases

- Blank bucket: show `Enter a bucket name to browse`, disable upload, and avoid list requests for blank bucket names.
- Missing bucket: treat storage 404 as an empty bucket instead of an error.
- Empty bucket: show the bucket-specific empty message and first-upload action.
- Drag leave: hide the drag overlay without uploading.
- Upload failure: keep the current list visible, clear the input, and show the shared recoverable error notice with the file-storage guide link.
- Delete failure: keep the dialog context recoverable through the current modal state and show an error toast.
- Non-image files: omit the preview action and keep download and copy actions available.

## Current implementation gaps

- Current: Unmocked browser coverage proves seeded file content, deterministic empty bucket text, and load-error retry recovery. It does not prove blank-bucket, 404-empty, drag overlay, image preview, signed URL copy, download URL copy, copy-name toast, CDN purge section, or upload-failure behavior.
- Target: Acceptance evidence should cover these visible target states when a stable unmocked fixture can exercise them without adding mocked routes or a parallel harness.
- Evidence: `ui/src/components/StorageBrowser.tsx`; `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`.
