'use strict';

const fs = require('fs');
const http = require('http');
const path = require('path');

const HOST = '127.0.0.1';
const PORT = 18089;

const monitoringExampleNames = [
  'argus',
  'beszel',
  'coraza',
  'docker-updates',
  'drone',
  'gotify',
  'healthchecks',
  'internet-speed',
  'portainer',
  'radarr',
  'restic',
  'sabnzbd',
  'sonarr',
  'tdarr',
  'uptime-kuma',
];

const monitoringExamples = Object.fromEntries(
  monitoringExampleNames.map((name) => [
    name,
    JSON.parse(
      fs.readFileSync(
        path.join(
          __dirname,
          `../../docs/examples/custom-api/monitoring/integrations/${name}/example.json`
        ),
        'utf8'
      )
    ),
  ])
);

const alertmanagerExample = JSON.parse(
  fs.readFileSync(
    path.join(__dirname, '../../docs/examples/custom-api/alertmanager/example.json'),
    'utf8'
  )
);

function sendJson(res, value) {
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(value));
}

const changeDetectionWatches = {
  'glance-release': {
    title: 'Glance Releases',
    url: 'https://github.com/samcro1967/glance/releases',
    last_changed: 1788876000,
    date_created: 1786284000,
    previous_md5: '9f81d7a6e5c4b321'
  },
  'documentation': {
    title: 'Documentation',
    url: 'https://github.com/samcro1967/glance/tree/main/docs',
    last_changed: 1788782400,
    date_created: 1786197600,
    previous_md5: '6a42bc9177ef3301'
  },
  'project-home': {
    title: 'Project Home',
    url: 'https://github.com/samcro1967/glance',
    last_changed: 1788616800,
    date_created: 1786024800,
    previous_md5: '318ef9c044b7d920'
  }
};

