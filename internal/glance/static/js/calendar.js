import { directions, easeOutQuint, slideFade } from "./animations.js";
import { setupCollapsibleList } from "./collapsible-list.js";
import { elem, repeat, text } from "./templating.js";

const FULL_MONTH_SLOTS = 7 * 6;
const WEEKDAY_ABBRS = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];
const MONTH_NAMES = [
    "January", "February", "March", "April", "May", "June",
    "July", "August", "September", "October", "November", "December"
];

const leftArrowSvg = `<svg stroke="var(--color-text-base)" fill="none" viewBox="0 0 24 24" stroke-width="1.5" xmlns="http://www.w3.org/2000/svg">
  <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5 8.25 12l7.5-7.5" />
</svg>`;

const rightArrowSvg = `<svg stroke="var(--color-text-base)" fill="none" viewBox="0 0 24 24" stroke-width="1.5" xmlns="http://www.w3.org/2000/svg">
  <path stroke-linecap="round" stroke-linejoin="round" d="m8.25 4.5 7.5 7.5-7.5 7.5" />
</svg>`;

const undoArrowSvg = `<svg stroke="var(--color-text-base)" fill="none" viewBox="0 0 24 24" stroke-width="1.5" xmlns="http://www.w3.org/2000/svg">
  <path stroke-linecap="round" stroke-linejoin="round" d="M9 15 3 9m0 0 6-6M3 9h12a6 6 0 0 1 0 12h-3" />
</svg>`;

const [datesExitLeft, datesExitRight] = directions(
    slideFade,
    { distance: "2rem", duration: 120, offset: 1 },
    "left",
    "right"
);

const [datesEntranceLeft, datesEntranceRight] = directions(
    slideFade,
    { distance: "0.8rem", duration: 500, easing: easeOutQuint },
    "left",
    "right"
);

const undoEntrance = slideFade({
    direction: "left",
    distance: "100%",
    duration: 300
});

export default function(element) {
    const firstDay = Number(element.dataset.firstDayOfWeek ?? 1);
    const collapseAfter = Number(element.dataset.collapseAfter ?? -1);
    const eventIndex = parseEventIndex(element);
    const minimumMonth = parseMonth(element.dataset.calendarMinimumMonth);
    const maximumMonth = parseMonth(element.dataset.calendarMaximumMonth);
    const openLinksInNewTab =
        element.dataset.calendarOpenLinksInNewTab === "true";
    const preservedMonth = parseMonth(
        element.dataset.calendarDisplayedMonth
    );
    const preservedDate = parseDate(
        element.dataset.calendarSelectedDate
    );

    const calendar = Calendar({
        firstDay,
        collapseAfter,
        eventIndex,
        minimumMonth,
        maximumMonth,
        openLinksInNewTab,
        preservedMonth,
        preservedDate
    });

    element.swapWith(calendar);

    return calendar;
}

