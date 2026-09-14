import { setupPresentation } from './presentation.js';
import { cleanupPopoversWithin, setupPopovers } from './popover.js';
import { setupMasonries } from './masonry.js';
import { throttledDebounce, isElementVisible } from './utils.js';

import { attachExpandToggleButton, setupCollapsibleList } from './collapsible-list.js';
import { setupSearchBoxes } from './search.js';
import { updateRelativeTimeForElements, setupDynamicRelativeTime } from './relative-time.js';
import { setupClocks, setupFooterMicroClocks, setupAnalogClocks } from './clocks.js';
import { initThemePicker } from './theme.js';

import {
    captureFrontendPerformanceSnapshot,
    frontendDiagnostic,
    frontendDiagnosticError,
    runFrontendDiagnosticAsyncStage,
    runFrontendDiagnosticStage
} from "./diagnostics.js";

const PAGE_CONTENT_FETCH_TIMEOUT_MS = 5000;

async function fetchPageContent(pageData) {
    const fetchStarted = performance.now();
    frontendDiagnostic("page_content_fetch_start");

    const controller = new AbortController();
    const timeout = setTimeout(
        () => controller.abort(),
        PAGE_CONTENT_FETCH_TIMEOUT_MS
    );

    try {
        const response = await fetch(
            `${pageData.baseURL}/api/pages/${pageData.slug}/content/`,
            { signal: controller.signal }
        );

        frontendDiagnostic("page_content_fetch_response", {
            status: response.status,
            elapsedMS: performance.now() - fetchStarted,
        });

        if (!response.ok) {
            throw new Error(
                `Failed to load page content: ${response.status} ${response.statusText}`
            );
        }

        const bodyStarted = performance.now();
        const content = await response.text();
        frontendDiagnostic("page_content_fetch_complete", {
            length: content.length,
            elapsedMS: performance.now() - bodyStarted,
        });

        return content;
    } finally {
        clearTimeout(timeout);
    }
}

function setupCarousels(root = document) {
    const carouselElements = root.querySelectorAll(".carousel-container");
    const cleanupCallbacks = [];

    if (carouselElements.length == 0) {
        return cleanupCallbacks;
    }

    for (let i = 0; i < carouselElements.length; i++) {
        const carousel = carouselElements[i];
        carousel.classList.add("show-right-cutoff");
        const itemsContainer = carousel.getElementsByClassName("carousel-items-container")[0];

        const determineSideCutoffs = () => {
            if (itemsContainer.scrollLeft != 0) {
                carousel.classList.add("show-left-cutoff");
            } else {
                carousel.classList.remove("show-left-cutoff");
            }

            if (Math.ceil(itemsContainer.scrollLeft) + itemsContainer.clientWidth < itemsContainer.scrollWidth) {
                carousel.classList.add("show-right-cutoff");
            } else {
                carousel.classList.remove("show-right-cutoff");
            }
        }

        const determineSideCutoffsRateLimited = throttledDebounce(determineSideCutoffs, 20, 100);

        itemsContainer.addEventListener("scroll", determineSideCutoffsRateLimited);
        window.addEventListener("resize", determineSideCutoffsRateLimited);

        const cleanup = () => {
            itemsContainer.removeEventListener("scroll", determineSideCutoffsRateLimited);
            window.removeEventListener("resize", determineSideCutoffsRateLimited);
        };

        if (root === document) {
            registerLiveWidgetCleanup(
                carousel.closest("[data-widget-id]"),
                cleanup
            );
            afterContentReady(determineSideCutoffs);
        } else {
            cleanupCallbacks.push(cleanup);
            determineSideCutoffs();
        }
    }

    return cleanupCallbacks;
}

const STATUS_BAR_TICKER_PIXELS_PER_SECOND = {
    slow: 30,
    normal: 45,
    fast: 70,
};

