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

function setupDynamicRelativeTime() {
    const updateInterval = 60 * 1000;
    let lastUpdateTime = Date.now();
    let timeout = null;

    const updateElementsAndTimestamp = () => {
        updateRelativeTimeForElements(
            document.querySelectorAll("[data-dynamic-relative-time]")
        );
        lastUpdateTime = Date.now();
    };

    const clearScheduledUpdate = () => {
        if (timeout === null) {
            return;
        }

        clearTimeout(timeout);
        timeout = null;
    };

    const scheduleUpdate = (delay = updateInterval) => {
        clearScheduledUpdate();
        timeout = setTimeout(() => {
            updateElementsAndTimestamp();
            scheduleUpdate();
        }, delay);
    };

    const resumeUpdates = () => {
        const delta = Date.now() - lastUpdateTime;

        if (delta >= updateInterval) {
            updateElementsAndTimestamp();
            scheduleUpdate();
            return;
        }

        scheduleUpdate(updateInterval - delta);
    };

    const handleVisibilityChange = () => {
        if (document.hidden) {
            clearScheduledUpdate();
            return;
        }

        resumeUpdates();
    };

    const handlePageHide = () => {
        clearScheduledUpdate();
    };

    const handlePageShow = (event) => {
        if (event.persisted && !document.hidden) {
            resumeUpdates();
        }
    };

    updateElementsAndTimestamp();

    if (document.hidden === undefined) {
        scheduleUpdate();
        return;
    }

    scheduleUpdate();
    document.addEventListener("visibilitychange", handleVisibilityChange);
    window.addEventListener("pagehide", handlePageHide);
    window.addEventListener("pageshow", handlePageShow);
}

export { updateRelativeTimeForElements };
export { setupDynamicRelativeTime };
