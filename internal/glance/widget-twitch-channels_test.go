package glance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestFetchChannelFromTwitchTaskCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchChannelFromTwitchTask(ctx, "test-channel")
	if err == nil {
		t.Fatal("expected canceled Twitch channel request to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestFetchChannelsFromTwitchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchChannelsFromTwitch(ctx, []string{
		"channel-1",
		"channel-2",
		"channel-3",
		"channel-4",
		"channel-5",
		"channel-6",
		"channel-7",
		"channel-8",
		"channel-9",
		"channel-10",
		"channel-11",
		"channel-12",
	})

	if err == nil {
		t.Fatal("expected canceled Twitch channels job to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestTwitchChannelOperationRequestUsesCurrentStreamMetadataQuery(t *testing.T) {
	body := fmt.Sprintf(twitchChannelStatusOperationRequestBody, "test", "test")
	if !strings.Contains(body, `"sha256Hash":"b57f9b910f8cd1a4659d894fe7550ccc81ec9052c01e438b290fd66a040b9b93"`) {
		t.Fatal("StreamMetadata persisted-query hash is not current")
	}
	if !strings.Contains(body, `"includeIsDJ":true`) {
		t.Fatal("StreamMetadata request is missing includeIsDJ")
	}
}

func TestTwitchOperationResponsePreservesGraphQLErrors(t *testing.T) {
	var response twitchOperationResponse
	err := json.Unmarshal([]byte(`{"errors":[{"message":"PersistedQueryNotFound"}],"extensions":{"operationName":"StreamMetadata"}}`), &response)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Errors) != 1 || response.Errors[0].Message != "PersistedQueryNotFound" {
		t.Fatalf("errors=%#v", response.Errors)
	}
}
