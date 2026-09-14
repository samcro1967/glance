import { frontendDiagnosticError } from "./diagnostics.js";

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
        // Invalid configured timezones are diagnosed and fall back to the browser local time.
        frontendDiagnosticError("timezone_invalid", e);
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

function setupFooterMicroClocks() {
    const clocks = document.getElementsByClassName("footer-micro-clock");

    for (const clock of clocks) {
        if (clock.dataset.microClockInitialized === "true") {
            continue;
        }

        clock.dataset.microClockInitialized = "true";

        const updateClock = () => {
            if (!clock.isConnected) {
                return;
            }

            const dateElement = clock.querySelector("[data-micro-clock-date]");
            const timeElement = clock.querySelector("[data-micro-clock-time]");
            if (!dateElement || !timeElement) {
                return;
            }

            const now = new Date();
            const dateOptions = {
                month: "short",
                day: "numeric",
            };
            const timeOptions = {
                hour: "2-digit",
                minute: "2-digit",
                hour12: clock.dataset.hourFormat === "12h",
                timeZoneName: "short",
            };

            if (clock.dataset.timezone) {
                dateOptions.timeZone = clock.dataset.timezone;
                timeOptions.timeZone = clock.dataset.timezone;
            }

            dateElement.textContent = new Intl.DateTimeFormat([], dateOptions).format(now);
            timeElement.textContent = new Intl.DateTimeFormat([], timeOptions).format(now);
            setTimeout(updateClock, (60 - now.getSeconds()) * 1000);
        };

        updateClock();
    }
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


export { setupClocks };
export { setupFooterMicroClocks };
export { setupAnalogClocks };