function Calendar({
    firstDay,
    collapseAfter,
    eventIndex,
    minimumMonth,
    maximumMonth,
    openLinksInNewTab,
    preservedMonth,
    preservedDate
}) {
    let calendar;
    let header;
    let dates;
    let details;
    let advanceTimeTicker;

    let now = new Date();

    let displayedMonth =
        preservedMonth !== null &&
        monthWithinBounds(preservedMonth, minimumMonth, maximumMonth)
            ? preservedMonth
            : monthStart(now);

    let selectedDate =
        preservedDate !== null &&
        monthWithinBounds(
            monthStart(preservedDate),
            minimumMonth,
            maximumMonth
        )
            ? preservedDate
            : dateStart(now);

    const update = (month, selected, animate = true) => {
        displayedMonth = monthStart(month);
        selectedDate = dateStart(selected);

        header.component.update(
            now,
            displayedMonth,
            minimumMonth,
            maximumMonth
        );

        dates.component.update(
            now,
            displayedMonth,
            selectedDate,
            eventIndex,
            animate
        );

        details.component.update(
            selectedDate,
            eventIndex[dateKey(selectedDate)] ?? [],
            openLinksInNewTab
        );

        calendar?.attr(
            "data-calendar-displayed-month",
            monthKey(displayedMonth)
        );
        calendar?.attr(
            "data-calendar-selected-date",
            dateKey(selectedDate)
        );

    };

    const selectDate = (date) => {
        const selectedMonth = monthStart(date);

        if (!datesWithinSameMonth(selectedMonth, displayedMonth)) {
            update(selectedMonth, date);
            return;
        }

        update(displayedMonth, date, false);
    };

    const adjacentMonth = (direction) =>
        new Date(
            displayedMonth.getFullYear(),
            displayedMonth.getMonth() + direction,
            1
        );

    const changeMonth = (direction) => {
        const target = adjacentMonth(direction);

        if (!monthWithinBounds(target, minimumMonth, maximumMonth)) {
            return;
        }

        update(target, target);
    };

    const undoClicked = () => {
        now = new Date();
        update(now, now);
    };

    const autoAdvanceNow = () => {
        advanceTimeTicker = setTimeout(() => {
            const previousNow = now;
            now = new Date();

            const followingToday =
                datesWithinSameDate(selectedDate, previousNow) &&
                datesWithinSameMonth(displayedMonth, previousNow);

            if (followingToday) {
                update(now, now, false);
            } else {
                update(displayedMonth, selectedDate, false);
            }

            autoAdvanceNow();
        }, msTillNextDay());
    };

    calendar = elem().classes("calendar").append(
        header = Header(
            () => changeMonth(-1),
            () => changeMonth(1),
            undoClicked
        ),
        dates = Dates(firstDay, selectDate),
        details = Details(collapseAfter)
    );

    update(displayedMonth, selectedDate, false);
    autoAdvanceNow();

    return calendar.component({
        suspend: () => clearTimeout(advanceTimeTicker)
    });
}

function Header(prevClicked, nextClicked, undoClicked) {
    let month;
    let monthNumber;
    let year;
    let undo;
    let previous;
    let next;

    const button = () => elem("button").classes("calendar-header-button", "glance-icon-button", "glance-button-quiet");

    const monthAndYear = elem().classes("size-h2", "color-highlight").append(
        month = text(),
        " ",
        year = elem("span").classes("size-h3"),
        undo = button()
            .hide()
            .classes("calendar-undo-button")
            .attr("title", "Back to today")
            .attr("aria-label", "Back to today")
            .on("click", undoClicked)
            .html(undoArrowSvg)
    );

    const monthSwitcher = elem()
        .classes("flex", "gap-7", "items-center")
        .append(
            previous = button()
                .attr("title", "Previous month")
                .attr("aria-label", "Previous month")
                .on("click", prevClicked)
                .html(leftArrowSvg),
            monthNumber = elem()
                .classes("color-highlight")
                .styles({ marginTop: "0.1rem" }),
            next = button()
                .attr("title", "Next month")
                .attr("aria-label", "Next month")
                .on("click", nextClicked)
                .html(rightArrowSvg)
        );

    return elem()
        .classes("flex", "justify-between", "items-center")
        .append(monthAndYear, monthSwitcher)
        .component({
            update(now, displayedMonth, minimumMonth, maximumMonth) {
                month.text(MONTH_NAMES[displayedMonth.getMonth()]);
                year.text(displayedMonth.getFullYear());

                const monthValue = displayedMonth.getMonth() + 1;
                monthNumber.text(
                    `${monthValue < 10 ? "0" : ""}${monthValue}`
                );

                if (!datesWithinSameMonth(now, displayedMonth)) {
                    if (undo.isHidden()) {
                        undo.show().animate(undoEntrance);
                    }
                } else {
                    undo.hide();
                }

                setButtonDisabled(
                    previous,
                    minimumMonth !== null &&
                        compareMonths(displayedMonth, minimumMonth) <= 0
                );

                setButtonDisabled(
                    next,
                    maximumMonth !== null &&
                        compareMonths(displayedMonth, maximumMonth) >= 0
                );

                return this;
            }
        });
}

