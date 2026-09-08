'use strict';

const http = require('http');

const HOST = '127.0.0.1';
const PORT = 18089;

function sendJson(res, value) {
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(value));
}

const changeDetectionWatches = {
  'glance-release': {
    title: 'Glance Releases',
    url: 'https://github.com/glanceapp/glance/releases',
    last_changed: 1788876000,
    date_created: 1786284000,
    previous_md5: '9f81d7a6e5c4b321'
  },
  'documentation': {
    title: 'Documentation',
    url: 'https://github.com/glanceapp/glance/tree/main/docs',
    last_changed: 1788782400,
    date_created: 1786197600,
    previous_md5: '6a42bc9177ef3301'
  },
  'project-home': {
    title: 'Project Home',
    url: 'https://github.com/glanceapp/glance',
    last_changed: 1788616800,
    date_created: 1786024800,
    previous_md5: '318ef9c044b7d920'
  }
};

const dockerContainers = [
  {
    Names: ['/glance'],
    Image: 'glanceapp/glance:latest',
    State: 'running',
    Status: 'Up 2 hours',
    Labels: {
      'glance.name': 'Glance',
      'glance.id': 'glance',
      'glance.description': 'Dashboard',
      'glance.url': 'https://github.com/glanceapp/glance'
    }
  },
  {
    Names: ['/visual-worker'],
    Image: 'example/worker:latest',
    State: 'running',
    Status: 'Up 1 hour (healthy)',
    Labels: {
      'glance.name': 'Visual Worker',
      'glance.parent': 'glance',
      'glance.description': 'Background worker'
    }
  },
  {
    Names: ['/fixture-api'],
    Image: 'example/api:latest',
    State: 'running',
    Status: 'Up 45 minutes',
    Labels: {
      'glance.name': 'Fixture API',
      'glance.description': 'Deterministic test service'
    }
  },
  {
    Names: ['/paused-service'],
    Image: 'example/service:latest',
    State: 'paused',
    Status: 'Paused',
    Labels: {
      'glance.name': 'Paused Service',
      'glance.description': 'Paused state example'
    }
  }
];

const server = http.createServer((req, res) => {
  const url = new URL(req.url, `http://${HOST}:${PORT}`);

  if (url.pathname === '/visual-test-extension') {
    res.writeHead(200, {
      'Content-Type': 'text/html; charset=utf-8',
      'Widget-Title': 'Extension',
      'Widget-Content-Type': 'html'
    });
    res.end(
      '<div class="glance-card">' +
        '<div class="size-h3 color-highlight">Extension Fixture</div>' +
        '<div class="glance-secondary">Deterministic local extension content</div>' +
        '<div class="margin-top-10">' +
          '<span class="glance-badge">Local</span> ' +
          '<span class="glance-status">Healthy</span>' +
        '</div>' +
      '</div>'
    );
    return;
  }

  if (url.pathname === '/api/v1/watch') {
    const ids = {};
    for (const id of Object.keys(changeDetectionWatches)) ids[id] = {};
    sendJson(res, ids);
    return;
  }

  if (url.pathname.startsWith('/api/v1/watch/')) {
    const id = decodeURIComponent(
      url.pathname.slice('/api/v1/watch/'.length)
    );
    const watch = changeDetectionWatches[id];

    if (!watch) {
      res.writeHead(404);
      res.end('Not found');
      return;
    }

    sendJson(res, watch);
    return;
  }

  if (url.pathname === '/control/stats') {
    sendJson(res, {
      num_dns_queries: 18472,
      num_blocked_filtering: 4318,
      avg_processing_time: 0.021,
      dns_queries: [
        620, 710, 845, 790, 930, 1010,
        1120, 980, 875, 760, 690, 640
      ],
      blocked_filtering: [
        130, 160, 210, 190, 240, 280,
        310, 260, 220, 180, 150, 135
      ],
      top_blocked_domains: [
        { 'ads.example.com': 824 },
        { 'tracker.example.net': 613 },
        { 'metrics.example.org': 401 },
        { 'telemetry.example.com': 287 },
        { 'promotions.example.net': 194 }
      ]
    });
    return;
  }

  if (url.pathname === '/containers/json') {
    sendJson(res, dockerContainers);
    return;
  }

  res.writeHead(404, { 'Content-Type': 'text/plain' });
  res.end('Not found');
});

server.listen(PORT, HOST, () => {
  console.log(`Visual fixture server listening on http://${HOST}:${PORT}`);
});

function shutdown() {
  server.close(() => process.exit(0));
}

process.on('SIGTERM', shutdown);
process.on('SIGINT', shutdown);
