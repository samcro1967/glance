package glance

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBraveSearchPreset(t *testing.T) {
	widget := &searchWidget{SearchEngine: "brave"}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}
	if got, want := widget.SearchEngine, "https://search.brave.com/search?q=!QUERY!"; got != want {
		t.Fatalf("SearchEngine = %q, want %q", got, want)
	}
}

func TestMonitorDescriptionConfiguration(t *testing.T) {
	var site monitorSite
	if err := yaml.Unmarshal([]byte("title: Example\nurl: https://example.com\ndescription: Primary service\n"), &site); err != nil {
		t.Fatal(err)
	}
	if site.Description != "Primary service" {
		t.Fatalf("Description = %q, want %q", site.Description, "Primary service")
	}
}

func TestForkUIEnhancementTemplateContracts(t *testing.T) {
	checks := map[string][]string{
		"templates/video-card-contents.html":     {`href="{{ .Video.Url | safeURL }}"`, `if .OpenLinksInNewTab`},
		"templates/videos-vertical-list.html":    {`href="{{ .Url | safeURL }}"`, `if $.OpenLinksInNewTab`},
		"templates/forum-posts.html":             {`.ThumbnailLink | safeURL`, `if $.OpenLinksInNewTab`},
		"templates/reddit-horizontal-cards.html": {`.DiscussionUrl | safeURL`, `if $.OpenLinksInNewTab`},
		"templates/reddit-vertical-cards.html":   {`.DiscussionUrl | safeURL`, `if $.OpenLinksInNewTab`},
		"templates/monitor.html":                 {`if .Description`, `text-truncate`},
	}
	for name, fragments := range checks {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Errorf("%s missing %q", name, fragment)
			}
		}
	}
}

func TestForumThumbnailLinkIsOptInPerPost(t *testing.T) {
	post := forumPost{ThumbnailUrl: "https://example.com/thumb.jpg"}
	if post.ThumbnailLink != "" {
		t.Fatalf("non-Reddit forum post unexpectedly has thumbnail link %q", post.ThumbnailLink)
	}
}
