import { createSudokuGame } from "./sudoku-game.js";

const PLAY_HELP = "Select a cell, then enter 1-9. Backspace, Delete, or 0 clears it.";

function formatTime(seconds) {
    const clamped = Math.max(0, Math.min(5999, seconds));
    return `${String(Math.floor(clamped / 60)).padStart(2, "0")}:${String(clamped % 60).padStart(2, "0")}`;
}

function initializeSudoku(element) {
    const boardElement = element.querySelector("[data-sudoku-board]");
    const difficultyElement = element.querySelector("[data-sudoku-difficulty]");
    const restartElement = element.querySelector("[data-sudoku-restart]");
    const timeElement = element.querySelector("[data-sudoku-time]");
    const statusElement = element.querySelector("[data-sudoku-status]");
    const keypadElement = element.querySelector("[data-sudoku-keypad]");
    const helpElement = element.querySelector("[data-sudoku-help]");
    if (!boardElement || !difficultyElement || !restartElement || !timeElement || !statusElement || !keypadElement || !helpElement)
        return () => {};

    let game;
    let selectedIndex = -1;
    let elapsed = 0;
    let timerID = null;
    let invalidTimerID = null;

    const stopTimer = () => {
        if (timerID !== null) window.clearInterval(timerID);
        timerID = null;
    };

    const clearInvalidFeedback = () => {
        if (invalidTimerID !== null) window.clearTimeout(invalidTimerID);
        invalidTimerID = null;
        for (const cell of boardElement.children) cell.classList.remove("is-invalid");
        helpElement.textContent = PLAY_HELP;
    };

    const showInvalidFeedback = index => {
        clearInvalidFeedback();
        const cell = boardElement.children[index];
        if (!cell) return;
        cell.classList.add("is-invalid");
        helpElement.textContent = "That number does not belong in this cell.";
        invalidTimerID = window.setTimeout(() => {
            cell.classList.remove("is-invalid");
            helpElement.textContent = PLAY_HELP;
            invalidTimerID = null;
        }, 900);
    };

    const startTimer = () => {
        stopTimer();
        timerID = window.setInterval(() => {
            elapsed = Math.min(5999, elapsed + 1);
            timeElement.textContent = formatTime(elapsed);
            if (elapsed >= 5999) stopTimer();
        }, 1000);
    };

    const relatedToSelection = index => {
        if (selectedIndex < 0) return false;
        const row = Math.floor(index / 9);
        const column = index % 9;
        const selectedRow = Math.floor(selectedIndex / 9);
        const selectedColumn = selectedIndex % 9;
        return row === selectedRow || column === selectedColumn ||
            (Math.floor(row / 3) === Math.floor(selectedRow / 3) && Math.floor(column / 3) === Math.floor(selectedColumn / 3));
    };

    const render = () => {
        const selectedValue = selectedIndex >= 0 ? game.values[selectedIndex] : 0;

        for (let index = 0; index < 81; index++) {
            const button = boardElement.children[index];
            const value = game.values[index];
            button.textContent = value === 0 ? "" : String(value);
            button.classList.toggle("is-given", game.givens[index]);
            button.classList.toggle("is-selected", index === selectedIndex);
            button.classList.toggle("is-related", index !== selectedIndex && relatedToSelection(index));
            button.classList.toggle("is-matching", selectedValue !== 0 && value === selectedValue && index !== selectedIndex);
            button.setAttribute("aria-selected", index === selectedIndex ? "true" : "false");
            button.setAttribute("aria-label", `Row ${Math.floor(index / 9) + 1}, column ${index % 9 + 1}${value === 0 ? ", empty" : `, ${value}`}${game.givens[index] ? ", given" : ""}`);
        }

        if (game.status === "won") {
            stopTimer();
            statusElement.textContent = `Puzzle solved in ${formatTime(elapsed)}`;
            helpElement.textContent = `Solved in ${formatTime(elapsed)}.`;
        }
    };

    const select = index => {
        selectedIndex = index;
        render();
    };

    const enterValue = value => {
        if (selectedIndex < 0 || game.givens[selectedIndex] || game.status === "won") return;
        if (game.setValue(selectedIndex, value)) {
            clearInvalidFeedback();
            statusElement.textContent = game.status === "won" ? `Puzzle solved in ${formatTime(elapsed)}` : "Puzzle in progress";
            render();
        } else if (value !== 0) {
            showInvalidFeedback(selectedIndex);
        }
    };

    const reset = (difficulty = difficultyElement.value) => {
        stopTimer();
        clearInvalidFeedback();
        const allowed = ["easy", "medium", "hard"];
        difficultyElement.value = allowed.includes(difficulty) ? difficulty : "easy";
        element.dataset.difficulty = difficultyElement.value;
        game = createSudokuGame({ difficulty: difficultyElement.value });
        selectedIndex = -1;
        elapsed = 0;
        timeElement.textContent = formatTime(elapsed);
        statusElement.textContent = "Ready";
        helpElement.textContent = PLAY_HELP;
        boardElement.replaceChildren();

        const fragment = document.createDocumentFragment();
        for (let index = 0; index < 81; index++) {
            const button = document.createElement("button");
            button.type = "button";
            button.className = "sudoku-cell";
            button.dataset.index = String(index);
            button.setAttribute("role", "gridcell");
            button.setAttribute("aria-rowindex", String(Math.floor(index / 9) + 1));
            button.setAttribute("aria-colindex", String(index % 9 + 1));
            fragment.appendChild(button);
        }
        boardElement.appendChild(fragment);
        render();
        startTimer();
    };

    const onBoardClick = event => {
        const button = event.target.closest(".sudoku-cell");
        if (!button) return;
        select(Number(button.dataset.index));
        button.focus();
    };

    const onBoardKeyDown = event => {
        const button = event.target.closest(".sudoku-cell");
        if (!button) return;
        const index = Number(button.dataset.index);
        if (/^[1-9]$/.test(event.key)) {
            event.preventDefault();
            selectedIndex = index;
            enterValue(Number(event.key));
        } else if (event.key === "0" || event.key === "Backspace" || event.key === "Delete") {
            event.preventDefault();
            selectedIndex = index;
            enterValue(0);
        }
    };

    const onKeypadClick = event => {
        const numberButton = event.target.closest("[data-sudoku-number]");
        if (numberButton) {
            enterValue(Number(numberButton.dataset.sudokuNumber));
            return;
        }
        if (event.target.closest("[data-sudoku-erase]")) enterValue(0);
    };

    const onRestart = () => reset();
    const onDifficulty = () => reset(difficultyElement.value);

    boardElement.addEventListener("click", onBoardClick);
    boardElement.addEventListener("keydown", onBoardKeyDown);
    keypadElement.addEventListener("click", onKeypadClick);
    restartElement.addEventListener("click", onRestart);
    difficultyElement.addEventListener("change", onDifficulty);

    reset(element.dataset.difficulty || "easy");

    return () => {
        stopTimer();
        clearInvalidFeedback();
        boardElement.removeEventListener("click", onBoardClick);
        boardElement.removeEventListener("keydown", onBoardKeyDown);
        keypadElement.removeEventListener("click", onKeypadClick);
        restartElement.removeEventListener("click", onRestart);
        difficultyElement.removeEventListener("change", onDifficulty);
    };
}

export default initializeSudoku;