function setupStatusBarTickers(root = document) {
    const cleanupCallbacks = [];
    const statusBars = root.querySelectorAll(".status-bar-mode-ticker");

    for (const statusBar of statusBars) {
        if (statusBar.dataset.tickerInitialized === "true") {
            continue;
        }

        const track = statusBar.querySelector(".status-bar-track");
        const items = statusBar.querySelector(".status-bar-items:not(.status-bar-items-duplicate)");

        if (track == null || items == null) {
            continue;
        }

        const speed = statusBar.dataset.tickerSpeed;
        const pixelsPerSecond = STATUS_BAR_TICKER_PIXELS_PER_SECOND[speed]
            || STATUS_BAR_TICKER_PIXELS_PER_SECOND.normal;

        const updateTickerGeometry = () => {
            const contentWidth = items.getBoundingClientRect().width;
            const containerWidth = statusBar.getBoundingClientRect().width;

            if (contentWidth <= 0 || containerWidth <= 0) {
                statusBar.dataset.tickerReady = "false";
                return;
            }

            const short = contentWidth < containerWidth;
            const distance = short
                ? containerWidth + contentWidth
                : contentWidth;

            track.style.setProperty(
                "--status-bar-ticker-content-width",
                `${contentWidth}px`
            );
            track.style.setProperty(
                "--status-bar-ticker-container-width",
                `${containerWidth}px`
            );
            track.style.setProperty(
                "--status-bar-ticker-duration",
                `${distance / pixelsPerSecond}s`
            );

            statusBar.dataset.tickerShort = short ? "true" : "false";
            statusBar.dataset.tickerReady = "true";
        };

        updateTickerGeometry();

        if (typeof ResizeObserver !== "undefined") {
            const resizeObserver = new ResizeObserver(updateTickerGeometry);
            resizeObserver.observe(items);
            resizeObserver.observe(statusBar);
            cleanupCallbacks.push(() => resizeObserver.disconnect());
        }

        const handlePointerUp = (event) => {
            if (event.pointerType !== "mouse") {
                return;
            }

            const link = event.target.closest("a");
            if (link != null) {
                link.blur();
            }
        };

        statusBar.addEventListener("pointerup", handlePointerUp);
        cleanupCallbacks.push(() => {
            statusBar.removeEventListener("pointerup", handlePointerUp);
        });

        statusBar.dataset.tickerInitialized = "true";
    }

    return cleanupCallbacks;
}

function setupGroups() {
    const groups = document.getElementsByClassName("widget-type-group");

    if (groups.length == 0) {
        return;
    }

    for (let g = 0; g < groups.length; g++) {
        const group = groups[g];

        const content = group.querySelector(":scope > .widget-content");
        if (content === null) {
            continue;
        }

        const header = content.querySelector(
            ":scope > .widget-group-header > .widget-header"
        );
        const contents = content.querySelector(
            ":scope > .widget-group-contents"
        );

        if (header === null || contents === null) {
            continue;
        }

        const titles = header.children;
        const tabs = contents.children;
        let current = 0;

        for (let t = 0; t < titles.length; t++) {
            const title = titles[t];

            if (title.dataset.titleUrl !== undefined) {
                title.addEventListener("mousedown", (event) => {
                    if (event.button != 1) {
                        return;
                    }

                    openURLInNewTab(title.dataset.titleUrl, false);
                    event.preventDefault();
                });
            }

            title.addEventListener("click", () => {
                if (t == current) {
                    if (title.dataset.titleUrl !== undefined) {
                        openURLInNewTab(title.dataset.titleUrl);
                    }

                    return;
                }

                for (let i = 0; i < titles.length; i++) {
                    titles[i].classList.remove("widget-group-title-current");
                    titles[i].setAttribute("aria-selected", "false");
                    tabs[i].classList.remove("widget-group-content-current");
                    tabs[i].setAttribute("aria-hidden", "true");
                }

                if (current < t) {
                    tabs[t].dataset.direction = "right";
                } else {
                    tabs[t].dataset.direction = "left";
                }

                current = t;

                title.classList.add("widget-group-title-current");
                title.setAttribute("aria-selected", "true");
                tabs[t].classList.add("widget-group-content-current");
                tabs[t].setAttribute("aria-hidden", "false");
            });
        }
    }
}

