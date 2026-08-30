package glance

import (
	"context"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func remainingValueTransport(t *testing.T, fn priorityRoundTripper) {
	t.Helper()
	old := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = fn
	t.Cleanup(func() { defaultHTTPClient.Transport = old })
}

func remainingValueResponse(r *http.Request, status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}

func TestRemainingValueConfigIncludeWrappersAndVariables(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child.yml")
	main := filepath.Join(dir, "main.yml")

	if err := os.WriteFile(child, []byte("- one\n- two"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte("items:\n  $include: child.yml\n"), 0600); err != nil {
		t.Fatal(err)
	}

	got, includes, err := recursiveParseYAMLIncludes(main, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "  - one") || len(includes) != 1 {
		t.Fatalf("got=%q includes=%#v", got, includes)
	}

	if _, _, err := recursiveParseYAMLIncludes(
		main,
		nil,
		CONFIG_INCLUDE_RECURSION_DEPTH_LIMIT+1,
	); err == nil {
		t.Fatal("expected depth error")
	}

	t.Setenv("REMAINING_VALUE_ENV", "value")

	if v, original, err := parseConfigVariableOfType(
		configVarTypeEnv,
		"REMAINING_VALUE_ENV",
	); err != nil || original || v != "value" {
		t.Fatalf("env=%q original=%v err=%v", v, original, err)
	}

	if _, original, err := parseConfigVariableOfType(
		configVarTypeEnv,
		"lowercase",
	); err != nil || !original {
		t.Fatalf("invalid env original=%v err=%v", original, err)
	}

	if _, original, err := parseConfigVariableOfType(
		"unknown",
		"ANYTHING",
	); err != nil || !original {
		t.Fatalf("unknown original=%v err=%v", original, err)
	}

	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("  secret-value\n"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REMAINING_VALUE_FILE", secret)

	if v, original, err := parseConfigVariableOfType(
		configVarTypeFileFromEnv,
		"REMAINING_VALUE_FILE",
	); err != nil || original || v != "secret-value" {
		t.Fatalf("file=%q original=%v err=%v", v, original, err)
	}
}

func TestRemainingValueConfigWatcherWrapperInitialCallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.yml")
	contents := []byte("pages: []\n")

	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}

	called := make(chan []byte, 1)

	stop, err := configFilesWatcher(
		path,
		contents,
		map[string]struct{}{},
		func(b []byte) {
			called <- append([]byte(nil), b...)
		},
		func(err error) {
			t.Errorf("watcher error: %v", err)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	select {
	case got := <-called:
		if string(got) != string(contents) {
			t.Fatalf("callback=%q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("initial callback not received")
	}
}

func TestRemainingValueThemeHandlerAndLoginPage(t *testing.T) {
	preset := &themeProperties{
		Light: true,
		CSS:   template.CSS("body{color:red}"),
	}

	presets, err := newOrderedYAMLMap(
		[]string{"light"},
		[]*themeProperties{preset},
	)
	if err != nil {
		t.Fatal(err)
	}

	a := &application{}
	a.Config.Theme.Presets = *presets
	a.Config.Theme.CSS = template.CSS("body{color:black}")
	a.Config.Server.BaseURL = "/glance"

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("key", "light")

	rr := httptest.NewRecorder()
	a.handleThemeChangeRequest(rr, req)

	if rr.Code != http.StatusOK ||
		rr.Header().Get("X-Scheme") != "light" ||
		rr.Body.String() != "body{color:red}" {
		t.Fatalf(
			"theme code=%d headers=%v body=%q",
			rr.Code,
			rr.Header(),
			rr.Body.String(),
		)
	}

	if len(rr.Result().Cookies()) != 1 ||
		rr.Result().Cookies()[0].Path != "/glance/" {
		t.Fatalf("cookies=%#v", rr.Result().Cookies())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("key", "missing")

	rr = httptest.NewRecorder()
	a.handleThemeChangeRequest(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing theme code=%d", rr.Code)
	}

	/*
		The login page is only rendered when authentication is enabled.
		When RequiresAuth is false the handler redirects away from /login.
	*/
	a.RequiresAuth = true

	loginReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	loginRR := httptest.NewRecorder()

	a.handleLoginPageRequest(loginRR, loginReq)

	if loginRR.Code != http.StatusOK || loginRR.Body.Len() == 0 {
		t.Fatalf(
			"login code=%d body=%q",
			loginRR.Code,
			loginRR.Body.String(),
		)
	}

	a.RequiresAuth = false

	loginRR = httptest.NewRecorder()
	a.handleLoginPageRequest(loginRR, loginReq)

	if loginRR.Code != http.StatusSeeOther {
		t.Fatalf("authorized login code=%d", loginRR.Code)
	}
}

func TestRemainingValueDiagnosticHTTPHelpers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			/*
				Only require X-Test when it is actually present.

				The first request uses testHttpRequestWithHeaders and
				should contain it. The second request intentionally uses
				testHttpRequest without custom headers so that we can test
				the unexpected-status path.
			*/
			if r.Header.Get("X-Test") != "" &&
				r.Header.Get("X-Test") != "yes" {
				t.Errorf("header=%q", r.Header.Get("X-Test"))
			}

			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(
				w,
				strings.Repeat("x", 60)+"\n",
			)
		},
	))
	defer srv.Close()

	info, err := testHttpRequestWithHeaders(
		http.MethodGet,
		srv.URL,
		map[string]string{"X-Test": "yes"},
		http.StatusCreated,
	)
	if err != nil ||
		!strings.Contains(info, "61 bytes") ||
		!strings.Contains(info, "...") {
		t.Fatalf("info=%q err=%v", info, err)
	}

	if _, err := testHttpRequest(
		http.MethodGet,
		srv.URL,
		http.StatusOK,
	); err == nil {
		t.Fatal("expected status mismatch")
	}
}

func TestRemainingValueDNSLifecycleAndRender(t *testing.T) {
	for _, service := range []string{
		dnsServiceAdguard,
		dnsServicePihole,
		dnsServicePiholeV6,
		dnsServiceTechnitium,
	} {
		w := &dnsStatsWidget{
			Service:    service,
			URL:        "https://dns.invalid",
			HourFormat: "24h",
		}

		if err := w.initialize(); err != nil {
			t.Fatalf("service %s: %v", service, err)
		}

		if service == dnsServicePihole ||
			service == dnsServicePiholeV6 {
			if w.TitleURL != "https://dns.invalid/admin" {
				t.Fatalf("title URL=%q", w.TitleURL)
			}
		}
	}

	if err := (&dnsStatsWidget{
		Service: "bad",
	}).initialize(); err == nil {
		t.Fatal("expected invalid service")
	}

	labels := makeDNSWidgetTimeLabels("15:00")
	for i, label := range labels {
		if label == "" {
			t.Fatalf("empty label %d: %#v", i, labels)
		}
	}

	remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/control/stats" {
			t.Fatalf("path=%q", r.URL.Path)
		}

		return remainingValueResponse(
			r,
			http.StatusOK,
			"application/json",
			`{
				"num_dns_queries":100,
				"dns_queries":[],
				"num_blocked_filtering":10,
				"blocked_filtering":[],
				"avg_processing_time":0.001,
				"top_blocked_domains":[]
			}`,
		), nil
	})

	w := &dnsStatsWidget{
		Service:    dnsServiceAdguard,
		URL:        "https://dns.invalid",
		HourFormat: "24h",
		HideGraph:  true,
	}

	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}

	w.update(context.Background())

	if w.Stats == nil ||
		w.Stats.TotalQueries != 100 ||
		w.TimeLabels[0] == "" {
		t.Fatalf(
			"stats=%#v labels=%#v",
			w.Stats,
			w.TimeLabels,
		)
	}

	if len(w.Render()) == 0 {
		t.Fatal("empty DNS render")
	}
}

