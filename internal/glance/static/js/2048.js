import { create2048Game } from "./2048-game.js";

const KEY_DIRECTIONS = {
    ArrowLeft: "left", a: "left", A: "left",
    ArrowRight: "right", d: "right", D: "right",
    ArrowUp: "up", w: "up", W: "up",
    ArrowDown: "down", s: "down", S: "down",
};
const SWIPE_THRESHOLD = 24;

function initialize2048(element) {
    const boardElement = element.querySelector("[data-2048-board]");
    const scoreElement = element.querySelector("[data-2048-score]");
    const restartElement = element.querySelector("[data-2048-restart]");
    const statusElement = element.querySelector("[data-2048-status]");
    const messageElement = element.querySelector("[data-2048-message]");
    const messageTextElement = element.querySelector("[data-2048-message-text]");
    const continueElement = element.querySelector("[data-2048-continue]");
    const tryAgainElement = element.querySelector("[data-2048-try-again]");
    if (!boardElement || !scoreElement || !restartElement || !statusElement || !messageElement || !messageTextElement || !continueElement || !tryAgainElement)
        return () => {};

    let game;
    let touchStart = null;
    const cells = [];

    for (let index = 0; index < 16; index++) {
        const cell = document.createElement("div");
        cell.className = "game-2048-cell";
        cell.setAttribute("role", "gridcell");
        cells.push(cell);
        boardElement.append(cell);
    }

    const render = () => {
        const values = game.board;
        values.forEach((value, index) => {
            const cell = cells[index];
            cell.textContent = value === 0 ? "" : String(value);
            if (value === 0) cell.removeAttribute("data-value");
            else cell.dataset.value = String(value);
            const row = Math.floor(index / 4) + 1;
            const column = index % 4 + 1;
            cell.setAttribute("aria-label", `Row ${row}, column ${column}, ${value || "empty"}`);
        });
        scoreElement.textContent = String(game.score);
        messageElement.hidden = game.status === "playing";
        continueElement.hidden = game.status !== "won";
        tryAgainElement.hidden = game.status !== "lost";
        if (game.status === "won") {
            messageTextElement.textContent = "You reached 2048!";
            statusElement.textContent = "You reached 2048";
        } else if (game.status === "lost") {
            messageTextElement.textContent = "Game over";
            statusElement.textContent = "Game over";
        } else {
            messageTextElement.textContent = "";
            statusElement.textContent = `Score ${game.score}`;
        }
    };

    const reset = () => {
        game = create2048Game();
        render();
        boardElement.focus({ preventScroll: true });
    };

    const move = direction => {
        if (!direction) return;
        game.move(direction);
        render();
    };

    const onKeyDown = event => {
        const direction = KEY_DIRECTIONS[event.key];
        if (!direction) return;
        event.preventDefault();
        move(direction);
    };

    const onTouchStart = event => {
        if (event.touches.length !== 1) return;
        touchStart = { x: event.touches[0].clientX, y: event.touches[0].clientY };
    };

    const onTouchEnd = event => {
        if (!touchStart || event.changedTouches.length !== 1) { touchStart = null; return; }
        const deltaX = event.changedTouches[0].clientX - touchStart.x;
        const deltaY = event.changedTouches[0].clientY - touchStart.y;
        touchStart = null;
        if (Math.max(Math.abs(deltaX), Math.abs(deltaY)) < SWIPE_THRESHOLD) return;
        if (Math.abs(deltaX) > Math.abs(deltaY)) move(deltaX > 0 ? "right" : "left");
        else move(deltaY > 0 ? "down" : "up");
    };

    const onContinue = () => { game.continueGame(); render(); boardElement.focus({ preventScroll: true }); };
    const onTryAgain = () => reset();
    const onRestart = () => reset();

    boardElement.addEventListener("keydown", onKeyDown);
    boardElement.addEventListener("touchstart", onTouchStart, { passive: true });
    boardElement.addEventListener("touchend", onTouchEnd, { passive: true });
    restartElement.addEventListener("click", onRestart);
    continueElement.addEventListener("click", onContinue);
    tryAgainElement.addEventListener("click", onTryAgain);
    reset();

    return () => {
        boardElement.removeEventListener("keydown", onKeyDown);
        boardElement.removeEventListener("touchstart", onTouchStart);
        boardElement.removeEventListener("touchend", onTouchEnd);
        restartElement.removeEventListener("click", onRestart);
        continueElement.removeEventListener("click", onContinue);
        tryAgainElement.removeEventListener("click", onTryAgain);
    };
}

export default initialize2048;
