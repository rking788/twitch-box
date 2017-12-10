package twitch

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/grafov/m3u8"
	"github.com/kpango/glg"
	"github.com/rking788/go-alexa/skillserver"
)

// The constant definitions for the URLs to be used to interact with the Twitch API.
const (
	GetCurrentTwitchUserURL     = "https://api.twitch.tv/helix/users"
	GetUserFollowsURLFormat     = "https://api.twitch.tv/helix/users/follows?from_id=%s"
	GetLiveStreamsURLFormat     = "https://api.twitch.tv/helix/streams?type=live&user_id=%s"
	GetChannelAccessTokenFormat = "https://api.twitch.tv/api/channels/%s/access_token?client_id=%s"
	GetStreamsURLFormat         = "https://usher.ttvnw.net/api/channel/hls/%s.m3u8?player=twitchweb&token=%s&sig=%s&allow_audio_only=true&allow_source=false&type=any&p=%d"
)

var redisConnPool *redis.Pool

// InitEnv provides a package level initialization point for any work that is environment specific
func InitEnv(redisURL string) {
	redisConnPool = newRedisPool(redisURL)
}

// Redis related functions

func newRedisPool(addr string) *redis.Pool {
	// 25 is the maximum number of active connections for the Heroku Redis free tier
	return &redis.Pool{
		MaxIdle:     3,
		MaxActive:   25,
		IdleTimeout: 240 * time.Second,
		Dial:        func() (redis.Conn, error) { return redis.DialURL(addr) },
	}
}

// SaveUsersCurrentStream will append the provided stream's User ID to the list
// of recently played. The list is set to automatically expire after 24 hours.
// This expiration time will be updated on each stream start.
func SaveUsersCurrentStream(user *User, stream *Stream) {
	if user == nil || stream == nil {
		glg.Warn("Cannot save current stream, nil user or stream param")
		return
	}

	conn := redisConnPool.Get()
	defer conn.Close()

	listName := fmt.Sprintf("twitch_recent_streams:%s", user.ID)
	conn.Send("MULTI")
	// Remove previous occurrences of this stream UserID if they exist already in the list
	conn.Send("LREM", listName, 0, stream.UserID)
	conn.Send("LPUSH", listName, stream.UserID)
	conn.Send("EXPIRE", listName, int((time.Hour * time.Duration(24)).Seconds()))
	_, err := conn.Do("EXEC")
	if err != nil {
		glg.Warnf("Failed to insert recent stream: %s", err.Error())
	}

	glg.Debugf("User(%s) recent streams: %+v", user.ID, getRecentStreamUserIDs(user))
}

// getRecentStreamUserIDs will return the full list of streams tied to the
// provided Twitch user, or an empty slice if none are present. This list will
// expire 24 hours after the last "begin stream" operation so if the list is empty,
// then the user has not started playing a stream within the last 24 hours.
func getRecentStreamUserIDs(user *User) (uids []string) {
	conn := redisConnPool.Get()
	defer conn.Close()

	listName := fmt.Sprintf("twitch_recent_streams:%s", user.ID)
	reply, err := redis.Strings(conn.Do("LRANGE", listName, 0, -1))
	if err != nil {
		glg.Errorf("Failed to get last stream User ID: %s", err.Error())
		return
	}

	return reply
}

// getCurrentStreamUserID will return the User ID value for the stream the user is currently
// viewing, if one exists; otherwise an empty string is returned.
func getCurrentStreamUserID(user *User) (uid string) {

	conn := redisConnPool.Get()
	defer conn.Close()

	listName := fmt.Sprintf("twitch_recent_streams:%s", user.ID)
	reply, err := redis.String(conn.Do("LINDEX", listName, 0))
	if err != nil {
		glg.Errorf("Failed to get current stream User ID: %s", err.Error())
	}

	glg.Debugf("Found current stream ID: %s", reply)
	return reply
}

// removeCurrentStream will pop the last stream off the list and return the previous
// stream's User ID. This should be used when moving to the 'previous' stream. This
// is a destructive operation, the current stream User ID will be lost.
func removeCurrentStream(user *User) (uid string) {
	conn := redisConnPool.Get()
	defer conn.Close()

	listName := fmt.Sprintf("twitch_recent_streams:%s", user.ID)
	conn.Do("LPOP", listName)

	reply, err := redis.String(conn.Do("LINDEX", listName, 0))
	if err != nil {
		glg.Errorf("Error trying to return new current stream User ID: %s", err.Error())
		return
	}

	return reply
}

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

	glg.Debugf("Get live streams response(%d): %+v", len(streamsJSON.Data), streamsJSON.Data)

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
	} else if userResponse.StatusCode != 200 {
		// TODO: need to figure out why this happens so much, refresh tokens aren't working maybe?
		glg.Errorf("Got error code from get user request: %d", userResponse.StatusCode)
	}

	userJSON := &UserResponse{}
	decoder := json.NewDecoder(userResponse.Body)
	err = decoder.Decode(userJSON)
	if err != nil {
		glg.Errorf("Failed to decode Twitch user JSON: %s", err.Error())
		return nil, err
	}

	glg.Debugf("Get user response: %+v", userJSON.Data)

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

	glg.Debugf("Get follows response: %+v", followsJSON.Data)

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
		if variant.Video == "audio_only" {
			audioOnlyVariant = variant
		}

		if strings.HasPrefix(variant.Video, streamQuality) {
			glg.Debug("Found stream URL with correct prefix")
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

func FindStreamForCommand(user *User, liveStreams []*Stream, command PlaybackCommand, response *skillserver.EchoResponse) *Stream {

	if command == PLAY {
		return liveStreams[0]
	}

	index := 0
	if command == RESUME || command == NEXT {
		streamerUserID := getCurrentStreamUserID(user)
		if streamerUserID != "" {
			currentStreamIndex := findIndexForStreamer(streamerUserID, liveStreams)
			if currentStreamIndex != -1 {
				if command == NEXT {
					if currentStreamIndex <= (len(liveStreams) - 2) {
						index = currentStreamIndex + 1
						glg.Infof("Found next stream with UserID: %s", liveStreams[index].UserID)
					}
				} else {
					index = currentStreamIndex
					glg.Infof("Resuming stream with UserID: %s", liveStreams[index].UserID)
				}
			} else {
				response.OutputSpeech("It looks like that user isn't streaming right now. ")
			}
		}
	} else if command == PREVIOUS {
		for {
			prevUID := removeCurrentStream(user)
			if prevUID == "" {
				response.OutputSpeech("It looks like none of your previously listened streams " + "are live right now")
				break
			}

			currentStreamIndex := findIndexForStreamer(prevUID, liveStreams)
			if currentStreamIndex != -1 {
				index = currentStreamIndex
				glg.Infof("Found previous stream with UserID: %s", liveStreams[index].UserID)
				break
			}
		}
	}

	return liveStreams[index]
}

// findIndexForStreamer will return the index in the live stream slice for the specified user
// ID. -1 is returned if the user ID is not found in the list.
func findIndexForStreamer(uid string, haystack []*Stream) int {
	needle := -1

	for index, stream := range haystack {
		if stream.UserID == uid {
			needle = index
		}
	}

	return needle
}
