export const GAME_2048_SIZE = 4;
export const GAME_2048_TARGET = 2048;

function emptyBoard() {
    return Array(GAME_2048_SIZE * GAME_2048_SIZE).fill(0);
}

function slideLine(line) {
    const values = line.filter(value => value !== 0);
    const result = [];
    let score = 0;

    for (let index = 0; index < values.length; index++) {
        if (index + 1 < values.length && values[index] === values[index + 1]) {
            const merged = values[index] * 2;
            result.push(merged);
            score += merged;
            index++;
        } else {
            result.push(values[index]);
        }
    }

    while (result.length < GAME_2048_SIZE) result.push(0);
    return { line: result, score };
}

function lineIndexes(direction, line) {
    const indexes = [];
    for (let offset = 0; offset < GAME_2048_SIZE; offset++) {
        if (direction === "left") indexes.push(line * GAME_2048_SIZE + offset);
        else if (direction === "right") indexes.push(line * GAME_2048_SIZE + (GAME_2048_SIZE - 1 - offset));
        else if (direction === "up") indexes.push(offset * GAME_2048_SIZE + line);
        else indexes.push((GAME_2048_SIZE - 1 - offset) * GAME_2048_SIZE + line);
    }
    return indexes;
}

export function moveBoard(board, direction) {
    if (!["left", "right", "up", "down"].includes(direction))
        return { board: board.slice(), score: 0, moved: false };

    const next = board.slice();
    let score = 0;
    for (let line = 0; line < GAME_2048_SIZE; line++) {
        const indexes = lineIndexes(direction, line);
        const movedLine = slideLine(indexes.map(index => board[index]));
        score += movedLine.score;
        indexes.forEach((index, offset) => { next[index] = movedLine.line[offset]; });
    }

    return {
        board: next,
        score,
        moved: next.some((value, index) => value !== board[index]),
    };
}

export function hasAvailableMove(board) {
    if (board.some(value => value === 0)) return true;
    for (let row = 0; row < GAME_2048_SIZE; row++) {
        for (let column = 0; column < GAME_2048_SIZE; column++) {
            const index = row * GAME_2048_SIZE + column;
            if (column + 1 < GAME_2048_SIZE && board[index] === board[index + 1]) return true;
            if (row + 1 < GAME_2048_SIZE && board[index] === board[index + GAME_2048_SIZE]) return true;
        }
    }
    return false;
}

export function addRandomTile(board, random = Math.random) {
    const empty = [];
    board.forEach((value, index) => { if (value === 0) empty.push(index); });
    if (empty.length === 0) return false;

    const choice = Math.min(empty.length - 1, Math.floor(random() * empty.length));
    const index = empty[choice];
    board[index] = random() < 0.9 ? 2 : 4;
    return true;
}

export function create2048Game({ random = Math.random, board: initialBoard } = {}) {
    let board = initialBoard ? initialBoard.slice() : emptyBoard();
    let score = 0;
    let status = "playing";
    let keepPlaying = false;

    if (!initialBoard) {
        addRandomTile(board, random);
        addRandomTile(board, random);
    }

    const updateStatus = () => {
        if (!keepPlaying && board.some(value => value >= GAME_2048_TARGET)) status = "won";
        else if (!hasAvailableMove(board)) status = "lost";
        else status = "playing";
    };

    updateStatus();

    const move = direction => {
        if (status === "lost" || (status === "won" && !keepPlaying)) return false;
        const result = moveBoard(board, direction);
        if (!result.moved) {
            updateStatus();
            return false;
        }

        board = result.board;
        score += result.score;
        addRandomTile(board, random);
        updateStatus();
        return true;
    };

    const continueGame = () => {
        if (status !== "won") return false;
        keepPlaying = true;
        updateStatus();
        return true;
    };

    return {
        move,
        continueGame,
        get board() { return board.slice(); },
        get score() { return score; },
        get status() { return status; },
    };
}
