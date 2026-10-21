import { frontendDiagnosticError } from './diagnostics.js';

// Date selection is browser-local and independent of widget refresh. Comparing
// MM-DD strings avoids DST offsets and leap-year day-number inconsistencies.
// Stable per-bulb timing avoids synchronized edge patterns and random state.
// Durations are 3-6 seconds per brightness transition; phases span a full cycle.
export function decorationBulbTiming(index) {
    const seed = Math.imul(index + 1, 0x9e3779b1) >>> 0;
    const duration = 3 + (seed % 3001) / 1000;
    const delay = -((seed >>> 8) % 10000) / 10000 * duration * 2;
    return { duration, delay };
}

export function selectDecoration(config, now = new Date()) {
    if (!config.auto) return config.design;
    const day = `${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
    return config.schedule.find(entry => entry.start <= entry.end
        ? day >= entry.start && day <= entry.end
        : day >= entry.start || day <= entry.end)?.design ?? null;
}

export function nextDecorationCheck(now) {
    const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
    // The bounded check also catches a changed device clock/timezone. Date
    // transitions still happen at midnight, not one minute after midnight.
    return Math.max(1, Math.min(60000, midnight.getTime() - now.getTime()));
}

const controllers = new WeakMap();

export function setupDecorations(root = document) {
    const host = root.querySelector('#decorations');
    const source = root.querySelector('#decoration-config');
    if (!host || !source) return;
    if (controllers.has(host)) return controllers.get(host);
    const config = JSON.parse(source.textContent);
    let timer = null;
    let current = undefined;
    let disposed = false;
    let imageFailureReported = false;
    const slots = [...root.querySelectorAll('[data-decoration-footer]')];

    function render(design) {
        host.replaceChildren();
        slots.forEach(slot => { slot.replaceChildren(); slot.hidden = true; });
        if (!design) return;
        host.classList.toggle('decoration-animated', config.animation);
        if (config.lights) {
            const wire = document.createElement("div");
            wire.className = "decoration-wire";
            host.append(wire);
            let bulbIndex = 0;
            for (const side of ['top', 'right', 'bottom', 'left']) {
                const edge = document.createElement('div');
                edge.className = `decoration-edge decoration-edge-${side}`;
                // A bounded count avoids a resize listener and unbounded DOM.
                const count = side === 'top' || side === 'bottom' ? 24 : 18;
                for (let i = 0; i < count; i++) {
                    const bulb = document.createElement('span');
                    bulb.className = 'decoration-bulb';
                    bulb.style.setProperty('--decoration-color', design.colors[i % design.colors.length]);
                    const timing = decorationBulbTiming(bulbIndex++);
                    bulb.style.setProperty("--decoration-duration", `${timing.duration}s`);
                    bulb.style.setProperty("--decoration-delay", `${timing.delay}s`);
                    edge.append(bulb);
                }
                host.append(edge);
            }
        }
        if (!config.footerIcons) return;
        for (const slot of slots) {
            const side = slot.dataset.decorationFooter;
            if (!design[side]) continue;
            const img = document.createElement('img');
            img.className = `decoration-footer-icon${design[`${side}Invert`] ? ' flat-icon' : ''}`;
            img.alt = '';
            img.draggable = false;
            img.addEventListener('error', () => {
                if (img.parentElement !== slot) return;
                slot.hidden = true;
                if (!imageFailureReported) {
                    imageFailureReported = true;
                    // Do not log resource URLs or custom asset names.
                    frontendDiagnosticError('decoration_image_error', new Error('Decoration artwork could not load'));
                }
            }, { once: true });
            img.src = design[side];
            slot.append(img);
            slot.hidden = false;
        }
    }

    function cancelTimer() {
        if (timer !== null) clearTimeout(timer);
        timer = null;
    }
    function update() {
        cancelTimer();
        if (disposed || !host.isConnected) return;
        const now = new Date();
        const design = selectDecoration(config, now);
        if (design !== current) { render(design); current = design; }
        if (config.auto && !document.hidden) timer = setTimeout(update, nextDecorationCheck(now));
    }
    function visibility() { if (document.hidden) cancelTimer(); else update(); }
    function pagehide() { cancelTimer(); }
    function dispose() {
        disposed = true;
        cancelTimer();
        document.removeEventListener('visibilitychange', visibility);
        window.removeEventListener('pagehide', pagehide);
        window.removeEventListener('pageshow', update);
        controllers.delete(host);
    }
    const controller = { dispose };
    controllers.set(host, controller);
    document.addEventListener('visibilitychange', visibility);
    window.addEventListener('pagehide', pagehide);
    window.addEventListener('pageshow', update);
    try { update(); } catch (error) { dispose(); throw error; }
    return controller;
}
