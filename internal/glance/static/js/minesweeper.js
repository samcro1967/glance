import { createMinesweeperGame } from "./minesweeper-game.js";

const DIFFICULTIES = {
    beginner: { rows: 9, columns: 9, mines: 10 },
    intermediate: { rows: 16, columns: 16, mines: 40 },
    expert: { rows: 16, columns: 30, mines: 99 },
};

const PLAY_HELP = "Tap or click to reveal. Right-click or long-press to flag.";

const LONG_PRESS_MS = 500;

function formatCounter(value) {
    const clamped = Math.max(-99, Math.min(999, value));
    if (clamped < 0) return `-${String(Math.abs(clamped)).padStart(2, "0")}`;
    return String(clamped).padStart(3, "0");
}

function initializeMinesweeper(element) {
    const boardElement = element.querySelector("[data-minesweeper-board]");
    const difficultyElement = element.querySelector("[data-minesweeper-difficulty]");
    const minesElement = element.querySelector("[data-minesweeper-mines]");
    const timeElement = element.querySelector("[data-minesweeper-time]");
    const restartElement = element.querySelector("[data-minesweeper-restart]");
    const statusElement = element.querySelector("[data-minesweeper-status]");
    const helpElement = element.querySelector("[data-minesweeper-help]");
    if (!boardElement || !difficultyElement || !minesElement || !timeElement || !restartElement || !statusElement || !helpElement)
        return () => {};

    let game;
    let config;
    let elapsed = 0;
    let timerID = null;
    let longPressID = null;
    let longPressTriggered = false;

    const stopTimer = () => {
        if (timerID !== null) window.clearInterval(timerID);
        timerID = null;
    };

    const startTimer = () => {
        if (timerID !== null) return;
        timerID = window.setInterval(() => {
            elapsed = Math.min(999, elapsed + 1);
            timeElement.textContent = formatCounter(elapsed);
            if (elapsed >= 999) stopTimer();
        }, 1000);
    };

    const flaggedCount = () => game.cells.reduce((count, cell) => count + (cell.flagged ? 1 : 0), 0);

    const updateCounters = () => {
        minesElement.textContent = formatCounter(config.mines - flaggedCount());
        timeElement.textContent = formatCounter(elapsed);
    };

    const setStatus = (text) => {
        statusElement.textContent = text;
    };

    const renderCell = (cell) => {
        const button = cell.element;
        button.classList.toggle("is-revealed", cell.revealed);
        button.classList.toggle("is-flagged", cell.flagged);
        const incorrectFlag = game.status === "lost" && cell.flagged && !cell.mine;
        button.classList.toggle("is-mine", cell.revealed && cell.mine);
        button.classList.toggle("is-exploded", cell.exploded);
        button.classList.toggle("is-incorrect-flag", incorrectFlag);
        button.removeAttribute("data-adjacent");

        if (incorrectFlag) {
            button.textContent = "×";
            button.setAttribute("aria-label", `Row ${cell.row + 1}, column ${cell.column + 1}, incorrect flag`);
        } else if (cell.flagged) {
            button.textContent = "⚑";
            button.setAttribute("aria-label", `Row ${cell.row + 1}, column ${cell.column + 1}, flagged mine`);
        } else if (!cell.revealed) {
            button.textContent = "";
            button.setAttribute("aria-label", `Row ${cell.row + 1}, column ${cell.column + 1}, hidden`);
        } else if (cell.mine) {
            button.textContent = "✹";
            button.setAttribute("aria-label", `Row ${cell.row + 1}, column ${cell.column + 1}, mine`);
        } else {
            button.textContent = cell.adjacent === 0 ? "" : String(cell.adjacent);
            if (cell.adjacent > 0) button.dataset.adjacent = String(cell.adjacent);
            button.setAttribute("aria-label", `Row ${cell.row + 1}, column ${cell.column + 1}, ${cell.adjacent} adjacent mines`);
        }
    };

    const renderBoard = () => {
        for (const cell of game.cells) renderCell(cell);
        updateCounters();
        if (game.status === "lost") {
            stopTimer();
            restartElement.textContent = "☹";
            setStatus("Game over");
            helpElement.innerHTML = `<span class="minesweeper-result-key"><span><span class="minesweeper-result-symbol is-flagged">⚑</span> Mine found</span><span><span class="minesweeper-result-symbol is-incorrect">×</span> Incorrect flag</span><span><span class="minesweeper-result-symbol is-mine">✹</span> Missed mine</span><span><span class="minesweeper-result-symbol is-exploded">✹</span> Detonated</span></span>`;
        } else if (game.status === "won") {
            stopTimer();
            restartElement.textContent = "☺";
            setStatus(`You won in ${elapsed} seconds`);
            helpElement.textContent = `You cleared all ${config.mines} mines in ${elapsed} seconds.`;
        }
    };

    const reveal = (index) => {
        const wasReady = game.status === "ready";
        game.reveal(index);
        if (wasReady && game.status !== "ready") {
            startTimer();
            setStatus("Game in progress");
        }
        renderBoard();
    };

    const toggleFlag = (index) => {
        game.toggleFlag(index);
        renderBoard();
    };

    const reset = (difficulty = difficultyElement.value) => {
        stopTimer();
        if (longPressID !== null) window.clearTimeout(longPressID);
        longPressID = null;
        config = DIFFICULTIES[difficulty] || DIFFICULTIES.beginner;
        difficultyElement.value = difficulty in DIFFICULTIES ? difficulty : "beginner";
        element.dataset.difficulty = difficultyElement.value;
        elapsed = 0;
        game = createMinesweeperGame(config);
        boardElement.replaceChildren();
        boardElement.style.setProperty("--minesweeper-columns", config.columns);
        boardElement.setAttribute("aria-rowcount", config.rows);
        boardElement.setAttribute("aria-colcount", config.columns);

        const fragment = document.createDocumentFragment();
        for (let index = 0; index < config.rows * config.columns; index++) {
            const row = Math.floor(index / config.columns);
            const column = index % config.columns;
            const button = document.createElement("button");
            button.type = "button";
            button.className = "minesweeper-cell";
            button.dataset.index = String(index);
            button.setAttribute("role", "gridcell");
            button.setAttribute("aria-rowindex", String(row + 1));
            button.setAttribute("aria-colindex", String(column + 1));
            const cell = game.cells[index];
            cell.element = button;
            renderCell(cell);
            fragment.appendChild(button);
        }
        boardElement.appendChild(fragment);
        restartElement.textContent = "☺";
        setStatus("Ready");
        helpElement.textContent = PLAY_HELP;
        updateCounters();
    };

    const cellFromEvent = (event) => event.target.closest(".minesweeper-cell");
    const onClick = (event) => {
        const button = cellFromEvent(event);
        if (!button || longPressTriggered) {
            longPressTriggered = false;
            return;
        }
        reveal(Number(button.dataset.index));
    };
    const onContextMenu = (event) => {
        const button = cellFromEvent(event);
        if (!button) return;
        event.preventDefault();
        if (longPressTriggered) {
            const suppressLongPressContextMenu = event.pointerType !== "mouse";
            longPressTriggered = false;
            if (suppressLongPressContextMenu) return;
        }
        toggleFlag(Number(button.dataset.index));
    };
    const onPointerDown = (event) => {
        const button = cellFromEvent(event);
        if (!button || event.pointerType === "mouse") return;
        longPressTriggered = false;
        longPressID = window.setTimeout(() => {
            longPressTriggered = true;
            toggleFlag(Number(button.dataset.index));
        }, LONG_PRESS_MS);
    };
    const cancelLongPress = () => {
        if (longPressID !== null) window.clearTimeout(longPressID);
        longPressID = null;
    };
    const onRestart = () => reset();
    const onDifficulty = () => reset(difficultyElement.value);

    boardElement.addEventListener("click", onClick);
    boardElement.addEventListener("contextmenu", onContextMenu);
    boardElement.addEventListener("pointerdown", onPointerDown);
    boardElement.addEventListener("pointerup", cancelLongPress);
    boardElement.addEventListener("pointercancel", cancelLongPress);
    boardElement.addEventListener("pointerleave", cancelLongPress);
    restartElement.addEventListener("click", onRestart);
    difficultyElement.addEventListener("change", onDifficulty);

    reset(element.dataset.difficulty || "beginner");

    return () => {
        stopTimer();
        cancelLongPress();
        boardElement.removeEventListener("click", onClick);
        boardElement.removeEventListener("contextmenu", onContextMenu);
        boardElement.removeEventListener("pointerdown", onPointerDown);
        boardElement.removeEventListener("pointerup", cancelLongPress);
        boardElement.removeEventListener("pointercancel", cancelLongPress);
        boardElement.removeEventListener("pointerleave", cancelLongPress);
        restartElement.removeEventListener("click", onRestart);
        difficultyElement.removeEventListener("change", onDifficulty);
    };
}

export default initializeMinesweeper;
