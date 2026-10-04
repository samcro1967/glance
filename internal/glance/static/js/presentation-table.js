import "./vendor/datatables/dataTables.min.js";
import "./vendor/datatables/dataTables.responsive.min.js";
import { frontendDiagnosticError } from "./diagnostics.js";
import { getPresentationConfig } from "./presentation-config.js";

const initializedTables = new WeakMap();
const transientTableState = new Map();

function dataBoolean(value, fallback) {
    if (value === undefined) return fallback;
    return value !== "false";
}

function tableConfig(table) {
    const name = table.dataset.glanceTable;
    if (!name) {
        return {
            responsive: dataBoolean(table.dataset.glanceTableResponsive, true),
            sortable: dataBoolean(table.dataset.glanceTableSortable, true),
            search: dataBoolean(table.dataset.glanceTableSearch, false),
            pagination: dataBoolean(table.dataset.glanceTablePagination, false),
            pageSize: Number.parseInt(table.dataset.glanceTablePageSize || "10", 10),
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
        const column = name ? config.columns?.[name] : undefined;
        const definition = { targets: index, orderSequence: ["asc", "desc", ""] };
        const type = column?.type || header.dataset.columnType;
        const priority = column?.priority || Number.parseInt(header.dataset.columnPriority || "0", 10);

        if (type) definition.type = type;
        if (priority > 0) definition.responsivePriority = priority;
        if (header.dataset.columnSortable === "false") definition.orderable = false;
        definitions.push(definition);
    });

    return definitions;
}

function setupTable(table) {
    if (initializedTables.has(table)) return null;

    const config = tableConfig(table);
    const stateName = table.dataset.glanceTableState;
    const stateScope = table.closest("[data-glance-table-state-scope]")?.dataset.glanceTableStateScope;
    const stateKey = stateName && stateScope ? `${stateScope}:${stateName}` : stateName;
    const savedState = stateKey ? transientTableState.get(stateKey) : undefined;
    const instance = new DataTable(table, {
        responsive: config.responsive !== false,
        ordering: config.sortable !== false,
        order: savedState?.order || [],
        searching: config.search === true,
        search: { search: savedState?.search || "" },
        paging: config.pagination === true,
        pageLength: config.pageSize || 10,
        columnDefs: columnDefinitions(table, config),
        info: false,
        lengthChange: false,
        language: config.search === true ? {
            search: "",
            searchPlaceholder: table.dataset.glanceTableSearchPlaceholder || "Filter…",
            zeroRecords: "No matching rows"
        } : undefined
    });

    initializedTables.set(table, instance);

    return () => {
        const current = initializedTables.get(table);
        if (current === undefined) return;
        if (stateKey) {
            transientTableState.set(stateKey, {
                order: current.order(),
                search: current.search()
            });
        }
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
