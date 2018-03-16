package alexa

import (
	"fmt"

	"github.com/kpango/glg"
	"github.com/rking788/go-alexa/skillserver"
	"github.com/rking788/twitch-box/providers"
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
func StartAudioStream(echoRequest *skillserver.EchoRequest, provider providers.StreamProvider) *skillserver.EchoResponse {

	response := skillserver.NewEchoResponse()
	accessToken := echoRequest.Session.User.AccessToken
	if accessToken == "" {
		response := skillserver.NewEchoResponse()
		response.
			OutputSpeech("Sorry, it looks like your account needs to be linked in " +
				"the Alexa app.").
			LinkAccountCard()
		return response
	}

	glg.Debugf("Loading user with access token: %s", accessToken)

	// TODO: The get followed channels method will now request the current user so there
	// is no need to do a separate call for that.

	// Use empty UID to get current user
	//user, err := twitch.GetUserByID(client, accessToken, "")
	// user, err := provider.GetCurrentUser(accessToken)
	// if err != nil {
	// 	fmt.Println("Error loading the current user: ", err.Error())
	// 	response.OutputSpeech(fmt.Sprintf("There was an error loading your %s account, please try again later.", provider.PlatformName()))
	// 	return
	// }
	// glg.Debugf("Found user: %+v\n", user)

	//follows, err := twitch.GetFollows(client, user)
	// follows, err := provider.GetFollowedChannels(accessToken, true)
	// if err != nil {
	// 	fmt.Println("Error loading user's follows: ", err.Error())
	// 	response.OutputSpeech("Failed to load your follows from Twitch, please try again later")
	// 	return
	// }

	// followIDs := follows.FollowIDsList()

	// Request all live streams based on all of the followed user_id values.
	// This will return only live channels and the first ID of that set should be used in
	// this next call.

	// TODO: How to handle this? This will need to be a method on the StreamProvider that will
	// pull out all live streams including UID, username, online status, etc. Any other fields
	// that will be needed to check for repeating streams and grab the HLS manifest
	//liveStreams, err := twitch.FindLiveStreams(client, followIDs)

	// if len(followIDs) <= 0 {
	// 	response.OutputSpeech("Sorry, it looks like none of your followed channels are live" +
	// 		" right now")
	// 	return
	// }

	var stream *providers.Stream
	var err error
	switch echoRequest.GetIntentName() {
	case "AMAZON.ResumeIntent":
		stream, err = provider.Resume(accessToken)
		// command = providers.RESUME
	case "AMAZON.PreviousIntent":
		stream, err = provider.Previous(accessToken)
		// command = providers.PREVIOUS
	case "AMAZON.NextIntent":
		stream, err = provider.Next(accessToken)
		// command = providers.NEXT
	case "AMAZON.PauseIntent":
		// TODO: This is handled automatically isn't it? maybe not for video?
		// command = providers.PAUSE
	default:
		stream, err = provider.Play(accessToken)
	}

	if err != nil {
		msg := fmt.Sprintf("Error trying to get next stream to play: %s", err.Error())
		glg.Warnf(msg)
		response.OutputSpeech(msg)
		return response
	}

	// selectedStream := provider.FindStreamForCommand(user, liveStreams.Data, command, response)
	// followedUser, err := provider.GetUserByID(client, accessToken, selectedStream.UserID)
	// if err != nil {
	// 	fmt.Println("Error loading followed channel's user data: ", err.Error())
	// 	response.OutputSpeech("Failed to find a followed stream, please try again later")
	// 	return
	// }

	// glg.Debugf("Found followed user: %+v\n", followedUser)

	// // If the device can play video, then play video; otherwise just play audio
	// streamQuality := "audio_only"
	// supportedInterfaces := echoRequest.Context.System.Device.SupportedIntefaces
	// supportsVideo := (supportedInterfaces["VideoPlayer"] != nil) || (supportedInterfaces["VideoApp"] != nil)
	// if supportsVideo {
	// 	glg.Debug("Looking for video stream...")
	// 	streamQuality = "720p"
	// } else {
	// 	glg.Debug("Only supports audio playback...")
	// 	streamQuality = "audio_only"
	// }

	// streamVariant, err := twitch.GetStream(client, followedUser.Login, accessToken, streamQuality)
	// if err != nil {
	// 	fmt.Println("Error loading stream Variant: ", err.Error())
	// 	response.OutputSpeech("Failed to find a stream URL, please try again later")
	// 	return
	// }

	glg.Debugf("Found stream URL: %s\n", stream.Variant.URI)

	response.OutputSpeech(fmt.Sprintf("Starting stream for %s", stream.Name))

	//twitch.SaveUsersCurrentStream(user, selectedStream)

	supportedInterfaces := echoRequest.Context.System.Device.SupportedIntefaces
	supportsVideo := (supportedInterfaces["VideoPlayer"] != nil) ||
		(supportedInterfaces["VideoApp"] != nil)

	if supportsVideo && stream.Variant.Video != "audio_only" {
		glg.Debug("Sending video directive response")
		directive := NewVideoDirectiveWithStreamURL(stream.Variant.URI, stream.Title, stream.Name)
		response.AppendVideoDirective(directive)
		glg.Debugf("%+v", directive)
	} else {
		glg.Debug("Sending Audio directive response")
		// TODO: This should only create a card if they are starting a new stream,
		// not resuming or skipping
		//widthReplaced := strings.Replace(selectedStream.ThumbnailURL, "{width}", "320", -1)
		//heightReplaced := strings.Replace(widthReplaced, "{height}", "180", -1)
		directive := NewAudioDirectiveWithStreamURL(stream.Variant.URI, stream.Name)
		response.AppendAudioDirective(directive)
		glg.Debugf("%+v", directive)
		//glg.Debugf("Setting card thumbnail to be: %s", heightReplaced)
		//response.StandardCard(followedUser.DisplayName, selectedStream.Title, heightReplaced, heightReplaced)
	}

	return response
}

// StartVideoStream currently just uses the audio stream method to start a video live stream
// if video playback is supported, otherwise falls back to an audio only stream.
func StartVideoStream(echoRequest *skillserver.EchoRequest, provider providers.StreamProvider) (response *skillserver.EchoResponse) {
	// TODO: This should just use the same method as the audio stream, if video is possible
	// it'll use that instead of just audio
	return StartAudioStream(echoRequest, provider)
}

// NewAudioDirectiveWithStreamURL will create a new AudioDirective that is initialized with the
// provided URL.
func NewAudioDirectiveWithStreamURL(url, name string) *skillserver.AudioDirective {
	return &skillserver.AudioDirective{
		Type:         "AudioPlayer.Play",
		PlayBehavior: "REPLACE_ALL",
		AudioItem: &skillserver.AudioItem{
			Stream: &skillserver.Stream{
				Token:    name,
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
