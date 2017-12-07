package alexa

import (
	"fmt"
	"net/http"

	"github.com/kpango/glg"
	"github.com/rking788/go-alexa/skillserver"
	"github.com/rking788/twitch-box/twitch"
)

// WelcomePrompt is responsible for returning a prompt to the user when launching the skill
func WelcomePrompt(echoRequest *skillserver.EchoRequest) (response *skillserver.EchoResponse) {

	response = skillserver.NewEchoResponse()
	flag := false
	response.OutputSpeech("Welcome, would you like to start playing one of you followed streams?").
		Reprompt("Should I start playing a Twitch stream?").
		EndSession(&flag)

	return
}

// StartAudioStream is responsible for getting the Twitch account for the currently linked
// account from the Alexa app. Then the user's followers will be requested and the audio will
// be played for one of their followed channels. If the device the user is interacting with supports
// video playback then a video stream will be returned.
func StartAudioStream(echoRequest *skillserver.EchoRequest) (response *skillserver.EchoResponse) {

	response = skillserver.NewEchoResponse()
	accessToken := echoRequest.Session.User.AccessToken
	if accessToken == "" {
		response := skillserver.NewEchoResponse()
		response.
			OutputSpeech("Sorry, it looks like your Twitch account needs to be linked in the Alexa app.").
			LinkAccountCard()
		return response
	}

	glg.Debugf("Loading user with access token: %s", accessToken)

	client := &http.Client{}

	// Use empty UID to get current user
	user, err := twitch.GetUserByID(client, accessToken, "")
	if err != nil {
		fmt.Println("Error loading the current user: ", err.Error())
		response.OutputSpeech("There was an error loading your Twitch account, please try again later.")
		return
	}
	glg.Debugf("Found user: %+v\n", user)

	follows, err := twitch.GetFollows(client, user)
	if err != nil {
		fmt.Println("Error loading user's follows: ", err.Error())
		response.OutputSpeech("Failed to load your follows from Twitch, please try again later")
		return
	}

	followIDs := follows.FollowIDsList()

	// Request all live streams based on all of the followed user_id values.
	// This will return only live channels and the first ID of that set should be used in
	// this next call.
	liveStreams, err := twitch.FindLiveStreams(client, followIDs)

	if len(liveStreams.Data) <= 0 {
		response.OutputSpeech("Sorry, it looks like none of your followed channels are live right now")
		return
	}

	selectedStream := liveStreams.Data[0]
	followedUser, err := twitch.GetUserByID(client, accessToken, selectedStream.UserID)
	if err != nil {
		fmt.Println("Error loading followed channel's user data: ", err.Error())
		response.OutputSpeech("Failed to find a followed stream, please try again later")
		return
	}

	glg.Debugf("Found followed user: %+v\n", followedUser)

	// If the device can play video, then play video; otherwise just play audio
	streamQuality := "audio_only"
	supportedInterfaces := echoRequest.Context.System.Device.SupportedIntefaces
	supportsVideo := (supportedInterfaces["VideoPlayer"] != nil) || (supportedInterfaces["VideoApp"] != nil)
	if supportsVideo {
		glg.Debug("Looking for video stream...")
		streamQuality = "720p"
	} else {
		glg.Debug("Only supports audio playback...")
		streamQuality = "audio_only"
	}

	streamVariant, err := twitch.GetStream(client, followedUser.Login, accessToken, streamQuality)
	if err != nil {
		fmt.Println("Error loading stream Variant: ", err.Error())
		response.OutputSpeech("Failed to find a stream URL, please try again later")
		return
	}

	glg.Debugf("Found stream URL: %s\n", streamVariant.URI)

	response.OutputSpeech(fmt.Sprintf("Starting stream for %s", followedUser.DisplayName))
	if streamVariant.Video == "audio_only" {
		glg.Debug("Sending Audio directive response")
		response.AppendAudioDirective(NewAudioDirectiveWithStreamURL(streamVariant.URI))
	} else {
		glg.Debug("Sending video directive response")
		response.AppendVideoDirective(NewVideoDirectiveWithStreamURL(streamVariant.URI, selectedStream.Title, followedUser.DisplayName))
	}

	return
}

// StartVideoStream currently just uses the audio stream method to start a video live stream
// if video playback is supported, otherwise falls back to an audio only stream.
func StartVideoStream(echoRequest *skillserver.EchoRequest) (response *skillserver.EchoResponse) {
	// TODO: This should just use the same method as the audio stream, if video is possible
	// it'll use that instead of just audio
	return StartAudioStream(echoRequest)
}

// NewAudioDirectiveWithStreamURL will create a new AudioDirective that is initialized with the
// provided URL.
func NewAudioDirectiveWithStreamURL(url string) *skillserver.AudioDirective {
	return &skillserver.AudioDirective{
		Type:         "AudioPlayer.Play",
		PlayBehavior: "REPLACE_ALL",
		AudioItem: &skillserver.AudioItem{
			Stream: &skillserver.Stream{
				Token:    "12345",
				URL:      url,
				OffsetMS: 0,
			},
		},
	}
}

// NewVideoDirectiveWithStreamURL will construct and initialize a new video directive to be
// returned the Alexa server.
func NewVideoDirectiveWithStreamURL(url, title, subtitle string) *skillserver.VideoDirective {

	return &skillserver.VideoDirective{
		Type: "VideoApp.Launch",
		VideoItem: &skillserver.VideoItem{
			Source: url,
			VideoMetadata: &skillserver.VideoMetadata{
				Title:    title,
				Subtitle: subtitle,
			},
		},
	}
}

// StopAudioDirective will construct and initialize a new Stop directive to stop the stream
// playback on the user's device.
func StopAudioDirective() (response *skillserver.EchoResponse) {

	response = skillserver.NewEchoResponse()

	stopAudioDirective := &skillserver.AudioDirective{
		Type: "AudioPlayer.Stop",
	}

	response.OutputSpeech("Twitch ya later")
	response.AppendAudioDirective(stopAudioDirective)

	return
}
