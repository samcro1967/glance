// Reconcile pending presentation after SSE opens. A completion can occur between
// HTML rendering and subscription; asking for current HTML closes that gap.
// Status-bar items resolve to their containing bar, including ticker duplicates.
export function reconcileLoadingWidgets(root, refresh) {
    const targets = new Set();
    root.querySelectorAll('[data-widget-state="loading"]').forEach((element) => {
        const target = element.closest('[data-widget-id]');
        const id = target?.dataset.widgetId;
        if (/^\d+$/.test(id ?? '')) targets.add(id);
    });
    for (const id of targets) refresh(id);
    return targets.size;
}
