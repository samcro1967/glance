import assert from "node:assert/strict";
import test from "node:test";

import { countSolutions, createSudokuGame, generatePuzzle, generateSolution, isValidSolution, SUDOKU_DIFFICULTIES } from "../../internal/glance/static/js/sudoku-game.js";

function seededRandom(seed) {
    let state = seed >>> 0;
    return () => {
        state = (1664525 * state + 1013904223) >>> 0;
        return state / 0x100000000;
    };
}

test("generated solutions satisfy every row, column, and box", () => {
    for (const seed of [1, 7, 42, 99])
        assert.equal(isValidSolution(generateSolution(seededRandom(seed))), true, `seed ${seed}`);
});

for (const difficulty of ["easy", "medium", "hard"]) {
    test(`${difficulty} puzzles preserve givens and have exactly one solution`, () => {
        for (const seed of [3, 17, 61]) {
            const generated = generatePuzzle({ difficulty, random: seededRandom(seed) });
            assert.equal(isValidSolution(generated.solution), true, `seed ${seed} solution`);
            assert.equal(countSolutions(generated.puzzle, 2), 1, `seed ${seed} uniqueness`);
            assert.ok(generated.clues >= SUDOKU_DIFFICULTIES[difficulty], `seed ${seed} clue floor`);
            assert.ok(generated.clues <= SUDOKU_DIFFICULTIES[difficulty] + 4, `seed ${seed} clue range`);
            for (let index = 0; index < 81; index++)
                if (generated.puzzle[index] !== 0) assert.equal(generated.puzzle[index], generated.solution[index]);
        }
    });
}

test("editable cells accept values while givens remain immutable", () => {
    const game = createSudokuGame({ difficulty: "easy", random: seededRandom(25) });
    const given = game.givens.findIndex(Boolean);
    const editable = game.givens.findIndex(value => !value);
    assert.equal(game.setValue(given, 0), false);
    assert.equal(game.values[given], game.puzzle[given]);
    assert.equal(game.setValue(editable, game.solution[editable]), true);
    assert.equal(game.values[editable], game.solution[editable]);
    assert.equal(game.setValue(editable, 0), true);
    assert.equal(game.values[editable], 0);
});

test("incorrect values are rejected without changing the board", () => {
    const game = createSudokuGame({ difficulty: "easy", random: seededRandom(77) });
    const editable = game.givens.findIndex(value => !value);
    const correct = game.solution[editable];
    const incorrect = correct === 9 ? 8 : 9;

    assert.equal(game.values[editable], 0);
    assert.equal(game.setValue(editable, incorrect), false);
    assert.equal(game.values[editable], 0);
    assert.equal(game.setValue(editable, correct), true);
    assert.equal(game.values[editable], correct);
});

test("accepted player values produce a valid completed Sudoku board", () => {
    const game = createSudokuGame({ difficulty: "easy", random: seededRandom(83) });

    for (let index = 0; index < 81; index++) {
        if (!game.givens[index])
            assert.equal(game.setValue(index, game.solution[index]), true);
    }

    assert.equal(isValidSolution(game.values), true);
});

test("entering the complete solution wins", () => {
    const game = createSudokuGame({ difficulty: "easy", random: seededRandom(101) });
    for (let index = 0; index < 81; index++)
        if (!game.givens[index]) game.setValue(index, game.solution[index]);
    assert.equal(game.status, "won");
});