func TestRemainingValueProviderWidgetUpdateAndRenderPaths(t *testing.T) {
	t.Run("changedetection", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/api/v1/watch" {
				/*
					ChangeDetection's watch-list endpoint is decoded into
					map[string]struct{}, not []string.
				*/
				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					`{"a":{},"b":{}}`,
				), nil
			}

			if strings.HasPrefix(r.URL.Path, "/api/v1/watch/") {
				id := strings.TrimPrefix(
					r.URL.Path,
					"/api/v1/watch/",
				)

				changed := "10"
				if id == "b" {
					changed = "20"
				}

				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					`{
						"title":"`+id+`",
						"url":"https://example.invalid/`+id+`",
						"last_changed":`+changed+`,
						"previous_md5":"123456789",
						"viewed":false
					}`,
				), nil
			}

			return remainingValueResponse(
				r,
				http.StatusNotFound,
				"application/json",
				`{}`,
			), nil
		})

		w := &changeDetectionWidget{
			InstanceURL: "https://change.invalid",
			Limit:       1,
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if len(w.WatchUUIDs) != 2 ||
			len(w.ChangeDetections) != 1 ||
			w.ChangeDetections[0].Title != "b" {
			t.Fatalf("widget=%#v", w)
		}
	})

	t.Run("extension", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			resp := remainingValueResponse(
				r,
				http.StatusOK,
				"text/html",
				`<strong>ok</strong>`,
			)

			resp.Header.Set(extensionHeaderTitle, "Remote")
			resp.Header.Set(
				extensionHeaderTitleURL,
				"https://example.invalid",
			)

			return resp, nil
		})

		w := &extensionWidget{
			URL:       "https://extension.invalid",
			AllowHtml: true,
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if w.Title != "Remote" ||
			w.TitleURL != "https://example.invalid" ||
			len(w.Render()) == 0 {
			t.Fatalf(
				"widget=%#v render=%q",
				w,
				w.Render(),
			)
		}
	})

	t.Run("hackernews", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "stories.json") {
				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					`[1,2]`,
				), nil
			}

			id := "1"
			score := "1"

			if strings.HasSuffix(r.URL.Path, "/2.json") {
				id = "2"
				score = "100"
			}

			return remainingValueResponse(
				r,
				http.StatusOK,
				"application/json",
				`{
					"id":`+id+`,
					"score":`+score+`,
					"title":"post`+id+`",
					"url":"https://example.invalid/`+id+`",
					"descendants":10,
					"time":10
				}`,
			), nil
		})

		w := &hackerNewsWidget{
			Limit:       1,
			ExtraSortBy: "engagement",
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if len(w.Posts) != 1 ||
			w.Posts[0].Title != "post2" ||
			len(w.Render()) == 0 {
			t.Fatalf("posts=%#v", w.Posts)
		}
	})

	t.Run("lobsters", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			return remainingValueResponse(
				r,
				http.StatusOK,
				"application/json",
				`[
					{
						"created_at":"2026-08-30T12:00:00Z",
						"title":"one",
						"url":"https://example.invalid/1",
						"score":1,
						"comment_count":2,
						"comments_url":"https://lobste.rs/s/1",
						"tags":[]
					},
					{
						"created_at":"2026-08-30T13:00:00Z",
						"title":"two",
						"url":"https://example.invalid/2",
						"score":2,
						"comment_count":3,
						"comments_url":"https://lobste.rs/s/2",
						"tags":[]
					}
				]`,
			), nil
		})

		w := &lobstersWidget{Limit: 1}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if len(w.Posts) != 1 || len(w.Render()) == 0 {
			t.Fatalf("posts=%#v", w.Posts)
		}
	})

	t.Run("markets", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			symbol := strings.TrimPrefix(
				r.URL.Path,
				"/v8/finance/chart/",
			)

			price := "101"
			prev := "100"

			if symbol == "BBB" {
				price = "120"
			}

			body := `{
				"chart":{
					"result":[{
						"meta":{
							"currency":"USD",
							"symbol":"` + symbol + `",
							"regularMarketPrice":` + price + `,
							"chartPreviousClose":` + prev + `,
							"shortName":"` + symbol + `",
							"priceHint":2
						},
						"indicators":{
							"quote":[{
								"close":[1,2]
							}]
						}
					}]
				}
			}`

			return remainingValueResponse(
				r,
				http.StatusOK,
				"application/json",
				body,
			), nil
		})

		w := &marketsWidget{
			MarketRequests: []marketRequest{
				{Symbol: "AAA"},
				{Symbol: "BBB"},
			},
			Sort: "change",
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if len(w.Markets) != 2 ||
			w.Markets[0].Name != "BBB" ||
			len(w.Render()) == 0 {
			t.Fatalf("markets=%#v", w.Markets)
		}
	})

	t.Run("repository", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			switch {
			case r.URL.Path == "/repos/example/project":
				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					`{
						"full_name":"example/project",
						"stargazers_count":7,
						"forks_count":2
					}`,
				), nil

			case strings.Contains(
				r.URL.Path,
				"/search/issues",
			) && strings.Contains(
				r.URL.Query().Get("q"),
				"is:pr",
			):
				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					`{"total_count":1,"items":[]}`,
				), nil

			case strings.Contains(r.URL.Path, "/search/issues"):
				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					`{"total_count":2,"items":[]}`,
				), nil

			default:
				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					`[]`,
				), nil
			}
		})

		w := &repositoryWidget{
			RequestedRepository: "example/project",
			PullRequestsLimit:   -1,
			IssuesLimit:         -1,
			CommitsLimit:        -1,
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if w.Repository.Name != "example/project" ||
			w.Repository.Stars != 7 ||
			len(w.Render()) == 0 {
			t.Fatalf("repo=%#v", w.Repository)
		}
	})

	t.Run("rss", func(t *testing.T) {
		srv := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(
					"Content-Type",
					"application/rss+xml",
				)

				_, _ = io.WriteString(
					w,
					`<?xml version="1.0"?>
					<rss version="2.0">
						<channel>
							<title>Feed</title>
							<link>https://example.invalid</link>
							<item>
								<title>Old</title>
								<link>https://example.invalid/old</link>
								<pubDate>Sat, 29 Aug 2026 12:00:00 GMT</pubDate>
							</item>
							<item>
								<title>New</title>
								<link>https://example.invalid/new</link>
								<pubDate>Sun, 30 Aug 2026 12:00:00 GMT</pubDate>
							</item>
						</channel>
					</rss>`,
				)
			}),
		)
		defer srv.Close()

		w := &rssWidget{
			FeedRequests: []rssFeedRequest{
				{URL: srv.URL},
			},
			Limit: 1,
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if len(w.Items) != 1 ||
			w.Items[0].Title != "New" ||
			len(w.Render()) == 0 {
			t.Fatalf("items=%#v", w.Items)
		}
	})

	t.Run("weather", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			if strings.Contains(r.URL.Host, "geocoding") {
				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					`{
						"results":[{
							"name":"Test",
							"latitude":40,
							"longitude":-80,
							"timezone":"UTC",
							"country":"United States",
							"admin1":"Ohio"
						}]
					}`,
				), nil
			}

			return remainingValueResponse(
				r,
				http.StatusOK,
				"application/json",
				`{
					"current":{
						"temperature_2m":20,
						"apparent_temperature":19,
						"weather_code":0
					},
					"daily":{
						"sunrise":[1788087600],
						"sunset":[1788138000]
					},
					"hourly":{
						"temperature_2m":[
							10,10,11,11,12,12,
							13,13,14,14,15,15,
							16,16,17,17,18,18,
							19,19,20,20,21,21
						],
						"precipitation_probability":[
							0,0,0,0,0,0,
							0,0,0,0,0,0,
							0,0,0,0,0,0,
							0,0,0,0,0,0
						]
					}
				}`,
			), nil
		})

		w := &weatherWidget{
			Location:   "Test, Ohio, United States",
			HourFormat: "24h",
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if w.Place == nil ||
			w.Weather == nil ||
			len(w.Render()) == 0 {
			t.Fatalf(
				"place=%#v weather=%#v",
				w.Place,
				w.Weather,
			)
		}
	})
}

