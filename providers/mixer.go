package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"

	"github.com/grafov/m3u8"
	"github.com/kpango/glg"
)

const (
	// DefaultMixerBaseURL is the URL that will be used for the Mixer API if a
	// custom one is not provided.
	DefaultMixerBaseURL = "https://mixer.com/api/v1"

	// CurrentUserPath is the URL path used to retrieve the currently authenticated
	// Mixer user, based off of the access token provided.
	CurrentUserPath = "/users/current"

	// FollowedChannelsPathFmt is a path format that takes the user's ID as a path paramter
	FollowedChannelsPathFmt = "/users/%d/follows"

	// ChannelByIDFmt is a path format for loading data about a specific channel given the ID
	ChannelByIDFmt = "/channels/%d"

	// LiveStreamsPathFmt is a path format that takes a username and returns
	// informration about a specific channel including live status and user ID which
	// can be used to request the channel's HLS manifest and streams.
	LiveStreamsPathFmt = "/channels/%s"

	// StreamsManifestPathFmt is a path format for requesting the HLS manifest for a particular
	// user. The path parameter needs ot be th user's ID.
	StreamsManifestPathFmt = "/channels/%d/manifest.m3u8?showAudioOnly=2"
)

// MixerClient is a type that will wrap properties needed to make requests
// to the Mixer public API.
type MixerClient struct {
	PlatformName string
	BaseURL      string
	*http.Client
}

/*
 * StreamProviders interface
 */

// Play is responsible for finding a random stream to play out of the current user's
// followed channels lists.
func (client *MixerClient) Play(token string) (*Stream, error) {

	// TODO: Maybe this should check to see if something is active before
	// just pulling a random follow?

	user, err := client.GetCurrentUser(token)
	if err != nil {
		return nil, errors.New("Failed to load your Mixer information, please try again later")
	}

	// Load the list of followed channels
	channels, err := client.GetFollowedChannels(user, true)
	if err != nil {
		return nil, err
	}

	if len(channels) <= 0 {
		return nil, errors.New("No online channels")
	}

	// Grab the first one
	channel := channels[0]
	variant, err := client.FindStreamWithChannel(channel, "audio-only")

	stream := &Stream{
		Name:      channel.Name,
		Title:     channel.Title,
		ChannelID: channel.ID,
		Variant:   variant,
	}

	saveRecentStream(user.UserID, channel.ID)

	glg.Infof("Playing stream with name: %s", stream.Name)
	return stream, nil
}

func saveRecentStream(userID uint, channelID uint) {

	conn := redisConnPool.Get()
	defer conn.Close()

	listName := fmt.Sprintf("mixer_recent_streams:%d", userID)
	conn.Send("MULTI")

	// Remove previous occurrences of this stream UserID if they exist already in the list
	conn.Send("LREM", listName, 0, channelID)
	conn.Send("LPUSH", listName, channelID)
	conn.Send("EXPIRE", listName, int((time.Hour * time.Duration(24)).Seconds()))
	_, err := conn.Do("EXEC")
	if err != nil {
		glg.Warnf("Failed to insert recent stream: %s", err.Error())
	}

	glg.Debugf("User(%d) recent streams: %+v", userID, findRecentStreams(userID))
}

func findRecentStreams(userID uint) []uint {

	recents := make([]uint, 0, 10)

	conn := redisConnPool.Get()
	defer conn.Close()

	listName := fmt.Sprintf("mixer_recent_streams:%d", userID)
	reply, err := redis.Ints(conn.Do("LRANGE", listName, 0, -1))
	if err != nil {
		glg.Errorf("Failed to get last stream User ID: %s", err.Error())
		return recents
	}

	for _, channelIDs := range reply {
		recents = append(recents, uint(channelIDs))
	}

	glg.Infof("Found recent channels: %+v", recents)

	return recents
}

func removeRecentStream(userID, channelID uint) {

	conn := redisConnPool.Get()
	defer conn.Close()

	listName := fmt.Sprintf("mixer_recent_streams:%d", userID)

	// Remove previous occurrences of this stream UserID if they exist already in the list
	conn.Send("LREM", listName, 0, channelID)

	glg.Debugf("User(%s) recent streams: %+v", userID, findRecentStreams(userID))
}

