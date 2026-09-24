function formatTime(milliseconds) {
    const totalSeconds = Math.floor(milliseconds / 1000);
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;

    return `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

export default function(container) {
    const display = container.querySelector("[data-stopwatch-display]");
    const toggleButton = container.querySelector("[data-stopwatch-toggle]");
    const resetButton = container.querySelector("[data-stopwatch-reset]");
    const lapButton = container.querySelector("[data-stopwatch-lap]");
    const laps = container.querySelector("[data-stopwatch-laps]");
    const playIcon = toggleButton.querySelector(".stopwatch-icon-play");
    const pauseIcon = toggleButton.querySelector(".stopwatch-icon-pause");
    const startOnOpen = container.dataset.stopwatchStartOnOpen === "true";

    let startedAt = null;
    let elapsed = 0;
    let running = false;
    let animationFrame = null;
    let lapCount = 0;

    const currentElapsed = () => running
        ? elapsed + performance.now() - startedAt
        : elapsed;

    const renderElapsed = () => {
        display.textContent = formatTime(currentElapsed());
    };

    const renderRunningState = () => {
        playIcon.classList.toggle("display-none", running);
        pauseIcon.classList.toggle("display-none", !running);
        toggleButton.title = running ? "Pause" : elapsed > 0 ? "Resume" : "Start";
        toggleButton.setAttribute("aria-label", `${toggleButton.title} stopwatch`);
    };

    const tick = () => {
        renderElapsed();
        animationFrame = requestAnimationFrame(tick);
    };

    const stopAnimation = () => {
        if (animationFrame !== null) {
            cancelAnimationFrame(animationFrame);
            animationFrame = null;
        }
    };

    const toggle = () => {
        if (running) {
            elapsed = currentElapsed();
            running = false;
            startedAt = null;
            stopAnimation();
            renderElapsed();
        } else {
            running = true;
            startedAt = performance.now();
            animationFrame = requestAnimationFrame(tick);
        }

        renderRunningState();
    };

    const reset = () => {
        running = false;
        stopAnimation();
        startedAt = null;
        elapsed = 0;
        lapCount = 0;
        display.textContent = "0:00:00";
        laps.replaceChildren();
        renderRunningState();
    };

    const recordLap = () => {
        const lapElapsed = currentElapsed();
        if (lapElapsed <= 0) return;

        lapCount++;

        const item = document.createElement("li");
        item.className = "stopwatch-lap";

        const number = document.createElement("span");
        number.className = "stopwatch-lap-number color-subdue";
        number.textContent = `Lap ${lapCount}`;

        const time = document.createElement("span");
        time.className = "stopwatch-lap-time";
        time.textContent = formatTime(lapElapsed);

        item.append(number, time);
        laps.prepend(item);
    };

    toggleButton.addEventListener("click", toggle);
    resetButton.addEventListener("click", reset);
    lapButton.addEventListener("click", recordLap);

    renderRunningState();

    if (startOnOpen) toggle();

    return () => {
        stopAnimation();
        toggleButton.removeEventListener("click", toggle);
        resetButton.removeEventListener("click", reset);
        lapButton.removeEventListener("click", recordLap);
    };
}
