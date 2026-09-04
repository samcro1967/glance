const OPERATORS = {
    "+": { precedence: 1, associativity: "left", fn: (a, b) => a + b },
    "-": { precedence: 1, associativity: "left", fn: (a, b) => a - b },
    "*": { precedence: 2, associativity: "left", fn: (a, b) => a * b },
    "/": {
        precedence: 2,
        associativity: "left",
        fn: (a, b) => {
            if (b === 0)
                throw new Error("divide by zero");

            return a / b;
        },
    },
    "^": {
        precedence: 3,
        associativity: "right",
        fn: (a, b) => Math.pow(a, b),
    },
    root: {
        precedence: 3,
        associativity: "right",
        fn: (a, b) => {
            if (b === 0)
                throw new Error("zeroth root");

            if (a < 0) {
                if (!Number.isInteger(b) || Math.abs(b % 2) !== 1)
                    throw new Error("invalid root");

                return -Math.pow(-a, 1 / b);
            }

            return Math.pow(a, 1 / b);
        },
    },
};

const DISPLAY_OPERATORS = {
    "+": "+",
    "-": "−",
    "*": "×",
    "/": "÷",
    "^": "^",
    root: "ʸ√",
};

function isNumberToken(token) {
    return typeof token === "number";
}

export function evaluateTokens(tokens) {
    const values = [];
    const operators = [];

    const applyOperator = () => {
        const operator = operators.pop();

        if (!OPERATORS[operator] || values.length < 2)
            throw new Error("invalid expression");

        const right = values.pop();
        const left = values.pop();
        const result = OPERATORS[operator].fn(left, right);

        if (!Number.isFinite(result))
            throw new Error("non-finite result");

        values.push(result);
    };

    for (const token of tokens) {
        if (isNumberToken(token)) {
            values.push(token);
            continue;
        }

        if (token === "(") {
            operators.push(token);
            continue;
        }

        if (token === ")") {
            while (
                operators.length > 0 &&
                operators[operators.length - 1] !== "("
            ) {
                applyOperator();
            }

            if (operators.pop() !== "(")
                throw new Error("unmatched parenthesis");

            continue;
        }

        const current = OPERATORS[token];

        if (!current)
            throw new Error("unknown operator");

        while (operators.length > 0) {
            const previousToken = operators[operators.length - 1];

            if (previousToken === "(")
                break;

            const previous = OPERATORS[previousToken];

            if (
                previous.precedence > current.precedence ||
                (
                    previous.precedence === current.precedence &&
                    current.associativity === "left"
                )
            ) {
                applyOperator();
                continue;
            }

            break;
        }

        operators.push(token);
    }

    while (operators.length > 0) {
        if (operators[operators.length - 1] === "(")
            throw new Error("unmatched parenthesis");

        applyOperator();
    }

    if (values.length !== 1)
        throw new Error("invalid expression");

    return values[0];
}

export function formatNumber(value) {
    if (!Number.isFinite(value))
        return "Error";

    if (Object.is(value, -0))
        value = 0;

    const absolute = Math.abs(value);

    if (absolute !== 0 && (absolute >= 1e12 || absolute < 1e-9)) {
        return value
            .toExponential(10)
            .replace(/\.?0+e/, "e")
            .replace("e+", "e");
    }

    return new Intl.NumberFormat(undefined, {
        maximumSignificantDigits: 12,
        useGrouping: true,
    }).format(value);
}

function displayExpression(tokens, entry = "") {
    const parts = tokens.map(token => {
        if (isNumberToken(token))
            return formatNumber(token);

        return DISPLAY_OPERATORS[token] || token;
    });

    if (entry !== "")
        parts.push(entry);

    return parts.join(" ");
}

export function calculatePercent(base, operator, percent) {
    switch (operator) {
        case "+":
        case "-":
            return base * percent / 100;

        case "*":
        case "/":
        default:
            return percent / 100;
    }
}

