export function neighbors(index, rows, columns) {
    const row = Math.floor(index / columns);
    const column = index % columns;
    const result = [];
    for (let rowOffset = -1; rowOffset <= 1; rowOffset++) {
        for (let columnOffset = -1; columnOffset <= 1; columnOffset++) {
            if (rowOffset === 0 && columnOffset === 0) continue;
            const nextRow = row + rowOffset;
            const nextColumn = column + columnOffset;
            if (nextRow >= 0 && nextRow < rows && nextColumn >= 0 && nextColumn < columns)
                result.push(nextRow * columns + nextColumn);
        }
    }
    return result;
}

export function createMinesweeperGame({ rows, columns, mines, random = Math.random }) {
    const cells = Array.from({ length: rows * columns }, (_, index) => ({
        index,
        row: Math.floor(index / columns),
        column: index % columns,
        mine: false,
        adjacent: 0,
        revealed: false,
        flagged: false,
        exploded: false,
    }));
    let status = "ready";

    const placeMines = (safeIndex) => {
        const blocked = new Set([safeIndex, ...neighbors(safeIndex, rows, columns)]);
        let candidates = cells.map(cell => cell.index).filter(index => !blocked.has(index));
        if (candidates.length < mines)
            candidates = cells.map(cell => cell.index).filter(index => index !== safeIndex);

        for (let i = candidates.length - 1; i > 0; i--) {
            const j = Math.floor(random() * (i + 1));
            [candidates[i], candidates[j]] = [candidates[j], candidates[i]];
        }
        for (const index of candidates.slice(0, mines)) cells[index].mine = true;
        for (const cell of cells)
            cell.adjacent = neighbors(cell.index, rows, columns).filter(index => cells[index].mine).length;
    };

    const lose = (cell) => {
        cell.exploded = true;
        status = "lost";
        for (const candidate of cells)
            if (candidate.mine) candidate.revealed = true;
    };

    const checkWin = () => {
        if (status === "lost" || cells.some(cell => !cell.mine && !cell.revealed)) return;
        status = "won";
        for (const cell of cells)
            if (cell.mine) cell.flagged = true;
    };

    const revealArea = (index) => {
        const queue = [index];
        const visited = new Set();
        while (queue.length > 0) {
            const currentIndex = queue.shift();
            if (visited.has(currentIndex)) continue;
            visited.add(currentIndex);
            const cell = cells[currentIndex];
            if (cell.flagged || cell.revealed || cell.mine) continue;
            cell.revealed = true;
            if (cell.adjacent === 0) {
                for (const next of neighbors(currentIndex, rows, columns))
                    if (!visited.has(next)) queue.push(next);
            }
        }
    };

    const reveal = (index) => {
        const cell = cells[index];
        if (!cell || status === "won" || status === "lost" || cell.flagged) return;
        if (status === "ready") {
            placeMines(index);
            status = "playing";
        }
        if (cell.revealed) {
            if (cell.adjacent === 0) return;
            const adjacent = neighbors(index, rows, columns);
            const flags = adjacent.filter(next => cells[next].flagged).length;
            if (flags !== cell.adjacent) return;
            for (const next of adjacent) {
                if (cells[next].flagged || cells[next].revealed) continue;
                if (cells[next].mine) {
                    lose(cells[next]);
                    return;
                }
                revealArea(next);
            }
            checkWin();
            return;
        }
        if (cell.mine) {
            lose(cell);
            return;
        }
        revealArea(index);
        checkWin();
    };

    const toggleFlag = (index) => {
        const cell = cells[index];
        if (!cell || status === "won" || status === "lost" || cell.revealed) return;
        cell.flagged = !cell.flagged;
    };

    return {
        rows,
        columns,
        mines,
        cells,
        reveal,
        toggleFlag,
        get status() { return status; },
    };
}