function setupLazyImages(root = document) {
    const images = root.querySelectorAll("img[loading=lazy]");

    if (images.length == 0) {
        return;
    }

    function imageFinishedTransition(image) {
        image.classList.add("finished-transition");
    }

    const initializeImages = () => {
        setTimeout(() => {
            for (let i = 0; i < images.length; i++) {
                const image = images[i];

                if (image.complete) {
                    image.classList.add("cached");
                    setTimeout(() => imageFinishedTransition(image), 1);
                } else {
                    image.addEventListener("load", () => {
                        image.classList.add("loaded");
                        setTimeout(() => imageFinishedTransition(image), 400);
                    });
                    image.addEventListener("error", () => {
                        image.classList.add("error");
                        imageFinishedTransition(image);
                    });
                }
            }
        }, 1);
    };

    if (root === document) {
        afterContentReady(initializeImages);
    } else {
        initializeImages();
    }
}

function setupCollapsibleLists(root = document) {
    const collapsibleLists = root.querySelectorAll(".list.collapsible-container");

    for (const list of collapsibleLists) {
        setupCollapsibleList(list);
    }
}

function setupCollapsibleGrids(root = document) {
    const collapsibleGridElements = root.querySelectorAll(".cards-grid.collapsible-container");
    const cleanupCallbacks = [];

    if (collapsibleGridElements.length == 0) {
        return cleanupCallbacks;
    }

    for (let i = 0; i < collapsibleGridElements.length; i++) {
        const gridElement = collapsibleGridElements[i];

        if (gridElement.dataset.collapseAfterRows === undefined) {
            continue;
        }

        const collapseAfterRows = parseInt(gridElement.dataset.collapseAfterRows);

        if (collapseAfterRows == -1) {
            continue;
        }

        const getCardsPerRow = () => {
            return parseInt(getComputedStyle(gridElement).getPropertyValue('--cards-per-row'));
        };

        const button = attachExpandToggleButton(gridElement);

        let cardsPerRow;

        const resolveCollapsibleItems = () => requestAnimationFrame(() => {
            const hideItemsAfterIndex = cardsPerRow * collapseAfterRows;

            if (hideItemsAfterIndex >= gridElement.children.length) {
                button.style.display = "none";
            } else {
                button.style.removeProperty("display");
            }

            let row = 0;

            for (let i = 0; i < gridElement.children.length; i++) {
                const child = gridElement.children[i];

                if (i >= hideItemsAfterIndex) {
                    child.classList.add("collapsible-item");
                    child.style.animationDelay = (row * 40).toString() + "ms";

                    if (i % cardsPerRow + 1 == cardsPerRow) {
                        row++;
                    }
                } else {
                    child.classList.remove("collapsible-item");
                    child.style.removeProperty("animation-delay");
                }
            }
        });

        const observer = new ResizeObserver(() => {
            if (!isElementVisible(gridElement)) {
                return;
            }

            const newCardsPerRow = getCardsPerRow();

            if (cardsPerRow == newCardsPerRow) {
                return;
            }

            cardsPerRow = newCardsPerRow;
            resolveCollapsibleItems();
        });

        const cleanup = () => observer.disconnect();

        if (root === document) {
            registerLiveWidgetCleanup(
                gridElement.closest("[data-widget-id]"),
                cleanup
            );
            afterContentReady(() => observer.observe(gridElement));
        } else {
            cleanupCallbacks.push(cleanup);
            observer.observe(gridElement);
        }
    }

    return cleanupCallbacks;
}

const contentReadyCallbacks = [];

function afterContentReady(callback) {
    contentReadyCallbacks.push(callback);
}