func TestRemainingValueDockerReleasesAndServerStatsUpdates(t *testing.T) {
	t.Run("docker remote update", func(t *testing.T) {
		srv := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/containers/json" ||
					r.URL.Query().Get("all") != "true" {
					t.Fatalf("request=%s", r.URL.String())
				}

				w.Header().Set(
					"Content-Type",
					"application/json",
				)

				_, _ = io.WriteString(
					w,
					`[
						{
							"Names":["/zeta"],
							"Image":"img:z",
							"State":"running",
							"Status":"Up",
							"Labels":{}
						},
						{
							"Names":["/alpha"],
							"Image":"img:a",
							"State":"exited",
							"Status":"Exited",
							"Labels":{}
						}
					]`,
				)
			}),
		)
		defer srv.Close()

		w := &dockerContainersWidget{
			SockPath: srv.URL,
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if len(w.Containers) != 2 ||
			w.Containers[0].Name != "alpha" ||
			w.Containers[0].StateIcon != dockerContainerStateIconWarn {
			t.Fatalf("containers=%#v", w.Containers)
		}

		if len(w.Render()) == 0 {
			t.Fatal("empty docker render")
		}
	})

	t.Run("releases update", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			if !strings.HasSuffix(
				r.URL.Path,
				"/releases/latest",
			) {
				t.Fatalf("path=%q", r.URL.Path)
			}

			return remainingValueResponse(
				r,
				http.StatusOK,
				"application/json",
				`{
					"tag_name":"v1.2.3",
					"published_at":"2026-08-30T12:00:00Z",
					"html_url":"https://example.invalid/release"
				}`,
			), nil
		})

		req := &releaseRequest{
			Repository: "example/project",
			source:     releaseSourceGithub,
		}

		w := &releasesWidget{
			Repositories: []*releaseRequest{req},
			Limit:        1,
		}

		w.Providers = &widgetProviders{
			assetResolver: func(path string) string {
				return "/assets/" + path
			},
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if len(w.Releases) != 1 ||
			w.Releases[0].Version != "v1.2.3" ||
			w.Releases[0].SourceIconURL !=
				"/assets/icons/github.svg" {
			t.Fatalf("releases=%#v", w.Releases)
		}

		if len(w.Render()) == 0 {
			t.Fatal("empty releases render")
		}
	})

	t.Run("remote server stats success and failure", func(t *testing.T) {
		srv := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/sysinfo/all" {
					t.Fatalf("path=%q", r.URL.Path)
				}

				w.Header().Set(
					"Content-Type",
					"application/json",
				)

				_, _ = io.WriteString(
					w,
					`{"hostname":"remote"}`,
				)
			}),
		)
		defer srv.Close()

		w := &serverStatsWidget{
			Servers: []serverStatsRequest{
				{
					Type: "remote",
					URL:  srv.URL,
				},
				{
					Type: "remote",
					URL:  "http://127.0.0.1:1",
					Name: "down",
				},
			},
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.Servers[1].Timeout =
			durationField(50 * time.Millisecond)

		w.update(context.Background())

		if !w.Servers[0].IsReachable ||
			w.Servers[0].Info == nil ||
			w.Servers[0].Info.Hostname != "remote" {
			t.Fatalf("success=%#v", w.Servers[0])
		}

		if w.Servers[1].IsReachable ||
			w.Servers[1].Info == nil {
			t.Fatalf("failure=%#v", w.Servers[1])
		}

		if len(w.Render()) == 0 {
			t.Fatal("empty server stats render")
		}
	})
}

