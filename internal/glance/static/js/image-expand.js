function removeDialog(dialog) {
    if (dialog === null) return;

    if (dialog.open) {
        dialog.close();
    }
    dialog.remove();
}

export default function(trigger) {
    let dialog = null;

    const closeDialog = () => {
        const currentDialog = dialog;
        dialog = null;
        removeDialog(currentDialog);
    };

    const openDialog = () => {
        if (dialog !== null) return;

        const source = trigger.dataset.imageExpandSrc;
        if (!source) return;

        const nextDialog = document.createElement("dialog");
        nextDialog.className = "image-expand-dialog";
        nextDialog.setAttribute("aria-label", trigger.getAttribute("aria-label") || "Expanded image");

        const image = document.createElement("img");
        image.className = "image-expand-dialog-image";
        image.src = source;
        image.alt = trigger.dataset.imageExpandAlt || "";

        const closeButton = document.createElement("button");
        closeButton.className = "image-expand-dialog-close glance-icon-button glance-button-quiet";
        closeButton.type = "button";
        closeButton.setAttribute("aria-label", "Close expanded image");
        closeButton.title = "Close";
        closeButton.textContent = "×";

        const onBackdropClick = (event) => {
            if (event.target === nextDialog) {
                closeDialog();
            }
        };

        closeButton.addEventListener("click", closeDialog);
        nextDialog.addEventListener("click", onBackdropClick);
        nextDialog.addEventListener("close", () => {
            if (dialog === nextDialog) {
                dialog = null;
            }
            nextDialog.remove();
        }, { once: true });

        nextDialog.append(image, closeButton);
        document.body.append(nextDialog);
        dialog = nextDialog;
        nextDialog.showModal();
    };

    trigger.addEventListener("click", openDialog);

    return () => {
        trigger.removeEventListener("click", openDialog);
        closeDialog();
    };
}