async function setupCalendars(root = document) {
    const elems = Array.from(root.getElementsByClassName("calendar"));

    if (elems.length === 0) {
        return [];
    }

    // Calendar is loaded lazily because most pages do not contain one.
    // Returning cleanup callbacks also lets live widget replacement stop
    // Calendar-owned timers before discarding the old DOM.
    const calendarModule = await import('./calendar.js');
    const cleanupCallbacks = [];

    for (const element of elems) {
        const calendar = calendarModule.default(element);

        if (calendar?.component?.suspend) {
            cleanupCallbacks.push(() => calendar.component.suspend());
        }
    }

    return cleanupCallbacks;
}

async function setupTimers() {
    const elems = Array.from(document.getElementsByClassName("timer"));
    if (elems.length == 0) return;

    const timer = await import('./timer.js');

    for (let i = 0; i < elems.length; i++) {
        timer.default(elems[i]);
    }
}

async function setupTodos() {
    const elems = Array.from(document.getElementsByClassName("todo"));
    if (elems.length == 0) return;

    const todo = await import ('./todo.js');

    for (let i = 0; i < elems.length; i++){
        todo.default(elems[i]);
    }
}

async function setupUnitConverters() {
    const elems = Array.from(
        document.getElementsByClassName("unit-converter")
    );
    if (elems.length == 0) return;

    const converter = await import('./unit-converter.js');

    for (let i = 0; i < elems.length; i++) {
        converter.default(elems[i]);
    }
}

async function setupCalculators() {
    const elems = Array.from(
        document.getElementsByClassName("calculator")
    );
    if (elems.length == 0) return;

    const calculator = await import('./calculator.js');

    for (let i = 0; i < elems.length; i++) {
        calculator.default(elems[i]);
    }
}

function setupTruncatedElementTitles(root = document) {
    const elements = root.querySelectorAll(".text-truncate, .single-line-titles .title, .text-truncate-2-lines, .text-truncate-3-lines");

    if (elements.length == 0) {
        return;
    }

    for (let i = 0; i < elements.length; i++) {
        const element = elements[i];
        if (element.getAttribute("title") === null)
            element.title = element.innerText.trim().replace(/\s+/g, " ");
    }
}

const liveWidgetCleanupCallbacks = new WeakMap();

function registerLiveWidgetCleanup(widgetElement, callback) {
    if (widgetElement === null) {
        return;
    }

    let callbacks = liveWidgetCleanupCallbacks.get(widgetElement);
    if (callbacks === undefined) {
        callbacks = [];
        liveWidgetCleanupCallbacks.set(widgetElement, callbacks);
    }

    callbacks.push(callback);
}

function cleanupLiveWidget(widgetElement) {
    cleanupPopoversWithin(widgetElement);

    const callbacks = liveWidgetCleanupCallbacks.get(widgetElement);
    if (callbacks === undefined) {
        return;
    }

    for (const callback of callbacks) {
        callback();
    }

    liveWidgetCleanupCallbacks.delete(widgetElement);
}

async function initializeContentRoot(root, diagnostics = false) {
    const cleanupCallbacks = [];

    const runStage = (name, callback) => {
        if (diagnostics) {
            return runFrontendDiagnosticStage(name, callback);
        }

        return callback();
    };

    const runAsyncStage = async (name, callback) => {
        if (diagnostics) {
            return runFrontendDiagnosticAsyncStage(name, callback);
        }

        return callback();
    };

    cleanupCallbacks.push(
        ...runStage("presentation", () => setupPresentation(root))
    );
    runStage("popovers", () => setupPopovers(root));

    cleanupCallbacks.push(
        ...await runAsyncStage("calendars", () => setupCalendars(root))
    );
    cleanupCallbacks.push(
        ...runStage("carousels", () => setupCarousels(root))
    );
    cleanupCallbacks.push(
        ...runStage("status_bar_tickers", () => setupStatusBarTickers(root))
    );

    runStage("collapsible_lists", () => setupCollapsibleLists(root));

    cleanupCallbacks.push(
        ...runStage("collapsible_grids", () => setupCollapsibleGrids(root))
    );
    cleanupCallbacks.push(
        ...runStage("masonries", () => setupMasonries(root))
    );

    runStage("lazy_images", () => setupLazyImages(root));

    updateRelativeTimeForElements(
        root.querySelectorAll("[data-dynamic-relative-time]")
    );

    return cleanupCallbacks;
}

