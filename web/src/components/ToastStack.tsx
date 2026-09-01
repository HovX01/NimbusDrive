export type ToastKind = "success" | "error" | "info";

export type Toast = {
  id: string;
  message: string;
  kind: ToastKind;
};

type Props = {
  items: Toast[];
  onDismiss: (id: string) => void;
};

export function ToastStack({ items, onDismiss }: Props) {
  if (!items.length) return null;
  return (
    <div className="toast-stack" aria-live="polite" aria-atomic="true">
      {items.map((t) => (
        <div key={t.id} className={`toast toast-${t.kind}`} role="status">
          <span>{t.message}</span>
          <button type="button" className="toast-close" aria-label="Dismiss" onClick={() => onDismiss(t.id)}>
            <i className="fa-solid fa-xmark" />
          </button>
        </div>
      ))}
    </div>
  );
}
