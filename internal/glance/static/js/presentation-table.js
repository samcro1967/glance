import "./vendor/datatables/dataTables.min.js";
import "./vendor/datatables/dataTables.responsive.min.js";
import { frontendDiagnosticError } from "./diagnostics.js";
import { getPresentationConfig } from "./presentation-config.js";

const initializedTables = new WeakMap();

function tableConfig(table) {
    const name = table.dataset.glanceTable;
    if (!name) {
        return {
            responsive: true,
            sortable: true,
            search: false,
            pagination: false,
            pageSize: 10,
            columns: {}
        };
    }

    const config = getPresentationConfig(table).tables?.[name];
    if (config === undefined) {
        throw new Error(`Unknown Glance table configuration: ${name}`);
    }
    return config;
}

function columnDefinitions(table, config) {
    const definitions = [];
    const headers = Array.from(table.querySelectorAll(":scope > thead > tr > th"));

    headers.forEach((header, index) => {
        const name = header.dataset.column;
        if (!name) return;
        const column = config.columns?.[name];
        if (column === undefined) return;

        const definition = { targets: index };
        if (column.type) definition.type = column.type;
        if (column.priority > 0) definition.responsivePriority = column.priority;
        definitions.push(definition);
    });

    return definitions;
}

function setupTable(table) {
    if (initializedTables.has(table)) return null;

    const config = tableConfig(table);
    const instance = new DataTable(table, {
        responsive: config.responsive !== false,
        ordering: config.sortable !== false,
        searching: config.search === true,
        paging: config.pagination === true,
        pageLength: config.pageSize || 10,
        columnDefs: columnDefinitions(table, config),
        info: false,
        lengthChange: false
    });

    initializedTables.set(table, instance);

    return () => {
        const current = initializedTables.get(table);
        if (current === undefined) return;
        initializedTables.delete(table);
        current.destroy();
    };
}

export function setupPresentationTables(root = document) {
    const cleanupCallbacks = [];
    const tables = [];

    if (root.matches?.("[data-glance-table]")) tables.push(root);
    tables.push(...root.querySelectorAll("[data-glance-table]"));

    for (const table of tables) {
        try {
            const cleanup = setupTable(table);
            if (cleanup !== null) cleanupCallbacks.push(cleanup);
        } catch (error) {
            frontendDiagnosticError("presentation_table_initialize_error", error);
            console.error("Failed to initialize Glance table", error);
        }
    }

    return cleanupCallbacks;
}