async function initializeLiveWidget(widgetElement) {
    const cleanupCallbacks = await initializeContentRoot(widgetElement);

    setupTruncatedElementTitles(widgetElement);

    if (cleanupCallbacks.length > 0) {
        liveWidgetCleanupCallbacks.set(widgetElement, cleanupCallbacks);
    }
}

const liveWidgetUpdatesInFlight = new Set();
const liveWidgetUpdatesPending = new Set();

async function refreshLiveWidget(widgetID) {
    const refreshStarted = performance.now();
    const selector = `[data-widget-id="${CSS.escape(widgetID)}"]`;
    const currentWidget = document.querySelector(selector);

    frontendDiagnostic("widget_refresh_start", { widget: widgetID });

    // The application-wide SSE stream includes widgets from every page.
    // Ignore notifications for widgets that are not present on this page.
    if (currentWidget === null) {
        frontendDiagnostic(
            "widget_refresh_ignored_not_on_page",
            { widget: widgetID }
        );
        return;
    }

    if (liveWidgetUpdatesInFlight.has(widgetID)) {
        liveWidgetUpdatesPending.add(widgetID);
        frontendDiagnostic("widget_refresh_pending", { widget: widgetID });
        return;
    }

    liveWidgetUpdatesInFlight.add(widgetID);

    try {
        do {
            liveWidgetUpdatesPending.delete(widgetID);

            const fetchStarted = performance.now();
            frontendDiagnostic("widget_fetch_start", { widget: widgetID });

            const response = await fetch(
                `${pageData.baseURL}/api/widgets/${encodeURIComponent(widgetID)}/content/`
            );

            frontendDiagnostic("widget_fetch_complete", {
                widget: widgetID,
                status: response.status,
                elapsedMS: performance.now() - fetchStarted,
            });

            if (!response.ok) {
                console.error(
                    `Failed to refresh widget ${widgetID}: ${response.status} ${response.statusText}`
                );
                frontendDiagnostic("widget_refresh_error", {
                    widget: widgetID,
                    status: response.status,
                });
                return;
            }

            const bodyStarted = performance.now();
            const html = await response.text();

            frontendDiagnostic("widget_body_complete", {
                widget: widgetID,
                length: html.length,
                elapsedMS: performance.now() - bodyStarted,
            });

            const template = document.createElement("template");
            const parseStarted = performance.now();

            frontendDiagnostic(
                "widget_parse_start",
                { widget: widgetID },
                true
            );

            template.innerHTML = html.trim();

            frontendDiagnostic("widget_parse_complete", {
                widget: widgetID,
                elapsedMS: performance.now() - parseStarted,
            });

            const replacement = template.content.firstElementChild;
            if (replacement === null || replacement.dataset.widgetId !== widgetID) {
                console.error(`Invalid replacement content for widget ${widgetID}`);
                frontendDiagnostic("widget_replacement_invalid", {
                    widget: widgetID,
                });
                return;
            }

            const liveCurrentWidget = document.querySelector(selector);
            if (liveCurrentWidget === null) {
                frontendDiagnostic("widget_current_missing", {
                    widget: widgetID,
                });
                return;
            }

            const cleanupStarted = performance.now();
            frontendDiagnostic(
                "widget_cleanup_start",
                { widget: widgetID },
                true
            );

            const currentCalendar =
                liveCurrentWidget.querySelector(".calendar");
            const replacementCalendar =
                replacement.querySelector(".calendar");

            if (currentCalendar !== null && replacementCalendar !== null) {
                const displayedMonth =
                    currentCalendar.dataset.calendarDisplayedMonth;
                const selectedDate =
                    currentCalendar.dataset.calendarSelectedDate;

                if (displayedMonth) {
                    replacementCalendar.dataset.calendarDisplayedMonth =
                        displayedMonth;
                }

                if (selectedDate) {
                    replacementCalendar.dataset.calendarSelectedDate =
                        selectedDate;
                }
            }

            cleanupLiveWidget(liveCurrentWidget);

            frontendDiagnostic("widget_cleanup_complete", {
                widget: widgetID,
                elapsedMS: performance.now() - cleanupStarted,
            });

            const replaceStarted = performance.now();
            frontendDiagnostic(
                "widget_replace_start",
                { widget: widgetID },
                true
            );

            liveCurrentWidget.replaceWith(replacement);

            frontendDiagnostic("widget_replace_complete", {
                widget: widgetID,
                elapsedMS: performance.now() - replaceStarted,
            });

            const initializeStarted = performance.now();
            frontendDiagnostic(
                "widget_initialize_start",
                { widget: widgetID },
                true
            );

            await initializeLiveWidget(replacement);

            frontendDiagnostic("widget_initialize_complete", {
                widget: widgetID,
                elapsedMS: performance.now() - initializeStarted,
            });

            if (liveWidgetUpdatesPending.has(widgetID)) {
                frontendDiagnostic("widget_refresh_repeat_pending", {
                    widget: widgetID,
                });
            }
        } while (liveWidgetUpdatesPending.has(widgetID));

        frontendDiagnostic("widget_refresh_complete", {
            widget: widgetID,
            elapsedMS: performance.now() - refreshStarted,
        });
    } catch (error) {
        console.error(`Failed to refresh widget ${widgetID}:`, error);
        frontendDiagnostic("widget_refresh_error", {
            widget: widgetID,
            detail: error instanceof Error ? error.message : String(error),
        }, true);
    } finally {
        liveWidgetUpdatesPending.delete(widgetID);
        liveWidgetUpdatesInFlight.delete(widgetID);
    }
}

