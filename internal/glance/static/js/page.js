import { cleanupPopoversWithin, setupPopovers } from './popover.js';
import { setupMasonries } from './masonry.js';
import { throttledDebounce, isElementVisible, openURLInNewTab } from './utils.js';
import { elem, find, findAll } from './templating.js';

const frontendDiagnosticsEnabled = pageData.frontendDiagnostics === true;
const frontendDiagnosticsBuffer = [];
const frontendDiagnosticsSession = frontendDiagnosticsEnabled
    ? (
        typeof crypto !== "undefined" &&
        typeof crypto.randomUUID === "function"
            ? crypto.randomUUID()
            : `${Date.now()}-${Math.random().toString(36).slice(2)}`
    ).slice(0, 64)
    : "";
let frontendDiagnosticsSequence = 0;
let frontendDiagnosticsFlushTimer = null;
let frontendDiagnosticsFlushInProgress = false;
let frontendDiagnosticsImmediateFlushPending = false;

function frontendDiagnostic(event, fields = {}, flush = false) {
    if (!frontendDiagnosticsEnabled) {
        return;
    }

    const diagnostic = {
        event,
        page: pageData.slug || "",
        session: frontendDiagnosticsSession,
        sequence: ++frontendDiagnosticsSequence,
    };

    if (fields.widget !== undefined) {
        diagnostic.widget = String(fields.widget);
    }

    if (fields.detail !== undefined && fields.detail !== "") {
        diagnostic.detail = String(fields.detail).slice(0, 256);
    }

    if (fields.elapsedMS !== undefined) {
        diagnostic.elapsed_ms = Math.max(0, fields.elapsedMS);
    }

    if (fields.status !== undefined) {
        diagnostic.status = fields.status;
    }

    if (fields.length !== undefined) {
        diagnostic.length = Math.max(0, fields.length);
    }

    if (fields.state !== undefined) {
        diagnostic.state = fields.state;
    }

    frontendDiagnosticsBuffer.push(diagnostic);

    if (flush) {
        if (frontendDiagnosticsFlushInProgress) {
            frontendDiagnosticsImmediateFlushPending = true;
        } else {
            flushFrontendDiagnostics();
        }
        return;
    }

    if (frontendDiagnosticsFlushTimer === null) {
        frontendDiagnosticsFlushTimer = setTimeout(
            flushFrontendDiagnostics,
            1000
        );
    }
}

function flushFrontendDiagnostics() {
    if (
        !frontendDiagnosticsEnabled ||
        frontendDiagnosticsFlushInProgress ||
        frontendDiagnosticsBuffer.length === 0
    ) {
        return;
    }

    if (frontendDiagnosticsFlushTimer !== null) {
        clearTimeout(frontendDiagnosticsFlushTimer);
        frontendDiagnosticsFlushTimer = null;
    }

    const events = frontendDiagnosticsBuffer.splice(0, 50);
    frontendDiagnosticsFlushInProgress = true;

    fetch(`${pageData.baseURL}/api/frontend-diagnostics`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify({ events }),
        keepalive: true,
    })
        .catch(() => {
            // Diagnostics must never interfere with normal page behavior.
        })
        .finally(() => {
            frontendDiagnosticsFlushInProgress = false;

            if (frontendDiagnosticsBuffer.length === 0) {
                frontendDiagnosticsImmediateFlushPending = false;
                return;
            }

            if (frontendDiagnosticsImmediateFlushPending) {
                frontendDiagnosticsImmediateFlushPending = false;
                flushFrontendDiagnostics();
                return;
            }

            frontendDiagnosticsFlushTimer = setTimeout(
                flushFrontendDiagnostics,
                1000
            );
        });
}

function frontendDiagnosticErrorDetail(value) {
    if (value instanceof Error) {
        return `${value.name}: ${value.message}`.slice(0, 256);
    }

    return String(value).slice(0, 256);
}

