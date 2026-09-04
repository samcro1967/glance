function unitLabel(unit) {
    if (!unit.symbol || unit.symbol === unit.name)
        return unit.name;

    return `${unit.name} (${unit.symbol})`;
}

function toCanonical(value, unit) {
    switch (unit.transform) {
        case "scale":
            return value * unit.scale;

        case "affine":
            return value * unit.scale + unit.offset;

        case "reciprocal":
            if (value === 0)
                return NaN;

            return unit.constant / value;

        default:
            return NaN;
    }
}

function fromCanonical(value, unit) {
    switch (unit.transform) {
        case "scale":
            return value / unit.scale;

        case "affine":
            return (value - unit.offset) / unit.scale;

        case "reciprocal":
            if (value === 0)
                return NaN;

            return unit.constant / value;

        default:
            return NaN;
    }
}

function formatNumber(value) {
    if (!Number.isFinite(value))
        return "";

    if (Object.is(value, -0))
        value = 0;

    const absolute = Math.abs(value);

    if (absolute !== 0 && (absolute >= 1e12 || absolute < 1e-9)) {
        return value
            .toExponential(10)
            .replace(/\.?0+e/, "e")
            .replace("e+", "e");
    }

    return new Intl.NumberFormat(undefined, {
        maximumSignificantDigits: 12,
        useGrouping: true,
    }).format(value);
}

function populateSelect(select, units) {
    select.replaceChildren();

    const sortedUnits = [...units].sort((a, b) =>
        a.name.localeCompare(b.name, undefined, {
            sensitivity: "base",
        })
    );

    for (const unit of sortedUnits) {
        const option = document.createElement("option");
        option.value = unit.id;
        option.textContent = unitLabel(unit);
        select.append(option);
    }
}

function findUnit(category, id) {
    return category.units.find(unit => unit.id === id);
}

function initializeConverter(element) {
    const catalogElement = element.querySelector(
        "[data-unit-converter-catalog]"
    );

    if (!catalogElement)
        return;

    let catalog;

    try {
        catalog = JSON.parse(catalogElement.textContent);
    } catch (error) {
        console.error("Failed to parse unit converter catalog:", error);
        return;
    }

    if (!Array.isArray(catalog.categories) || catalog.categories.length === 0)
        return;

    const categorySelect = element.querySelector(
        "[data-unit-converter-category]"
    );
    const fromSelect = element.querySelector("[data-unit-converter-from]");
    const toSelect = element.querySelector("[data-unit-converter-to]");
    const valueInput = element.querySelector("[data-unit-converter-value]");
    const result = element.querySelector("[data-unit-converter-result]");

    const currentCategory = () =>
        catalog.categories.find(
            category => category.id === categorySelect.value
        );

    const updateResult = () => {
        const category = currentCategory();

        if (!category) {
            result.textContent = "";
            return;
        }

        const fromUnit = findUnit(category, fromSelect.value);
        const toUnit = findUnit(category, toSelect.value);
        const input = valueInput.value.trim();

        if (!fromUnit || !toUnit || input === "") {
            result.textContent = "";
            return;
        }

        const value = Number(input);

        if (!Number.isFinite(value)) {
            result.textContent = "Invalid value";
            return;
        }

        const canonical = toCanonical(value, fromUnit);
        const converted = fromCanonical(canonical, toUnit);

        if (!Number.isFinite(converted)) {
            result.textContent = "Undefined";
            return;
        }

        const formatted = formatNumber(converted);
        result.textContent = toUnit.symbol
            ? `${formatted} ${toUnit.symbol}`
            : formatted;
    };

    const updateUnits = () => {
        const category = currentCategory();

        if (!category)
            return;

        populateSelect(fromSelect, category.units);
        populateSelect(toSelect, category.units);

        fromSelect.value = category.default_from;
        toSelect.value = category.default_to;

        if (fromSelect.selectedIndex === -1)
            fromSelect.selectedIndex = 0;

        if (toSelect.selectedIndex === -1)
            toSelect.selectedIndex =
                category.units.length > 1 ? 1 : 0;

        updateResult();
    };

    const sortedCategories = [...catalog.categories].sort((a, b) =>
        a.name.localeCompare(b.name, undefined, {
            sensitivity: "base",
        })
    );

    for (const category of sortedCategories) {
        const option = document.createElement("option");
        option.value = category.id;
        option.textContent = category.name;
        categorySelect.append(option);
    }

    categorySelect.addEventListener("change", updateUnits);
    fromSelect.addEventListener("change", updateResult);
    toSelect.addEventListener("change", updateResult);
    valueInput.addEventListener("input", updateResult);

    updateUnits();
}

export default function(element) {
    initializeConverter(element);
}