function setupLiveWidgetUpdates() {
    if (typeof EventSource === "undefined") {
        frontendDiagnostic("live_updates_unsupported");
        return;
    }

    let events = null;

    function connect() {
        if (events !== null && events.readyState !== EventSource.CLOSED) {
            return;
        }

        frontendDiagnostic("live_updates_connect");

        const liveUpdateURL = new URL(
            `${pageData.baseURL}/api/live-updates`,
            window.location.href
        );

        const widgetIDs = new Set();
        document.querySelectorAll("[data-widget-id]").forEach((widgetElement) => {
            const widgetID = widgetElement.dataset.widgetId;
            if (/^\d+$/.test(widgetID)) {
                widgetIDs.add(widgetID);
            }
        });

        for (const widgetID of widgetIDs) {
            liveUpdateURL.searchParams.append("widget", widgetID);
        }

        events = new EventSource(liveUpdateURL.toString());
        const currentEvents = events;

        currentEvents.addEventListener("open", () => {
            frontendDiagnostic("live_updates_open", {
                state: currentEvents.readyState,
            });
        });

        currentEvents.addEventListener("error", () => {
            frontendDiagnostic("live_updates_error", {
                state: currentEvents.readyState,
            }, true);
        });

        currentEvents.addEventListener("diagnostic", (event) => {
            let command;

            try {
                command = JSON.parse(event.data);
            } catch (error) {
                frontendDiagnosticError(
                    "diagnostic_command_invalid",
                    error,
                    { length: event.data.length }
                );
                return;
            }

            if (
                !Number.isSafeInteger(command.id) ||
                command.id <= 0 ||
                command.command !== "performance_snapshot"
            ) {
                frontendDiagnostic("diagnostic_command_unsupported", {
                    detail: String(command.command ?? "").slice(0, 128),
                }, true);
                return;
            }

            frontendDiagnostic("diagnostic_command_received", {
                detail: `id=${command.id} command=${command.command}`,
            }, true);

            captureFrontendPerformanceSnapshot(
                `command_${command.id}`
            );
        });

        currentEvents.addEventListener("widget", (event) => {
            if (!/^\d+$/.test(event.data)) {
                frontendDiagnostic("live_update_invalid", {
                    length: event.data.length,
                });
                return;
            }

            frontendDiagnostic("live_update_received", {
                widget: event.data,
            });

            refreshLiveWidget(event.data);
        });
    }

    function close(event) {
        frontendDiagnostic("page_hide", {
            detail: `persisted=${event.persisted} visibility=${document.visibilityState}`,
        });

        if (events !== null && events.readyState !== EventSource.CLOSED) {
            frontendDiagnostic("live_updates_close", {
                state: events.readyState,
                detail: `persisted=${event.persisted}`,
            }, true);

            events.close();
        } else {
            frontendDiagnostic("live_updates_close", {
                state: events === null ? EventSource.CLOSED : events.readyState,
                detail: `persisted=${event.persisted} already_closed=true`,
            }, true);
        }
    }

    window.addEventListener("pagehide", close);

    window.addEventListener("pageshow", (event) => {
        if (!event.persisted) {
            return;
        }

        frontendDiagnostic("live_updates_restore", {
            detail: "persisted=true",
        }, true);

        connect();
    });

    connect();
}

