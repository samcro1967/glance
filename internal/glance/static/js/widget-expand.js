import { frontendDiagnosticError } from "./diagnostics.js";

const EXPANDED_WIDGET_FETCH_TIMEOUT_MS = 5000;

function removeDialog(dialog) {
    if (dialog === null) return;
    if (dialog.open) dialog.close();
    dialog.remove();
}

export default function(trigger, widgetElement, baseURL) {
    let dialog = null;
    let controller = null;

    const closeDialog = () => {
        if (controller !== null) {
            controller.abort();
            controller = null;
        }
        const currentDialog = dialog;
        dialog = null;
        removeDialog(currentDialog);
    };

    const openDialog = async () => {
        if (dialog !== null) return;

        const widgetID = widgetElement.dataset.widgetId;
        if (!widgetID) return;

        const nextDialog = document.createElement("dialog");
        nextDialog.className = "widget-expand-dialog";
        nextDialog.setAttribute("aria-label", `Expanded ${widgetElement.querySelector(".widget-header h2")?.textContent?.trim() || "widget"}`);

        const content = document.createElement("div");
        content.className = "widget-expand-dialog-content";
        content.textContent = "Loading…";

        const closeButton = document.createElement("button");
        closeButton.className = "widget-expand-dialog-close glance-icon-button glance-button-quiet";
        closeButton.type = "button";
        closeButton.setAttribute("aria-label", "Close expanded widget");
        closeButton.title = "Close";
        closeButton.textContent = "×";

        closeButton.addEventListener("click", closeDialog);
        nextDialog.addEventListener("click", (event) => {
            if (event.target === nextDialog) closeDialog();
        });
        nextDialog.addEventListener("close", () => {
            if (dialog === nextDialog) dialog = null;
            nextDialog.remove();
        }, { once: true });

        nextDialog.append(content, closeButton);
        document.body.append(nextDialog);
        dialog = nextDialog;
        nextDialog.showModal();

        const requestController = new AbortController();
        controller = requestController;
        const timeout = setTimeout(() => requestController.abort(), EXPANDED_WIDGET_FETCH_TIMEOUT_MS);

        try {
            const response = await fetch(`${baseURL}/api/widgets/${encodeURIComponent(widgetID)}/expanded/`, { signal: requestController.signal });
            if (!response.ok) throw new Error(`Failed to load expanded widget: ${response.status} ${response.statusText}`);
            content.innerHTML = await response.text();
        } catch (error) {
            if (error.name === "AbortError" && dialog === null) return;
            frontendDiagnosticError("widget_expand_failed", error, { widget: widgetID });
            content.textContent = "Unable to load expanded widget.";
        } finally {
            clearTimeout(timeout);
            if (controller === requestController) controller = null;
        }
    };

    trigger.addEventListener("click", openDialog);

    return () => {
        trigger.removeEventListener("click", openDialog);
        closeDialog();
    };
}
