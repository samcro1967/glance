export function getPresentationConfig(element) {
    const scope = element.closest("[data-glance-presentation-scope], .widget");
    const script = scope?.querySelector(
        ":scope > script[data-glance-presentation-config], :scope > .widget-content > script[data-glance-presentation-config]"
    );

    if (script === null || script === undefined) {
        return {};
    }

    return JSON.parse(script.textContent);
}