// Next will find the next stream in the ordered list of followed channels. This will either
// be the stream right after the current one or a new stream if the current one is no longer
// online.
func (client *MixerClient) Next(token string) (*Stream, error) {

	user, err := client.GetCurrentUser(token)
	if err != nil {
		return nil, errors.New("Failed to load your Mixer information, please try again later")
	}

	// Load the list of followed channels
	channels, err := client.GetFollowedChannels(user, true)
	if err != nil {
		return nil, err
	}

	ids := make([]uint, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.ID)
	}
	glg.Infof("Before sorted IDs: %+v", ids)

	sort.Sort(AscendingChannelsIDs(channels))

	sortedIDs := make([]uint, 0, len(channels))
	for _, channel := range channels {
		sortedIDs = append(sortedIDs, channel.ID)
	}
	glg.Infof("Sorted channel IDs: %+v", sortedIDs)

	if len(channels) <= 0 {
		return nil, errors.New("No online channels")
	}

	recents := findRecentStreams(user.UserID)

	var channel *MixerChannel
	if len(recents) != 0 {
		current := recents[0]
		for i, c := range channels {
			if c.ID == current {
				if (i + 1) < len(channels) {
					channel = channels[i+1]
				} else {
					channel = channels[0]
				}
			}
		}
	} else {
		channel = channels[0]
	}

	if channel == nil {
		// If there were no new channels in the list, just play the first one again
		index := rand.Intn(len(channels))
		glg.Infof("Didn't find a stream that wasn't already listened, using random index: %d",
			index)
		channel = channels[index]
	}

	variant, err := client.FindStreamWithChannel(channel, "audio-only")

	stream := &Stream{
		Name:      channel.Name,
		Title:     channel.Title,
		ChannelID: channel.ID,
		Variant:   variant,
	}

	saveRecentStream(user.UserID, channel.ID)

	glg.Infof("Playing stream with name: %s", stream.Name)

	return stream, nil
}

// Resume will attempt to find the last played stream which is still online and resume
// playback of that stream.
func (client *MixerClient) Resume(token string) (*Stream, error) {

	// NOTE: Might be worth testing but I'm assuming the user cannot issue a resume
	// command if the audio is already playing.
	user, err := client.GetCurrentUser(token)
	if err != nil {
		return nil, errors.New("Unable to get your user information from Mixer")
	}

	recentChannelIDs := findRecentStreams(user.UserID)
	if len(recentChannelIDs) <= 0 {
		return nil, errors.New("You have no recently listened streams to resume")
	}

	follows, err := client.GetFollowedChannels(user, true)
	if err != nil {
		return nil, errors.New("An error occurred trying to resume your last stream")
	}

	found := false
	var resumeChannel *MixerChannel
	for {
		if found || (len(recentChannelIDs) <= 0) {
			break
		}
		mostRecent := recentChannelIDs[0]

		for _, channel := range follows {
			if channel.ID == mostRecent {
				found = true
				resumeChannel = channel
				break
			}
		}

		removeRecentStream(user.UserID, mostRecent)
		recentChannelIDs = recentChannelIDs[1:]
	}

	if !found {
		return nil, errors.New("None of your recent streams are still live")
	}

	variant, err := client.FindStreamWithChannel(resumeChannel, "audio-only")

	stream := &Stream{
		Name:      resumeChannel.Name,
		Title:     resumeChannel.Title,
		ChannelID: resumeChannel.ID,
		Variant:   variant,
	}

	saveRecentStream(user.UserID, resumeChannel.ID)

	glg.Debugf("Playing stream with name: %s", stream.Name)

	return nil, nil
}

// Previous will play the last stream that is still online. Excluding the currently playing
// stream.
func (client *MixerClient) Previous(token string) (*Stream, error) {

	user, err := client.GetCurrentUser(token)
	if err != nil {
		return nil, errors.New("There was an error finding your Mixer account")
	}

	recentChannelIDs := findRecentStreams(user.UserID)
	if recentChannelIDs == nil || len(recentChannelIDs) <= 1 {
		// Check for less than or equal to 1 here because we want the user to be able
		// to go back from the current stream, if there is only one item then that is
		// the stream they are currently listening to.
		return nil, errors.New("There are no recent streams to be played")
	}
	removeRecentStream(user.UserID, recentChannelIDs[0])
	recentChannelIDs = recentChannelIDs[1:]

	var targetChannel *MixerChannel
	for _, channelID := range recentChannelIDs {
		channel, err := client.FindChannelByID(channelID)
		if err != nil || channel == nil || channel.Online == false {
			removeRecentStream(user.UserID, channelID)
			continue
		}

		targetChannel = channel
		break
	}

	if targetChannel == nil {
		return nil, errors.New("There are no recent streams to be played")
	}

	variant, err := client.FindStreamWithChannel(targetChannel, "audio-only")
	if err != nil {
		return nil, errors.New("An error occurred trying to play the previous steram")
	}

	stream := &Stream{
		Name:      targetChannel.Name,
		Title:     targetChannel.Title,
		ChannelID: targetChannel.ID,
		Variant:   variant,
	}

	saveRecentStream(user.UserID, targetChannel.ID)

	glg.Infof("Playing stream with name: %s", stream.Name)

	return stream, nil
}

// NewMixerClient will initialize a new client for interacting with the the Mixer
// public API. The `baseURL` parameter can be used to modify the host requests are
// sent to. Useful for testing.
func NewMixerClient(baseURL string) *MixerClient {
	base := DefaultMixerBaseURL
	if baseURL != "" {
		base = baseURL
	}

	return &MixerClient{
		PlatformName: "Mixer",
		BaseURL:      base,
		Client:       http.DefaultClient,
	}
}

// MixerUser represents the required properties to describe a specific user returned
// by the Mixer platform. TODO: This should be generalized into an interface probably
type MixerUser struct {
	UserID   uint   `json:"id"`
	Username string `json:"username"`
	Channel  *struct {
		ID uint `json:"id"`
	} `json:"channel"`
}