function initializeCalculator(element) {
    const expressionElement = element.querySelector(
        "[data-calculator-expression]"
    );
    const resultElement = element.querySelector("[data-calculator-result]");

    let tokens = [];
    let entry = "0";
    let entering = false;
    let error = false;
    let justEvaluated = false;
    let repeatOperator = null;
    let repeatOperand = null;

    const render = () => {
        expressionElement.textContent = displayExpression(
            tokens,
            entering ? entry : ""
        );
        resultElement.textContent = error
            ? "Error"
            : formatNumber(Number(entry));
    };

    const reset = () => {
        tokens = [];
        entry = "0";
        entering = false;
        error = false;
        justEvaluated = false;
        repeatOperator = null;
        repeatOperand = null;
        render();
    };

    const showError = () => {
        tokens = [];
        entry = "0";
        entering = false;
        error = true;
        justEvaluated = false;
        repeatOperator = null;
        repeatOperand = null;
        render();
    };

    const currentValue = () => {
        const value = Number(entry);

        if (!Number.isFinite(value))
            throw new Error("invalid value");

        return value;
    };

    const beginFreshIfNeeded = () => {
        if (!justEvaluated)
            return;

        tokens = [];
        entry = "0";
        entering = false;
        justEvaluated = false;
        repeatOperator = null;
        repeatOperand = null;
    };

    const inputDigit = digit => {
        if (error)
            reset();

        beginFreshIfNeeded();

        if (!entering || entry === "0") {
            entry = digit;
            entering = true;
        } else if (entry === "-0") {
            entry = `-${digit}`;
        } else if (entry.replace("-", "").replace(".", "").length < 15) {
            entry += digit;
        }

        render();
    };

    const inputDecimal = () => {
        if (error)
            reset();

        beginFreshIfNeeded();

        if (!entering) {
            entry = "0.";
            entering = true;
        } else if (!entry.includes(".")) {
            entry += ".";
        }

        render();
    };

    const pushEntry = () => {
        if (!entering)
            return false;

        tokens.push(currentValue());
        entering = false;
        return true;
    };

    const inputOperator = operator => {
        if (error)
            return;

        if (justEvaluated) {
            tokens = [currentValue()];
            justEvaluated = false;
        } else {
            pushEntry();
        }

        if (tokens.length === 0)
            tokens.push(currentValue());

        const last = tokens[tokens.length - 1];

        if (OPERATORS[last]) {
            tokens[tokens.length - 1] = operator;
        } else if (last !== "(") {
            tokens.push(operator);
        }

        repeatOperator = null;
        repeatOperand = null;
        render();
    };

    const inputParen = paren => {
        if (error)
            reset();

        beginFreshIfNeeded();

        if (paren === "(") {
            if (entering)
                return;

            const last = tokens[tokens.length - 1];

            if (
                tokens.length === 0 ||
                OPERATORS[last] ||
                last === "("
            ) {
                tokens.push("(");
            }
        } else {
            pushEntry();

            const opens = tokens.filter(token => token === "(").length;
            const closes = tokens.filter(token => token === ")").length;
            const last = tokens[tokens.length - 1];

            if (
                opens > closes &&
                tokens.length > 0 &&
                !OPERATORS[last] &&
                last !== "("
            ) {
                tokens.push(")");
            }
        }

        render();
    };

    const applyPercent = () => {
        if (error)
            return;

        let value = currentValue();

        if (tokens.length >= 2) {
            const operator = tokens[tokens.length - 1];

            if (OPERATORS[operator]) {
                try {
                    const baseTokens = tokens.slice(0, -1);
                    const base = evaluateTokens(baseTokens);
                    value = calculatePercent(base, operator, value);
                } catch {
                    value /= 100;
                }
            } else {
                value /= 100;
            }
        } else {
            value /= 100;
        }

        if (!Number.isFinite(value)) {
            showError();
            return;
        }

        entry = String(value);
        entering = true;
        justEvaluated = false;
        render();
    };

    const applyUnary = operation => {
        if (error)
            return;

        if (operation === "percent") {
            applyPercent();
            return;
        }

        let value = currentValue();

        switch (operation) {
            case "reciprocal":
                if (value === 0) {
                    showError();
                    return;
                }
                value = 1 / value;
                break;

            case "square":
                value *= value;
                break;

            case "square-root":
                if (value < 0) {
                    showError();
                    return;
                }
                value = Math.sqrt(value);
                break;

            case "sign":
                value = -value;
                break;
        }

        if (!Number.isFinite(value)) {
            showError();
            return;
        }

        entry = String(value);
        entering = true;
        justEvaluated = false;
        render();
    };

    const clearEntry = () => {
        if (error) {
            reset();
            return;
        }

        entry = "0";
        entering = false;
        justEvaluated = false;
        repeatOperator = null;
        repeatOperand = null;
        render();
    };

    const backspace = () => {
        if (error) {
            reset();
            return;
        }

        if (!entering)
            return;

        entry = entry.slice(0, -1);

        if (entry === "" || entry === "-") {
            entry = "0";
            entering = false;
        }

        render();
    };

    const equals = () => {
        if (error)
            return;

        if (
            justEvaluated &&
            repeatOperator !== null &&
            repeatOperand !== null
        ) {
            try {
                const left = currentValue();
                const value = OPERATORS[repeatOperator].fn(
                    left,
                    repeatOperand
                );

                if (!Number.isFinite(value))
                    throw new Error("non-finite result");

                expressionElement.textContent =
                    `${formatNumber(left)} ` +
                    `${DISPLAY_OPERATORS[repeatOperator]} ` +
                    `${formatNumber(repeatOperand)} =`;
                resultElement.textContent = formatNumber(value);

                entry = String(value);
            } catch {
                showError();
            }

            return;
        }

        pushEntry();

        if (tokens.length === 0)
            return;

        while (tokens[tokens.length - 1] === "(")
            tokens.pop();

        if (OPERATORS[tokens[tokens.length - 1]])
            tokens.pop();

        if (tokens.length === 0)
            return;

        const expression = displayExpression(tokens);

        try {
            const value = evaluateTokens(tokens);

            if (
                tokens.length >= 3 &&
                isNumberToken(tokens[tokens.length - 1]) &&
                OPERATORS[tokens[tokens.length - 2]]
            ) {
                repeatOperator = tokens[tokens.length - 2];
                repeatOperand = tokens[tokens.length - 1];
            } else {
                repeatOperator = null;
                repeatOperand = null;
            }

            expressionElement.textContent = `${expression} =`;
            resultElement.textContent = formatNumber(value);

            tokens = [];
            entry = String(value);
            entering = false;
            justEvaluated = true;
        } catch {
            showError();
        }
    };

    element.addEventListener("click", event => {
        const button = event.target.closest("button");

        if (!button || !element.contains(button))
            return;

        if (button.dataset.calculatorDigit !== undefined) {
            inputDigit(button.dataset.calculatorDigit);
        } else if (button.dataset.calculatorOperator) {
            inputOperator(button.dataset.calculatorOperator);
        } else if (button.dataset.calculatorParen) {
            inputParen(button.dataset.calculatorParen);
        } else {
            switch (button.dataset.calculatorAction) {
                case "decimal":
                    inputDecimal();
                    break;
                case "equals":
                    equals();
                    break;
                case "clear":
                    reset();
                    break;
                case "clear-entry":
                    clearEntry();
                    break;
                case "backspace":
                    backspace();
                    break;
                case "reciprocal":
                case "square":
                case "square-root":
                case "percent":
                case "sign":
                    applyUnary(button.dataset.calculatorAction);
                    break;
            }
        }

        element.focus({ preventScroll: true });
    });

    element.addEventListener("keydown", event => {
        if (/^[0-9]$/.test(event.key)) {
            inputDigit(event.key);
        } else {
            switch (event.key) {
                case ".":
                    inputDecimal();
                    break;
                case "+":
                case "-":
                case "*":
                case "/":
                case "^":
                    inputOperator(event.key);
                    break;
                case "(":
                case ")":
                    inputParen(event.key);
                    break;
                case "Enter":
                case "=":
                    equals();
                    break;
                case "Backspace":
                    backspace();
                    break;
                case "Delete":
                    clearEntry();
                    break;
                case "Escape":
                    reset();
                    break;
                case "%":
                    applyUnary("percent");
                    break;
                default:
                    return;
            }
        }

        event.preventDefault();
    });

    render();
}

export default function(element) {
    initializeCalculator(element);
}
