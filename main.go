package main

import (
	"net/http"
	"os"
	"time"

	"github.com/kpango/glg"
	"github.com/rking788/go-alexa/skillserver"
	"github.com/rking788/twitch-box/alexa"
)

// AlexaHandler is the type of function that should be used to respond to a specific intent.
type AlexaHandler func(*skillserver.EchoRequest) *skillserver.EchoResponse

// AlexaHandlers are the handler functions mapped by the intent name that they should handle.
var (
	AlexaHandlers = map[string]AlexaHandler{
		"StartAudioStream": alexa.StartAudioStream,
		"StartVideoStream": alexa.StartVideoStream,
	}
)

// Applications is a definition of the Alexa applications running on this server.
var applications map[string]interface{}

// InitEnv is responsible for initializing all components (including sub-packages) that
// depend on a specific deployment environment configuration.
func InitEnv() {
	applications = map[string]interface{}{
		"/echo/twitch-box": skillserver.EchoApplication{ // Route
			AppID:          os.Getenv("ALEXA_APP_ID"), // Echo App ID from Amazon Dashboard
			OnIntent:       EchoIntentHandler,
			OnLaunch:       EchoIntentHandler,
			OnSessionEnded: EchoSessionEndedHandler,
		},
		"/health": skillserver.StdApplication{
			Methods: "GET",
			Handler: healthHandler,
		},
	}
}

func main() {

	//	flag.Parse()

	//	config = loadConfig(configPath)

	//	glg.Infof("Loaded config : %+v\n", config)
	InitEnv()

	//	defer CloseLogger()

	glg.Printf("Version=%s, BuildDate=%v", Version, BuildDate)

	// writeHeapProfile()

	// if config.Environment == "production" {
	// 	port := ":443"
	// 	err := skillserver.RunSSL(applications, port, config.SSLCertPath, config.SSLKeyPath)
	// 	if err != nil {
	// 		glg.Errorf("Error starting the application! : %s", err.Error())
	// 	}
	// } else {
	// Heroku makes us read a random port from the environment and our app is a
	// subdomain of theirs so we get SSL for free
	port := os.Getenv("PORT")
	skillserver.Run(applications, port)
	//}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Up"))
}

// Alexa skill related functions

// EchoSessionEndedHandler is responsible for cleaning up an open session since the
// user has quit the session.
func EchoSessionEndedHandler(echoRequest *skillserver.EchoRequest,
	echoResponse *skillserver.EchoResponse) {

	*echoResponse = *skillserver.NewEchoResponse()

	//alexa.ClearSession(echoRequest.GetSessionID())
}

// EchoIntentHandler is a handler method that is responsible for receiving the
// call from a Alexa command and returning the correct speech or cards.
func EchoIntentHandler(echoRequest *skillserver.EchoRequest, echoResponse *skillserver.EchoResponse) {
	// Time the intent handler to determine if it is taking longer than normal
	startTime := time.Now()
	defer func(start time.Time) {
		glg.Infof("IntentHandler execution time: %v", time.Since(start))
	}(startTime)

	var response *skillserver.EchoResponse

	intentName := echoRequest.GetIntentName()

	glg.Infof("RequestType: %s, IntentName: %s", echoRequest.GetRequestType(), intentName)

	// During this time, users can invoke the following built-in playback control intents without using your skill’s invocation name:

	// AMAZON.CancelIntent
	// AMAZON.LoopOffIntent
	// AMAZON.LoopOnIntent
	// AMAZON.NextIntent
	// AMAZON.PreviousIntent
	// AMAZON.RepeatIntent
	// AMAZON.ResumeIntent
	// AMAZON.ShuffleOffIntent
	// AMAZON.ShuffleOnIntent
	// AMAZON.StartOverIntent

	handler, ok := AlexaHandlers[intentName]
	if echoRequest.GetRequestType() == "LaunchRequest" {
		response = alexa.WelcomePrompt(echoRequest)
	} else if intentName == "AMAZON.StopIntent" {
		response = skillserver.NewEchoResponse()
	} else if intentName == "AMAZON.CancelIntent" {
		response = skillserver.NewEchoResponse()
	} else if intentName == "AMAZON.PauseIntent" {
		// Send stop directive
		// TODO: Should remember the last stream the user listened to so that it can be resumed.
		response = alexa.StopAudioDirective()
	} else if ok {
		response = handler(echoRequest)
	} else {
		response = skillserver.NewEchoResponse()
		response.OutputSpeech("Sorry Guardian, I did not understand your request.")
	}

	*echoResponse = *response
}

// func dumpRequest(ctx *gin.Context) {

// 	data, err := httputil.DumpRequest(ctx.Request, true)
// 	if err != nil {
// 		glg.Errorf("Failed to dump the request: %s", err.Error())
// 		return
// 	}

// 	glg.Debug(string(data))
// }