function Dates(firstDay, selectDate) {
    let dates;
    let lastRenderedMonth;

    const updateFullMonth = (
        now,
        displayedMonth,
        selectedDate,
        eventIndex
    ) => {
        const gridStart = calendarGridStart(displayedMonth, firstDay);
        const children = dates.children;

        for (let i = 0; i < FULL_MONTH_SLOTS; i++) {
            const date = addDays(gridStart, i);
            const key = dateKey(date);
            const events = eventIndex[key] ?? [];
            const spillover = !datesWithinSameMonth(date, displayedMonth);
            const today = datesWithinSameDate(date, now);
            const selected = datesWithinSameDate(date, selectedDate);

            const child = children[i];

            child.clearClasses(
                "calendar-spillover-date",
                "calendar-current-date",
                "calendar-selected-date",
                "calendar-date-has-events"
            );

            child.classesIf(spillover, "calendar-spillover-date");
            child.classesIf(today, "calendar-current-date");
            child.classesIf(selected, "calendar-selected-date");
            child.classesIf(events.length > 0, "calendar-date-has-events");

            child
                .attr("data-calendar-date", key)
                .attr("aria-label", calendarDateAriaLabel(date, events.length))
                .attr("aria-pressed", selected ? "true" : "false")
                .html("");

            child.append(
                elem("span")
                    .classes("calendar-date-number")
                    .text(date.getDate())
            );

            if (events.length > 0) {
                child.append(
                    elem("span")
                        .classes("calendar-event-count")
                        .attr(
                            "aria-label",
                            `${events.length} event${events.length === 1 ? "" : "s"}`
                        )
                        .text(events.length)
                );
            }

            child._calendarDate = date;
        }

        lastRenderedMonth = displayedMonth;
    };

    const update = (
        now,
        displayedMonth,
        selectedDate,
        eventIndex,
        animate
    ) => {
        if (
            !animate ||
            lastRenderedMonth === undefined ||
            datesWithinSameMonth(displayedMonth, lastRenderedMonth)
        ) {
            updateFullMonth(
                now,
                displayedMonth,
                selectedDate,
                eventIndex
            );
            return;
        }

        const next = displayedMonth > lastRenderedMonth;

        dates.animateUpdate(
            () => updateFullMonth(
                now,
                displayedMonth,
                selectedDate,
                eventIndex
            ),
            next ? datesExitLeft : datesExitRight,
            next ? datesEntranceRight : datesEntranceLeft
        );
    };

    const dateButtons = repeat(
        FULL_MONTH_SLOTS,
        () => elem("button")
            .classes("calendar-date")
            .attr("type", "button")
            .on("click", function() {
                if (this._calendarDate !== undefined) {
                    selectDate(this._calendarDate);
                }
            })
    );

    return elem().append(
        elem().classes("calendar-dates", "margin-top-15").append(
            ...repeat(
                7,
                (i) => elem()
                    .classes("size-h6", "color-subdue")
                    .text(WEEKDAY_ABBRS[(firstDay + i) % 7])
            )
        ),
        dates = elem()
            .classes("calendar-dates", "margin-top-3")
            .append(...dateButtons)
    ).component({ update });
}

function Details(collapseAfter) {
    let heading;
    let list;

    return elem()
        .classes("calendar-details", "margin-top-15")
        .append(
            heading = elem("h3")
                .classes("calendar-details-heading", "size-h4", "color-highlight"),
            list = elem()
                .classes("calendar-event-list", "list", "collapsible-container")
                .attr("data-collapse-after", collapseAfter)
        )
        .component({
            update(selectedDate, events, openLinksInNewTab) {
                heading.text(formatSelectedDate(selectedDate));
                list.html("");
                setupCollapsibleList(list);

                if (events.length === 0) {
                    list.append(
                        elem("p")
                            .classes("color-subdue")
                            .text("No events.")
                    );
                    return this;
                }

                for (const event of events) {
                    list.append(
                        EventRow(
                            event,
                            selectedDate,
                            openLinksInNewTab
                        )
                    );
                }

                setupCollapsibleList(list);

                return this;
            }
        });
}

function EventRow(event, selectedDate, openLinksInNewTab) {
    const row = elem().classes("calendar-event");

    const timeLabel = elem()
        .classes("calendar-event-time", "size-h6", "color-subdue")
        .text(eventTimeLabel(event, selectedDate));

    let title;

    if (event.url) {
        title = elem("a")
            .classes("calendar-event-title", "color-primary")
            .attr("href", event.url);

        if (openLinksInNewTab) {
            title
                .attr("target", "_blank")
                .attr("rel", "noreferrer");
        }

        title.text(event.title);
    } else {
        title = elem()
            .classes("calendar-event-title")
            .text(event.title);
    }

    const content = elem()
        .classes("calendar-event-content")
        .append(title);

    const metadata = [];

    if (event.source) {
        metadata.push(event.source);
    }

    if (event.location) {
        metadata.push(event.location);
    }

    if (metadata.length > 0) {
        content.append(
            elem()
                .classes("calendar-event-meta", "size-h6", "color-subdue")
                .text(metadata.join(" · "))
        );
    }

    return row.append(timeLabel, content);
}

