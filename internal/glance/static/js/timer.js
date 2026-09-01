import { elem, fragment } from "./templating.js";
import { verticallyReorderable } from "./todo.js";

const trashIconSvg = `<svg fill="currentColor" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <path fill-rule="evenodd" d="M5 3.25V4H2.75a.75.75 0 0 0 0 1.5h.3l.815 8.15A1.5 1.5 0 0 0 5.357 15h5.285a1.5 1.5 0 0 0 1.493-1.35l.815-8.15h.3a.75.75 0 0 0 0-1.5H11v-.75A2.25 2.25 0 0 0 8.75 1h-1.5A2.25 2.25 0 0 0 5 3.25Zm2.25-.75a.75.75 0 0 1 .75-.75h1.5a.75.75 0 0 1 .75.75V4h-3v-.75ZM6.05 6a.75.75 0 0 1 .787.713l.275 5.5a.75.75 0 0 1-1.498.075l-.275-5.5A.75.75 0 0 1 6.05 6Zm3.9 0a.75.75 0 0 1 .712.787l-.275 5.5a.75.75 0 0 1-1.498-.075l.275-5.5a.75.75 0 0 1 .786-.711Z" clip-rule="evenodd" />
</svg>`;

export default function(element) {
    element.swapWith(Timer(element.dataset.timerId, element.dataset.hourFormat));
}

function loadFromLocalStorage(id) {
    try {
        const data = JSON.parse(localStorage.getItem(`timer-${id}`) || "[]");
        return Array.isArray(data) ? data : [];
    } catch {
        return [];
    }
}

function newTimerId() {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function")
        return crypto.randomUUID();

    return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function ensureTimerIds(timers) {
    let changed = false;

    for (const timer of timers) {
        if (!timer.id) {
            timer.id = newTimerId();
            changed = true;
        }
    }

    return changed;
}

function saveToLocalStorage(id, data) {
    localStorage.setItem(`timer-${id}`, JSON.stringify(data));
}

function targetDate(timer) {
    const parts = timer.date.split("-").map(Number);
    const time = timer.time.split(":").map(Number);

    if (parts.length !== 3 || time.length !== 2)
        return null;

    const target = new Date(parts[0], parts[1] - 1, parts[2], time[0], time[1]);
    return Number.isNaN(target.getTime()) ? null : target;
}

function formatTarget(timer, hourFormat) {
    const target = targetDate(timer);
    if (!target) return "";

    const date = target.toLocaleDateString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric"
    });

    const time = target.toLocaleTimeString(undefined, {
        hour: "numeric",
        minute: "2-digit",
        hour12: hourFormat === "12h"
    });

    return `${date} · ${time}`;
}

function formatRemaining(timer, now = new Date()) {
    const target = targetDate(timer);
    if (!target) return "";

    const deltaMilliseconds = target.getTime() - now.getTime();
    const deltaMinutes = deltaMilliseconds < 0
        ? Math.floor(deltaMilliseconds / 60000)
        : Math.ceil(deltaMilliseconds / 60000);
    const past = deltaMinutes < 0;
    const minutes = Math.abs(deltaMinutes);

    if (minutes === 0) return "Now";

    const days = Math.floor(minutes / 1440);
    const hours = Math.floor((minutes % 1440) / 60);
    const remainingMinutes = minutes % 60;
    let value;

    if (days > 0) {
        value = `${days}d${hours > 0 ? ` ${hours}h` : ""}${remainingMinutes > 0 ? ` ${remainingMinutes}m` : ""}`;
    } else if (hours > 0) {
        value = `${hours}h${remainingMinutes > 0 ? ` ${remainingMinutes}m` : ""}`;
    } else {
        value = `${remainingMinutes}m`;
    }

    return past ? `${value} ago` : value;
}

