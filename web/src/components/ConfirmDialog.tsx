import { Portal } from "./Portal";

type Props = {
  title: string;
  message: string;
  confirmLabel?: string;
  danger?: boolean;
  busy?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
};

export function ConfirmDialog({
  title,
  message,
  confirmLabel = "Confirm",
  danger = false,
  busy = false,
  onCancel,
  onConfirm,
}: Props) {
  return (
    <Portal>
      <div className="modal-backdrop" role="presentation" onClick={onCancel}>
        <div
          className="modal confirm-modal"
          role="alertdialog"
          aria-modal="true"
          aria-labelledby="confirm-title"
          onClick={(e) => e.stopPropagation()}
        >
          <header className="modal-head">
            <div>
              <h2 id="confirm-title">{title}</h2>
              <p className="meta">{message}</p>
            </div>
          </header>
          <footer className="modal-foot row end">
            <button type="button" className="btn ghost" disabled={busy} onClick={onCancel}>
              Cancel
            </button>
            <button
              type="button"
              className={danger ? "btn danger-ghost" : "btn-create"}
              disabled={busy}
              onClick={onConfirm}
            >
              {confirmLabel}
            </button>
          </footer>
        </div>
      </div>
    </Portal>
  );
}