func TestRemainingValueRedditFetchUpdateTokenAndRender(t *testing.T) {
	oldClient := redditHTTPClient
	oldLoid := getRedditLoidCookie

	t.Cleanup(func() {
		redditHTTPClient = oldClient
		getRedditLoidCookie = oldLoid
	})

	getRedditLoidCookie = func() (string, error) {
		return "test-loid", nil
	}

	redditHTTPClient = &http.Client{
		Transport: priorityRoundTripper(
			func(r *http.Request) (*http.Response, error) {
				cookie, err := r.Cookie("loid")
				if err != nil ||
					cookie.Value != "test-loid" {
					t.Fatalf(
						"loid cookie=%#v err=%v",
						cookie,
						err,
					)
				}

				body := `{
					"data":{
						"children":[
							{
								"data":{
									"id":"sticky",
									"title":"Sticky",
									"ups":99,
									"created":10,
									"num_comments":1,
									"domain":"self",
									"permalink":"/r/test/sticky",
									"stickied":true
								}
							},
							{
								"data":{
									"id":"one",
									"title":"One &amp; Only",
									"ups":5,
									"url":"https://example.invalid/one",
									"created":20,
									"num_comments":3,
									"domain":"example.invalid",
									"permalink":"/r/test/one",
									"thumbnail":"https://img.invalid/one.jpg",
									"link_flair_text":"News"
								}
							},
							{
								"data":{
									"id":"two",
									"title":"Two",
									"ups":50,
									"url":"https://example.invalid/two",
									"created":30,
									"num_comments":4,
									"domain":"example.invalid",
									"permalink":"/r/test/two"
								}
							}
						]
					}
				}`

				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					body,
				), nil
			},
		),
	}

	w := &redditWidget{
		Subreddit:           "test",
		Limit:               1,
		ExtraSortBy:         "engagement",
		ShowFlairs:          true,
		CommentsURLTemplate: "https://comments.invalid/{SUBREDDIT}/{POST-ID}/{POST-PATH}",
	}

	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}

	w.update(context.Background())

	/*
		The provider applies the configured limit while processing the
		non-stickied listing. With this fixture the retained post is the
		first eligible non-stickied post: "One & Only".
	*/
	if len(w.Posts) != 1 ||
		w.Posts[0].Title != "One & Only" {
		t.Fatalf("posts=%#v", w.Posts)
	}

	if len(w.Render()) == 0 {
		t.Fatal("empty reddit default render")
	}

	w.Style = "horizontal-cards"
	if len(w.Render()) == 0 {
		t.Fatal("empty reddit horizontal render")
	}

	w.Style = "vertical-cards"
	if len(w.Render()) == 0 {
		t.Fatal("empty reddit vertical render")
	}

	tokenClient := &http.Client{
		Transport: priorityRoundTripper(
			func(r *http.Request) (*http.Response, error) {
				user, pass, ok := r.BasicAuth()

				if !ok ||
					user != "client" ||
					pass != "secret" ||
					r.Header.Get("User-Agent") != "app/1.0" {
					t.Fatalf(
						"token request auth=%q/%q ok=%v headers=%v",
						user,
						pass,
						ok,
						r.Header,
					)
				}

				return remainingValueResponse(
					r,
					http.StatusOK,
					"application/json",
					`{
						"access_token":"token",
						"expires_in":3600
					}`,
				), nil
			},
		),
	}

	authWidget := &redditWidget{}
	authWidget.Proxy.client = tokenClient
	authWidget.AppAuth.Name = "app"
	authWidget.AppAuth.ID = "client"
	authWidget.AppAuth.Secret = "secret"

	if err := authWidget.fetchNewAppAccessToken(
		context.Background(),
	); err != nil {
		t.Fatal(err)
	}

	if authWidget.AppAuth.accessToken != "token" ||
		time.Until(
			authWidget.AppAuth.tokenExpiresAt,
		) < 59*time.Minute {
		t.Fatalf(
			"app auth=%#v",
			authWidget.AppAuth,
		)
	}
}

