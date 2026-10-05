const SIZE = 9;
const CELL_COUNT = SIZE * SIZE;
const BOX_SIZE = 3;

export const SUDOKU_DIFFICULTIES = {
    easy: 40,
    medium: 32,
    hard: 26,
};

function shuffled(values, random) {
    const result = [...values];
    for (let index = result.length - 1; index > 0; index--) {
        const swap = Math.floor(random() * (index + 1));
        [result[index], result[swap]] = [result[swap], result[index]];
    }
    return result;
}

function pattern(row, column) {
    return (BOX_SIZE * (row % BOX_SIZE) + Math.floor(row / BOX_SIZE) + column) % SIZE;
}

export function generateSolution(random = Math.random) {
    const bands = shuffled([0, 1, 2], random);
    const stacks = shuffled([0, 1, 2], random);
    const rows = bands.flatMap(band => shuffled([0, 1, 2], random).map(row => band * BOX_SIZE + row));
    const columns = stacks.flatMap(stack => shuffled([0, 1, 2], random).map(column => stack * BOX_SIZE + column));
    const numbers = shuffled([1, 2, 3, 4, 5, 6, 7, 8, 9], random);
    return rows.flatMap(row => columns.map(column => numbers[pattern(row, column)]));
}

function candidates(board, index) {
    if (board[index] !== 0) return [];
    const row = Math.floor(index / SIZE);
    const column = index % SIZE;
    const boxRow = Math.floor(row / BOX_SIZE) * BOX_SIZE;
    const boxColumn = Math.floor(column / BOX_SIZE) * BOX_SIZE;
    const used = new Set();

    for (let offset = 0; offset < SIZE; offset++) {
        used.add(board[row * SIZE + offset]);
        used.add(board[offset * SIZE + column]);
    }
    for (let rowOffset = 0; rowOffset < BOX_SIZE; rowOffset++)
        for (let columnOffset = 0; columnOffset < BOX_SIZE; columnOffset++)
            used.add(board[(boxRow + rowOffset) * SIZE + boxColumn + columnOffset]);

    return [1, 2, 3, 4, 5, 6, 7, 8, 9].filter(number => !used.has(number));
}

export function countSolutions(board, limit = 2) {
    const working = [...board];
    let solutions = 0;

    const solve = () => {
        if (solutions >= limit) return;
        let bestIndex = -1;
        let bestCandidates = null;
        for (let index = 0; index < CELL_COUNT; index++) {
            if (working[index] !== 0) continue;
            const options = candidates(working, index);
            if (options.length === 0) return;
            if (bestCandidates === null || options.length < bestCandidates.length) {
                bestIndex = index;
                bestCandidates = options;
                if (options.length === 1) break;
            }
        }
        if (bestIndex === -1) {
            solutions++;
            return;
        }
        for (const number of bestCandidates) {
            working[bestIndex] = number;
            solve();
            working[bestIndex] = 0;
            if (solutions >= limit) return;
        }
    };

    solve();
    return solutions;
}

export function isValidSolution(board) {
    if (!Array.isArray(board) || board.length !== CELL_COUNT) return false;
    const validGroup = values => values.length === SIZE && new Set(values).size === SIZE && values.every(value => value >= 1 && value <= 9);
    for (let row = 0; row < SIZE; row++)
        if (!validGroup(board.slice(row * SIZE, row * SIZE + SIZE))) return false;
    for (let column = 0; column < SIZE; column++) {
        const values = [];
        for (let row = 0; row < SIZE; row++) values.push(board[row * SIZE + column]);
        if (!validGroup(values)) return false;
    }
    for (let boxRow = 0; boxRow < BOX_SIZE; boxRow++) {
        for (let boxColumn = 0; boxColumn < BOX_SIZE; boxColumn++) {
            const values = [];
            for (let row = 0; row < BOX_SIZE; row++)
                for (let column = 0; column < BOX_SIZE; column++)
                    values.push(board[(boxRow * BOX_SIZE + row) * SIZE + boxColumn * BOX_SIZE + column]);
            if (!validGroup(values)) return false;
        }
    }
    return true;
}

export function generatePuzzle({ difficulty = "easy", random = Math.random } = {}) {
    const targetClues = SUDOKU_DIFFICULTIES[difficulty] || SUDOKU_DIFFICULTIES.easy;
    let best = null;

    for (let attempt = 0; attempt < 8; attempt++) {
        const solution = generateSolution(random);
        const puzzle = [...solution];
        let clues = CELL_COUNT;
        for (const index of shuffled(Array.from({ length: CELL_COUNT }, (_, value) => value), random)) {
            if (clues <= targetClues) break;
            const previous = puzzle[index];
            puzzle[index] = 0;
            if (countSolutions(puzzle, 2) !== 1) {
                puzzle[index] = previous;
            } else {
                clues--;
            }
        }
        if (best === null || clues < best.clues) best = { puzzle, solution, clues };
        if (clues <= targetClues) return { puzzle, solution, clues, difficulty };
    }

    return { ...best, difficulty };
}

export function createSudokuGame({ difficulty = "easy", random = Math.random } = {}) {
    const generated = generatePuzzle({ difficulty, random });
    const givens = generated.puzzle.map(value => value !== 0);
    const values = [...generated.puzzle];
    let status = "playing";

    const setValue = (index, value) => {
        if (status === "won" || index < 0 || index >= CELL_COUNT || givens[index]) return false;
        if (!Number.isInteger(value) || value < 0 || value > 9) return false;
        if (value !== 0 && value !== generated.solution[index]) return false;

        values[index] = value;
        if (values.every((candidate, candidateIndex) => candidate === generated.solution[candidateIndex])) status = "won";
        return true;
    };

    return {
        difficulty,
        puzzle: [...generated.puzzle],
        solution: [...generated.solution],
        givens,
        values,
        clues: generated.clues,
        setValue,
        get status() { return status; },
    };
}
