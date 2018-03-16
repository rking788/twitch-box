package providers

import (
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/grafov/m3u8"
)

// PlaybackCommand will describe a requested playback action.
type PlaybackCommand int

const (
	// PLAY means a new stream should start playback
	PLAY PlaybackCommand = iota

	// RESUME should play the currently paused/stopped stream
	RESUME

	// PAUSE means the player should temporarily pause audio playback
	PAUSE

	// PREVIOUS will request that the last stream before the currently playing one be played
	PREVIOUS

	// NEXT should play the next stream in a list of channels.
	// Could be follows or some other list of channels
	NEXT

	// STOP should permanently stop playback of the current stream
	STOP
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

// StreamProvider defines the actions that should be supported by any type that will be
// providing stream data for a particular platform.
type StreamProvider interface {
	Play(string) (*Stream, error)
	Next(string) (*Stream, error)
	Resume(string) (*Stream, error)
	Previous(string) (*Stream, error)
}

// Stream represents a particular stream on a platform.
type Stream struct {
	Name      string
	Title     string
	ChannelID uint
	*m3u8.Variant
}

// StreamsResponse container around the Twitch streams response.
type StreamsResponse struct {
	Data []*TwitchStream
	*Pagination
}

// TwitchStream describes the properties for a particular stream on Twitch
type TwitchStream struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id"`
	CommunityIDs []string `json:"community_ids"`
	Type         string   `json:"type"`
	Title        string   `json:"title"`
	ViewerCount  int      `json:"viewer_count"`
	ThumbnailURL string   `json:"thumbnail_url"`
}

func (s *Stream) String() string {
	return fmt.Sprintf("%+v", *s)
}

// UserResponse is a container around the response from the Twitch /users endpoint
type UserResponse struct {
	Data []*User
}

// User contains all the properties for a particular Twitch user.
type User struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	Type            string `json:"type"`
	BroadcasterType string `json:"broadcaster_type"`
}

func (u *User) String() string {
	return fmt.Sprintf("%+v", *u)
}

// Follows is a wrapper around the response when requesting a set of follower relationships
type Follows struct {
	Data []*Follow
}

// FollowIDsList will extract the user IDs from the calling Follows struct into a single slice
func (follows *Follows) FollowIDsList() []string {

	result := make([]string, 0, len(follows.Data))
	for _, f := range follows.Data {
		result = append(result, fmt.Sprintf("%d", f.ToID))
	}

	return result
}

// FollowList is a special type that wraps a collection of Follows.
type FollowList []*Follow

// FollowIDsList will transforma list of Follows into a slice of the IDs for all of the follows.
func (followList FollowList) FollowIDsList() []uint {

	result := make([]uint, 0, len(followList))
	for _, f := range followList {
		result = append(result, f.ToID)
	}

	return result
}

// Follow describes a single follower relationship, FromID is the user doing the following
// and ToID is the user being followed.
type Follow struct {
	FromID uint `json:"from_id"`
	ToID   uint `json:"to_id"`
}

func (f *Follow) String() string {
	return fmt.Sprintf("%+v", *f)
}

// Pagination wraps the cursor used to perform pagination on endpoints that support it. The
// cursor should be used in following requests to indicate the current page.
type Pagination struct {
	Cursor string `json:"cursor"`
}

// ChannelAccessToken is used for loading the stream URL for a specific channel. For some reason
// this type of request auth needs to be used instead of the other oauth process.
type ChannelAccessToken struct {
	Sig   string `json:"sig"`
	Token string `json:"token"`
}