func TestRemainingValueTwitchProviderAndUpdatePaths(t *testing.T) {
	t.Run("channel task live and offline", func(t *testing.T) {
		var calls atomic.Int32

		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			calls.Add(1)

			/*
				StreamMetadata maps the stream timestamp from "startedAt".
			*/
			body := `[
				{
					"data":{
						"userOrError":{
							"__typename":"User",
							"displayName":"Example",
							"profileImageURL":"https://img",
							"stream":{
								"viewersCount":42
							}
						}
					},
					"extensions":{
						"operationName":"ChannelShell"
					}
				},
				{
					"data":{
						"userOrNull":{
							"stream":{
								"startedAt":"2026-08-30T12:00:00Z",
								"game":{
									"name":"Game",
									"slug":"game"
								}
							},
							"lastBroadcast":{
								"title":"Live title"
							}
						}
					},
					"extensions":{
						"operationName":"StreamMetadata"
					}
				}
			]`

			return remainingValueResponse(
				r,
				http.StatusOK,
				"application/json",
				body,
			), nil
		})

		ch, err := fetchChannelFromTwitchTask(
			context.Background(),
			"Example",
		)

		if err != nil ||
			!ch.Exists ||
			!ch.IsLive ||
			ch.Name != "Example" ||
			ch.AvatarUrl != "https://img" ||
			ch.ViewersCount != 42 {
			t.Fatalf(
				"channel=%#v err=%v",
				ch,
				err,
			)
		}

		if calls.Load() != 1 {
			t.Fatalf("calls=%d", calls.Load())
		}
	})

	t.Run("channels partial and widget sorting", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			bodyBytes, _ := io.ReadAll(r.Body)
			body := string(bodyBytes)

			if strings.Contains(body, "bad") {
				return remainingValueResponse(
					r,
					http.StatusInternalServerError,
					"application/json",
					`{}`,
				), nil
			}

			viewers := "5"
			name := "One"

			if strings.Contains(body, "two") {
				viewers = "50"
				name = "Two"
			}

			response := `[
				{
					"data":{
						"userOrError":{
							"__typename":"User",
							"displayName":"` + name + `",
							"profileImageURL":"",
							"stream":{
								"viewersCount":` + viewers + `
							}
						}
					},
					"extensions":{
						"operationName":"ChannelShell"
					}
				},
				{
					"data":{
						"userOrNull":{
							"stream":{
								"startedAt":"2026-08-30T12:00:00Z"
							}
						}
					},
					"extensions":{
						"operationName":"StreamMetadata"
					}
				}
			]`

			return remainingValueResponse(
				r,
				http.StatusOK,
				"application/json",
				response,
			), nil
		})

		list, err := fetchChannelsFromTwitch(
			context.Background(),
			[]string{"one", "bad", "two"},
		)

		if err == nil || len(list) != 2 {
			t.Fatalf(
				"list=%#v err=%v",
				list,
				err,
			)
		}

		w := &twitchChannelsWidget{
			ChannelsRequest: []string{"one", "two"},
			SortBy:          "viewers",
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if len(w.Channels) != 2 ||
			w.Channels[0].Name != "Two" ||
			len(w.Render()) == 0 {
			t.Fatalf(
				"channels=%#v",
				w.Channels,
			)
		}
	})

	t.Run("top games provider and widget", func(t *testing.T) {
		remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
			body := `[
				{
					"data":{
						"directoriesWithTags":{
							"edges":[
								{
									"node":{
										"slug":"skip",
										"name":"Skip",
										"avatarURL":"https://img/285x380",
										"viewersCount":100,
										"tags":[],
										"originalReleaseDate":"2020-01-01T00:00:00Z"
									}
								},
								{
									"node":{
										"slug":"keep",
										"name":"Keep",
										"avatarURL":"https://img/285x380",
										"viewersCount":90,
										"tags":[
											{"tagName":"a"},
											{"tagName":"b"},
											{"tagName":"c"}
										],
										"originalReleaseDate":"` +
				time.Now().UTC().Format(
					"2006-01-02T15:04:05Z",
				) + `"
									}
								}
							]
						}
					}
				}
			]`

			return remainingValueResponse(
				r,
				http.StatusOK,
				"application/json",
				body,
			), nil
		})

		cats, err := fetchTopGamesFromTwitch(
			context.Background(),
			[]string{"skip"},
			10,
		)

		if err != nil ||
			len(cats) != 1 ||
			cats[0].Slug != "keep" ||
			len(cats[0].Tags) != 2 ||
			!cats[0].IsNew ||
			strings.Contains(
				cats[0].AvatarUrl,
				"285x380",
			) {
			t.Fatalf(
				"cats=%#v err=%v",
				cats,
				err,
			)
		}

		w := &twitchGamesWidget{
			Exclude: []string{"skip"},
			Limit:   1,
		}

		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}

		w.update(context.Background())

		if len(w.Categories) != 1 ||
			w.Categories[0].Slug != "keep" ||
			len(w.Render()) == 0 {
			t.Fatalf(
				"widget=%#v",
				w,
			)
		}
	})
}