// GetCurrentUser will load the current user from the Mixer API using the provided
// access token.
func (client *MixerClient) GetCurrentUser(accessToken string) (*MixerUser, error) {

	url := client.BaseURL + CurrentUserPath
	request, err := http.NewRequest("GET", url, nil)
	request.Header.Add("Authorization", "Bearer "+accessToken)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	mixerUser := &MixerUser{}
	bodyResponse, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glg.Warnf("Failed to read current user body: %s", err.Error)
	}

	glg.Infof("Found current user response body: %s", string(bodyResponse))
	deocoder := json.NewDecoder(strings.NewReader(string(bodyResponse)))
	err = deocoder.Decode(mixerUser)
	if err != nil {
		return nil, err
	}

	return mixerUser, nil
}

// AscendingChannelsIDs is a type used to sort a list of Mixer channels based on the
// channel ID.
type AscendingChannelsIDs []*MixerChannel

func (channels AscendingChannelsIDs) Len() int { return len(channels) }
func (channels AscendingChannelsIDs) Swap(i, j int) {
	channels[i], channels[j] = channels[j], channels[i]
}
func (channels AscendingChannelsIDs) Less(i, j int) bool {
	return channels[i].ID < channels[j].ID
}

// GetFollowedChannels will load the list of followed channels for the provided Mixer user.
func (client *MixerClient) GetFollowedChannels(user *MixerUser, onlineOnly bool) ([]*MixerChannel, error) {

	url := client.BaseURL + fmt.Sprintf(FollowedChannelsPathFmt, user.UserID)

	glg.Infof("Making request with url: %s", url)

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	// NOTE: For Mixer each Follow contains a channel ID to and a Channel ID from. With Twitch it is
	// user IDs instead of Channels.
	mixerChannels := make([]*MixerChannel, 0, 10)
	bodyString, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glg.Warnf("Couldn't read response body: %s", err.Error)
	}

	decoder := json.NewDecoder(strings.NewReader(string(bodyString)))
	err = decoder.Decode(&mixerChannels)
	if err != nil {
		return nil, err
	}

	result := make([]*MixerChannel, 0, len(mixerChannels))
	for _, c := range mixerChannels {
		if onlineOnly && c.Online == false {
			continue
		}

		result = append(result, c)
	}

	glg.Infof("Found %d live streams", len(result))

	return result, nil
}

// FindChannelByID will load channel details for the channel specified by the ID parameter.
func (client *MixerClient) FindChannelByID(channelID uint) (*MixerChannel, error) {

	url := client.BaseURL + fmt.Sprintf(ChannelByIDFmt, channelID)

	glg.Infof("Making request with url: %s", url)

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	bodyString, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glg.Warnf("Couldn't read response body: %s", err.Error)
	}

	decoder := json.NewDecoder(strings.NewReader(string(bodyString)))

	channel := &MixerChannel{}
	err = decoder.Decode(&channel)
	if err != nil {
		return nil, err
	}

	if channelID != channel.ID {
		glg.Fatalf("Channel ID of response did NOT match provided channel ID")
	}

	return channel, nil
}

// MixerChannel contains details for a particular channel returned by the Mixer platform.
// This should also be separated into an interface or something that can be shared between providers
type MixerChannel struct {
	ID     uint   `json:"id"`
	Online bool   `json:"online"`
	UserID uint   `json:"userId"`
	Title  string `json:"name"`
	Name   string `json:"token"`
}

// FindStreamWithChannel will find the HLS manifest data for a specific channel, the channel
// must be live and should be specified by the provided paramter.
func (client *MixerClient) FindStreamWithChannel(channel *MixerChannel, streamQuality string) (*m3u8.Variant, error) {

	url := client.BaseURL + fmt.Sprintf(StreamsManifestPathFmt, channel.ID)
	glg.Debugf("Requesting stream manifest for channel(%s) with url: %s", channel.Name, url)
	streamsReq, err := http.NewRequest("GET", url, nil)

	streamResponse, err := client.Do(streamsReq)
	if err != nil {
		return nil, err
	}

	glg.Debugf("Stream response code : %d", streamResponse.StatusCode)

	playlist := m3u8.NewMasterPlaylist()
	err = playlist.DecodeFrom(streamResponse.Body, false)
	if err != nil {
		glg.Errorf("Failed to decode m3u file as a master playlist: %s", err.Error())
		return nil, err
	}

	return findBestVariant(playlist, streamQuality), nil
}

func findBestVariant(playlist *m3u8.MasterPlaylist, streamQuality string) *m3u8.Variant {
	var streamVariant *m3u8.Variant
	var audioOnlyVariant *m3u8.Variant

	if len(playlist.Variants) == 0 {
		glg.Error("Found 0 stream variants, this is a bad situation!")
		return nil
	}

	glg.Debugf("Found %d stream variants\n", len(playlist.Variants))

	for _, variant := range playlist.Variants {
		glg.Debugf("Variant=%+v", variant)
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

	return streamVariant
}
