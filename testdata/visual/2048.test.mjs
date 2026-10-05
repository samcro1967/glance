import assert from "node:assert/strict";
import test from "node:test";

import { addRandomTile, create2048Game, hasAvailableMove, moveBoard } from "../../internal/glance/static/js/2048-game.js";

function sequenceRandom(values) {
    let index = 0;
    return () => values[Math.min(index++, values.length - 1)];
}

function row(...values) {
    return [...values, ...Array(12).fill(0)];
}

test("new games start with exactly two tiles", () => {
    const game = create2048Game({ random: sequenceRandom([0, 0, 0.99, 0]) });
    assert.equal(game.board.filter(Boolean).length, 2);
    assert.deepEqual(game.board.filter(Boolean), [2, 2]);
    assert.equal(game.score, 0);
    assert.equal(game.status, "playing");
});

test("random tile spawning uses the selected empty cell and 90/10 value split", () => {
    const board2 = Array(16).fill(0);
    assert.equal(addRandomTile(board2, sequenceRandom([0.5, 0.899999])), true);
    assert.equal(board2.filter(Boolean).length, 1);
    assert.equal(board2[8], 2);

    const board4 = Array(16).fill(0);
    addRandomTile(board4, sequenceRandom([0, 0.9]));
    assert.equal(board4[0], 4);
});

test("left moves compress and merge each tile at most once", () => {
    assert.deepEqual(moveBoard(row(2, 0, 2, 0), "left"), { board: row(4, 0, 0, 0), score: 4, moved: true });
    assert.deepEqual(moveBoard(row(2, 2, 2, 2), "left"), { board: row(4, 4, 0, 0), score: 8, moved: true });
    assert.deepEqual(moveBoard(row(2, 2, 4, 4), "left"), { board: row(4, 8, 0, 0), score: 12, moved: true });
    assert.deepEqual(moveBoard(row(4, 4, 8, 8), "left"), { board: row(8, 16, 0, 0), score: 24, moved: true });
});

test("all four directions preserve directional ordering", () => {
    assert.deepEqual(moveBoard(row(2, 2, 0, 0), "right").board.slice(0, 4), [0, 0, 0, 4]);
    const vertical = [2,0,0,0, 2,0,0,0, 0,0,0,0, 0,0,0,0];
    assert.deepEqual(moveBoard(vertical, "up").board, [4,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0]);
    assert.deepEqual(moveBoard(vertical, "down").board, [0,0,0,0, 0,0,0,0, 0,0,0,0, 4,0,0,0]);
});

test("ineffective moves do not change score or spawn a tile", () => {
    let randomCalls = 0;
    const game = create2048Game({ board: row(2, 4, 8, 16), random: () => { randomCalls++; return 0; } });
    assert.equal(game.move("left"), false);
    assert.deepEqual(game.board, row(2, 4, 8, 16));
    assert.equal(game.score, 0);
    assert.equal(randomCalls, 0);
});

test("effective moves add exactly one tile and accumulate merge score", () => {
    let randomCalls = 0;
    const random = () => { randomCalls++; return 0; };
    const game = create2048Game({ board: row(2, 2, 0, 0), random });
    assert.equal(game.move("left"), true);
    assert.equal(game.score, 4);
    assert.equal(game.board.filter(Boolean).length, 2);
    assert.equal(randomCalls, 2);
});

test("reaching 2048 wins and continue resumes play", () => {
    const game = create2048Game({ board: row(1024, 1024, 0, 0), random: () => 0 });
    assert.equal(game.move("left"), true);
    assert.equal(game.status, "won");
    assert.equal(game.score, 2048);
    assert.equal(game.move("right"), false);
    assert.equal(game.continueGame(), true);
    assert.equal(game.status, "playing");
});

test("game over requires a full board with no adjacent merge", () => {
    const noMoves = [2,4,2,4, 4,2,4,2, 2,4,2,4, 4,2,4,2];
    const mergeAvailable = [2,2,4,8, 16,32,64,128, 256,512,1024,2, 4,8,16,32];
    assert.equal(hasAvailableMove(noMoves), false);
    assert.equal(hasAvailableMove(mergeAvailable), true);
    assert.equal(create2048Game({ board: noMoves }).status, "lost");
    assert.equal(create2048Game({ board: mergeAvailable }).status, "playing");
});

test("invalid directions leave the board unchanged", () => {
    const board = row(2, 2, 0, 0);
    assert.deepEqual(moveBoard(board, "diagonal"), { board, score: 0, moved: false });
});
