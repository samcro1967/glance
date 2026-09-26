export function getPresentationConfig(element) {
    const widget = element.closest(".widget");
    const script = widget?.querySelector(
        ":scope > .widget-content > script[data-glance-presentation-config]"
    );

    if (script === null || script === undefined) {
        return {};
    }

    return JSON.parse(script.textContent);
}