function setupFrontendDiagnosticsLifecycle() {
    if (!frontendDiagnosticsEnabled) {
        return;
    }

    window.addEventListener("error", (event) => {
        frontendDiagnostic("window_error", {
            detail: frontendDiagnosticErrorDetail(
                event.error ?? event.message ?? "unknown error"
            ),
        }, true);
    });

    window.addEventListener("unhandledrejection", (event) => {
        frontendDiagnostic("unhandled_rejection", {
            detail: frontendDiagnosticErrorDetail(event.reason),
        }, true);
    });

    window.addEventListener("pageshow", (event) => {
        frontendDiagnostic("page_show", {
            detail: `persisted=${event.persisted} visibility=${document.visibilityState} online=${navigator.onLine}`,
        }, true);
    });

    document.addEventListener("visibilitychange", () => {
        frontendDiagnostic("visibility_change", {
            detail: `visibility=${document.visibilityState}`,
        });
    });

    window.addEventListener("online", () => {
        frontendDiagnostic("network_online");
    });

    window.addEventListener("offline", () => {
        frontendDiagnostic("network_offline");
    });
}

setupFrontendDiagnosticsLifecycle();

function runFrontendDiagnosticStage(name, callback) {
    if (!frontendDiagnosticsEnabled) {
        return callback();
    }

    const started = performance.now();

    frontendDiagnostic("page_initialize_start", {
        detail: name,
    }, true);

    try {
        const result = callback();

        frontendDiagnostic("page_initialize_complete", {
            detail: name,
            elapsedMS: performance.now() - started,
        });

        return result;
    } catch (error) {
        frontendDiagnostic("page_initialize_error", {
            detail: `${name}: ${frontendDiagnosticErrorDetail(error)}`,
            elapsedMS: performance.now() - started,
        }, true);

        throw error;
    }
}

async function runFrontendDiagnosticAsyncStage(name, callback) {
    if (!frontendDiagnosticsEnabled) {
        return callback();
    }

    const started = performance.now();

    frontendDiagnostic("page_initialize_start", {
        detail: name,
    }, true);

    try {
        const result = await callback();

        frontendDiagnostic("page_initialize_complete", {
            detail: name,
            elapsedMS: performance.now() - started,
        });

        return result;
    } catch (error) {
        frontendDiagnostic("page_initialize_error", {
            detail: `${name}: ${frontendDiagnosticErrorDetail(error)}`,
            elapsedMS: performance.now() - started,
        }, true);

        throw error;
    }
}