func TestRemainingValueContainerAndTemplateBehavior(t *testing.T) {
	first := newRefreshTestWidget()
	second := newRefreshTestWidget()

	close(first.updateBlock)
	close(second.updateBlock)

	c := &containerWidgetBase{
		Widgets: widgets{
			first,
			second,
		},
	}

	now := time.Now()

	if !c._requiresUpdate(&now) {
		t.Fatal("expected update required")
	}

	c._update(context.Background())

	if first.updateCount.Load() != 1 ||
		second.updateCount.Load() != 1 {
		t.Fatalf(
			"counts=%d,%d",
			first.updateCount.Load(),
			second.updateCount.Load(),
		)
	}

	later := time.Now()

	if c._requiresUpdate(&later) {
		t.Fatal("expected children scheduled")
	}

	base := &widgetBase{
		ContentAvailable: true,
	}

	good := template.Must(
		template.New("good").Parse(
			`hello {{.Title}}`,
		),
	)

	base.Title = "world"

	if got := base.renderTemplate(
		base,
		good,
	); got != "hello world" {
		t.Fatalf("render=%q", got)
	}

	bad := template.Must(
		template.New("bad").Parse(
			`{{template "missing" .}}`,
		),
	)

	got := base.renderTemplate(base, bad)

	if got != "" ||
		base.ContentAvailable ||
		base.Error == nil {
		t.Fatalf(
			"bad render=%q available=%v err=%v",
			got,
			base.ContentAvailable,
			base.Error,
		)
	}

	base.update(context.Background())
}