const dockerContainers = [
  {
    Names: ['/glance'],
    Image: 'ghcr.io/samcro1967/glance:latest',
    State: 'running',
    Status: 'Up 2 hours',
    Labels: {
      'glance.name': 'Glance',
      'glance.id': 'glance',
      'com.docker.compose.project': 'glance-stack',
      'glance.description': 'Dashboard',
      'glance.url': 'https://github.com/samcro1967/glance'
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
      'com.docker.compose.project': 'worker-stack',
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
      'com.docker.compose.project': 'fixture-services',
      'glance.description': 'Deterministic test service'
    }
  },
  {
    Names: ['/fixture-web'],
    Image: 'example/web:latest',
    State: 'running',
    Status: 'Up 30 minutes',
    Labels: {
      'glance.name': 'Fixture Web',
      'com.docker.compose.project': 'fixture-services',
      'glance.description': 'Frontend test service',
      'glance.url': 'https://example.com/'
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
  },
  {
    Names: ['/failed-service'],
    Image: 'example/service:latest',
    State: 'exited',
    Status: 'Exited (1) 5 minutes ago',
    Labels: {
      'glance.name': 'Failed Service',
      'glance.description': 'Critical state visual fixture'
    }
  }
];

const server = http.createServer((req, res) => {
  const url = new URL(req.url, `http://${HOST}:${PORT}`);

  if (url.pathname === '/micro-custom-api') {
    sendJson(res, {
      items: [
        {
          line1: 'Fixture API',
          line2: 'Healthy',
          url: 'https://example.com/'
        },
        {
          line1: '12 ms latency'
        }
      ]
    });
    return;
  }

  if (url.pathname === '/micro-rss.xml') {
    res.writeHead(200, { 'Content-Type': 'application/rss+xml; charset=utf-8' });
    res.end(
      '<?xml version="1.0" encoding="UTF-8"?>' +
      '<rss version="2.0"><channel>' +
      '<title>Glance Fixture Feed</title>' +
      '<link>https://example.com/</link>' +
      '<description>Deterministic micro-widget RSS fixture</description>' +
      '<item><title>Micro widgets share one registry</title><link>https://example.com/micro-registry</link><pubDate>Sun, 05 Oct 2026 12:00:00 GMT</pubDate></item>' +
      '<item><title>Footer and Status Bar use the same sources</title><link>https://example.com/shared-sources</link><pubDate>Sun, 05 Oct 2026 11:00:00 GMT</pubDate></item>' +
      '</channel></rss>'
    );
    return;
  }

  if (url.pathname === '/custom-api-presentation') {
    sendJson(res, {
      full_name: 'samcro1967/glance',
      description: 'Deterministic Custom API presentation fixture',
      stargazers_count: 1967,
      services: [
        { name: 'Dashboard', status: 'Healthy', latency: 12 },
        { name: 'Fixture API', status: 'Healthy', latency: 7 }
      ],
      chart: {
        labels: ['Mon', 'Tue', 'Wed', 'Thu'],
        series: [
          { label: 'CPU', values: [42, 55, 48, 61] }
        ]
      }
    });
    return;
  }

  if (url.pathname === '/visual-test-extension-presentation') {
    res.writeHead(200, {
      'Content-Type': 'application/json; charset=utf-8',
      'Widget-Title': 'Native Extension',
      'Widget-Content-Type': 'presentation-v1'
    });
    res.end(JSON.stringify({
      blocks: [
        { type: 'metrics', items: [{ label: 'REQUESTS', value: '12,402' }, { label: 'LATENCY', value: '42 ms' }] },
        { type: 'status', text: 'Healthy', variant: 'positive' },
        { type: 'progress', label: 'Storage', value: 67, text: '67%' },
        { type: 'table', columns: [{ key: 'service', label: 'Service' }, { key: 'latency', label: 'Latency', type: 'number', priority: 1 }], rows: [{ service: 'Dashboard', latency: 12 }, { service: 'Fixture API', latency: 7 }] },
        { type: 'chart', 'chart-type': 'line', data: { labels: ['Mon', 'Tue', 'Wed', 'Thu'], series: [{ label: 'CPU', values: [42, 55, 48, 61] }] } }
      ]
    }));
    return;
  }

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

  if (url.pathname === '/api/v1/query_range') {
    if (req.method !== 'POST') {
      res.writeHead(405, { 'Content-Type': 'text/plain' });
      res.end('Method not allowed');
      return;
    }

    const now = Math.floor(Date.now() / 1000);
    const values = [
      42, 46, 44, 51, 57, 54, 63, 68,
      64, 72, 78, 74, 82, 88, 84, 91,
      87, 94, 89, 96, 92, 99, 95, 102
    ].map((value, index, all) => [
      now - (all.length - 1 - index) * 3600,
      String(value)
    ]);

    sendJson(res, {
      status: 'success',
      data: {
        resultType: 'matrix',
        result: [
          {
            metric: { __name__: 'visual_fixture_requests_per_second' },
            values
          }
        ]
      }
    });
    return;
  }

  if (url.pathname === '/api/v2/torrents/info') {
    if (req.headers.authorization !== 'Bearer visual-fixture-token') {
      res.writeHead(401, { 'Content-Type': 'text/plain' });
      res.end('Unauthorized');
      return;
    }

    sendJson(res, [
      { name: 'Ubuntu Server 26.04', state: 'downloading', progress: 0.64, downloaded: 2748779069, size: 4294967296, eta: 732 },
      { name: 'Glance Documentation Archive', state: 'stalledUP', progress: 1, downloaded: 734003200, size: 734003200, eta: 8640000 },
      { name: 'Media Backup', state: 'pausedDL', progress: 0.31, downloaded: 3328599654, size: 10737418240, eta: 8640000 }
    ]);
    return;
  }

  if (url.pathname === '/api/v3/calendar') {
    if (req.headers['x-api-key'] !== 'visual-fixture-arr-token') {
      res.writeHead(401, { 'Content-Type': 'text/plain' });
      res.end('Unauthorized');
      return;
    }

    sendJson(res, [
      { id: 1, title: 'The Long Way Home', year: 2026, overview: 'A deterministic Radarr fixture for visual validation.', monitored: true, hasFile: false, inCinemas: '2026-06-12T00:00:00Z', digitalRelease: '2026-09-27T00:00:00Z', physicalRelease: '2026-10-06T00:00:00Z', images: [] },
      { id: 2, title: 'Signal Lost', year: 2025, overview: 'Already available in the library.', monitored: true, hasFile: true, digitalRelease: '2026-09-29T00:00:00Z', images: [] },
      { id: 3, title: 'Northbound', year: 2026, overview: 'An unmonitored release used to exercise normalized state.', monitored: false, hasFile: false, digitalRelease: '2026-10-02T00:00:00Z', images: [] }
    ]);
    return;
  }

  if (url.pathname === '/api/v1/discover/trending') {
    if (req.headers['x-api-key'] !== 'visual-fixture-seerr-token') {
      res.writeHead(401, { 'Content-Type': 'text/plain' });
      res.end('Unauthorized');
      return;
    }

    sendJson(res, {
      page: 1,
      totalPages: 1,
      totalResults: 3,
      results: [
        { id: 101, mediaType: 'movie', title: 'Northbound', overview: 'A deterministic Seerr movie used for visual validation.', posterPath: '', releaseDate: '2026-10-02' },
        { id: 102, mediaType: 'tv', name: 'Signal Lost', overview: 'A deterministic Seerr television fixture.', posterPath: '', firstAirDate: '2025-09-29' },
        { id: 103, mediaType: 'movie', title: 'The Long Way Home', overview: 'Discovery content normalized through the native Seerr widget.', posterPath: '', releaseDate: '2026-06-12' }
      ]
    });
    return;
  }

  if (url.pathname === '/library/recentlyAdded') {
    if (req.headers['x-plex-token'] !== 'visual-fixture-media-token') { res.writeHead(401); res.end('Unauthorized'); return; }
    sendJson(res, { MediaContainer: { Metadata: [
      { ratingKey: '201', type: 'movie', title: 'Northbound', year: 2026, summary: 'Newest deterministic library addition.', addedAt: 1790899200, duration: 7140000, thumb: '' },
      { ratingKey: '202', type: 'episode', title: 'The Return', grandparentTitle: 'Signal Lost', parentIndex: 2, index: 4, summary: 'A deterministic television episode.', addedAt: 1790812800, duration: 3120000, thumb: '' },
      { ratingKey: '203', type: 'movie', title: 'The Long Way Home', year: 2025, summary: 'An older deterministic library addition.', addedAt: 1790726400, duration: 6480000, thumb: '' }
    ] } });
    return;
  }

  if (url.pathname === '/status/sessions') {
    if (req.headers['x-plex-token'] !== 'visual-fixture-media-token') { res.writeHead(401); res.end('Unauthorized'); return; }
    sendJson(res, { MediaContainer: { Metadata: [
      { type: 'episode', title: 'The Return', grandparentTitle: 'Signal Lost', parentTitle: 'Season 2', parentIndex: 2, index: 4, duration: 3120000, viewOffset: 780000, User: { title: 'alex' }, Player: { title: 'Living Room', product: 'Plex Web', state: 'playing' }, Media: [{ Part: [{ decision: 'directplay' }] }] },
      { type: 'movie', title: 'Northbound', year: 2026, duration: 7140000, viewOffset: 3570000, User: { title: 'sam' }, Player: { title: 'Bedroom TV', product: 'Plex', state: 'paused' }, Media: [{ Part: [{ decision: 'directplay' }] }] }
    ] } });
    return;
  }

  if (url.pathname === '/status/sessions/history/all') {
    if (req.headers['x-plex-token'] !== 'visual-fixture-media-token') { res.writeHead(401); res.end('Unauthorized'); return; }
    sendJson(res, { MediaContainer: { Metadata: [
      { ratingKey: '301', type: 'movie', title: 'Northbound', year: 2026, summary: 'Recently watched deterministic movie.', viewedAt: 1790899200, duration: 7140000, thumb: '' },
      { ratingKey: '302', type: 'episode', title: 'The Return', grandparentTitle: 'Signal Lost', parentIndex: 2, index: 4, summary: 'Recently watched deterministic episode.', viewedAt: 1790812800, duration: 3120000, thumb: '' },
      { ratingKey: '303', type: 'movie', title: 'The Long Way Home', year: 2025, summary: 'Older deterministic playback.', viewedAt: 1790726400, duration: 6480000, thumb: '' }
    ] } });
    return;
  }

  if (url.pathname === '/containers/json') {
    sendJson(res, dockerContainers);
    return;
  }

  if (url.pathname === '/monitor-critical') {
    res.writeHead(503, { 'Content-Type': 'text/plain' });
    res.end('Service Unavailable');
    return;
  }

  if (url.pathname === '/examples/custom-api/alertmanager') {
    sendJson(res, alertmanagerExample);
    return;
  }

  const monitoringExamplePrefix = '/examples/custom-api/monitoring/';
  if (url.pathname.startsWith(monitoringExamplePrefix)) {
    const name = url.pathname.slice(monitoringExamplePrefix.length);
    const example = monitoringExamples[name];

    if (example !== undefined) {
      sendJson(res, example);
      return;
    }
  }

  if (url.pathname === '/daily-discovery/wiktionary') {
    res.writeHead(200, { 'Content-Type': 'application/atom+xml; charset=utf-8' });
    res.end('<?xml version="1.0" encoding="UTF-8"?>' +
      '<feed xmlns="http://www.w3.org/2005/Atom">' +
      '<entry><title>Word of the day for 2026-10-02</title><updated>2026-10-02T00:00:00Z</updated>' +
      '<link rel="alternate" href="https://en.wiktionary.org/wiki/serendipity" />' +
      '<summary type="html">&lt;div&gt;&lt;span id="WOTD-rss-title"&gt;serendipity&lt;/span&gt;' +
      '&lt;div id="WOTD-rss-description"&gt;&lt;ol&gt;&lt;li&gt;An unexpected discovery that brings a welcome result.&lt;/li&gt;&lt;/ol&gt;&lt;/div&gt;&lt;/div&gt;</summary>' +
      '</entry></feed>');
    return;
  }

  if (url.pathname === '/daily-discovery/trivia') {
    sendJson(res, {
      response_code: 0,
      results: [{
        category: 'Science &amp; Nature',
        type: 'multiple',
        difficulty: 'medium',
        question: 'Which planet is known as the &quot;Red Planet&quot;?',
        correct_answer: 'Mars',
        incorrect_answers: ['Venus', 'Jupiter', 'Mercury']
      }]
    });
    return;
  }

  if (url.pathname.startsWith('/daily-discovery/on-this-day/')) {
    sendJson(res, {
      selected: [
        { year: 1950, text: 'A deterministic historical event used to validate the On This Day widget.', pages: [{ content_urls: { desktop: { page: 'https://en.wikipedia.org/wiki/History' } } }] },
        { year: 1967, text: 'A second fixture event demonstrates multiple entries and chronological metadata.', pages: [{ content_urls: { desktop: { page: 'https://en.wikipedia.org/wiki/1967' } } }] },
        { year: 2001, text: 'A third fixture event exercises the default three-item display limit.', pages: [{ content_urls: { desktop: { page: 'https://en.wikipedia.org/wiki/2001' } } }] }
      ]
    });
    return;
  }

  if (url.pathname === '/daily-discovery/nasa-apod') {
    sendJson(res, [{
      date: '2026-10-03',
      title: 'Webb View of a Spiral Galaxy',
      permalink: 'https://science.nasa.gov/universe/',
      media_type: 'image',
      explanation: 'A <strong>deterministic</strong> NASA Astronomy Picture of the Day fixture &amp; explanation used for visual validation.',
      credit: 'NASA &amp; ESA',
      copyright: 'Visual Fixture',
      alt: 'A spiral galaxy used as the deterministic NASA APOD fixture',
      hdurl: 'http://127.0.0.1:18089/daily-discovery/animal.png'
    }]);
    return;
  }

  if (url.pathname === '/daily-discovery/animal') {
    sendJson(res, {
      results: [{
        count: 12456,
        taxon: {
          id: 41638,
          rank: 'species',
          is_active: true,
          extinct: false,
          name: 'Ailurus fulgens',
          preferred_common_name: 'Red Panda',
          wikipedia_url: 'https://en.wikipedia.org/wiki/Red_panda',
          default_photo: {
            license_code: 'cc-by',
            attribution: 'Deterministic visual fixture, CC BY',
            medium_url: 'http://127.0.0.1:18089/daily-discovery/animal.png'
          },
          conservation_status: {
            authority: 'IUCN',
            status_name: 'endangered'
          }
        }
      }]
    });
    return;
  }

  if (url.pathname === '/daily-discovery/animal.png') {
    const image = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAUAAAAC0CAIAAABqhmJGAAAD10lEQVR42u3dwVHbQBSAYZlJL5TBjA85ugJXAA1QAg2YYnJzGe4mVwaEELL27b7d7ztmgllp9s+TwHIOp/NxAnJ6cApAwICAAQGDgAEBAwIGBAwCBgQMCBgEDAgYEDAgYBAwIGBAwICAQcCAgAEBg4ABAQMCBgQMAgYEDAgYmKZp+uMUdOPl6XH9X75cb86YgElT7PLX6lnAtB7typcVs4DJ0e3y91KygMmU7uy3lrGASZaujAVM+nRlLGDSpytjAZM+XRm3yTux1DvQmk1gZGAUm8B0NMSMYgGr17HgEtp2dzltAqNeRydgBt3fGhawne1IEbA97XgFjN3sqAWsXseOgAEBG0HOgICxd50HAdu1zgYChgF5L3RDA+f57d/XP3x//Vv3WPZa1cvTo3dK7+5wOh+dher1zkZSPeMSq9KwS+je/NjJyr8zwqoQcFvjd30DkbWUW5WfZgl4rNkb33Cbq0LAbY3fbfu+dC0BqzKEBQwIONv4LT2Ew1ZlCAsYEHCN8YszJmBAwIaJ8yZgQMCAgDNcB97zcEK5BxviV+UqWsAgYMJtG1mlnytsc1UIuIeGYzppc1UIuJUb4G27P7KTyFW5DRZw/3M4fsq1uSo+8ZlYDTXc2mditbkqBNzJLfHIq8IlNAh4YH764kwKGBAwCBgQMCBgQMAgYEDAgIABAYOAAQEDAgYBA0l4oH+7y/VW6Dm4jx+CUf15+oDFXK4328kEBgEDAgYEPKiPt5qzHwrZ0w0wAq7GT1+cQwEDAu5XravoulfvCLiT22DrQcC5b+Hih2HMd3QDLGBD2PgVMM03HDmE/fZIwK6is17Whv1L4fpZwMNdSJeu69PrG78CNoTTNBxZr/Er4KHn8L4Zf31BszeFw+l8dBb2Uvq/yZyN9s7SSrym8WsCM63savM0/u4LzV4T2BAua7nYhQI3f6Hx2yYfqZN1FC+kuG0gG7wmMHFD+J5WK6Zr/JrAzLS3oWQj1wSm/hD+7Uyu2K3xK2ANZ6XeEvwaCQSMgeNsCNiudR4QsL3rDAgYEDAjjyDjV8D2saNGwHaz4xUw9rQjFbCGHSMCtr8d3fA8jVRtl3f2ZmnpmsBGsWNBwPa9o3AJjctp6ZrAdFuCek1gUo5i6QqYlBlLV8CkzFi6AiZlxtIVMCkzlq6A2bmlgJJ1K2Ai6toxZtEKmMqXuL/qWbECpumeGYF3YoGAAQEDAgYBAwIGBAwIGAQMCBgQMAgYEDAgYEDAIGBAwICAAQGDgAEBAwIGAQMCBgQMCBgEDAgYEDAgYBAwIGBAwCBgQMCAgAEBg4CBlvwHVeiYgNK8j80AAAAASUVORK5CYII=', 'base64');
    res.writeHead(200, {
      'Content-Type': 'image/png',
      'Content-Length': image.length,
      'Cache-Control': 'public, max-age=3600'
    });
    res.end(image);
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
