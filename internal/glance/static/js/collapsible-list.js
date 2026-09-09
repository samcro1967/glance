export function attachExpandToggleButton(collapsibleContainer) {
    const showMoreText = "Show more";
    const showLessText = "Show less";

    let expanded = false;
    const button = document.createElement("button");
    const icon = document.createElement("span");
    icon.classList.add("expand-toggle-button-icon");
    const textNode = document.createTextNode(showMoreText);
    button.classList.add("expand-toggle-button");
    button.append(textNode, icon);
    button.addEventListener("click", () => {
        expanded = !expanded;

        if (expanded) {
            collapsibleContainer.classList.add("container-expanded");
            button.classList.add("container-expanded");
            textNode.nodeValue = showLessText;
            return;
        }

        const topBefore = button.getClientRects()[0].top;

        collapsibleContainer.classList.remove("container-expanded");
        button.classList.remove("container-expanded");
        textNode.nodeValue = showMoreText;

        const topAfter = button.getClientRects()[0].top;

        if (topAfter > 0)
            return;

        window.scrollBy({
            top: topAfter - topBefore,
            behavior: "instant"
        });
    });

    collapsibleContainer.after(button);

    return button;
};


export function setupCollapsibleList(list) {
    const existingButton = list._collapseToggleButton;

    if (existingButton !== undefined) {
        existingButton.remove();
        list._collapseToggleButton = undefined;
    }

    list.classList.remove("container-expanded");

    for (const child of list.children) {
        child.classList.remove("collapsible-item");
        child.style.animationDelay = "";
    }

    if (list.dataset.collapseAfter === undefined) {
        return;
    }

    const collapseAfter = parseInt(list.dataset.collapseAfter);

    if (collapseAfter == -1 || list.children.length <= collapseAfter) {
        return;
    }

    list._collapseToggleButton = attachExpandToggleButton(list);

    for (let c = collapseAfter; c < list.children.length; c++) {
        const child = list.children[c];
        child.classList.add("collapsible-item");
        child.style.animationDelay = ((c - collapseAfter) * 20).toString() + "ms";
    }
}
