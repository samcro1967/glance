import { refreshPresentationTheme } from "./presentation.js";
import { elem, find, findAll } from "./templating.js";
import { frontendDiagnostic, frontendDiagnosticErrorDetail } from "./diagnostics.js";

function updateThemeCustomCSS(pageData, path) {
    const current = find("#theme-custom-css");

    if (!path) {
        current?.remove();
        return;
    }

    const href = `${path}?v=${pageData.createdAt}`;
    if (current) {
        current.href = href;
        return;
    }

    const link = elem("link");
    link.id = "theme-custom-css";
    link.rel = "stylesheet";
    link.href = href;

    const pageCustomCSS = find("#page-custom-css");
    if (pageCustomCSS) {
        document.head.insertBefore(link, pageCustomCSS);
        return;
    }

    document.head.appendChild(link);
}

async function changeTheme(pageData, key, onChanged) {
    const themeStyleElem = find("#theme-style");
    const themeChangeStarted = performance.now();

    frontendDiagnostic("theme_change_start", {
        detail: String(key).slice(0, 64),
    });

    try {
        const pageQuery = pageData.slug
            ? `?page=${encodeURIComponent(pageData.slug)}`
            : "";

        const response = await fetch(`${pageData.baseURL}/api/set-theme/${key}${pageQuery}`, {
            method: "POST",
        });

        frontendDiagnostic("theme_change_response", {
            detail: String(key).slice(0, 64),
            status: response.status,
            elapsedMS: performance.now() - themeChangeStarted,
        });

        if (response.status != 200) {
            frontendDiagnostic("theme_change_error", {
                detail: String(key).slice(0, 64),
                status: response.status,
                elapsedMS: performance.now() - themeChangeStarted,
            }, true);

            alert("Failed to set theme: " + response.statusText);
            return;
        }

        const newThemeStyle = await response.text();

        const tempStyle = elem("style")
            .html("* { transition: none !important; }")
            .appendTo(document.head);

        themeStyleElem.html(newThemeStyle);
        updateThemeCustomCSS(pageData, response.headers.get("X-Theme-Custom-CSS"));
        document.documentElement.setAttribute("data-theme", key);
        document.documentElement.setAttribute("data-scheme", response.headers.get("X-Scheme"));
        typeof onChanged == "function" && onChanged();
        refreshPresentationTheme();
        setTimeout(() => { tempStyle.remove(); }, 10);

        frontendDiagnostic("theme_change_complete", {
            detail: String(key).slice(0, 64),
            elapsedMS: performance.now() - themeChangeStarted,
        });
    } catch (error) {
        frontendDiagnostic("theme_change_error", {
            detail: frontendDiagnosticErrorDetail(error),
            elapsedMS: performance.now() - themeChangeStarted,
        }, true);

        alert("Failed to set theme");
    }
}

function initThemePicker(pageData) {
    const themeChoicesInMobileNav = find(".mobile-navigation .theme-choices");
    if (!themeChoicesInMobileNav) return;

    const themeChoicesInHeader = find(".header-container .theme-choices");

    if (themeChoicesInHeader) {
        themeChoicesInHeader.replaceWith(
            themeChoicesInMobileNav.cloneNode(true)
        );
    }

    const presetElems = findAll(".theme-choices .theme-preset");
    let themePreviewElems = document.getElementsByClassName("current-theme-preview");
    let isLoading = false;

    presetElems.forEach((presetElement) => {
        const themeKey = presetElement.dataset.key;

        if (themeKey === undefined) {
            return;
        }

        if (themeKey == pageData.theme) {
            presetElement.classList.add("current");
        }

        presetElement.addEventListener("click", () => {
            if (themeKey == pageData.theme) return;
            if (isLoading) return;

            isLoading = true;
            changeTheme(pageData, themeKey, function() {
                pageData.theme = themeKey;
                presetElems.forEach((e) => { e.classList.remove("current"); });

                Array.from(themePreviewElems).forEach((preview) => {
                    preview.querySelector(".theme-preset").replaceWith(
                        presetElement.cloneNode(true)
                    );
                })

                presetElems.forEach((e) => {
                    if (e.dataset.key != themeKey) return;
                    e.classList.add("current");
                });
            }).finally(() => {
                isLoading = false;
            });
        });
    })
}


export { initThemePicker };