function Timer(id, hourFormat) {
    let timers = loadFromLocalStorage(id);
    if (ensureTimerIds(timers))
        saveToLocalStorage(id, timers);

    let items;
    let reorderable;
    let form;
    let nameInput;
    let dateInput;
    let timeInput;
    let saveButton;
    let editingId = null;
    let isDragging = false;

    const save = () => saveToLocalStorage(id, timers);

    const updateCountdowns = () => {
        const now = new Date();

        for (const item of items.children) {
            const timer = timers.find(timer => timer.id === item.dataset.timerId);
            if (!timer) continue;

            const countdown = item.querySelector("[data-timer-countdown]");
            countdown.textContent = formatRemaining(timer, now);

            const target = targetDate(timer);
            item.classList.toggle("timer-item-expired", target && target < now);
        }
    };

    const updateFormState = () => {
        saveButton.disabled = !nameInput.value.trim() || !dateInput.value || !timeInput.value;
    };

    const hideForm = () => {
        editingId = null;
        form.classList.add("display-none");
        nameInput.value = "";
        dateInput.value = "";
        timeInput.value = "";
        saveButton.textContent = "Add";
        updateFormState();
    };

    const showAddForm = () => {
        editingId = null;
        nameInput.value = "";
        dateInput.value = "";
        timeInput.value = "";
        saveButton.textContent = "Add";
        form.classList.remove("display-none");
        updateFormState();
        nameInput.focus();
    };

    const showEditForm = (timer) => {
        if (isDragging) return;

        editingId = timer.id;
        nameInput.value = timer.title;
        dateInput.value = timer.date;
        timeInput.value = timer.time;
        saveButton.textContent = "Save";
        form.classList.remove("display-none");
        updateFormState();
        nameInput.focus();
        nameInput.select();
    };

    const saveForm = () => {
        const title = nameInput.value.trim();
        const date = dateInput.value;
        const time = timeInput.value;

        if (!title || !date || !time) return;

        if (editingId) {
            const timer = timers.find(timer => timer.id === editingId);
            if (!timer) return;

            timer.title = title;
            timer.date = date;
            timer.time = time;
        } else {
            timers.push({ id: newTimerId(), title, date, time });
        }

        save();
        hideForm();
        renderItems();
    };

    const saveOrder = () => {
        const byId = new Map(timers.map(timer => [timer.id, timer]));
        timers = items.children
            .map(item => byId.get(item.dataset.timerId))
            .filter(Boolean);
        save();
    };

    const onDragStart = (event, item) => {
        isDragging = true;
        reorderable.component.onDragStart(event, item);
    };

    const onDragEnd = () => {
        isDragging = false;
    };

    const renderItems = () => {
        items.replaceChildren();

        timers.forEach(timer => {
            let item;

            item = elem().classes("timer-item").attrs({
                "data-timer-id": timer.id
            }).append(
                elem().classes("timer-item-drag-handle").on("mousedown", event => onDragStart(event, item)),
                elem("button")
                    .classes("timer-item-main", "min-width-0")
                    .attrs({ "aria-label": `Edit ${timer.title}` })
                    .append(
                        elem().classes("timer-item-title", "text-truncate").text(timer.title),
                        elem().classes("timer-item-target", "color-subdue").text(formatTarget(timer, hourFormat))
                    )
                    .on("click", () => showEditForm(timer)),
                elem().classes("timer-item-countdown", "shrink-0").attrs({
                    "data-timer-countdown": ""
                }),
                elem("button")
                    .classes("timer-item-delete", "shrink-0")
                    .attrs({ "aria-label": `Delete ${timer.title}` })
                    .html(trashIconSvg)
                    .on("click", () => {
                        timers = timers.filter(item => item.id !== timer.id);
                        save();
                        if (editingId === timer.id) hideForm();
                        renderItems();
                    })
            );

            items.append(item);
        });

        updateCountdowns();
    };

    items = elem().classes("timer-items");

    form = elem().classes("timer-form", "display-none").append(
        elem("input").classes("timer-form-name").attrs({
            type: "text",
            placeholder: "Timer name",
            spellcheck: "false"
        }).tap(input => nameInput = input).on("input", updateFormState),
        elem("input").classes("timer-form-date").attrs({
            type: "date"
        }).tap(input => dateInput = input).on("input", updateFormState),
        elem("input").classes("timer-form-time").attrs({
            type: "time"
        }).tap(input => timeInput = input).on("input", updateFormState),
        elem().classes("timer-form-actions").append(
            elem("button").classes("timer-form-cancel").text("Cancel").on("click", hideForm),
            elem("button").classes("timer-form-save").text("Add").disable().tap(button => saveButton = button).on("click", saveForm)
        )
    );

    const root = fragment().append(
        elem("button").classes("timer-add").text("+ Add a timer").on("click", showAddForm),
        form,
        reorderable = verticallyReorderable(items, saveOrder, onDragEnd)
    );

    renderItems();

    const scheduleUpdate = () => {
        const now = new Date();
        setTimeout(() => {
            if (!items.isConnected) return;
            updateCountdowns();
            scheduleUpdate();
        }, (60 - now.getSeconds()) * 1000 - now.getMilliseconds());
    };

    scheduleUpdate();
    return root;
}
