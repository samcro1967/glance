import assert from "node:assert/strict";
import test from "node:test";

import { createMinesweeperGame, neighbors } from "../../internal/glance/static/js/minesweeper-game.js";

function seededRandom(seed) {
    let state = seed >>> 0;
    return () => {
        state = (1664525 * state + 1013904223) >>> 0;
        return state / 0x100000000;
    };
}

function independentNeighbors(index, rows, columns) {
    const row = Math.floor(index / columns);
    const column = index % columns;
    const result = [];
    for (let candidate = 0; candidate < rows * columns; candidate++) {
        if (candidate === index) continue;
        const candidateRow = Math.floor(candidate / columns);
        const candidateColumn = candidate % columns;
        if (Math.abs(candidateRow - row) <= 1 && Math.abs(candidateColumn - column) <= 1)
            result.push(candidate);
    }
    return result;
}

function assertBoardAccurate(game) {
    assert.equal(game.cells.filter(cell => cell.mine).length, game.mines);
    for (const cell of game.cells) {
        const expected = independentNeighbors(cell.index, game.rows, game.columns)
            .filter(index => game.cells[index].mine).length;
        assert.equal(cell.adjacent, expected, `cell ${cell.index} adjacency`);
    }
}

test("neighbor topology is correct at corners, edges, and interior cells", () => {
    assert.deepEqual(neighbors(0, 3, 3).sort((a, b) => a - b), [1, 3, 4]);
    assert.deepEqual(neighbors(1, 3, 3).sort((a, b) => a - b), [0, 2, 3, 4, 5]);
    assert.deepEqual(neighbors(4, 3, 3).sort((a, b) => a - b), [0, 1, 2, 3, 5, 6, 7, 8]);
});

for (const [name, config] of Object.entries({
    beginner: { rows: 9, columns: 9, mines: 10 },
    intermediate: { rows: 16, columns: 16, mines: 40 },
    expert: { rows: 16, columns: 30, mines: 99 },
})) {
    test(`${name} boards have exact mines, safe first neighborhood, and accurate numbers`, () => {
        for (const [seed, firstIndex] of [[1, 0], [7, Math.floor(config.rows * config.columns / 2)], [42, config.rows * config.columns - 1]]) {
            const game = createMinesweeperGame({ ...config, random: seededRandom(seed) });
            game.reveal(firstIndex);
            assert.notEqual(game.status, "lost");
            assert.equal(game.cells[firstIndex].mine, false);
            for (const index of independentNeighbors(firstIndex, config.rows, config.columns))
                assert.equal(game.cells[index].mine, false, `seed ${seed} first-click neighbor ${index}`);
            assertBoardAccurate(game);
        }
    });
}

test("flags do not alter the mine map or adjacency values", () => {
    const game = createMinesweeperGame({ rows: 9, columns: 9, mines: 10, random: seededRandom(11) });
    game.reveal(40);
    const before = game.cells.map(cell => [cell.mine, cell.adjacent]);
    const hidden = game.cells.find(cell => !cell.revealed);
    game.toggleFlag(hidden.index);
    game.toggleFlag(hidden.index);
    assert.deepEqual(game.cells.map(cell => [cell.mine, cell.adjacent]), before);
});

test("zero-cell flood reveal never reveals a mine and exposes its numbered boundary", () => {
    const game = createMinesweeperGame({ rows: 9, columns: 9, mines: 10, random: seededRandom(21) });
    game.reveal(40);
    for (const cell of game.cells.filter(cell => cell.revealed)) assert.equal(cell.mine, false);
    for (const cell of game.cells.filter(cell => cell.revealed && cell.adjacent === 0)) {
        for (const index of independentNeighbors(cell.index, game.rows, game.columns)) {
            const neighbor = game.cells[index];
            assert.equal(neighbor.mine, false);
            assert.equal(neighbor.revealed, true);
        }
    }
});

test("correct chording reveals unflagged safe neighbors without changing board math", () => {
    const game = createMinesweeperGame({ rows: 9, columns: 9, mines: 10, random: seededRandom(31) });
    game.reveal(40);
    const target = game.cells.find(cell => cell.revealed && cell.adjacent > 0 &&
        independentNeighbors(cell.index, game.rows, game.columns).some(index => !game.cells[index].revealed));
    assert.ok(target);
    const adjacent = independentNeighbors(target.index, game.rows, game.columns);
    for (const index of adjacent) if (game.cells[index].mine) game.toggleFlag(index);
    game.reveal(target.index);
    assert.notEqual(game.status, "lost");
    for (const index of adjacent)
        if (!game.cells[index].mine) assert.equal(game.cells[index].revealed, true);
    assertBoardAccurate(game);
});

test("incorrect chord flags can expose an unflagged mine and lose", () => {
    const game = createMinesweeperGame({ rows: 9, columns: 9, mines: 10, random: seededRandom(41) });
    game.reveal(40);
    const target = game.cells.find(cell => {
        if (!cell.revealed || cell.adjacent === 0) return false;
        const adjacent = independentNeighbors(cell.index, game.rows, game.columns);
        return adjacent.some(index => game.cells[index].mine) &&
            adjacent.some(index => !game.cells[index].mine && !game.cells[index].revealed);
    });
    assert.ok(target);
    const adjacent = independentNeighbors(target.index, game.rows, game.columns);
    const mines = adjacent.filter(index => game.cells[index].mine);
    const safeHidden = adjacent.filter(index => !game.cells[index].mine && !game.cells[index].revealed);
    game.toggleFlag(safeHidden[0]);
    for (const index of mines.slice(1)) game.toggleFlag(index);
    assert.equal(adjacent.filter(index => game.cells[index].flagged).length, target.adjacent);
    game.reveal(target.index);
    assert.equal(game.status, "lost");
    assert.equal(game.cells.filter(cell => cell.exploded).length, 1);
    assertBoardAccurate(game);
});

test("revealing every safe cell wins and flags every mine", () => {
    const game = createMinesweeperGame({ rows: 9, columns: 9, mines: 10, random: seededRandom(51) });
    game.reveal(40);
    for (const cell of game.cells) {
        if (!cell.mine && !cell.revealed) game.reveal(cell.index);
        if (game.status === "won") break;
    }
    assert.equal(game.status, "won");
    assert.equal(game.cells.filter(cell => cell.mine && cell.flagged).length, 10);
    assertBoardAccurate(game);
});