function eventTimeLabel(event, selectedDate) {
    if (event.allDay) {
        return "All day";
    }

    const start = new Date(event.start);
    const end = new Date(event.end);

    if (!datesWithinSameDate(start, selectedDate)) {
        return "Continues";
    }

    if (start.getTime() === end.getTime()) {
        return formatTime(start);
    }

    return `${formatTime(start)}–${formatTime(end)}`;
}

function formatTime(date) {
    return date.toLocaleTimeString([], {
        hour: "numeric",
        minute: "2-digit"
    });
}

function formatSelectedDate(date) {
    return date.toLocaleDateString([], {
        weekday: "long",
        month: "long",
        day: "numeric",
        year: "numeric"
    });
}

function parseEventIndex(element) {
    const payload = element.querySelector("[data-calendar-events]");

    if (payload === null) {
        return {};
    }

    try {
        return JSON.parse(payload.textContent) ?? {};
    } catch (error) {
        console.error("Failed to parse calendar event data:", error);
        return {};
    }
}

function parseMonth(value) {
    if (!/^\d{4}-\d{2}$/.test(value ?? "")) {
        return null;
    }

    const [year, month] = value.split("-").map(Number);
    return new Date(year, month - 1, 1);
}

function parseDate(value) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value ?? "")) {
        return null;
    }

    const [year, month, day] = value.split("-").map(Number);
    const date = new Date(year, month - 1, day);

    if (
        date.getFullYear() !== year ||
        date.getMonth() !== month - 1 ||
        date.getDate() !== day
    ) {
        return null;
    }

    return date;
}

function monthKey(date) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, "0");

    return `${year}-${month}`;
}

function calendarGridStart(month, firstDay) {
    const first = monthStart(month);
    let offset = (first.getDay() - firstDay + 7) % 7;

    if (offset === 0) {
        offset = 7;
    }

    return addDays(first, -offset);
}

function monthStart(date) {
    return new Date(date.getFullYear(), date.getMonth(), 1);
}

function dateStart(date) {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

function addDays(date, days) {
    return new Date(
        date.getFullYear(),
        date.getMonth(),
        date.getDate() + days
    );
}

function dateKey(date) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, "0");
    const day = String(date.getDate()).padStart(2, "0");

    return `${year}-${month}-${day}`;
}

function calendarDateAriaLabel(date, eventCount) {
    const label = date.toLocaleDateString([], {
        month: "long",
        day: "numeric"
    });

    if (eventCount === 0) {
        return `${label}, no events`;
    }

    return `${label}, ${eventCount} event${eventCount === 1 ? "" : "s"}`;
}

function compareMonths(a, b) {
    return (
        a.getFullYear() * 12 +
        a.getMonth() -
        (b.getFullYear() * 12 + b.getMonth())
    );
}

function monthWithinBounds(month, minimumMonth, maximumMonth) {
    if (
        minimumMonth !== null &&
        compareMonths(month, minimumMonth) < 0
    ) {
        return false;
    }

    if (
        maximumMonth !== null &&
        compareMonths(month, maximumMonth) > 0
    ) {
        return false;
    }

    return true;
}

function datesWithinSameMonth(d1, d2) {
    return (
        d1.getFullYear() === d2.getFullYear() &&
        d1.getMonth() === d2.getMonth()
    );
}

function datesWithinSameDate(d1, d2) {
    return (
        datesWithinSameMonth(d1, d2) &&
        d1.getDate() === d2.getDate()
    );
}

function setButtonDisabled(button, disabled) {
    button
        .tap((element) => element.toggleAttribute("disabled", disabled))
        .attr("aria-disabled", disabled ? "true" : "false");
}

function msTillNextDay(now = new Date()) {
    return 86_400_000 - (
        now.getMilliseconds() +
        now.getSeconds() * 1000 +
        now.getMinutes() * 60_000 +
        now.getHours() * 3_600_000
    );
}
