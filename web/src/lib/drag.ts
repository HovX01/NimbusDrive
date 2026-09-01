export const NIMBUS_DRAG_TYPE = "application/x-nimbus-nodes";

export function setNodeDragData(e: DragEvent, ids: string[]) {
  const payload = JSON.stringify(ids);
  e.dataTransfer?.setData(NIMBUS_DRAG_TYPE, payload);
  e.dataTransfer?.setData("text/plain", payload);
  if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
}

export function readNodeDragData(e: DragEvent): string[] {
  const raw = e.dataTransfer?.getData(NIMBUS_DRAG_TYPE) || e.dataTransfer?.getData("text/plain") || "";
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((id) => typeof id === "string" && id) : [];
  } catch {
    return [];
  }
}

export function isNodeDrag(e: DragEvent): boolean {
  return Array.from(e.dataTransfer?.types ?? []).includes(NIMBUS_DRAG_TYPE);
}

export function isFileDrag(e: DragEvent): boolean {
  return Array.from(e.dataTransfer?.types ?? []).includes("Files");
}
