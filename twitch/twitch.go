package twitch

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"

	"github.com/grafov/m3u8"
	"github.com/kpango/glg"
)

// The constant definitions for the URLs to be used to interact with the Twitch API.
const (
	GetCurrentTwitchUserURL     = "https://api.twitch.tv/helix/users"
	GetUserFollowsURLFormat     = "https://api.twitch.tv/helix/users/follows?from_id=%s"
	GetLiveStreamsURLFormat     = "https://api.twitch.tv/helix/streams?type=live&user_id=%s"
	GetChannelAccessTokenFormat = "https://api.twitch.tv/api/channels/%s/access_token?client_id=%s"
	GetStreamsURLFormat         = "https://usher.ttvnw.net/api/channel/hls/%s.m3u8?player=twitchweb&token=%s&sig=%s&allow_audio_only=true&allow_source=false&type=any&p=%d"
)

// FindLiveStreams will request the data for all currently live streams on Twitch for the
// provided list of user IDs.
func FindLiveStreams(client *http.Client, uids []string) (*StreamsResponse, error) {

	joinedUIDList := strings.Join(uids, "&user_id=")
	url := fmt.Sprintf(GetLiveStreamsURLFormat, joinedUIDList)
	glg.Debugf("Making live stream request with url: %s", url)
	req, err := http.NewRequest("GET", url, nil)

	req.Header.Add("Client-ID", os.Getenv("TWITCH_API_CLIENT_ID"))

	streamsResponse, err := client.Do(req)
	if err != nil {
		fmt.Println("Failed to read the token response from Twitch!: ", err.Error())
		return nil, errors.New("Reading response from get live streams failed: " + err.Error())
	}

	streamsJSON := &StreamsResponse{}
	decoder := json.NewDecoder(streamsResponse.Body)
	err = decoder.Decode(streamsJSON)
	if err != nil {
		glg.Errorf("Failed to decode Twitch streams JSON: %s", err.Error())
		return nil, err
	}

	glg.Debugf("Get live streams response: %+v", streamsJSON)

	return streamsJSON, nil
}

// GetUserByID will load details for the user specified by the provided id. If the ID is the
// empty string, the current user will be determined from the provided access token.
func GetUserByID(client *http.Client, accessToken, id string) (*User, error) {

	url := GetCurrentTwitchUserURL
	if id != "" {
		url += "?id=" + id
	}
	req, err := http.NewRequest("GET", url, nil)

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Client-ID", os.Getenv("TWITCH_API_CLIENT_ID"))

	userResponse, err := client.Do(req)
	if err != nil {
		glg.Errorf("Failed to read the token response from Twitch!: %s", err.Error())
		return nil, errors.New("Reading response from get current user failed: " + err.Error())
	}

	userJSON := &UserResponse{}
	decoder := json.NewDecoder(userResponse.Body)
	err = decoder.Decode(userJSON)
	if err != nil {
		glg.Errorf("Failed to decode Twitch user JSON: %s", err.Error())
		return nil, err
	}

	glg.Debugf("Get user response: %+v", userJSON)

	return userJSON.Data[0], nil
}

// GetFollows will load the following information for the provided Twitch user.
// The channels returned will be all of the channels followed by this user.
func GetFollows(client *http.Client, user *User) (*Follows, error) {

	url := fmt.Sprintf(GetUserFollowsURLFormat, user.ID)
	req, err := http.NewRequest("GET", url, nil)

	req.Header.Add("Client-ID", os.Getenv("TWITCH_API_CLIENT_ID"))

	followsResponse, err := client.Do(req)
	if err != nil {
		glg.Errorf("Failed to read the token response from Twitch!: %s", err.Error())
		return nil, errors.New("Reading response from get current user failed: " + err.Error())
	}

	followsJSON := &Follows{}
	decoder := json.NewDecoder(followsResponse.Body)
	err = decoder.Decode(followsJSON)
	if err != nil {
		glg.Errorf("Failed to decode Twitch follows JSON: %s", err.Error())
		return nil, err
	}

	glg.Debugf("Get follows response: %+v", followsJSON)

	return followsJSON, nil
}

// GetStream will load the stream details for the provided channel name. The streamQuality parameter
// should be either audio_only or a target video resolution.
func GetStream(client *http.Client, channelName, accessToken, streamQuality string) (*m3u8.Variant, error) {
	// First get the access token data for the stream
	url := fmt.Sprintf(GetChannelAccessTokenFormat, channelName, os.Getenv("TWITCH_API_CLIENT_ID"))

	glg.Debugf("Get channel access token url : %v", url)
	req, err := http.NewRequest("GET", url, nil)

	accessTokenResponse, err := client.Do(req)
	if err != nil {
		glg.Errorf("Failed to read the token response from Twitch!: %s", err.Error())
		return nil, errors.New("Reading response from get channel access token: " + err.Error())
	}

	channelAccessTokenJSON := &ChannelAccessToken{}
	decoder := json.NewDecoder(accessTokenResponse.Body)
	decoder.Decode(channelAccessTokenJSON)

	glg.Debugf("Get channel access token decoded response: %+v", channelAccessTokenJSON)

	getStreamURL := fmt.Sprintf(GetStreamsURLFormat, channelName, channelAccessTokenJSON.Token,
		channelAccessTokenJSON.Sig, rand.Intn(999999))

	glg.Debugf("Get Stream URL Request : %v", getStreamURL)
	streamRequest, err := http.NewRequest("GET", getStreamURL, nil)

	streamResponse, err := client.Do(streamRequest)
	glg.Debugf("Stream response code : %d", streamResponse.StatusCode)

	playlist := m3u8.NewMasterPlaylist()
	err = playlist.DecodeFrom(streamResponse.Body, false)
	if err != nil {
		glg.Errorf("Failed to decode m3u file as a master playlist: %s", err.Error())
		return nil, err
	}

	var streamVariant *m3u8.Variant
	var audioOnlyVariant *m3u8.Variant

	if len(playlist.Variants) == 0 {
		glg.Error("Found 0 stream variants, this is a bad situation!")
		return nil, errors.New("Zero stream variants found")
	}

	glg.Debugf("Found %d streams variants\n", len(playlist.Variants))
	for _, variant := range playlist.Variants {
		glg.Debugf("Variant.Video = %s", variant.Video)
	}

	for _, variant := range playlist.Variants {
		if variant.Video == "audio_only" {
			audioOnlyVariant = variant
		}

		if strings.HasPrefix(variant.Video, streamQuality) {
			glg.Debug("Found video stream URL with correct prefix")
			streamVariant = variant
			break
		}
	}

	if streamVariant == nil {
		if audioOnlyVariant != nil {
			// If a stream did not match the requested one then fallback to audio_only...
			glg.Debug("Didn't find a stream with the correct quality so falling back to audio")
			streamVariant = audioOnlyVariant
		} else {
			// If the requested one and audio_only are both NOT available,
			// then use the lowest quality available
			glg.Warn("Didn't find a stream with the correct quality or audio_only so falling" +
				" back to the last stream URL")
			streamVariant = playlist.Variants[len(playlist.Variants)-1]
		}
	}

	return streamVariant, nil
}
