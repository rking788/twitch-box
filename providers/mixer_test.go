package providers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestParseCurrentUser(t *testing.T) {

	r, err := sampleResponseReader("current-user.json")
	if err != nil {
		t.Fatalf("Error reading sample response from disk: %s", err.Error())
	}

	decoder := json.NewDecoder(r)
	var user MixerUser
	err = decoder.Decode(&user)
	if err != nil {
		t.Fatalf("Failed to decoder current user JSON: %s", err.Error())
	}

	if user.UserID == 0 {
		t.Fatalf("Incorrect user ID for current user")
	} else if user.Username == "" {
		t.Fatalf("Incorrect username from current user")
	} else if user.Channel == nil {
		t.Fatalf("Empty channel details for current user")
	}

	channel := user.Channel
	if channel.ID == 0 {
		t.Fatalf("Incorrect channelID for current user")
	}
}

func TestParseFollowedStreams(t *testing.T) {
	r, err := sampleResponseReader("follows.json")
	if err != nil {
		t.Fatalf("Error reading sample response from disk: %s", err.Error())
	}

	decoder := json.NewDecoder(r)
	channels := make([]*MixerChannel, 0, 10)
	err = decoder.Decode(&channels)
	if err != nil {
		t.Fatalf("Failed to decode followed channels JSON: %s", err.Error())
	}

	if len(channels) == 0 {
		t.Fatal("Parsed zero channels from follows list")
	}

	fmt.Printf("Found %d channels in follows list\n", len(channels))
}

func sampleResponseReader(filename string) (io.Reader, error) {

	f, err := os.Open("../local_tools/samples/mixer/" + filename)
	if err != nil {
		return nil, err
	}

	return bufio.NewReader(f), nil
}
