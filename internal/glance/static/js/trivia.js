export function setupTrivia(root = document) {
    const cleanups = [];
    for (const trivia of root.querySelectorAll("[data-trivia]")) {
        const answers = [...trivia.querySelectorAll("[data-trivia-answer]")];
        const result = trivia.querySelector("[data-trivia-result]");
        const onClick = (event) => {
            const selected = event.currentTarget;
            const correct = selected.dataset.correct === "true";
            for (const answer of answers) {
                answer.disabled = true;
                if (answer.dataset.correct === "true") answer.classList.add("is-correct");
            }
            if (!correct) selected.classList.add("is-incorrect");
            if (result) result.textContent = correct ? "Correct!" : "Not quite — the correct answer is highlighted.";
        };
        for (const answer of answers) answer.addEventListener("click", onClick);
        cleanups.push(() => { for (const answer of answers) answer.removeEventListener("click", onClick); });
    }
    return cleanups;
}
