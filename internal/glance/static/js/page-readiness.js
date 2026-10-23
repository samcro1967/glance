// Page visibility is independent of optional widget enhancements. The controller
// also starts observers registered after reveal, including during live updates.
export function createPageReadiness(page, reportError) {
    let ready = false;
    const callbacks = [];
    const invoke = callback => {
        try {
            callback();
        } catch (error) {
            reportError(error);
        }
    };
    return {
        afterReady(callback) {
            if (ready) invoke(callback);
            else callbacks.push(callback);
        },
        reveal() {
            if (ready) return;
            ready = true;
            page.classList.add('content-ready');
            page.setAttribute('aria-busy', 'false');
            for (const callback of callbacks.splice(0)) invoke(callback);
        },
    };
}

// Independent enhancement families start together. A rejected import or storage
// read is reported without preventing unrelated controls from initializing.
export async function runPageEnhancements(tasks, reportError) {
    return Promise.all(tasks.map(async ([name, initialize]) => {
        try {
            return await initialize();
        } catch (error) {
            reportError(name, error);
            return [];
        }
    }));
}