async function setupPage() {
    const setupStarted = performance.now();
    frontendDiagnostic("page_setup_start");

    runFrontendDiagnosticStage(
        "theme_picker",
        () => initThemePicker(pageData)
    );

    const pageElement = document.getElementById("page");
    const pageContentElement = document.getElementById("page-content");

    let pageContent;

    try {
        pageContent = await fetchPageContent(pageData);
    } catch (error) {
        frontendDiagnosticError("page_content_load_error", error);

        const pageLoadError = pageElement.querySelector(".page-load-error");
        if (pageLoadError) {
            pageLoadError.hidden = false;
        }

        pageElement.classList.add("content-ready");
        pageElement.setAttribute("aria-busy", "false");
        return;
    }

    const parseStarted = performance.now();
    frontendDiagnostic("page_parse_start", {}, true);
    pageContentElement.innerHTML = pageContent;
    frontendDiagnostic("page_parse_complete", {
        elapsedMS: performance.now() - parseStarted,
    });

    try {
        await initializeContentRoot(document, true);

        runFrontendDiagnosticStage("clocks", () => setupClocks());
        runFrontendDiagnosticStage("footer_micro_clocks", () => setupFooterMicroClocks());
        runFrontendDiagnosticStage("analog_clocks", () => setupAnalogClocks());
        await runFrontendDiagnosticAsyncStage(
            "timers",
            () => setupTimers()
        );
        await runFrontendDiagnosticAsyncStage(
            "todos",
            () => setupTodos()
        );
        await runFrontendDiagnosticAsyncStage(
            "unit_converters",
            () => setupUnitConverters()
        );
        await runFrontendDiagnosticAsyncStage(
            "calculators",
            () => setupCalculators()
        );
        runFrontendDiagnosticStage("search_boxes", () => setupSearchBoxes());
        runFrontendDiagnosticStage("groups", () => setupGroups());
        runFrontendDiagnosticStage(
            "relative_time",
            () => setupDynamicRelativeTime()
        );
        runFrontendDiagnosticStage(
            "live_updates",
            () => setupLiveWidgetUpdates()
        );
    } finally {
        pageElement.classList.add("content-ready");
        pageElement.setAttribute("aria-busy", "false");

        for (let i = 0; i < contentReadyCallbacks.length; i++) {
            contentReadyCallbacks[i]();
        }

        setTimeout(() => {
            setupTruncatedElementTitles();
        }, 50);

        setTimeout(() => {
            document.body.classList.add("page-columns-transitioned");
        }, 300);

        frontendDiagnostic("page_setup_complete", {
            elapsedMS: performance.now() - setupStarted,
        }, true);

        captureFrontendPerformanceSnapshot("page_setup_complete");
    }
}

setupPage();
