'use strict';

const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const ROOT = path.resolve(__dirname, '../..');

// Only committed, public decoration profiles are accepted. The existing Make
// runtime owns builds, validation, mutual exclusion and cleanup. Never point
// this helper at production configuration or implement another server launcher.
function validateTestConfig(config) {
  if (config === 'test-instance.yml') return config;
  if (!/^testdata\/decorations\/[a-z0-9-]+\.yml$/.test(config) ||
      !fs.statSync(path.join(ROOT, config), { throwIfNoEntry: false })?.isFile()) {
    throw new Error(`Invalid decoration test profile: ${config}`);
  }
  return config;
}

function createRuntimeSelector(run = execFileSync) {
  let active = 'test-instance.yml';
  return config => {
    validateTestConfig(config);
    if (config === active) return;
    run('make', ['--no-print-directory', 'test-instance-start', `TEST_CONFIG=${config}`], {
      cwd: ROOT, stdio: 'inherit',
    });
    active = config;
  };
}

module.exports = { validateTestConfig, createRuntimeSelector };