async function fetchPageContent(pageData) {
    // TODO: handle non 200 status codes/time outs
    // TODO: add retries
    const fetchStarted = performance.now();
    frontendDiagnostic("page_content_fetch_start");

    const response = await fetch(`${pageData.baseURL}/api/pages/${pageData.slug}/content/`);
    frontendDiagnostic("page_content_fetch_response", {
        status: response.status,
        elapsedMS: performance.now() - fetchStarted,
    });

    const bodyStarted = performance.now();
    const content = await response.text();
    frontendDiagnostic("page_content_fetch_complete", {
        length: content.length,
        elapsedMS: performance.now() - bodyStarted,
    });

    return content;
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

const minuteInSeconds = 60;
const hourInSeconds = minuteInSeconds * 60;
const dayInSeconds = hourInSeconds * 24;
const monthInSeconds = dayInSeconds * 30.4;
const yearInSeconds = dayInSeconds * 365;

function timestampToRelativeTime(timestamp) {
    let delta = Math.round((Date.now() / 1000) - timestamp);
    let prefix = "";

    if (delta < 0) {
        delta = -delta;
        prefix = "in ";
    }

    if (delta < minuteInSeconds) {
        return prefix + "1m";
    }
    if (delta < hourInSeconds) {
        return prefix + Math.floor(delta / minuteInSeconds) + "m";
    }
    if (delta < dayInSeconds) {
        return prefix + Math.floor(delta / hourInSeconds) + "h";
    }
    if (delta < monthInSeconds) {
        return prefix + Math.floor(delta / dayInSeconds) + "d";
    }
    if (delta < yearInSeconds) {
        return prefix + Math.floor(delta / monthInSeconds) + "mo";
    }

    return prefix + Math.floor(delta / yearInSeconds) + "y";
}

function updateRelativeTimeForElements(elements)
{
    for (let i = 0; i < elements.length; i++)
    {
        const element = elements[i];
        const timestamp = element.dataset.dynamicRelativeTime;

        if (timestamp === undefined)
            continue

        element.textContent = timestampToRelativeTime(timestamp);
    }
}

function setupSearchBoxes() {
    const searchWidgets = document.getElementsByClassName("search");

    if (searchWidgets.length == 0) {
        return;
    }

    for (let i = 0; i < searchWidgets.length; i++) {
        const widget = searchWidgets[i];
        const defaultSearchUrl = widget.dataset.defaultSearchUrl;
        const target = widget.dataset.target || "_blank";
        const newTab = widget.dataset.newTab === "true";
        const inputElement = widget.getElementsByClassName("search-input")[0];
        const bangElement = widget.getElementsByClassName("search-bang")[0];
        const bangs = widget.querySelectorAll(".search-bangs > input");
        const bangsMap = {};
        const kbdElement = widget.getElementsByTagName("kbd")[0];
        let currentBang = null;
        let lastQuery = "";

        for (let j = 0; j < bangs.length; j++) {
            const bang = bangs[j];
            bangsMap[bang.dataset.shortcut] = bang;
        }

        const handleKeyDown = (event) => {
            if (event.key == "Escape") {
                inputElement.blur();
                return;
            }

            if (event.key == "Enter") {
                const input = inputElement.value.trim();
                let query;
                let searchUrlTemplate;

                if (currentBang != null) {
                    query = input.slice(currentBang.dataset.shortcut.length + 1);
                    searchUrlTemplate = currentBang.dataset.url;
                } else {
                    query = input;
                    searchUrlTemplate = defaultSearchUrl;
                }
                if (query.length == 0 && currentBang == null) {
                    return;
                }

                const url = searchUrlTemplate.replace("!QUERY!", encodeURIComponent(query));

                if (newTab && !event.ctrlKey || !newTab && event.ctrlKey) {
                    window.open(url, target).focus();
                } else {
                    window.location.href = url;
                }

                lastQuery = query;
                inputElement.value = "";
                changeCurrentBang(null);

                return;
            }

            if (event.key == "ArrowUp" && lastQuery.length > 0) {
                inputElement.value = lastQuery;
                return;
            }
        };

        const changeCurrentBang = (bang) => {
            currentBang = bang;
            bangElement.textContent = bang != null ? bang.dataset.title : "";
        }

        const handleInput = (event) => {
            const value = event.target.value.trim();
            if (value in bangsMap) {
                changeCurrentBang(bangsMap[value]);
                return;
            }

            const words = value.split(" ");
            if (words.length >= 2 && words[0] in bangsMap) {
                changeCurrentBang(bangsMap[words[0]]);
                return;
            }

            changeCurrentBang(null);
        };

        inputElement.addEventListener("focus", () => {
            document.addEventListener("keydown", handleKeyDown);
            document.addEventListener("input", handleInput);
        });
        inputElement.addEventListener("blur", () => {
            document.removeEventListener("keydown", handleKeyDown);
            document.removeEventListener("input", handleInput);
        });

        document.addEventListener("keydown", (event) => {
            if (['INPUT', 'TEXTAREA'].includes(document.activeElement.tagName)) return;
            if (event.code != "KeyS") return;

            inputElement.focus();
            event.preventDefault();
        });

        kbdElement.addEventListener("mousedown", () => {
            requestAnimationFrame(() => inputElement.focus());
        });

        // Handle autofocus for dynamically loaded content
        if (inputElement.hasAttribute("autofocus")) {
            // Use requestAnimationFrame to ensure DOM is fully ready, especially for Firefox
            requestAnimationFrame(() => {
                inputElement.focus();
            });
        }
    }
}

function setupDynamicRelativeTime() {
    const updateInterval = 60 * 1000;
    let lastUpdateTime = Date.now();

    const updateElementsAndTimestamp = () => {
        updateRelativeTimeForElements(
            document.querySelectorAll("[data-dynamic-relative-time]")
        );
        lastUpdateTime = Date.now();
    };

    updateElementsAndTimestamp();

    const scheduleRepeatingUpdate = () => setInterval(updateElementsAndTimestamp, updateInterval);

    if (document.hidden === undefined) {
        scheduleRepeatingUpdate();
        return;
    }

    let timeout = scheduleRepeatingUpdate();

    document.addEventListener("visibilitychange", () => {
        if (document.hidden) {
            clearTimeout(timeout);
            return;
        }

        const delta = Date.now() - lastUpdateTime;

        if (delta >= updateInterval) {
            updateElementsAndTimestamp();
            timeout = scheduleRepeatingUpdate();
            return;
        }

        timeout = setTimeout(() => {
            updateElementsAndTimestamp();
            timeout = scheduleRepeatingUpdate();
        }, updateInterval - delta);
    });
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
                    // TODO: also handle error event
                    image.addEventListener("load", () => {
                        image.classList.add("loaded");
                        setTimeout(() => imageFinishedTransition(image), 400);
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

function attachExpandToggleButton(collapsibleContainer) {
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


function setupCollapsibleLists(root = document) {
    const collapsibleLists = root.querySelectorAll(".list.collapsible-container");

    if (collapsibleLists.length == 0) {
        return;
    }

    for (let i = 0; i < collapsibleLists.length; i++) {
        const list = collapsibleLists[i];

        if (list.dataset.collapseAfter === undefined) {
            continue;
        }

        const collapseAfter = parseInt(list.dataset.collapseAfter);

        if (collapseAfter == -1) {
            continue;
        }

        if (list.children.length <= collapseAfter) {
            continue;
        }

        attachExpandToggleButton(list);

        for (let c = collapseAfter; c < list.children.length; c++) {
            const child = list.children[c];
            child.classList.add("collapsible-item");
            child.style.animationDelay = ((c - collapseAfter) * 20).toString() + "ms";
        }
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

const weekDayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const monthNames = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];

function makeSettableTimeElement(element, hourFormat) {
    const fragment = document.createDocumentFragment();
    const hour = document.createElement('span');
    const minute = document.createElement('span');
    const amPm = document.createElement('span');
    fragment.append(hour, document.createTextNode(':'), minute);

    if (hourFormat == '12h') {
        fragment.append(document.createTextNode(' '), amPm);
    }

    element.append(fragment);

    return (date) => {
        const hours = date.getHours();

        if (hourFormat == '12h') {
            amPm.textContent = hours < 12 ? 'AM' : 'PM';
            hour.textContent = hours % 12 || 12;
        } else {
            hour.textContent = hours < 10 ? '0' + hours : hours;
        }

        const minutes = date.getMinutes();
        minute.textContent = minutes < 10 ? '0' + minutes : minutes;
    };
};

function timeInZone(now, zone) {
    let timeInZone;

    try {
        timeInZone = new Date(now.toLocaleString('en-US', { timeZone: zone }));
    } catch (e) {
        // TODO: indicate to the user that this is an invalid timezone
        console.error(e);
        timeInZone = now
    }

    const diffInMinutes = Math.round((timeInZone.getTime() - now.getTime()) / 1000 / 60);

    return { time: timeInZone, diffInMinutes: diffInMinutes };
}

function zoneDiffText(diffInMinutes) {
    if (diffInMinutes == 0) {
        return "";
    }

    const sign = diffInMinutes < 0 ? "-" : "+";
    const signText = diffInMinutes < 0 ? "behind" : "ahead";

    diffInMinutes = Math.abs(diffInMinutes);

    const hours = Math.floor(diffInMinutes / 60);
    const minutes = diffInMinutes % 60;
    const hourSuffix = hours == 1 ? "" : "s";

    if (minutes == 0) {
        return { text: `${sign}${hours}h`, title: `${hours} hour${hourSuffix} ${signText}` };
    }

    if (hours == 0) {
        return { text: `${sign}${minutes}m`, title: `${minutes} minutes ${signText}` };
    }

    return { text: `${sign}${hours}h~`, title: `${hours} hour${hourSuffix} and ${minutes} minutes ${signText}` };
}

function setupClocks() {
    const clocks = document.getElementsByClassName('clock');

    if (clocks.length == 0) {
        return;
    }

    const updateCallbacks = [];

    for (var i = 0; i < clocks.length; i++) {
        const clock = clocks[i];
        const hourFormat = clock.dataset.hourFormat;
        const localTimeContainer = clock.querySelector('[data-local-time]');
        const localDateElement = localTimeContainer.querySelector('[data-date]');
        const localWeekdayElement = localTimeContainer.querySelector('[data-weekday]');
        const localYearElement = localTimeContainer.querySelector('[data-year]');
        const timeZoneContainers = clock.querySelectorAll('[data-time-in-zone]');

        const setLocalTime = makeSettableTimeElement(
            localTimeContainer.querySelector('[data-time]'),
            hourFormat
        );

        updateCallbacks.push((now) => {
            setLocalTime(now);
            localDateElement.textContent = now.getDate() + ' ' + monthNames[now.getMonth()];
            localWeekdayElement.textContent = weekDayNames[now.getDay()];
            localYearElement.textContent = now.getFullYear();
        });

        for (var z = 0; z < timeZoneContainers.length; z++) {
            const timeZoneContainer = timeZoneContainers[z];
            const diffElement = timeZoneContainer.querySelector('[data-time-diff]');

            const setZoneTime = makeSettableTimeElement(
                timeZoneContainer.querySelector('[data-time]'),
                hourFormat
            );

            updateCallbacks.push((now) => {
                const { time, diffInMinutes } = timeInZone(now, timeZoneContainer.dataset.timeInZone);
                setZoneTime(time);
                const { text, title } = zoneDiffText(diffInMinutes);
                diffElement.textContent = text;
                diffElement.title = title;
            });
        }
    }

    const updateClocks = () => {
        const now = new Date();

        for (var i = 0; i < updateCallbacks.length; i++)
            updateCallbacks[i](now);

        setTimeout(updateClocks, (60 - now.getSeconds()) * 1000);
    };

    updateClocks();
}

function setupAnalogClocks() {
    const clocks = document.getElementsByClassName('analog-clock');

    if (clocks.length == 0) {
        return;
    }

    const updateCallbacks = [];

    function createAnalogClockUpdater(faceContainer) {
        const face = faceContainer.querySelector('.analog-clock-face');
        const hourHand = face.querySelector('.analog-clock-hour-hand');
        const minuteHand = face.querySelector('.analog-clock-minute-hand');
        const secondHand = face.querySelector('.analog-clock-second-hand');
        const amPmElement = face.querySelector('[data-am-pm]');
        const dateElement = face.querySelector('[data-date]');
        const timezone = faceContainer.dataset.timeInZone;

        return (now) => {
            let date = now;

            if (timezone) {
                date = timeInZone(now, timezone).time;
            }

            const seconds = date.getSeconds();
            const minutes = date.getMinutes();
            const hours = date.getHours();

            const hourRotation = ((hours % 12) * 30) + (minutes * 0.5) - 90;
            const minuteRotation = (minutes * 6) + (seconds * 0.1) - 90;
            const secondRotation = (seconds * 6) - 90;

            hourHand.style.transform = `rotate(${hourRotation}deg)`;
            minuteHand.style.transform = `rotate(${minuteRotation}deg)`;
            secondHand.style.transform = `rotate(${secondRotation}deg)`;

            if (amPmElement) {
                amPmElement.textContent = hours < 12 ? 'AM' : 'PM';
            }

            if (dateElement) {
                dateElement.textContent = `${date.getDate()} ${monthNames[date.getMonth()].slice(0, 3)}`;
            }
        };
    }

    for (var i = 0; i < clocks.length; i++) {
        const faceContainers = clocks[i].querySelectorAll('[data-analog-clock-face]');

        for (var z = 0; z < faceContainers.length; z++) {
            updateCallbacks.push(createAnalogClockUpdater(faceContainers[z]));
        }
    }

    const updateAnalogClocks = () => {
        const now = new Date();

        for (var i = 0; i < updateCallbacks.length; i++) {
            updateCallbacks[i](now);
        }

        setTimeout(updateAnalogClocks, 1000 - now.getMilliseconds());
    };

    updateAnalogClocks();
}

async function setupCalendars() {
    const elems = document.getElementsByClassName("calendar");
    if (elems.length == 0) return;

    // TODO: implement prefetching, currently loads as a nasty waterfall of requests
    const calendar = await import ('./calendar.js');

    for (let i = 0; i < elems.length; i++)
        calendar.default(elems[i]);
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

async function changeTheme(key, onChanged) {
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
        document.documentElement.setAttribute("data-theme", key);
        document.documentElement.setAttribute("data-scheme", response.headers.get("X-Scheme"));
        typeof onChanged == "function" && onChanged();
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

        throw error;
    }
}

function initThemePicker() {
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
            changeTheme(themeKey, function() {
                isLoading = false;
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
            });
        });
    })
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

function initializeLiveWidget(widgetElement) {
    const cleanupCallbacks = [];

    cleanupCallbacks.push(...setupCarousels(widgetElement));
    cleanupCallbacks.push(...setupCollapsibleGrids(widgetElement));
    cleanupCallbacks.push(...setupMasonries(widgetElement));

    setupPopovers(widgetElement);
    setupCollapsibleLists(widgetElement);
    setupLazyImages(widgetElement);
    setupTruncatedElementTitles(widgetElement);

    updateRelativeTimeForElements(
        widgetElement.querySelectorAll("[data-dynamic-relative-time]")
    );

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

            initializeLiveWidget(replacement);

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

        events = new EventSource(`${pageData.baseURL}/api/live-updates`);
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
        () => initThemePicker()
    );

    const pageElement = document.getElementById("page");
    const pageContentElement = document.getElementById("page-content");
    const pageContent = await fetchPageContent(pageData);

    const parseStarted = performance.now();
    frontendDiagnostic("page_parse_start", {}, true);
    pageContentElement.innerHTML = pageContent;
    frontendDiagnostic("page_parse_complete", {
        elapsedMS: performance.now() - parseStarted,
    });

    try {
        runFrontendDiagnosticStage("popovers", () => setupPopovers());
        runFrontendDiagnosticStage("clocks", () => setupClocks());
        runFrontendDiagnosticStage("analog_clocks", () => setupAnalogClocks());
        await runFrontendDiagnosticAsyncStage(
            "calendars",
            () => setupCalendars()
        );
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
        runFrontendDiagnosticStage("carousels", () => setupCarousels());
        runFrontendDiagnosticStage("search_boxes", () => setupSearchBoxes());
        runFrontendDiagnosticStage(
            "collapsible_lists",
            () => setupCollapsibleLists()
        );
        runFrontendDiagnosticStage(
            "collapsible_grids",
            () => setupCollapsibleGrids()
        );
        runFrontendDiagnosticStage("groups", () => setupGroups());
        runFrontendDiagnosticStage("masonries", () => setupMasonries());
        runFrontendDiagnosticStage(
            "relative_time",
            () => setupDynamicRelativeTime()
        );
        runFrontendDiagnosticStage("lazy_images", () => setupLazyImages());
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
    }
}

setupPage();
